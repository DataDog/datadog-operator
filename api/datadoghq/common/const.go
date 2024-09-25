// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

// TODO move most of these constants out of common

// This file tracks constants related to setting up the Datadog Agents

const (
	// AgentDeploymentNameLabelKey label key use to link a Resource to a DatadogAgent
	AgentDeploymentNameLabelKey = "agent.datadoghq.com/name"
	// AgentDeploymentComponentLabelKey label key use to know with component is it
	AgentDeploymentComponentLabelKey = "agent.datadoghq.com/component"
	// MD5AgentDeploymentProviderLabelKey label key is used to identify which provider is being used
	MD5AgentDeploymentProviderLabelKey = "agent.datadoghq.com/provider"
	// MD5AgentDeploymentAnnotationKey annotation key used on a Resource in order to identify which AgentDeployment have been used to generate it.
	MD5AgentDeploymentAnnotationKey = "agent.datadoghq.com/agentspechash"
	// MD5ChecksumAnnotationKey annotation key is used to identify customConfig configurations
	MD5ChecksumAnnotationKey = "checksum/%s-custom-config"
)

// Annotations
const (
	SystemProbeAppArmorAnnotationKey   = "container.apparmor.security.beta.kubernetes.io/system-probe"
	SystemProbeAppArmorAnnotationValue = "unconfined"

	AgentAppArmorAnnotationKey   = "container.apparmor.security.beta.kubernetes.io/agent"
	AgentAppArmorAnnotationValue = "unconfined"
)

