// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/utils"
)

const (
	ProcessConfigRunInCoreAgentMinVersion = "7.60.0-0"
	// ADPDogstatsdDelegationMinVersion is the minimum Agent version that natively disables Core Agent
	// DogStatsD when data_plane.enabled and data_plane.dogstatsd.enabled are both true. Below this
	// version the Operator must set DD_USE_DOGSTATSD=false explicitly to avoid a bind conflict.
	ADPDogstatsdDelegationMinVersion = "7.75.0-0"
	// AgentIPCUnixSocketMinVersion is the minimum Agent version supporting the
	// agent_ipc unix socket transport; older agents ignore it and would silently
	// lose config sync, so they keep the TCP port.
	AgentIPCUnixSocketMinVersion = "7.87.0-0"
	// DefaultDataPlaneMinAgentVersion is the minimum Agent version that receives the Operator-level Data Plane default.
	// Explicit Data Plane configuration remains available for older versions.
	DefaultDataPlaneMinAgentVersion   = "7.84.0-0"
	EnableADPAnnotation               = "agent.datadoghq.com/adp-enabled"
	EnableFineGrainedKubeletAuthz     = "agent.datadoghq.com/fine-grained-kubelet-authorization-enabled"
	EnableKSMApiServerCacheAnnotation = "agent.datadoghq.com/ksm-use-apiserver-cache"

	EnableInstrumentationCRDAnnotation = "agent.datadoghq.com/instrumentation-crd-enabled"

	EnableFlightRecorderAnnotation = "agent.datadoghq.com/flightrecorder-enabled"
	EnableNetworkCRDsAnnotation    = "agent.datadoghq.com/network-crds-enabled"

	EnablePrivateActionRunnerAnnotation                     = "agent.datadoghq.com/private-action-runner-enabled"
	EnablePrivateActionRunnerSplitAnnotation                = "agent.datadoghq.com/private-action-runner-split-enabled"
	PrivateActionRunnerConfigDataAnnotation                 = "agent.datadoghq.com/private-action-runner-configdata"
	EnablePrivateActionRunnerSystemdAnnotation              = "agent.datadoghq.com/private-action-runner-systemd-enabled"
	PrivateActionRunnerSystemdJournalStorageAnnotation      = "agent.datadoghq.com/private-action-runner-systemd-journal-storage"
	EnablePrivateActionRunnerSystemdJournalVacuumAnnotation = "agent.datadoghq.com/private-action-runner-systemd-journal-vacuum-enabled"
	EnablePrivateActionRunnerPacketCaptureAnnotation        = "agent.datadoghq.com/private-action-runner-packet-capture-enabled"

	EnableCNMDirectSendAnnotation = "agent.datadoghq.com/cnm-direct-send-enabled"
	EnableKueueAnnotation         = "agent.datadoghq.com/kueue-enabled"
	KueueConfigAnnotation         = "agent.datadoghq.com/kueue-config"

	EnableKAISchedulerAnnotation = "agent.datadoghq.com/kai-scheduler-enabled"

	// DefaultAgentIpcPort and DefaultAgentIpcConfigRefreshInterval configure config sync, which
	// serves resolved config (notably a secret-backed api_key) from the core agent to sub-processes.
	DefaultAgentIpcPort                  = "5009"
	DefaultAgentIpcConfigRefreshInterval = "60"

	EnableClusterAgentPrivateActionRunnerAnnotation      = "cluster-agent.datadoghq.com/private-action-runner-enabled"
	ClusterAgentPrivateActionRunnerConfigDataAnnotation  = "cluster-agent.datadoghq.com/private-action-runner-configdata"
	ClusterAgentPrivateActionRunnerK8sRemediationEnabled = "cluster-agent.datadoghq.com/private-action-runner-k8s-remediation-enabled"

	EnableCheckRunnerAnnotation = "agent.datadoghq.com/check-runner-enabled"
)

// Host Profiler annotations are deprecated in favor of spec.features.hostProfiler and will be removed
// in a future release. Until then, spec.features.hostProfiler takes precedence and the annotations
// are still read as a fallback for settings not defined in the spec.
const (
	// Deprecated: use spec.features.hostProfiler.enabled.
	EnableHostProfilerAnnotation = "agent.datadoghq.com/host-profiler-enabled"
	// EnableHostProfilerSeccompAnnotation controls whether the host-profiler applies its localhost
	// seccomp profile (and the init container that installs it on the node). Defaults to enabled;
	// set to "false" to disable both the seccomp profile and its setup init container.
	//
	// Deprecated: use spec.features.hostProfiler.seccomp.enabled.
	EnableHostProfilerSeccompAnnotation = "agent.datadoghq.com/host-profiler-seccomp-enabled"
	// Deprecated: use spec.features.hostProfiler.seccomp.loggingEnabled.
	EnableHostProfilerLoggingSeccompAnnotation = "agent.datadoghq.com/host-profiler-logging-seccomp-enabled"
	// HostProfilerSELinuxTypeAnnotation overrides the SELinux type applied to the host-profiler container.
	//
	// Deprecated: use spec.features.hostProfiler.seLinuxType.
	HostProfilerSELinuxTypeAnnotation = "agent.datadoghq.com/host-profiler-selinux-type"
	// Deprecated: use spec.features.hostProfiler.runAsRoot (inverted: "true" here means runAsRoot: false).
	EnableHostProfilerNonRootAnnotation = "agent.datadoghq.com/host-profiler-non-root-enabled"
)

