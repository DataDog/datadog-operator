// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	goruntime "runtime"
	"strings"
	"time"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	toolscache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	klog "k8s.io/klog/v2"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"gopkg.in/DataDog/dd-trace-go.v1/profiler"

	edsdatadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/DataDog/extendeddaemonset/pkg/controller/metrics"
	"github.com/go-logr/logr"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/controllers"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/controller/debug"
	"github.com/DataDog/datadog-operator/pkg/secrets"
	"github.com/DataDog/datadog-operator/pkg/version"
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

const (
	// ExtendedDaemonset default configuration values from https://github.com/DataDog/extendeddaemonset/blob/main/api/v1alpha1/extendeddaemonset_default.go
	defaultCanaryAutoPauseEnabled = true
	defaultCanaryAutoFailEnabled  = true
	// default to 0, to use default value from EDS.
	defaultCanaryAutoPauseMaxRestarts          = 0
	defaultCanaryAutoFailMaxRestarts           = 0
	defaultCanaryAutoPauseMaxSlowStartDuration = 0
)

type options struct {
	// Observability options
	metricsAddr      string
	profilingEnabled bool
	logLevel         *zapcore.Level
	logEncoder       string
	printVersion     bool
	pprofActive      bool

	// Leader Election options
	enableLeaderElection        bool
	leaderElectionResourceLock  string
	leaderElectionLeaseDuration time.Duration

	// Controllers options
	supportExtendedDaemonset               bool
	edsMaxPodUnavailable                   string
	edsMaxPodSchedulerFailure              string
	edsCanaryDuration                      time.Duration
	edsCanaryReplicas                      string
	edsCanaryAutoPauseEnabled              bool
	edsCanaryAutoPauseMaxRestarts          int
	edsCanaryAutoFailEnabled               bool
	edsCanaryAutoFailMaxRestarts           int
	edsCanaryAutoPauseMaxSlowStartDuration time.Duration
	supportCilium                          bool
	datadogAgentEnabled                    bool
	datadogMonitorEnabled                  bool
	datadogSLOEnabled                      bool
	operatorMetricsEnabled                 bool
	webhookEnabled                         bool
	v2APIEnabled                           bool
	maximumGoroutines                      int
	introspectionEnabled                   bool
	datadogAgentProfileEnabled             bool

	// Secret Backend options
	secretBackendCommand string
	secretBackendArgs    stringSlice
}

