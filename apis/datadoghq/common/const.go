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
	// DefaultSysprobeSocketPath default system probe socket path
	DefaultSysprobeSocketPath = "/var/run/sysprobe/sysprobe.sock"

	// Liveness probe default config
	DefaultLivenessProbeInitialDelaySeconds int32 = 15
	DefaultLivenessProbePeriodSeconds       int32 = 15
	DefaultLivenessProbeTimeoutSeconds      int32 = 5
	DefaultLivenessProbeSuccessThreshold    int32 = 1
	DefaultLivenessProbeFailureThreshold    int32 = 6
	DefaultAgentHealthPort                  int32 = 5555
	DefaultLivenessProbeHTTPPath                  = "/live"

	// Readiness probe default config
	DefaultReadinessProbeInitialDelaySeconds int32 = 15
	DefaultReadinessProbePeriodSeconds       int32 = 15
	DefaultReadinessProbeTimeoutSeconds      int32 = 5
	DefaultReadinessProbeSuccessThreshold    int32 = 1
	DefaultReadinessProbeFailureThreshold    int32 = 6
	DefaultReadinessProbeHTTPPath                  = "/ready"
)

// Datadog volume names and mount paths
const (
	ConfdVolumeName               = "confd"
	ConfdVolumePath               = "/conf.d"
	ConfigVolumeName              = "config"
	ConfigVolumePath              = "/etc/datadog-agent"
	KubeStateMetricCoreVolumeName = "ksm-core-config"

	ProcdirVolumeName = "procdir"
	ProcdirHostPath   = "/proc"
	ProcdirMountPath  = "/host/proc"

	CgroupsVolumeName = "cgroups"
	CgroupsHostPath   = "/sys/fs/cgroup"
	CgroupsMountPath  = "/host/sys/fs/cgroup"

	DebugfsVolumeName = "debugfs"
	DebugfsVolumePath = "/sys/kernel/debug"

	SysprobeSocketVolumeName = "sysprobe-socket-dir"
	SysprobeSocketVolumePath = "/var/run/sysprobe"

	ModulesVolumeName = "modules"
	// same path on host and container
	ModulesVolumePath = "/lib/modules"
	SrcVolumeName     = "src"
	// same path on host and container
	SrcVolumePath              = "/usr/src"
	LogDatadogVolumeName       = "logdatadog"
	LogDatadogVolumePath       = "/var/log/datadog"
	TmpVolumeName              = "tmp"
	TmpVolumePath              = "/tmp"
	CertificatesVolumeName     = "certificates"
	CertificatesVolumePath     = "/etc/datadog-agent/certificates"
	AuthVolumeName             = "datadog-agent-auth"
	AuthVolumePath             = "/etc/datadog-agent/auth"
	InstallInfoVolumeName      = "installinfo"
	InstallInfoVolumeSubPath   = "install_info"
	InstallInfoVolumePath      = "/etc/datadog-agent/install_info"
	InstallInfoVolumeReadOnly  = true
	PointerVolumeName          = "pointerdir"
	PointerVolumePath          = "/opt/datadog-agent/run"
	LogTempStoragePath         = "/var/lib/datadog-agent/logs"
	PodLogVolumeName           = "logpodpath"
	PodLogVolumePath           = "/var/log/pods"
	ContainerLogVolumeName     = "logcontainerpath"
	ContainerLogVolumePath     = "/var/lib/docker/containers"
	SymlinkContainerVolumeName = "symlinkcontainerpath"
	SymlinkContainerVolumePath = "/var/log/containers"
)
