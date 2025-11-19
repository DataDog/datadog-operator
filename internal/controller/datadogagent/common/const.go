// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

// This file tracks constants used in features, component default code

// Resource names
const (
	DatadogTokenOldResourceName          = "datadogtoken"            // Kept for backward compatibility with agent <7.37.0
	DatadogLeaderElectionOldResourceName = "datadog-leader-election" // Kept for backward compatibility with agent <7.37.0
	DatadogCustomMetricsResourceName     = "datadog-custom-metrics"
	DatadogClusterIDResourceName         = "datadog-cluster-id"
	ExtensionAPIServerAuthResourceName   = "extension-apiserver-authentication"
	KubeSystemResourceName               = "kube-system"

	NodeAgentSuffix    = "node"
	ChecksRunnerSuffix = "ccr"
	ClusterAgentSuffix = "dca"

	CustomResourceDefinitionsName = "customresourcedefinitions"

	DefaultAgentInstallType = "k8s_manual"
)

// APM Telemetry
const (
	APMTelemetryConfigMapName  = "datadog-apm-telemetry-kpi"
	APMTelemetryInstallIdKey   = "install_id"
	APMTelemetryInstallTimeKey = "install_time"
	APMTelemetryInstallTypeKey = "install_type"
)

// Annotations
const (
	AppArmorAnnotationKey = "container.apparmor.security.beta.kubernetes.io"

	SystemProbeAppArmorAnnotationKey   = "container.apparmor.security.beta.kubernetes.io/system-probe"
	SystemProbeAppArmorAnnotationValue = "unconfined"
)

// Condition types
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
)

const (
	// DefaultTokenKey default token key (use in secret for instance).
	DefaultTokenKey = "token"
	// DefaultClusterAgentServicePort default cluster-agent service port
	DefaultClusterAgentServicePort = 5005
	// DefaultDogstatsdPort default dogstatsd port
	DefaultDogstatsdPort = 8125
	// DefaultSystemProbeSocketPath default System Probe socket path
	DefaultSystemProbeSocketPath string = "/var/run/sysprobe/sysprobe.sock"
)

// Volumes and paths
const (
	ConfdVolumeName   = "confd"
	ConfdVolumePath   = "/conf.d"
	ConfigVolumeName  = "config"
	ConfigVolumePath  = "/etc/datadog-agent"
	ChecksdVolumeName = "checksd"
	ChecksdVolumePath = "/checks.d"

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

	SystemProbeSocketVolumeName = "sysprobe-socket-dir"
	SystemProbeSocketVolumePath = "/var/run/sysprobe"

	DebugfsVolumeName = "debugfs"
	// same path on host and container
	DebugfsPath = "/sys/kernel/debug"

	ModulesVolumeName = "modules"
	// same path on host and container
	ModulesVolumePath = "/lib/modules"

	SrcVolumeName = "src"
	// same path on host and container
	SrcVolumePath = "/usr/src"

	LogDatadogVolumeName      = "logdatadog"
	LogDatadogVolumePath      = "/var/log/datadog"
	DefaultLogTempStoragePath = "/var/lib/datadog-agent/logs"
	TmpVolumeName             = "tmp"
	TmpVolumePath             = "/tmp"
	CertificatesVolumeName    = "certificates"
	CertificatesVolumePath    = "/etc/datadog-agent/certificates"
	AuthVolumeName            = "datadog-agent-auth"
	AuthVolumePath            = "/etc/datadog-agent/auth"
	InstallInfoVolumeName     = "installinfo"
	InstallInfoVolumeSubPath  = "install_info"
	InstallInfoVolumePath     = "/etc/datadog-agent/install_info"
	InstallInfoVolumeReadOnly = true

	DogstatsdSocketVolumeName  = "dsdsocket"
	DogstatsdAPMSocketHostPath = "/var/run/datadog"
	DogstatsdSocketLocalPath   = "/var/run/datadog"
	DogstatsdSocketName        = "dsd.socket"

	HostCriSocketPathPrefix = "/host"
	CriSocketVolumeName     = "runtimesocketdir"
	RuntimeDirVolumePath    = "/var/run"

	KubeletAgentCAPath            = "/var/run/host-kubelet-ca.crt"
	KubeletPodResourcesVolumeName = "kubelet-pod-resources"

	APMSocketVolumeName = "apmsocket"
	APMSocketName       = "apm.socket"

	SeccompSecurityVolumeName                   = "datadog-agent-security"
	SeccompSecurityVolumePath                   = "/etc/config"
	SeccompRootVolumeName                       = "seccomp-root"
	SeccompRootVolumePath                       = "/host/var/lib/kubelet/seccomp"
	SeccompRootPath                             = "/var/lib/kubelet/seccomp"
	SystemProbeSeccompKey                       = "system-probe-seccomp.json"
	SystemProbeAgentSecurityConfigMapSuffixName = "system-probe-seccomp"
	SystemProbeSeccompProfileName               = "system-probe"

	HostRunVolumeName = "hostrun"
	HostRunPath       = "/run"
	HostRunMountPath  = "/host/run"

	EventSocketVolumeName = "eventsocket"
	EventSocketMountPath  = "/opt/datadog-agent/run"
)

// Field paths
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