func (opts *options) Parse() {
	// Observability flags
	flag.StringVar(&opts.metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&opts.profilingEnabled, "profiling-enabled", false, "Enable Datadog profile in the Datadog Operator process.")
	opts.logLevel = zap.LevelFlag("loglevel", zapcore.InfoLevel, "Set log level")
	flag.StringVar(&opts.logEncoder, "logEncoder", "json", "log encoding ('json' or 'console')")
	flag.BoolVar(&opts.printVersion, "version", false, "Print version and exit")
	flag.BoolVar(&opts.pprofActive, "pprof", false, "Enable pprof endpoint")

	// Leader Election options flags
	flag.BoolVar(&opts.enableLeaderElection, "enable-leader-election", true,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&opts.leaderElectionResourceLock, "leader-election-resource", resourcelock.ConfigMapsLeasesResourceLock, "determines which resource lock to use for leader election. option:[configmapsleases|endpointsleases|leases]")
	flag.DurationVar(&opts.leaderElectionLeaseDuration, "leader-election-lease-duration", 60*time.Second, "Define LeaseDuration as well as RenewDeadline (leaseDuration / 2) and RetryPeriod (leaseDuration / 4)")

	// Custom flags
	flag.StringVar(&opts.secretBackendCommand, "secretBackendCommand", "", "Secret backend command")
	flag.Var(&opts.secretBackendArgs, "secretBackendArgs", "Space separated arguments of the secret backend command")
	flag.BoolVar(&opts.supportCilium, "supportCilium", false, "Support usage of Cilium network policies.")
	flag.BoolVar(&opts.datadogAgentEnabled, "datadogAgentEnabled", true, "Enable the DatadogAgent controller")
	flag.BoolVar(&opts.datadogMonitorEnabled, "datadogMonitorEnabled", false, "Enable the DatadogMonitor controller")
	flag.BoolVar(&opts.datadogSLOEnabled, "datadogSLOEnabled", false, "Enable the DatadogSLO controller")
	flag.BoolVar(&opts.operatorMetricsEnabled, "operatorMetricsEnabled", true, "Enable sending operator metrics to Datadog")
	flag.BoolVar(&opts.v2APIEnabled, "v2APIEnabled", true, "Enable the v2 api")
	flag.BoolVar(&opts.webhookEnabled, "webhookEnabled", false, "Enable CRD conversion webhook.")
	flag.IntVar(&opts.maximumGoroutines, "maximumGoroutines", defaultMaximumGoroutines, "Override health check threshold for maximum number of goroutines.")
	flag.BoolVar(&opts.introspectionEnabled, "introspectionEnabled", false, "Enable introspection (beta)")
	flag.BoolVar(&opts.datadogAgentProfileEnabled, "datadogAgentProfileEnabled", false, "Enable DatadogAgentProfile controller (beta)")

	// ExtendedDaemonset configuration
	flag.BoolVar(&opts.supportExtendedDaemonset, "supportExtendedDaemonset", false, "Support usage of Datadog ExtendedDaemonset CRD.")
	flag.StringVar(&opts.edsMaxPodUnavailable, "edsMaxPodUnavailable", "", "ExtendedDaemonset number of max unavailable pods during the rolling update")
	flag.StringVar(&opts.edsMaxPodSchedulerFailure, "edsMaxPodSchedulerFailure", "", "ExtendedDaemonset number of max pod scheduler failures")
	flag.DurationVar(&opts.edsCanaryDuration, "edsCanaryDuration", 10*time.Minute, "ExtendedDaemonset canary duration")
	flag.StringVar(&opts.edsCanaryReplicas, "edsCanaryReplicas", "", "ExtendedDaemonset number of canary pods")
	flag.BoolVar(&opts.edsCanaryAutoPauseEnabled, "edsCanaryAutoPauseEnabled", defaultCanaryAutoPauseEnabled, "ExtendedDaemonset canary auto pause enabled")
	flag.IntVar(&opts.edsCanaryAutoPauseMaxRestarts, "edsCanaryAutoPauseMaxRestarts", defaultCanaryAutoPauseMaxRestarts, "ExtendedDaemonset canary auto pause max restart count")
	flag.BoolVar(&opts.edsCanaryAutoFailEnabled, "edsCanaryAutoFailEnabled", defaultCanaryAutoFailEnabled, "ExtendedDaemonset canary auto fail enabled")
	flag.IntVar(&opts.edsCanaryAutoFailMaxRestarts, "edsCanaryAutoFailMaxRestarts", defaultCanaryAutoFailMaxRestarts, "ExtendedDaemonset canary auto fail max restart count")
	flag.DurationVar(&opts.edsCanaryAutoPauseMaxSlowStartDuration, "edsCanaryAutoPauseMaxSlowStartDuration", defaultCanaryAutoPauseMaxSlowStartDuration*time.Minute, "ExtendedDaemonset canary max slow start duration")

	// Parsing flags
	flag.Parse()
}

func main() {
	var opts options
	opts.Parse()

	if err := run(&opts); err != nil {
		os.Exit(1)
	}

	os.Exit(0)
}

