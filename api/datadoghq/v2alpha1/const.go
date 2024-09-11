package v2alpha1

const (

	// ClusterAgentReconcileConditionType ReconcileConditionType for Cluster Agent component
	ClusterAgentReconcileConditionType = "ClusterAgentReconcile"
	// AgentReconcileConditionType ReconcileConditionType for Agent component
	AgentReconcileConditionType = "AgentReconcile"
	// ClusterChecksRunnerReconcileConditionType ReconcileConditionType for Cluster Checks Runner component
	ClusterChecksRunnerReconcileConditionType = "ClusterChecksRunnerReconcile"
	// OverrideReconcileConflictConditionType ReconcileConditionType for override conflict
	OverrideReconcileConflictConditionType = "OverrideReconcileConflict"
	// DatadogAgentReconcileErrorConditionType ReconcileConditionType for DatadogAgent reconcile error
	DatadogAgentReconcileErrorConditionType = "DatadogAgentReconcileError"

	// ExtraConfdConfigMapName is the name of the ConfigMap storing Custom Confd data
	ExtraConfdConfigMapName = "%s-extra-confd"
	// ExtraChecksdConfigMapName is the name of the ConfigMap storing Custom Checksd data
	ExtraChecksdConfigMapName = "%s-extra-checksd"

	// DefaultAgentHealthPort default agent health port
	DefaultAgentHealthPort int32 = 5555

	// Liveness probe default config
	DefaultLivenessProbeInitialDelaySeconds int32 = 15
	DefaultLivenessProbePeriodSeconds       int32 = 15
	DefaultLivenessProbeTimeoutSeconds      int32 = 5
	DefaultLivenessProbeSuccessThreshold    int32 = 1
	DefaultLivenessProbeFailureThreshold    int32 = 6
	DefaultLivenessProbeHTTPPath                  = "/live"

	// Readiness probe default config
	DefaultReadinessProbeInitialDelaySeconds int32 = 15
	DefaultReadinessProbePeriodSeconds       int32 = 15
	DefaultReadinessProbeTimeoutSeconds      int32 = 5
	DefaultReadinessProbeSuccessThreshold    int32 = 1
	DefaultReadinessProbeFailureThreshold    int32 = 6
	DefaultReadinessProbeHTTPPath                  = "/ready"

	// Startup probe default config
	DefaultStartupProbeInitialDelaySeconds int32 = 15
	DefaultStartupProbePeriodSeconds       int32 = 15
	DefaultStartupProbeTimeoutSeconds      int32 = 5
	DefaultStartupProbeSuccessThreshold    int32 = 1
	DefaultStartupProbeFailureThreshold    int32 = 6
	DefaultStartupProbeHTTPPath                  = "/startup"

	// DefaultApmPort default apm port
	DefaultApmPort = 8126
	// DefaultApmPortName default apm port name
	DefaultApmPortName = "traceport"
)