func agentSupportsRunInCoreAgent(ddaSpec *v2alpha1.DatadogAgentSpec) bool {
	return utils.IsAboveMinVersion(common.GetComponentVersionFromSpec(ddaSpec, v2alpha1.NodeAgentComponentName), ProcessConfigRunInCoreAgentMinVersion, nil)
}

// ShouldRunProcessChecksInCoreAgent determines whether process checks should run in the core agent
// based on the provider and agent version. Windows agents always run process checks in the
// standalone process-agent. Agents >= 7.60.0 support running process checks in the core agent on
// Linux. As of Agent 7.78, process checks always run in the core agent on Linux and the
// DD_PROCESS_CONFIG_RUN_IN_CORE_AGENT_ENABLED envvar is no longer recognized.
func ShouldRunProcessChecksInCoreAgent(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec) bool {
	return !isWindowsProfile(dda) && agentSupportsRunInCoreAgent(ddaSpec)
}

func isWindowsProfile(obj metav1.Object) bool {
	return obj.GetLabels()[constants.ProfileLabelKey] != "" &&
		obj.GetAnnotations()[kubernetes.ProviderAnnotationKey] == kubernetes.WindowsProvider
}

func HasFeatureEnableAnnotation(dda metav1.Object, annotation string) bool {
	if value, ok := dda.GetAnnotations()[annotation]; ok {
		return value == "true"
	}
	return false
}

// HasFeatureDisableAnnotation returns true if the annotation is explicitly set to "false".
// It is used by features that are enabled by default and can be opted out of.
func HasFeatureDisableAnnotation(dda metav1.Object, annotation string) bool {
	if value, ok := dda.GetAnnotations()[annotation]; ok {
		return value == "false"
	}
	return false
}

func GetFeatureConfigAnnotation(dda metav1.Object, annotation string) (string, bool) {
	value, ok := dda.GetAnnotations()[annotation]
	return value, ok
}

// AgentSupportsADPDogstatsdDelegation returns true if the agent version is >= 7.75.0, meaning it
// natively disables Core DogStatsD when data_plane.enabled + data_plane.dogstatsd.enabled are true.
// For older agents the Operator must set DD_USE_DOGSTATSD=false explicitly.
func AgentSupportsADPDogstatsdDelegation(ddaSpec *v2alpha1.DatadogAgentSpec) bool {
	return utils.IsAboveMinVersion(common.GetComponentVersionFromSpec(ddaSpec, v2alpha1.NodeAgentComponentName), ADPDogstatsdDelegationMinVersion, nil)
}

// IsDataPlaneEnabled returns true if the Data Plane is enabled.
// CRD configuration takes precedence over the legacy annotation, which takes precedence over defaultEnabled.
func IsDataPlaneEnabled(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, defaultEnabled bool) bool {
	// CRD takes precedence.
	if ddaSpec.Features != nil && ddaSpec.Features.DataPlane != nil && ddaSpec.Features.DataPlane.Enabled != nil {
		return *ddaSpec.Features.DataPlane.Enabled
	}

	// Fall back to the legacy annotation before applying the Operator default.
	if HasFeatureDisableAnnotation(dda, EnableADPAnnotation) {
		return false
	}
	if HasFeatureEnableAnnotation(dda, EnableADPAnnotation) {
		return true
	}

	return defaultEnabled && AgentSupportsDefaultDataPlane(ddaSpec)
}

// AgentSupportsDefaultDataPlane checks the Agent image selected by the spec.
// The experimental per-container image override can later replace that image.
// Unlike other version checks in this file, an unparseable version defaults to false: this gates an
// implicit, Operator-level default rather than an explicit user opt-in, so an unknown Agent version
// must not silently enable the Data Plane.
func AgentSupportsDefaultDataPlane(ddaSpec *v2alpha1.DatadogAgentSpec) bool {
	return utils.IsAboveMinVersion(common.GetComponentVersionFromSpec(ddaSpec, v2alpha1.NodeAgentComponentName), DefaultDataPlaneMinAgentVersion, new(false))
}

// IsDataPlaneDogstatsdEnabled returns true if the Data Plane should handle DogStatsD.
// Defaults to true: when data_plane.enabled=true, ADP handles DogStatsD unless explicitly disabled.
func IsDataPlaneDogstatsdEnabled(ddaSpec *v2alpha1.DatadogAgentSpec) bool {
	if ddaSpec.Features != nil && ddaSpec.Features.DataPlane != nil &&
		ddaSpec.Features.DataPlane.Dogstatsd != nil && ddaSpec.Features.DataPlane.Dogstatsd.Enabled != nil {
		return *ddaSpec.Features.DataPlane.Dogstatsd.Enabled
	}
	return true
}