// run allow to use defer func() paradigm properly.
// do not use `os.Exit()` in this function
func run(opts *options) error {
	// Logging setup
	if err := customSetupLogging(*opts.logLevel, opts.logEncoder); err != nil {
		return setupErrorf(setupLog, err, "Unable to setup the logger")
	}

	// Print version information
	if opts.printVersion {
		version.PrintVersionWriter(os.Stdout, "text")
		return nil
	}
	version.PrintVersionLogs(setupLog)

	if !opts.v2APIEnabled {
		setupLog.Error(nil, "The 'v2APIEnabled' flag is deprecated since v1.2.0+ and will be removed in v1.7.0. "+
			"Once removed, the Datadog Operator cannot be configured to reconcile the v1alpha1 DatadogAgent CRD. "+
			"However, you will still be able to apply a v1alpha1 manifest with the conversion webhook enabled (using the flag 'webhookEnabled'). "+
			"DatadogAgent v1alpha1 and the conversion webhook will be removed in v1.8.0. "+
			"See the migration page for instructions on migrating to v2alpha1: https://docs.datadoghq.com/containers/guide/datadogoperator_migration/")
	}

	if opts.profilingEnabled {
		setupLog.Info("Starting datadog profiler")
		if err := profiler.Start(
			profiler.WithVersion(version.Version),
			profiler.WithProfileTypes(profiler.CPUProfile, profiler.HeapProfile),
		); err != nil {
			return setupErrorf(setupLog, err, "unable to start datadog profiler")
		}

		defer profiler.Stop()
	}

	// Dispatch CLI flags to each package
	secrets.SetSecretBackendCommand(opts.secretBackendCommand)
	secrets.SetSecretBackendArgs(opts.secretBackendArgs)

	renewDeadline := opts.leaderElectionLeaseDuration / 2
	retryPeriod := opts.leaderElectionLeaseDuration / 4

	restConfig := ctrl.GetConfigOrDie()
	restConfig.UserAgent = "datadog-operator"
	mgr, err := ctrl.NewManager(restConfig, config.ManagerOptionsWithNamespaces(setupLog, ctrl.Options{
		Scheme:                     scheme,
		MetricsBindAddress:         opts.metricsAddr,
		HealthProbeBindAddress:     ":8081",
		Port:                       9443,
		LeaderElection:             opts.enableLeaderElection,
		LeaderElectionID:           "datadog-operator-lock",
		LeaderElectionResourceLock: opts.leaderElectionResourceLock,
		LeaseDuration:              &opts.leaderElectionLeaseDuration,
		RenewDeadline:              &renewDeadline,
		RetryPeriod:                &retryPeriod,
		NewCache:                   cache.BuilderWithOptions(cacheOptions()),
	}))
	if err != nil {
		return setupErrorf(setupLog, err, "Unable to start manager")

	}

	// Custom setup
	customSetupHealthChecks(setupLog, mgr, &opts.maximumGoroutines)
	customSetupEndpoints(opts.pprofActive, mgr)

	creds, err := config.NewCredentialManager().GetCredentials()
	if err != nil && opts.datadogMonitorEnabled {
		return setupErrorf(setupLog, err, "Unable to get credentials for DatadogMonitor")
	}

	options := controllers.SetupOptions{
		SupportExtendedDaemonset: controllers.ExtendedDaemonsetOptions{
			Enabled:                             opts.supportExtendedDaemonset,
			MaxPodUnavailable:                   opts.edsMaxPodUnavailable,
			CanaryDuration:                      opts.edsCanaryDuration,
			CanaryReplicas:                      opts.edsCanaryReplicas,
			CanaryAutoPauseEnabled:              opts.edsCanaryAutoPauseEnabled,
			CanaryAutoPauseMaxRestarts:          opts.edsCanaryAutoPauseMaxRestarts,
			CanaryAutoFailEnabled:               opts.edsCanaryAutoFailEnabled,
			CanaryAutoFailMaxRestarts:           opts.edsCanaryAutoFailMaxRestarts,
			CanaryAutoPauseMaxSlowStartDuration: opts.edsCanaryAutoPauseMaxSlowStartDuration,
			MaxPodSchedulerFailure:              opts.edsMaxPodSchedulerFailure,
		},
		SupportCilium:              opts.supportCilium,
		Creds:                      creds,
		DatadogAgentEnabled:        opts.datadogAgentEnabled,
		DatadogMonitorEnabled:      opts.datadogMonitorEnabled,
		DatadogSLOEnabled:          opts.datadogSLOEnabled,
		OperatorMetricsEnabled:     opts.operatorMetricsEnabled,
		V2APIEnabled:               opts.v2APIEnabled,
		IntrospectionEnabled:       opts.introspectionEnabled,
		DatadogAgentProfileEnabled: opts.datadogAgentProfileEnabled,
	}

	if err = controllers.SetupControllers(setupLog, mgr, options); err != nil {
		return setupErrorf(setupLog, err, "Unable to start controllers")
	}

	if opts.webhookEnabled && opts.datadogAgentEnabled {
		if err = (&datadoghqv2alpha1.DatadogAgent{}).SetupWebhookWithManager(mgr); err != nil {
			return setupErrorf(setupLog, err, "unable to create webhook", "webhook", "DatadogAgent")
		}
	}

	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		return setupErrorf(setupLog, err, "Problem running manager")
	}

	return nil
}

