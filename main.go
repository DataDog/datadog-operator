// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	goruntime "runtime"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	klog "k8s.io/klog/v2"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	edsdatadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/DataDog/extendeddaemonset/pkg/controller/metrics"
	"github.com/go-logr/logr"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/controllers"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/config/remote"
	"github.com/DataDog/datadog-operator/pkg/config/remote/data"
	"github.com/DataDog/datadog-operator/pkg/config/remote/service"
	"github.com/DataDog/datadog-operator/pkg/controller/debug"
	"github.com/DataDog/datadog-operator/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-operator/pkg/secrets"
	"github.com/DataDog/datadog-operator/pkg/version"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	// +kubebuilder:scaffold:imports
)

const (
	defaultMaximumGoroutines = 400
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	klog.SetLogger(ctrl.Log.WithName("klog"))

	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(apiregistrationv1.AddToScheme(scheme))

	utilruntime.Must(datadoghqv1alpha1.AddToScheme(scheme))
	utilruntime.Must(edsdatadoghqv1alpha1.AddToScheme(scheme))
	utilruntime.Must(datadoghqv2alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

// stringSlice implements flag.Value
type stringSlice []string

func (ss *stringSlice) String() string {
	return fmt.Sprintf("%s", *ss)
}

func (ss *stringSlice) Set(value string) error {
	*ss = strings.Split(value, " ")
	return nil
}

// LogConf ...
type LogConf struct {
	Enabled string `json:"logs_enabled"`
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var leaderElectionResourceLock string
	var leaderElectionLeaseDuration time.Duration
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", true,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&leaderElectionResourceLock, "leader-election-resource", resourcelock.ConfigMapsLeasesResourceLock, "determines which resource lock to use for leader election. option:[configmapsleases|endpointsleases|configmaps]")
	flag.DurationVar(&leaderElectionLeaseDuration, "leader-election-lease-duration", 60*time.Second, "Define LeaseDuration as well as RenewDeadline (leaseDuration / 2) and RetryPeriod (leaseDuration / 4)")

	// Custom flags
	var printVersion, pprofActive, supportExtendedDaemonset, supportCilium, datadogAgentEnabled, datadogMonitorEnabled, operatorMetricsEnabled, webhookEnabled, v2APIEnabled bool
	var logEncoder, secretBackendCommand string
	var secretBackendArgs stringSlice
	flag.StringVar(&logEncoder, "logEncoder", "json", "log encoding ('json' or 'console')")
	flag.StringVar(&secretBackendCommand, "secretBackendCommand", "", "Secret backend command")
	flag.Var(&secretBackendArgs, "secretBackendArgs", "Space separated arguments of the secret backend command")
	logLevel := zap.LevelFlag("loglevel", zapcore.InfoLevel, "Set log level")
	flag.BoolVar(&printVersion, "version", false, "Print version and exit")
	flag.BoolVar(&pprofActive, "pprof", false, "Enable pprof endpoint")
	flag.BoolVar(&supportExtendedDaemonset, "supportExtendedDaemonset", false, "Support usage of Datadog ExtendedDaemonset CRD.")
	flag.BoolVar(&supportCilium, "supportCilium", false, "Support usage of Cilium network policies.")
	flag.BoolVar(&datadogAgentEnabled, "datadogAgentEnabled", true, "Enable the DatadogAgent controller")
	flag.BoolVar(&datadogMonitorEnabled, "datadogMonitorEnabled", false, "Enable the DatadogMonitor controller")
	flag.BoolVar(&operatorMetricsEnabled, "operatorMetricsEnabled", true, "Enable sending operator metrics to Datadog")
	flag.BoolVar(&v2APIEnabled, "v2APIEnabled", true, "Enable the v2 api")
	flag.BoolVar(&webhookEnabled, "webhookEnabled", true, "Enable CRD conversion webhook.")
	maximumGoroutines := flag.Int("maximumGoroutines", defaultMaximumGoroutines, "Override health check threshold for maximum number of goroutines.")

	// Parsing flags
	flag.Parse()

	// Logging setup
	if err := customSetupLogging(*logLevel, logEncoder); err != nil {
		setupLog.Error(err, "Unable to setup the logger")
		os.Exit(1)
	}

	// Print version information
	if printVersion {
		version.PrintVersionWriter(os.Stdout, "text")
		os.Exit(0)
	}
	version.PrintVersionLogs(setupLog)

	// Dispatch CLI flags to each package
	secrets.SetSecretBackendCommand(secretBackendCommand)
	secrets.SetSecretBackendArgs(secretBackendArgs)

	renewDeadline := leaderElectionLeaseDuration / 2
	retryPeriod := leaderElectionLeaseDuration / 4

	setupLog.Info("creating manager with LeaderElectionResourceLock", "lock", leaderElectionResourceLock)
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), config.ManagerOptionsWithNamespaces(setupLog, ctrl.Options{
		Scheme:                     scheme,
		MetricsBindAddress:         metricsAddr,
		HealthProbeBindAddress:     ":8081",
		Port:                       9443,
		LeaderElection:             enableLeaderElection,
		LeaderElectionID:           "datadog-operator-lock",
		LeaderElectionResourceLock: leaderElectionResourceLock,
		LeaseDuration:              &leaderElectionLeaseDuration,
		RenewDeadline:              &renewDeadline,
		RetryPeriod:                &retryPeriod,
	}))
	if err != nil {
		setupLog.Error(err, "Unable to start manager")
		os.Exit(1)
	}
	setupLog.Info("creating manager succeeded")
	// Custom setup
	customSetupHealthChecks(setupLog, mgr, maximumGoroutines)
	customSetupEndpoints(pprofActive, mgr)

	creds, err := config.NewCredentialManager().GetCredentials()
	if err != nil && datadogMonitorEnabled {
		setupLog.Error(err, "Unable to get credentials for DatadogMonitor")
		os.Exit(1)
	}

	options := controllers.SetupOptions{
		SupportExtendedDaemonset: supportExtendedDaemonset,
		SupportCilium:            supportCilium,
		Creds:                    creds,
		DatadogAgentEnabled:      datadogAgentEnabled,
		DatadogMonitorEnabled:    datadogMonitorEnabled,
		OperatorMetricsEnabled:   operatorMetricsEnabled,
		V2APIEnabled:             v2APIEnabled,
	}

	if err = controllers.SetupControllers(setupLog, mgr, options); err != nil {
		setupLog.Error(err, "Unable to start controllers")
		os.Exit(1)
	}

	if webhookEnabled && datadogAgentEnabled {
		if err = (&datadoghqv2alpha1.DatadogAgent{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "DatadogAgent")
			os.Exit(1)
		}
	}

	callback := func(data []byte) {
		setupLog.Info("callback called")
		var conf LogConf
		errRc  := json.Unmarshal(data, &conf)
		if errRc != nil {
			setupLog.Error(errRc, "failed to fetch configurations from RC")
			return
		}
		setupLog.Info(fmt.Sprintf("log conf %t", conf.Enabled == "true"))

		// .. do stuff
	}

	configService, errRC := service.NewService(callback)
	if errRC != nil {
		setupLog.Error(errRC, "can't create remote config service")
		os.Exit(1)
	}
	mainCtx, mainCtxCancel := context.WithCancel(context.Background())
	defer mainCtxCancel()
	configService.Start(mainCtx)

	// fetch configurations directly from the RC service without any grpc
	updater := remote.NewConfigUpdater(configService)

	configClient, errRC := remote.NewClient(
		"agentName",
		updater,
		version.Version,
		[]data.Product{data.ProductDebug}, // the test product
		time.Second*5,                     // fetch updates from the config server every 5 seconds
	)
	if err != nil {
		setupLog.Error(errRC, "error setting up RC client")
	}

	// register function
	configClient.RegisterDebug(func(update map[string]state.DebugConfig) {
		for file, conf := range update {
			setupLog.Info("file: %s - content: %+v", file, conf)
		}
	})
	configClient.Start()

	if state, errRC := configService.ConfigGetState(); err == nil {
		setupLog.Info("configService.ConfigGetState()", "GetConfigState", state.GetConfigState(), "GetDirectorState", state.GetDirectorState())
	} else {
		setupLog.Error(errRC, "error in ConfigGetState")
	}

	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Problem running manager")
		os.Exit(1)
	}
}

