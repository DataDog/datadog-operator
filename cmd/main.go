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

	edsdatadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/go-logr/logr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/DataDog/dd-trace-go.v1/profiler"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	apimversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/discovery"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/klog/v2"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller"
	"github.com/DataDog/datadog-operator/internal/controller/metrics"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/debug"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/metadata"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/remoteconfig"
	"github.com/DataDog/datadog-operator/pkg/secrets"
	"github.com/DataDog/datadog-operator/pkg/utils"
	"github.com/DataDog/datadog-operator/pkg/version"

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	// nolint:gci
	// +kubebuilder:scaffold:imports
)

const (
	defaultMaximumGoroutines = 400
)

var (
	scheme      = runtime.NewScheme()
	setupLog    = ctrl.Log.WithName("setup")
	metadataLog = ctrl.Log.WithName("metadata")
)

func init() {
	klog.SetLogger(ctrl.Log.WithName("klog"))

	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(apiregistrationv1.AddToScheme(scheme))

	utilruntime.Must(datadoghqv1alpha1.AddToScheme(scheme))
	utilruntime.Must(edsdatadoghqv1alpha1.AddToScheme(scheme))
	utilruntime.Must(datadoghqv2alpha1.AddToScheme(scheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(scheme))
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
	secureMetrics    bool
	profilingEnabled bool
	logLevel         *zapcore.Level
	logEncoder       string
	printVersion     bool
	pprofActive      bool

	// Leader Election options
	enableLeaderElection        bool
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
	edsSlowStartAdditiveIncrease           string
	supportCilium                          bool
	datadogAgentEnabled                    bool
	datadogAgentInternalEnabled            bool
	datadogMonitorEnabled                  bool
	datadogSLOEnabled                      bool
	operatorMetricsEnabled                 bool
	maximumGoroutines                      int
	introspectionEnabled                   bool
	datadogAgentProfileEnabled             bool
	remoteConfigEnabled                    bool
	datadogDashboardEnabled                bool
	datadogGenericResourceEnabled          bool

	// Secret Backend options
	secretBackendCommand  string
	secretBackendArgs     stringSlice
	secretRefreshInterval time.Duration
}

func (opts *options) Parse() {
	// Observability flags
	flag.StringVar(&opts.metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&opts.secureMetrics, "metrics-secure", false, "If true, the metrics endpoint is served securely via HTTPS. Use false to use HTTP instead.")
	flag.BoolVar(&opts.profilingEnabled, "profiling-enabled", false, "Enable Datadog profile in the Datadog Operator process.")
	opts.logLevel = zap.LevelFlag("loglevel", zapcore.InfoLevel, "Set log level")
	flag.StringVar(&opts.logEncoder, "logEncoder", "json", "log encoding ('json' or 'console')")
	flag.BoolVar(&opts.printVersion, "version", false, "Print version and exit")
	flag.BoolVar(&opts.pprofActive, "pprof", false, "Enable pprof endpoint")

	// Leader Election options flags
	flag.BoolVar(&opts.enableLeaderElection, "enable-leader-election", true,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.DurationVar(&opts.leaderElectionLeaseDuration, "leader-election-lease-duration", 60*time.Second, "Define LeaseDuration as well as RenewDeadline (leaseDuration / 2) and RetryPeriod (leaseDuration / 4)")

	// Custom flags
	flag.StringVar(&opts.secretBackendCommand, "secretBackendCommand", "", "Secret backend command")
	flag.Var(&opts.secretBackendArgs, "secretBackendArgs", "Space separated arguments of the secret backend command")
	flag.DurationVar(&opts.secretRefreshInterval, "secretRefreshInterval", 0, "Interval for refreshing secrets from secret backend")
	flag.BoolVar(&opts.supportCilium, "supportCilium", false, "Support usage of Cilium network policies.")
	flag.BoolVar(&opts.datadogAgentEnabled, "datadogAgentEnabled", true, "Enable the DatadogAgent controller")
	flag.BoolVar(&opts.datadogMonitorEnabled, "datadogMonitorEnabled", false, "Enable the DatadogMonitor controller")
	flag.BoolVar(&opts.datadogSLOEnabled, "datadogSLOEnabled", false, "Enable the DatadogSLO controller")
	flag.BoolVar(&opts.operatorMetricsEnabled, "operatorMetricsEnabled", true, "Enable sending operator metrics to Datadog")
	flag.IntVar(&opts.maximumGoroutines, "maximumGoroutines", defaultMaximumGoroutines, "Override health check threshold for maximum number of goroutines.")
	flag.BoolVar(&opts.introspectionEnabled, "introspectionEnabled", false, "Enable introspection (beta)")
	flag.BoolVar(&opts.datadogAgentProfileEnabled, "datadogAgentProfileEnabled", false, "Enable DatadogAgentProfile controller (beta)")
	flag.BoolVar(&opts.remoteConfigEnabled, "remoteConfigEnabled", false, "Enable RemoteConfig capabilities in the Operator (beta)")
	flag.BoolVar(&opts.datadogDashboardEnabled, "datadogDashboardEnabled", false, "Enable the DatadogDashboard controller")
	flag.BoolVar(&opts.datadogGenericResourceEnabled, "datadogGenericResourceEnabled", false, "Enable the DatadogGenericResource controller")

	// DatadogAgentInternal
	flag.BoolVar(&opts.datadogAgentInternalEnabled, "datadogAgentInternalEnabled", true, "Enable the DatadogAgentInternal controller")

	// ExtendedDaemonset configuration
	flag.BoolVar(&opts.supportExtendedDaemonset, "supportExtendedDaemonset", false, "Support usage of Datadog ExtendedDaemonset CRD.")
	flag.StringVar(&opts.edsMaxPodUnavailable, "edsMaxPodUnavailable", "", "ExtendedDaemonset number of max unavailable pods during the rolling update")
	flag.StringVar(&opts.edsSlowStartAdditiveIncrease, "edsSlowStartAdditiveIncrease", "", "ExtendedDaemonset slow start additive increase")
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

	if opts.datadogAgentEnabled {
		setupLog.Error(nil, "[WARNING] Agent DaemonSet selector changed in Operator v1.21. If you rely on Datadog Agent pod labels e.g. in NetworkPolicies, verify if you may be impacted. See README for details.")
		if opts.datadogAgentProfileEnabled {
			setupLog.Error(nil, "[WARNING] Selector changed in Agent DaemonSets managed by DAPs in Operator v1.18 and v1.21. If you rely on Datadog Agent pod labels, e.g. in NetworkPolicies, verify if you may be impacted. See README for details.")
		}
	}

	// submits the maximum go routine setting as a metric
	metrics.MaxGoroutines.Set(float64(opts.maximumGoroutines))

	if opts.profilingEnabled {
		setupLog.Info("Starting datadog profiler")
		if err := profiler.Start(
			profiler.WithVersion(version.Version),
			profiler.WithProfileTypes(
				profiler.CPUProfile,
				profiler.HeapProfile,
				profiler.BlockProfile,
				profiler.MutexProfile,
				profiler.GoroutineProfile,
			),
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

	metricsServerOptions := metricsserver.Options{
		BindAddress:   opts.metricsAddr,
		SecureServing: opts.secureMetrics,
		ExtraHandlers: debug.GetExtraMetricHandlers(opts.pprofActive),
	}

	if opts.secureMetrics {
		// FilterProvider is used to protect the metrics endpoint with authn/authz.
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	restConfig := ctrl.GetConfigOrDie()
	restConfig.UserAgent = "datadog-operator"
	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme:                     scheme,
		Metrics:                    metricsServerOptions,
		HealthProbeBindAddress:     ":8081",
		LeaderElection:             opts.enableLeaderElection,
		LeaderElectionID:           "datadog-operator-lock",
		LeaderElectionResourceLock: resourcelock.LeasesResourceLock,
		LeaseDuration:              &opts.leaderElectionLeaseDuration,
		RenewDeadline:              &renewDeadline,
		RetryPeriod:                &retryPeriod,
		Cache: config.CacheOptions(setupLog, config.WatchOptions{
			DatadogAgentEnabled:           opts.datadogAgentEnabled,
			DatadogAgentInternalEnabled:   opts.datadogAgentInternalEnabled,
			DatadogMonitorEnabled:         opts.datadogMonitorEnabled,
			DatadogSLOEnabled:             opts.datadogSLOEnabled,
			DatadogAgentProfileEnabled:    opts.datadogAgentProfileEnabled,
			IntrospectionEnabled:          opts.introspectionEnabled,
			DatadogDashboardEnabled:       opts.datadogDashboardEnabled,
			DatadogGenericResourceEnabled: opts.datadogGenericResourceEnabled,
		}),
	})
	if err != nil {
		return setupErrorf(setupLog, err, "Unable to start manager")

	}

	// Client is needed when Creds should be resolved from DDA so cached client is fine
	credsManager := config.NewCredentialManagerWithDecryptor(mgr.GetClient(), secrets.NewSecretBackend())
	creds, err := credsManager.GetCredentials()
	if err != nil && opts.datadogMonitorEnabled {
		return setupErrorf(setupLog, err, "Unable to get credentials for DatadogMonitor")
	}

	// Checks if credentials are mandatory due to a resource controller being enabled
	if checkErr := checkRequiredCredentials(opts, err); checkErr != nil {
		return checkErr
	}

	if opts.secretRefreshInterval > 0 && opts.secretBackendCommand == "" {
		setupLog.Error(nil, "secretRefreshInterval is set but secretBackendCommand is not configured")
	} else if opts.secretBackendCommand != "" && opts.secretRefreshInterval > 0 {
		go credsManager.StartCredentialRefreshRoutine(opts.secretRefreshInterval, setupLog)
	}

	// Custom setup
	customSetupHealthChecks(setupLog, mgr, &opts.maximumGoroutines)

	if opts.remoteConfigEnabled {
		go func() {
			// Block until this controller manager is elected leader. We presume the
			// entire process will terminate if we lose leadership, so we don't need
			// to handle that.
			<-mgr.Elected()

			err = remoteconfig.NewRemoteConfigUpdater(mgr.GetClient(), ctrl.Log.WithName("remote_config")).Setup(creds)
			if err != nil {
				setupErrorf(setupLog, err, "Unable to set up Remote Config service")
			}
		}()
	}

	// Cleanup leftover DatadogAgentInternal resources if DDAI controller is disabled
	if opts.datadogAgentEnabled && !opts.datadogAgentInternalEnabled {
		go func() {
			// Block until this controller manager is elected leader and controllers are set up
			<-mgr.Elected()

			// Wait a bit more to ensure reconciliation has had a chance to patch ownerRefs
			time.Sleep(60 * time.Second)

			setupLog.Info("Starting cleanup of DatadogAgentInternal resources")
			if err = controller.CleanupDatadogAgentInternalResources(setupLog, restConfig); err != nil {
				setupLog.Error(err, "Failed to cleanup DatadogAgentInternal resources")
			}
		}()
	}

	options := controller.SetupOptions{
		SupportExtendedDaemonset: controller.ExtendedDaemonsetOptions{
			Enabled:                             opts.supportExtendedDaemonset,
			MaxPodUnavailable:                   opts.edsMaxPodUnavailable,
			SlowStartAdditiveIncrease:           opts.edsSlowStartAdditiveIncrease,
			CanaryDuration:                      opts.edsCanaryDuration,
			CanaryReplicas:                      opts.edsCanaryReplicas,
			CanaryAutoPauseEnabled:              opts.edsCanaryAutoPauseEnabled,
			CanaryAutoPauseMaxRestarts:          opts.edsCanaryAutoPauseMaxRestarts,
			CanaryAutoFailEnabled:               opts.edsCanaryAutoFailEnabled,
			CanaryAutoFailMaxRestarts:           opts.edsCanaryAutoFailMaxRestarts,
			CanaryAutoPauseMaxSlowStartDuration: opts.edsCanaryAutoPauseMaxSlowStartDuration,
			MaxPodSchedulerFailure:              opts.edsMaxPodSchedulerFailure,
		},
		SupportCilium:                 opts.supportCilium,
		CredsManager:                  credsManager,
		Creds:                         creds,
		SecretRefreshInterval:         opts.secretRefreshInterval,
		DatadogAgentEnabled:           opts.datadogAgentEnabled,
		DatadogAgentInternalEnabled:   opts.datadogAgentInternalEnabled,
		DatadogMonitorEnabled:         opts.datadogMonitorEnabled,
		DatadogSLOEnabled:             opts.datadogSLOEnabled,
		OperatorMetricsEnabled:        opts.operatorMetricsEnabled,
		V2APIEnabled:                  true,
		IntrospectionEnabled:          opts.introspectionEnabled,
		DatadogAgentProfileEnabled:    opts.datadogAgentProfileEnabled,
		DatadogDashboardEnabled:       opts.datadogDashboardEnabled,
		DatadogGenericResourceEnabled: opts.datadogGenericResourceEnabled,
	}

	versionInfo, platformInfo, err := getVersionAndPlatformInfo(rest.CopyConfig(mgr.GetConfig()))
	if err != nil {
		return err
	}

	if versionInfo != nil {
		gitVersion := versionInfo.GitVersion
		if !utils.IsAboveMinVersion(gitVersion, "1.16-0", nil) {
			setupLog.Error(nil, "Detected Kubernetes version <1.16 which requires CRD version apiextensions.k8s.io/v1beta1. "+
				"CRDs of this version were removed in v1.10.0.")
		}
	}
	// START controllers setup
	if err = controller.SetupControllers(setupLog, mgr, platformInfo, options); err != nil {
		return setupErrorf(setupLog, err, "Unable to start controllers")
	}

	go func() {
		// Block until this controller manager is elected leader
		<-mgr.Elected()
		setupLog.Info("Starting metadata forwarders")
		setupAndStartOperatorMetadataForwarder(metadataLog, mgr.GetAPIReader(), versionInfo.String(), opts, options.CredsManager)
		setupAndStartHelmMetadataForwarder(metadataLog, mgr.GetAPIReader(), versionInfo.String(), opts, options.CredsManager)
		setupAndStartCRDMetadataForwarder(metadataLog, mgr.GetAPIReader(), versionInfo.String(), opts, options.CredsManager)
	}()

	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		return setupErrorf(setupLog, err, "Problem running manager")
	}

	return nil
}

func getVersionAndPlatformInfo(configCopy *rest.Config) (*apimversion.Info, kubernetes.PlatformInfo, error) {
	// Never use original mgr.GetConfig(), always copy as clients might modify the configuration
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(configCopy)
	if err != nil {
		return nil, kubernetes.PlatformInfo{}, fmt.Errorf("unable to get discovery client: %w", err)
	}

	versionInfo, err := discoveryClient.ServerVersion()
	if err != nil {
		return nil, kubernetes.PlatformInfo{}, fmt.Errorf("unable to get APIServer version: %w", err)
	}

	groups, resources, err := getServerGroupsAndResources(setupLog, discoveryClient)
	if err != nil {
		return nil, kubernetes.PlatformInfo{}, fmt.Errorf("unable to get API resource versions: %w", err)
	}
	platformInfo := kubernetes.NewPlatformInfo(versionInfo, groups, resources)

	return versionInfo, platformInfo, nil

}

func getServerGroupsAndResources(log logr.Logger, discoveryClient *discovery.DiscoveryClient) ([]*v1.APIGroup, []*v1.APIResourceList, error) {
	groups, resources, err := discoveryClient.ServerGroupsAndResources()
	if err != nil {
		if !discovery.IsGroupDiscoveryFailedError(err) {
			log.Info("GetServerGroupsAndResources ERROR", "err", err)
			return nil, nil, err
		}
	}
	return groups, resources, nil
}

func customSetupLogging(logLevel zapcore.Level, logEncoder string) error {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	var encoder zapcore.Encoder
	switch logEncoder {
	case "console":
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	case "json":
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	default:
		return fmt.Errorf("unknow log encoder: %s", logEncoder)
	}

	zapOpts := ctrlzap.Options{}
	zapOpts.BindFlags(flag.CommandLine)
	zapOpts.Level = zap.NewAtomicLevelAt(logLevel)

	core := zap.WrapCore(func(c zapcore.Core) zapcore.Core {
		stdoutLevel := zap.LevelEnablerFunc(func(level zapcore.Level) bool {
			return (level == zapcore.InfoLevel || level == zapcore.DebugLevel) && zapOpts.Level.Enabled(level)
		})

		stderrLevel := zap.LevelEnablerFunc(func(level zapcore.Level) bool {
			return (level != zapcore.InfoLevel && level != zapcore.DebugLevel) && zapOpts.Level.Enabled(level)
		})

		stdoutSyncer := zapcore.Lock(os.Stdout)
		stderrSyncer := zapcore.Lock(os.Stderr)

		tee := zapcore.NewTee(
			zapcore.NewCore(encoder, stderrSyncer, stderrLevel),
			zapcore.NewCore(encoder, stdoutSyncer, stdoutLevel),
		)

		return tee
	})

	zapOpts.ZapOpts = append(zapOpts.ZapOpts, core)
	ctrl.SetLogger(ctrlzap.New(ctrlzap.UseFlagOptions(&zapOpts)))

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

func setupErrorf(logger logr.Logger, err error, msg string, keysAndValues ...any) error {
	setupLog.Error(err, msg, keysAndValues...)
	return fmt.Errorf("%s, err:%w", msg, err)
}

// checkRequiredCredentials checks if credentials are required by any enabled controllers
// and returns an error if they are required but the provided error indicates they
// could not be obtained.
func checkRequiredCredentials(opts *options, credErr error) error {
	// Check if credentials are required by any enabled controllers
	requireCreds := opts.datadogMonitorEnabled || opts.datadogDashboardEnabled || opts.datadogSLOEnabled || opts.datadogGenericResourceEnabled

	if requireCreds && credErr != nil {
		return setupErrorf(setupLog, credErr, "Unable to retrieve Datadog API credentials required by one or more enabled controllers",
			"DatadogMonitor", opts.datadogMonitorEnabled,
			"DatadogDashboard", opts.datadogDashboardEnabled,
			"DatadogSLO", opts.datadogSLOEnabled,
			"DatadogGenericResource", opts.datadogGenericResourceEnabled)
	}

	return nil
}

func setupAndStartOperatorMetadataForwarder(logger logr.Logger, client client.Reader, kubernetesVersion string, options *options, credsManager *config.CredentialManager) {
	omf := metadata.NewOperatorMetadataForwarder(logger, client, kubernetesVersion, version.GetVersion(), credsManager)
	omf.OperatorMetadata = metadata.OperatorMetadata{
		OperatorVersion:               version.GetVersion(),
		KubernetesVersion:             kubernetesVersion,
		InstallMethodTool:             "datadog-operator",
		InstallMethodToolVersion:      version.GetVersion(),
		IsLeader:                      true,
		DatadogAgentEnabled:           options.datadogAgentEnabled,
		DatadogMonitorEnabled:         options.datadogMonitorEnabled,
		DatadogDashboardEnabled:       options.datadogDashboardEnabled,
		DatadogSLOEnabled:             options.datadogSLOEnabled,
		DatadogGenericResourceEnabled: options.datadogGenericResourceEnabled,
		DatadogAgentProfileEnabled:    options.datadogAgentProfileEnabled,
		DatadogAgentInternalEnabled:   options.datadogAgentInternalEnabled,
		LeaderElectionEnabled:         options.enableLeaderElection,
		ExtendedDaemonSetEnabled:      options.supportExtendedDaemonset,
		RemoteConfigEnabled:           options.remoteConfigEnabled,
		IntrospectionEnabled:          options.introspectionEnabled,
		ConfigDDURL:                   os.Getenv(constants.DDURL),
		ConfigDDSite:                  os.Getenv(constants.DDSite),
		ResourceCounts:                make(map[string]int),
	}

	omf.Start()
}

func setupAndStartCRDMetadataForwarder(logger logr.Logger, client client.Reader, kubernetesVersion string, options *options, credsManager *config.CredentialManager) {
	cmf := metadata.NewCRDMetadataForwarder(
		logger,
		client,
		kubernetesVersion,
		version.GetVersion(),
		credsManager,
		metadata.EnabledCRDKindsConfig{
			DatadogAgentEnabled:         options.datadogAgentEnabled,
			DatadogAgentInternalEnabled: options.datadogAgentInternalEnabled,
			DatadogAgentProfileEnabled:  options.datadogAgentProfileEnabled,
		},
	)
	cmf.Start()
}

func setupAndStartHelmMetadataForwarder(logger logr.Logger, client client.Reader, kubernetesVersion string, options *options, credsManager *config.CredentialManager) {
	hmf := metadata.NewHelmMetadataForwarder(logger, client, kubernetesVersion, version.GetVersion(), credsManager)
	hmf.Start()
}
