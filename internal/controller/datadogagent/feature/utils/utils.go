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
	"github.com/DataDog/datadog-operator/pkg/images"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/utils"
)

const (
	ProcessConfigRunInCoreAgentMinVersion = "7.60.0-0"
	// ADPDogstatsdDelegationMinVersion is the minimum Agent version that natively disables Core Agent
	// DogStatsD when data_plane.enabled and data_plane.dogstatsd.enabled are both true. Below this
	// version the Operator must set DD_USE_DOGSTATSD=false explicitly to avoid a bind conflict.
	ADPDogstatsdDelegationMinVersion = "7.75.0-0"
	EnableADPAnnotation              = "agent.datadoghq.com/adp-enabled"
	EnableFineGrainedKubeletAuthz    = "agent.datadoghq.com/fine-grained-kubelet-authorization-enabled"
	EnableHostProfilerAnnotation     = "agent.datadoghq.com/host-profiler-enabled"
	// EnableHostProfilerSeccompAnnotation controls whether the host-profiler applies its localhost
	// seccomp profile (and the init container that installs it on the node). Defaults to enabled;
	// set to "false" to disable both the seccomp profile and its setup init container.
	EnableHostProfilerSeccompAnnotation        = "agent.datadoghq.com/host-profiler-seccomp-enabled"
	EnableHostProfilerLoggingSeccompAnnotation = "agent.datadoghq.com/host-profiler-logging-seccomp-enabled"
	// HostProfilerSELinuxTypeAnnotation overrides the SELinux type applied to the host-profiler container.
	HostProfilerSELinuxTypeAnnotation = "agent.datadoghq.com/host-profiler-selinux-type"
	EnableKSMApiServerCacheAnnotation = "agent.datadoghq.com/ksm-use-apiserver-cache"

	EnableInstrumentationCRDAnnotation = "agent.datadoghq.com/instrumentation-crd-enabled"

	EnableFlightRecorderAnnotation = "agent.datadoghq.com/flightrecorder-enabled"
	EnableNetworkCRDsAnnotation    = "agent.datadoghq.com/network-crds-enabled"

	EnablePrivateActionRunnerAnnotation                     = "agent.datadoghq.com/private-action-runner-enabled"
	PrivateActionRunnerConfigDataAnnotation                 = "agent.datadoghq.com/private-action-runner-configdata"
	EnablePrivateActionRunnerSystemdAnnotation              = "agent.datadoghq.com/private-action-runner-systemd-enabled"
	PrivateActionRunnerSystemdJournalStorageAnnotation      = "agent.datadoghq.com/private-action-runner-systemd-journal-storage"
	EnablePrivateActionRunnerSystemdJournalVacuumAnnotation = "agent.datadoghq.com/private-action-runner-systemd-journal-vacuum-enabled"

	EnableCNMDirectSendAnnotation = "agent.datadoghq.com/cnm-direct-send-enabled"

	// DefaultAgentIpcPort and DefaultAgentIpcConfigRefreshInterval configure config sync, which
	// serves resolved config (notably a secret-backed api_key) from the core agent to sub-processes.
	DefaultAgentIpcPort                  = "5009"
	DefaultAgentIpcConfigRefreshInterval = "60"

	EnableClusterAgentPrivateActionRunnerAnnotation      = "cluster-agent.datadoghq.com/private-action-runner-enabled"
	ClusterAgentPrivateActionRunnerConfigDataAnnotation  = "cluster-agent.datadoghq.com/private-action-runner-configdata"
	ClusterAgentPrivateActionRunnerK8sRemediationEnabled = "cluster-agent.datadoghq.com/private-action-runner-k8s-remediation-enabled"
)

func agentSupportsRunInCoreAgent(ddaSpec *v2alpha1.DatadogAgentSpec) bool {
	// Agent version must >= 7.60.0 to run feature in core agent
	if nodeAgent, ok := ddaSpec.Override[v2alpha1.NodeAgentComponentName]; ok {
		if nodeAgent.Image != nil {
			return utils.IsAboveMinVersion(common.GetAgentVersionFromImage(*nodeAgent.Image), ProcessConfigRunInCoreAgentMinVersion, nil)
		}
	}
	return utils.IsAboveMinVersion(images.AgentLatestVersion, ProcessConfigRunInCoreAgentMinVersion, nil)
}

// ShouldRunProcessChecksInCoreAgent determines whether process checks should run in the core agent
// based on the agent version. Agents >= 7.60.0 support running process checks in the core agent.
// Note: As of Agent 7.78, process checks always run in the core agent on Linux and the
// DD_PROCESS_CONFIG_RUN_IN_CORE_AGENT_ENABLED envvar is no longer recognized.
func ShouldRunProcessChecksInCoreAgent(ddaSpec *v2alpha1.DatadogAgentSpec) bool {
	return agentSupportsRunInCoreAgent(ddaSpec)
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
	if nodeAgent, ok := ddaSpec.Override[v2alpha1.NodeAgentComponentName]; ok {
		if nodeAgent.Image != nil {
			return utils.IsAboveMinVersion(common.GetAgentVersionFromImage(*nodeAgent.Image), ADPDogstatsdDelegationMinVersion, nil)
		}
	}
	return utils.IsAboveMinVersion(images.AgentLatestVersion, ADPDogstatsdDelegationMinVersion, nil)
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

	return defaultEnabled
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

// EnableConfigSyncForDirectSend sets the agent_ipc env vars that config sync requires, on the
// given containers. system-probe wires the no-op secrets component, so with direct send it cannot
// resolve an ENC[...] api_key itself; config sync is what supplies the resolved value from the
// core agent. Both a refresh interval and an IPC transport are needed, since agent_ipc.port
// defaults to 0 and is gated independently of agent_ipc.config_refresh_interval.
//
// Values match the otelcollector and hostprofiler features, which set the same core agent env
// vars for the same reason; a mismatch there would leave the port up to feature ordering.
// User-provided values win.
func EnableConfigSyncForDirectSend(managers feature.PodTemplateManagers, containers []apicommon.AgentContainerName) {
	keepCurrent := func(current, _ *corev1.EnvVar) (*corev1.EnvVar, error) { return current, nil }
	for _, envVar := range []*corev1.EnvVar{
		{Name: common.DDAgentIpcPort, Value: DefaultAgentIpcPort},
		{Name: common.DDAgentIpcConfigRefreshInterval, Value: DefaultAgentIpcConfigRefreshInterval},
	} {
		for _, container := range containers {
			managers.EnvVar().AddEnvVarToContainerWithMergeFunc(container, envVar, keepCurrent)
		}
	}
}