func customSetupLogging(logLevel zapcore.Level, logEncoder string) error {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	encoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder

	var encoder zapcore.Encoder
	switch logEncoder {
	case "console":
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	case "json":
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	default:
		return fmt.Errorf("unknow log encoder: %s", logEncoder)
	}

	ctrl.SetLogger(ctrlzap.New(
		ctrlzap.Encoder(encoder),
		ctrlzap.Level(logLevel),
		ctrlzap.StacktraceLevel(zapcore.PanicLevel)),
	)

	return nil
}

func customSetupHealthChecks(logger logr.Logger, mgr manager.Manager, maximumGoroutines *int) {
	setupLog.Info("configuring manager health check", "maximumGoroutines", *maximumGoroutines)
	err := mgr.AddHealthzCheck("goroutines-number", func(req *http.Request) error {
		if goruntime.NumGoroutine() > *maximumGoroutines {
			return fmt.Errorf("too many goroutines: %d > limit: %d", goruntime.NumGoroutine(), *maximumGoroutines)
		}
		return nil
	})
	if err != nil {
		setupLog.Error(err, "Unable to add healthchecks")
	}
}

func customSetupEndpoints(pprofActive bool, mgr manager.Manager) {
	if pprofActive {
		if err := debug.RegisterEndpoint(mgr.AddMetricsExtraHandler, nil); err != nil {
			setupLog.Error(err, "Unable to register pprof endpoint")
		}
	}

	if err := metrics.RegisterEndpoint(mgr, mgr.AddMetricsExtraHandler); err != nil {
		setupLog.Error(err, "Unable to register custom metrics endpoints")
	}
}