func ShouldCreateLocalAgentService(ddaSpec *v2alpha1.DatadogAgentSpec, platformInfo kubernetes.PlatformInfo) bool {
	forceEnableLocalService := false
	if ddaSpec != nil && ddaSpec.Global != nil && ddaSpec.Global.LocalService != nil {
		forceEnableLocalService = apiutils.BoolValue(ddaSpec.Global.LocalService.ForceEnableLocalService)
	}

	return common.ShouldCreateAgentLocalService(platformInfo.GetVersionInfo(), forceEnableLocalService)
}

// AgentIPCConfigSyncEnvVars returns the agent_ipc env vars that config sync
// requires: the unix socket transport on agents that support it, the TCP port
// otherwise.
func AgentIPCConfigSyncEnvVars(ddaSpec *v2alpha1.DatadogAgentSpec) []*corev1.EnvVar {
	transportEnvVar := &corev1.EnvVar{
		Name:  common.DDAgentIpcPort,
		Value: DefaultAgentIpcPort,
	}
	if utils.IsAboveMinVersion(common.GetComponentVersionFromSpec(ddaSpec, v2alpha1.NodeAgentComponentName), AgentIPCUnixSocketMinVersion, nil) {
		transportEnvVar = &corev1.EnvVar{
			Name:  common.DDAgentIpcUseSocket,
			Value: "true",
		}
	}
	return []*corev1.EnvVar{
		transportEnvVar,
		{Name: common.DDAgentIpcConfigRefreshInterval, Value: DefaultAgentIpcConfigRefreshInterval},
	}
}

// EnableConfigSyncForDirectSend sets the agent_ipc env vars that config sync requires
// on the given containers: with direct send, system-probe cannot resolve an ENC[...]
// api_key itself and needs the value resolved by the core agent. Values match the
// otelcollector and hostprofiler features; user-provided values win.
func EnableConfigSyncForDirectSend(managers feature.PodTemplateManagers, ddaSpec *v2alpha1.DatadogAgentSpec, containers []apicommon.AgentContainerName) {
	keepCurrent := func(current, _ *corev1.EnvVar) (*corev1.EnvVar, error) { return current, nil }
	for _, envVar := range AgentIPCConfigSyncEnvVars(ddaSpec) {
		for _, container := range containers {
			managers.EnvVar().AddEnvVarToContainerWithMergeFunc(container, envVar, keepCurrent)
		}
	}
}

// IsKubernetesActionsRCEnabled returns whether Kubernetes Actions should be delivered to the
// Cluster Agent through Remote Configuration. Defaults to true when unset.
func IsKubernetesActionsRCEnabled(ddaSpec *v2alpha1.DatadogAgentSpec) bool {
	if ddaSpec == nil || ddaSpec.Features == nil || ddaSpec.Features.KubernetesActions == nil {
		return false
	}
	ka := ddaSpec.Features.KubernetesActions
	if !apiutils.BoolValue(ka.Enabled) {
		return false
	}
	if ka.Communication == nil || ka.Communication.RemoteConfiguration == nil || ka.Communication.RemoteConfiguration.Enabled == nil {
		return true
	}
	return *ka.Communication.RemoteConfiguration.Enabled
}

// KubernetesActionsMinVersion is the minimum Cluster Agent version that supports the Kubernetes
// Actions feature, through both Remote Configuration and the Private Action Runner.
const KubernetesActionsMinVersion = "7.84.0-0"

// IsKubernetesActionsPARRequested returns whether the DatadogAgent asks for Kubernetes Actions to be
// delivered through the Cluster Agent Private Action Runner, regardless of the Cluster Agent version.
// Defaults to true when unset.
func IsKubernetesActionsPARRequested(ddaSpec *v2alpha1.DatadogAgentSpec) bool {
	if ddaSpec == nil || ddaSpec.Features == nil || ddaSpec.Features.KubernetesActions == nil {
		return false
	}
	ka := ddaSpec.Features.KubernetesActions
	if !apiutils.BoolValue(ka.Enabled) {
		return false
	}
	if ka.Communication == nil || ka.Communication.PrivateActionRunner == nil || ka.Communication.PrivateActionRunner.Enabled == nil {
		return true
	}
	return *ka.Communication.PrivateActionRunner.Enabled
}

// IsKubernetesActionsPAREnabled returns whether Kubernetes Actions should be delivered to the
// Cluster Agent through the Cluster Agent Private Action Runner: it is requested and the Cluster
// Agent supports Kubernetes Actions.
func IsKubernetesActionsPAREnabled(ddaSpec *v2alpha1.DatadogAgentSpec) bool {
	return IsKubernetesActionsPARRequested(ddaSpec) &&
		utils.IsAboveMinVersion(common.GetComponentVersionFromSpec(ddaSpec, v2alpha1.ClusterAgentComponentName), KubernetesActionsMinVersion, nil)
}
