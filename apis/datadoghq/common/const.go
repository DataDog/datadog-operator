// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

// Datadog const value
const (
	// AgentDeploymentNameLabelKey label key use to link a Resource to a DatadogAgent
	AgentDeploymentNameLabelKey = "agent.datadoghq.com/name"
	// AgentDeploymentComponentLabelKey label key use to know with component is it
	AgentDeploymentComponentLabelKey = "agent.datadoghq.com/component"
	// MD5AgentDeploymentAnnotationKey annotation key used on a Resource in order to identify which AgentDeployment have been used to generate it.
	MD5AgentDeploymentAnnotationKey = "agent.datadoghq.com/agentspechash"

	// DefaultAgentResourceSuffix use as suffix for agent resource naming
	DefaultAgentResourceSuffix = "agent"
	// DefaultClusterAgentResourceSuffix use as suffix for cluster-agent resource naming
	DefaultClusterAgentResourceSuffix = "cluster-agent"
	// DefaultClusterChecksRunnerResourceSuffix use as suffix for cluster-checks-runner resource naming
	DefaultClusterChecksRunnerResourceSuffix = "cluster-checks-runner"
	// DefaultMetricsServerResourceSuffix use as suffix for cluster-agent metrics-server resource naming
	DefaultMetricsServerResourceSuffix = "cluster-agent-metrics-server"
	// DefaultAPPKeyKey default app-key key (use in secret for instance).
	DefaultAPPKeyKey = "app_key"
	// DefaultAPIKeyKey default api-key key (use in secret for instance).
	DefaultAPIKeyKey = "api_key"
	// DefaultTokenKey default token key (use in secret for instance).
	DefaultTokenKey = "token"
	// DefaultClusterAgentServicePort default cluster-agent service port
	DefaultClusterAgentServicePort = 5005
	// DefaultMetricsServerServicePort default metrics-server port
	DefaultMetricsServerServicePort = 443
	// DefaultMetricsServerTargetPort default metrics-server pod port
	DefaultMetricsServerTargetPort = int(DefaultMetricsProviderPort)
	// DefaultAdmissionControllerServicePort default admission controller service port
	DefaultAdmissionControllerServicePort = 443
	// DefaultAdmissionControllerTargetPort default admission controller pod port
	DefaultAdmissionControllerTargetPort = 8000
	// DefaultDogstatsdPort default dogstatsd port
	DefaultDogstatsdPort = 8125
	// DefaultDogstatsdPortName default dogstatsd port name
	DefaultDogstatsdPortName = "dogstatsd"
	// DefaultApmPortName default apm port name
	DefaultApmPortName = "apm"
	// DefaultMetricsProviderPort default metrics provider port
	DefaultMetricsProviderPort int32 = 8443
	// DefaultKubeStateMetricsCoreConf default ksm core ConfigMap name
	DefaultKubeStateMetricsCoreConf string = "kube-state-metrics-core-config"
)

// Datadog volume names and mount paths
const (
	ConfdVolumeName               = "confd"
	ConfdVolumePath               = "/conf.d"
	ConfigVolumeName              = "config"
	ConfigVolumePath              = "/etc/datadog-agent"
	KubeStateMetricCoreVolumeName = "ksm-core-config"

	PointerVolumeName     = "pointerdir"
	PointerVolumePath     = "/opt/datadog-agent/run"
	LogTempStoragePath    = "/var/lib/datadog-agent/logs"
	PointerVolumeReadOnly = false

	LogPodVolumeName     = "logpodpath"
	LogPodVolumePath     = "/var/log/pods"
	LogPodVolumeReadOnly = true

	LogContainerVolumeName     = "logcontainerpath"
	LogContainerVolumePath     = "/var/lib/docker/containers"
	LogContainerVolumeReadOnly = true

	SymlinkContainerVolumeName     = "symlinkcontainerpath"
	SymlinkContainerVolumePath     = "/var/log/containers"
	SymlinkContainerVolumeReadOnly = true
)
