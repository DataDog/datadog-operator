package common

// Resource names
const (
	DatadogTokenResourceName           = "datadogtoken"
	DatadogLeaderElectionResourceName  = "datadog-leader-election"
	DatadogCustomMetricsResourceName   = "datadog-custom-metrics"
	DatadogClusterIDResourceName       = "datadog-cluster-id"
	ExtensionAPIServerAuthResourceName = "extension-apiserver-authentication"
	KubeSystemResourceName             = "kube-system"

	CheckRunnersSuffix = "ccr"
	ClusterAgentSuffix = "dca"
)

// container names
const (
	ClusterAgentContainerName = "cluster-agent"
	AgentContainerName        = "agent"
)
