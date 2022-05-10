package common

// Resource names
const (
	DatadogTokenOldResourceName          = "datadogtoken"            // Kept for backward compatibility with agent <7.37.0
	DatadogLeaderElectionOldResourceName = "datadog-leader-election" // Kept for backward compatibility with agent <7.37.0
	DatadogCustomMetricsResourceName     = "datadog-custom-metrics"
	DatadogClusterIDResourceName         = "datadog-cluster-id"
	ExtensionAPIServerAuthResourceName   = "extension-apiserver-authentication"
	KubeSystemResourceName               = "kube-system"

	CheckRunnersSuffix = "ccr"
	ClusterAgentSuffix = "dca"
)
