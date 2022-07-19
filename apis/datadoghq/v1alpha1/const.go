// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

// Datadog env var names
const (
	// Datadog volume names and mount paths
	LogDatadogVolumeName   = "logdatadog"
	LogDatadogVolumePath   = "/var/log/datadog"
	TmpVolumeName          = "tmp"
	TmpVolumePath          = "/tmp"
	CertificatesVolumeName = "certificates"
	CertificatesVolumePath = "/etc/datadog-agent/certificates"
	APMSocketVolumeName    = "apmsocket"
	APMSocketVolumePath    = "/var/run/datadog/apm"
	ChecksdVolumeName      = "checksd"
	ChecksdVolumePath      = "/checks.d"

	ProcVolumeName                       = "procdir"
	ProcVolumePath                       = "/host/proc"
	ProcVolumeReadOnly                   = true
	PasswdVolumeName                     = "passwd"
	PasswdVolumePath                     = "/etc/passwd"
	GroupVolumeName                      = "group"
	GroupVolumePath                      = "/etc/group"
	CgroupsVolumeName                    = "cgroups"
	CgroupsVolumePath                    = "/host/sys/fs/cgroup"
	CgroupsVolumeReadOnly                = true
	SystemProbeSocketVolumeName          = "sysprobe-socket-dir"
	SystemProbeSocketVolumePath          = "/var/run/sysprobe"
	CriSocketVolumeName                  = "runtimesocketdir"
	CriSocketVolumeReadOnly              = true
	PointerVolumeName                    = "pointerdir"
	PointerVolumePath                    = "/opt/datadog-agent/run"
	LogPodVolumeName                     = "logpodpath"
	LogPodVolumePath                     = "/var/log/pods"
	LogPodVolumeReadOnly                 = true
	LogContainerVolumeName               = "logcontainerpath"
	LogContainerVolumeReadOnly           = true
	SymlinkContainerVolumeName           = "symlinkcontainerpath"
	SymlinkContainerVolumeReadOnly       = true
	SystemProbeDebugfsVolumeName         = "debugfs"
	SystemProbeDebugfsVolumePath         = "/sys/kernel/debug"
	SystemProbeConfigVolumeName          = "system-probe-config"
	SystemProbeConfigVolumeSubPath       = "system-probe.yaml"
	SystemProbeAgentSecurityVolumeName   = "datadog-agent-security"
	SystemProbeAgentSecurityVolumePath   = "/etc/config"
	SystemProbeSecCompRootVolumeName     = "seccomp-root"
	SystemProbeSecCompRootVolumePath     = "/host/var/lib/kubelet/seccomp"
	SystemProbeLibModulesVolumeName      = "modules"
	SystemProbeLibModulesVolumePath      = "/lib/modules"
	SystemProbeUsrSrcVolumeName          = "src"
	SystemProbeUsrSrcVolumePath          = "/usr/src"
	AgentCustomConfigVolumeName          = "custom-datadog-yaml"
	AgentCustomConfigVolumeSubPath       = "datadog.yaml"
	KubeletCAVolumeName                  = "kubelet-ca"
	DefaultKubeletAgentCAPath            = "/var/run/host-kubelet-ca.crt"
	OrchestratorExplorerConfigVolumeName = "orchestrator-explorer-config"

	HostCriSocketPathPrefix = "/host"

	SecurityAgentRuntimeCustomPoliciesVolumeName     = "customruntimepolicies"
	SecurityAgentRuntimePoliciesDirVolumeName        = "runtimepoliciesdir"
	SecurityAgentRuntimePoliciesDirVolumePath        = "/etc/datadog-agent/runtime-security.d"
	SecurityAgentComplianceCustomConfigDirVolumeName = "customcompliancebenchmarks"
	SecurityAgentComplianceConfigDirVolumeName       = "compliancedir"
	SecurityAgentComplianceConfigDirVolumePath       = "/etc/datadog-agent/compliance.d"

	ClusterAgentCustomConfigVolumeName    = "custom-datadog-yaml"
	ClusterAgentCustomConfigVolumePath    = "/etc/datadog-agent/datadog-cluster.yaml"
	ClusterAgentCustomConfigVolumeSubPath = "datadog-cluster.yaml"

	SysteProbeAppArmorAnnotationKey   = "container.apparmor.security.beta.kubernetes.io/system-probe"
	SysteProbeSeccompAnnotationKey    = "container.seccomp.security.alpha.kubernetes.io/system-probe"
	SystemProbeOSReleaseDirVolumeName = "host-osrelease"
	SystemProbeOSReleaseDirVolumePath = "/etc/os-release"
	SystemProbeOSReleaseDirMountPath  = "/host/etc/os-release"
)
