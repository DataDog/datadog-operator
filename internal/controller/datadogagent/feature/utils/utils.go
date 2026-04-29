// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/pkg/images"
	"github.com/DataDog/datadog-operator/pkg/utils"
)

const (
	ProcessConfigRunInCoreAgentMinVersion = "7.60.0-0"
	EnableADPAnnotation                   = "agent.datadoghq.com/adp-enabled"
	EnableFineGrainedKubeletAuthz         = "agent.datadoghq.com/fine-grained-kubelet-authorization-enabled"
	EnableHostProfilerAnnotation          = "agent.datadoghq.com/host-profiler-enabled"

	EnableFlightRecorderAnnotation = "agent.datadoghq.com/flightrecorder-enabled"

	EnablePrivateActionRunnerAnnotation     = "agent.datadoghq.com/private-action-runner-enabled"
	PrivateActionRunnerConfigDataAnnotation = "agent.datadoghq.com/private-action-runner-configdata"

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

func GetFeatureConfigAnnotation(dda metav1.Object, annotation string) (string, bool) {
	value, ok := dda.GetAnnotations()[annotation]
	return value, ok
}

// IsDataPlaneEnabled returns true if the Data Plane is enabled.
// CRD configuration takes precedence over the annotation.
// If the annotation is used, a deprecation warning is logged.
func IsDataPlaneEnabled(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec) bool {
	// CRD takes precedence
	if ddaSpec.Features != nil && ddaSpec.Features.DataPlane != nil && ddaSpec.Features.DataPlane.Enabled != nil {
		return *ddaSpec.Features.DataPlane.Enabled
	}

	// Fall back to annotation
	if HasFeatureEnableAnnotation(dda, EnableADPAnnotation) {
		return true
	}

	return false
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