// This function is used to configure the cache used by the manager. It is very
// important to reduce memory usage.
// For the profiles feature we need to list the agent pods, but we're only
// interested in the node name and the labels. This function removes all the
// rest of fields to reduce memory usage.
// Also for the profiles feature, we need to list the nodes, but we're only
// interested in the node name and the labels.
// Note that if in the future we need to list or get pods or nodes and use other
// fields we'll need to modify this function.
func cacheOptions() cache.Options {
	return cache.Options{
		SelectorsByObject: cache.SelectorsByObject{
			// Store pods only if they are node agent pods.
			&corev1.Pod{}: {
				Label: labels.SelectorFromSet(map[string]string{
					apicommon.AgentDeploymentComponentLabelKey: apicommon.DefaultAgentResourceSuffix,
				}),
			},
		},
		TransformByObject: map[client.Object]toolscache.TransformFunc{
			// Store only the node name and the labels of the pod.
			&corev1.Pod{}: func(obj interface{}) (interface{}, error) {
				pod := obj.(*corev1.Pod)

				newPod := &corev1.Pod{
					TypeMeta: pod.TypeMeta,
					ObjectMeta: v1.ObjectMeta{
						Namespace: pod.Namespace,
						Name:      pod.Name,
						Labels:    pod.Labels,
					},
					Spec: corev1.PodSpec{
						NodeName: pod.Spec.NodeName,
					},
				}

				return newPod, nil
			},

			// Store only the node name and its labels.
			&corev1.Node{}: func(obj interface{}) (interface{}, error) {
				node := obj.(*corev1.Node)

				newNode := &corev1.Node{
					TypeMeta: node.TypeMeta,
					ObjectMeta: v1.ObjectMeta{
						Name:   node.Name,
						Labels: node.Labels,
					},
				}

				return newNode, nil
			},
		},
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
		setupErrorf(setupLog, err, "Unable to add healthchecks")
	}
}

func customSetupEndpoints(pprofActive bool, mgr manager.Manager) {
	if pprofActive {
		if err := debug.RegisterEndpoint(mgr.AddMetricsExtraHandler, nil); err != nil {
			setupErrorf(setupLog, err, "Unable to register pprof endpoint")
		}
	}

	if err := metrics.RegisterEndpoint(mgr, mgr.AddMetricsExtraHandler); err != nil {
		setupErrorf(setupLog, err, "Unable to register custom metrics endpoints")
	}
}

func setupErrorf(logger logr.Logger, err error, msg string, keysAndValues ...any) error {
	setupLog.Error(err, msg, keysAndValues...)
	return fmt.Errorf("%s, err:%w", msg, err)
}