// Datadog volume names and mount paths
const (
	ConfdVolumeName                = "confd"
	ConfdVolumePath                = "/conf.d"
	ConfigVolumeName               = "config"
	ConfigVolumePath               = "/etc/datadog-agent"
	KubeStateMetricCoreVolumeName  = "ksm-core-config"
	OrchestratorExplorerVolumeName = "orchestrator-explorer-config"
	ChecksdVolumeName              = "checksd"
	ChecksdVolumePath              = "/checks.d"

	HostRootVolumeName = "hostroot"
	HostRootHostPath   = "/"
	HostRootMountPath  = "/host/root"

	GroupVolumeName = "group"
	GroupHostPath   = "/etc/group"
	GroupMountPath  = "/etc/group"

	PasswdVolumeName = "passwd"
	PasswdHostPath   = "/etc/passwd"
	PasswdMountPath  = "/etc/passwd"

	ProcdirVolumeName = "procdir"
	ProcdirHostPath   = "/proc"
	ProcdirMountPath  = "/host/proc"

	CgroupsVolumeName = "cgroups"
	CgroupsHostPath   = "/sys/fs/cgroup"
	CgroupsMountPath  = "/host/sys/fs/cgroup"

	SystemProbeOSReleaseDirVolumeName = "host-osrelease"
	SystemProbeOSReleaseDirVolumePath = "/etc/os-release"
	SystemProbeOSReleaseDirMountPath  = "/host/etc/os-release"

	ContainerdDirVolumeName = "host-containerd-dir"
	ContainerdDirVolumePath = "/var/lib/containerd"
	ContainerdDirMountPath  = "/host/var/lib/containerd"

	ApkDirVolumeName = "host-apk-dir"
	ApkDirVolumePath = "/var/lib/apk"
	ApkDirMountPath  = "/host/var/lib/apk"

	DpkgDirVolumeName = "host-dpkg-dir"
	DpkgDirVolumePath = "/var/lib/dpkg"
	DpkgDirMountPath  = "/host/var/lib/dpkg"

	RpmDirVolumeName = "host-rpm-dir"
	RpmDirVolumePath = "/var/lib/rpm"
	RpmDirMountPath  = "/host/var/lib/rpm"

	RedhatReleaseVolumeName = "etc-redhat-release"
	RedhatReleaseVolumePath = "/etc/redhat-release"
	RedhatReleaseMountPath  = "/host/etc/redhat-release"

	FedoraReleaseVolumeName = "etc-fedora-release"
	FedoraReleaseVolumePath = "/etc/fedora-release"
	FedoraReleaseMountPath  = "/host/etc/fedora-release"

	LsbReleaseVolumeName = "etc-lsb-release"
	LsbReleaseVolumePath = "/etc/lsb-release"
	LsbReleaseMountPath  = "/host/etc/lsb-release"

	SystemReleaseVolumeName = "etc-system-release"
	SystemReleaseVolumePath = "/etc/system-release"
	SystemReleaseMountPath  = "/host/etc/system-release"

	SystemProbeSocketVolumeName = "sysprobe-socket-dir"
	SystemProbeSocketVolumePath = "/var/run/sysprobe"

	DebugfsVolumeName = "debugfs"
	// same path on host and container
	DebugfsPath = "/sys/kernel/debug"

	TracefsVolumeName = "tracefs"
	TracefsPath       = "/sys/kernel/tracing"

	SecurityfsVolumeName = "securityfs"
	SecurityfsVolumePath = "/sys/kernel/security"
	SecurityfsMountPath  = "/host/sys/kernel/security"

	ModulesVolumeName = "modules"
	// same path on host and container
	ModulesVolumePath = "/lib/modules"

	SrcVolumeName = "src"
	// same path on host and container
	SrcVolumePath = "/usr/src"

	AgentCustomConfigVolumePath = "/etc/datadog-agent/datadog.yaml"
	SystemProbeConfigVolumePath = "/etc/datadog-agent/system-probe.yaml"
	OtelCustomConfigVolumePath  = "/etc/datadog-agent/otel-config.yaml"

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
	DogstatsdHostPortName      = "dogstatsdport"
	DogstatsdHostPortHostPort  = 8125
	DogstatsdSocketVolumeName  = "dsdsocket"
	DogstatsdAPMSocketHostPath = "/var/run/datadog"
	DogstatsdSocketLocalPath   = "/var/run/datadog"
	DogstatsdSocketName        = "dsd.socket"
	// SecurityAgentComplianceCustomConfigDirVolumeName = "customcompliancebenchmarks" // Not used
	SecurityAgentComplianceConfigDirVolumeName   = "compliancedir"
	SecurityAgentComplianceConfigDirVolumePath   = "/etc/datadog-agent/compliance.d"
	SecurityAgentRuntimeCustomPoliciesVolumeName = "customruntimepolicies"
	SecurityAgentRuntimeCustomPoliciesVolumePath = "/etc/datadog-agent-runtime-policies"
	SecurityAgentRuntimePoliciesDirVolumeName    = "runtimepoliciesdir"
	SecurityAgentRuntimePoliciesDirVolumePath    = "/etc/datadog-agent/runtime-security.d"
	HostCriSocketPathPrefix                      = "/host"
	CriSocketVolumeName                          = "runtimesocketdir"
	RuntimeDirVolumePath                         = "/var/run"
	KubeletAgentCAPath                           = "/var/run/host-kubelet-ca.crt"
	KubeletCAVolumeName                          = "kubelet-ca"
	APMHostPortName                              = "traceport"
	// APMHostPortHostPort                              = 8126 // Not used
	APMSocketVolumeName                         = "apmsocket"
	APMSocketVolumeLocalPath                    = "/var/run/datadog"
	APMSocketName                               = "apm.socket"
	AdmissionControllerPortName                 = "admissioncontrollerport"
	AdmissionControllerSocketCommunicationMode  = "socket"
	ExternalMetricsPortName                     = "metricsapi"
	ExternalMetricsAPIServiceName               = "v1beta1.external.metrics.k8s.io"
	OTLPGRPCPortName                            = "otlpgrpcport"
	OTLPHTTPPortName                            = "otlphttpport"
	SeccompSecurityVolumeName                   = "datadog-agent-security"
	SeccompSecurityVolumePath                   = "/etc/config"
	SeccompRootVolumeName                       = "seccomp-root"
	SeccompRootVolumePath                       = "/host/var/lib/kubelet/seccomp"
	SeccompRootPath                             = "/var/lib/kubelet/seccomp"
	SystemProbeSeccompKey                       = "system-probe-seccomp.json"
	SystemProbeAgentSecurityConfigMapSuffixName = "system-probe-seccomp"
	SystemProbeSeccompProfileName               = "system-probe"

	AppArmorAnnotationKey = "container.apparmor.security.beta.kubernetes.io"

	AgentCustomConfigVolumeName    = "custom-datadog-yaml"
	AgentCustomConfigVolumeSubPath = "datadog.yaml"

	ClusterAgentCustomConfigVolumeName    = "custom-cluster-agent-yaml"
	ClusterAgentCustomConfigVolumePath    = "/etc/datadog-agent/datadog-cluster.yaml"
	ClusterAgentCustomConfigVolumeSubPath = "datadog-cluster.yaml"

	HelmCheckConfigVolumeName                = "helm-check-config"
	KubernetesAPIServerCheckConfigVolumeName = "kubernetes-apiserver-check-config"

	FIPSProxyCustomConfigVolumeName = "fips-proxy-cfg"
	FIPSProxyCustomConfigFileName   = "datadog-fips-proxy.cfg"
	FIPSProxyCustomConfigMapName    = "%s-fips-config"
	FIPSProxyCustomConfigMountPath  = "/etc/datadog-fips-proxy/datadog-fips-proxy.cfg"
)

const (
	// FieldPathSpecNodeName used as FieldPath for selecting the NodeName
	FieldPathSpecNodeName = "spec.nodeName"

	// FieldPathStatusHostIP used as FieldPath to retrieve the host ip
	FieldPathStatusHostIP = "status.hostIP"

	// FieldPathStatusPodIP used as FieldPath to retrieve the pod ip
	FieldPathStatusPodIP = "status.podIP"

	// FieldPathMetaName used as FieldPath to retrieve the pod name
	FieldPathMetaName = "metadata.name"
)
