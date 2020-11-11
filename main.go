// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	goruntime "runtime"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	edsdatadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/DataDog/extendeddaemonset/pkg/controller/metrics"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
	"github.com/DataDog/datadog-operator/controllers"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/controller/debug"
	"github.com/DataDog/datadog-operator/pkg/secrets"
	"github.com/DataDog/datadog-operator/pkg/version"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	// +kubebuilder:scaffold:imports
)

const (
	maximumGoroutines = 200
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(apiregistrationv1.AddToScheme(scheme))

	utilruntime.Must(datadoghqv1alpha1.AddToScheme(scheme))
	utilruntime.Must(edsdatadoghqv1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", true,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")

	// Custom flags
	var printVersion, pprofActive, supportExtendedDaemonset bool
	var logEncoder, secretBackendCommand string
	flag.StringVar(&logEncoder, "logEncoder", "json", "log encoding ('json' or 'console')")
	flag.StringVar(&secretBackendCommand, "secretBackendCommand", "", "Secret backend command")
	logLevel := zap.LevelFlag("loglevel", zapcore.InfoLevel, "Set log level")
	flag.BoolVar(&printVersion, "version", false, "Print version and exit")
	flag.BoolVar(&pprofActive, "pprof", false, "Enable pprof endpoint")
	flag.BoolVar(&supportExtendedDaemonset, "supportExtendedDaemonset", false, "Support usage of Datadog ExtendedDaemonset CRD.")

	// Parsing flags
	flag.Parse()

	// Logging setup
	if err := customSetupLogging(*logLevel, logEncoder); err != nil {
		setupLog.Error(err, "unable to setup the logger")
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

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), config.ManagerOptionsWithNamespaces(setupLog, ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		HealthProbeBindAddress: ":8081",
		Port:                   9443,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "datadog-operator-lock",
	}))
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Custom setup
	customSetupHealthChecks(mgr)
	customSetupEndpoints(pprofActive, mgr)

	// Get some information about Kubernetes version
	if err = controllers.SetupControllers(mgr, supportExtendedDaemonset); err != nil {
		setupLog.Error(err, "unable to start controllers")
		os.Exit(1)
	}

	// if err = (&controllers.DatadogMonitorReconciler{
	// 	Client: mgr.GetClient(),
	// 	Log:    ctrl.Log.WithName("controllers").WithName("DatadogMonitor"),
	// 	Scheme: mgr.GetScheme(),
	// }).SetupWithManager(mgr); err != nil {
	// 	setupLog.Error(err, "unable to create controller", "controller", "DatadogMonitor")
	// 	os.Exit(1)
	// }
	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
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

func customSetupHealthChecks(mgr manager.Manager) {
	err := mgr.AddHealthzCheck("goroutines-number", func(req *http.Request) error {
		if goruntime.NumGoroutine() > maximumGoroutines {
			return fmt.Errorf("too much goroutines: %d > limit: %d", goruntime.NumGoroutine(), maximumGoroutines)
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
