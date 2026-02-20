// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/pkg/images"
	"github.com/DataDog/datadog-operator/pkg/utils"
)

const ProcessConfigRunInCoreAgentMinVersion = "7.60.0-0"
const EnableADPAnnotation = "agent.datadoghq.com/adp-enabled"
const EnableFineGrainedKubeletAuthz = "agent.datadoghq.com/fine-grained-kubelet-authorization-enabled"
const EnableHostProfilerAnnotion = "agent.datadoghq.com/host-profiler-enabled"
const HostProfilerConfigDataAnnotion = "agent.datadoghq.com/host-profiler-configdata"

// Config map item must be `host-profiler-config.yaml`
const HostProfilerConfigMapNameAnnotion = "agent.datadoghq.com/host-profiler-configmap-name"

const EnablePrivateActionRunnerAnnotation = "agent.datadoghq.com/private-action-runner-enabled"
const PrivateActionRunnerConfigDataAnnotation = "agent.datadoghq.com/private-action-runner-configdata"

func agentSupportsRunInCoreAgent(ddaSpec *v2alpha1.DatadogAgentSpec) bool {
	// Agent version must >= 7.60.0 to run feature in core agent
	if nodeAgent, ok := ddaSpec.Override[v2alpha1.NodeAgentComponentName]; ok {
		if nodeAgent.Image != nil {
			return utils.IsAboveMinVersion(common.GetAgentVersionFromImage(*nodeAgent.Image), ProcessConfigRunInCoreAgentMinVersion, nil)
		}
	}
	return utils.IsAboveMinVersion(images.AgentLatestVersion, ProcessConfigRunInCoreAgentMinVersion, nil)
}

// ShouldRunProcessChecksInCoreAgent determines whether allow process checks to run in core agent based on
// environment variables and the agent version.
func ShouldRunProcessChecksInCoreAgent(ddaSpec *v2alpha1.DatadogAgentSpec) bool {

	// Prioritize env var override
	if nodeAgent, ok := ddaSpec.Override[v2alpha1.NodeAgentComponentName]; ok {
		for _, env := range nodeAgent.Env {
			if env.Name == common.DDProcessConfigRunInCoreAgent {
				val, err := strconv.ParseBool(env.Value)
				if err == nil {
					return val
				}
			}
		}
	}

	// Check if agent version supports process checks running in core agent
	if !agentSupportsRunInCoreAgent(ddaSpec) {
		return false
	}

	return true
}

func hasFeatureEnableAnnotation(dda metav1.Object, annotation string) bool {
	if value, ok := dda.GetAnnotations()[annotation]; ok {
		return value == "true"
	}
	return false
}

// HasAgentDataPlaneAnnotation returns true if the Agent Data Plane is enabled via the dedicated `agent.datadoghq.com/adp-enabled` annotation
func HasAgentDataPlaneAnnotation(dda metav1.Object) bool {
	return hasFeatureEnableAnnotation(dda, EnableADPAnnotation)
}

// HasHostProfilerAnnotation returns true if the Host Profiler is enabled via the dedicated `agent.datadoghq.com/host-profiler` annotation
func HasHostProfilerAnnotation(dda metav1.Object) bool {
	return hasFeatureEnableAnnotation(dda, EnableHostProfilerAnnotion)
}

// HasHostProfilerConfigAnnotion returns true if the Host Profiler has a config annotation, and returns the config.
func HasHostProfilerConfigAnnotion(dda metav1.Object, annotationName string) (string, bool) {
	value, ok := dda.GetAnnotations()[annotationName]
	return value, ok
}

// HasFineGrainedKubeletAuthz returns true if the feature is enabled via the dedicated `agent.datadoghq.com/fine-grained-kubelet-authorization-enabled` annotation
func HasFineGrainedKubeletAuthz(dda metav1.Object) bool {
	return hasFeatureEnableAnnotation(dda, EnableFineGrainedKubeletAuthz)
}

// HasPrivateActionRunnerAnnotation returns true if the Private Action Runner is enabled via the dedicated annotation
func HasPrivateActionRunnerAnnotation(dda metav1.Object) bool {
	return hasFeatureEnableAnnotation(dda, EnablePrivateActionRunnerAnnotation)
}

// HasPrivateActionRunnerConfigAnnotation returns the value and presence of a Private Action Runner config annotation
func HasPrivateActionRunnerConfigAnnotation(dda metav1.Object, annotationName string) (string, bool) {
	value, ok := dda.GetAnnotations()[annotationName]
	return value, ok
}
