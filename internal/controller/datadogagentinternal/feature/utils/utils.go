// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"strconv"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/common"
	"github.com/DataDog/datadog-operator/pkg/defaulting"
	"github.com/DataDog/datadog-operator/pkg/utils"
)

const ProcessConfigRunInCoreAgentMinVersion = "7.60.0-0"
const EnableADPAnnotation = "agent.datadoghq.com/adp-enabled"

func agentSupportsRunInCoreAgent(dda *v2alpha1.DatadogAgent) bool {
	// Agent version must >= 7.60.0 to run feature in core agent
	if nodeAgent, ok := dda.Spec.Override[v2alpha1.NodeAgentComponentName]; ok {
		if nodeAgent.Image != nil {
			return utils.IsAboveMinVersion(common.GetAgentVersionFromImage(*nodeAgent.Image), ProcessConfigRunInCoreAgentMinVersion)
		}
	}
	return utils.IsAboveMinVersion(defaulting.AgentLatestVersion, ProcessConfigRunInCoreAgentMinVersion)
}

// OverrideProcessConfigRunInCoreAgent determines whether to respect the currentVal based on
// environment variables and the agent version.
func OverrideProcessConfigRunInCoreAgent(dda *v2alpha1.DatadogAgent, currentVal bool) bool {
	if nodeAgent, ok := dda.Spec.Override[v2alpha1.NodeAgentComponentName]; ok {
		for _, env := range nodeAgent.Env {
			if env.Name == common.DDProcessConfigRunInCoreAgent {
				val, err := strconv.ParseBool(env.Value)
				if err == nil {
					return val
				}
			}
		}
	}

	if !agentSupportsRunInCoreAgent(dda) {
		return false
	}

	return currentVal
}

func hasFeatureEnableAnnotation(dda *v2alpha1.DatadogAgent, annotation string) bool {
	if value, ok := dda.ObjectMeta.Annotations[annotation]; ok {
		return value == "true"
	}
	return false
}

// HasAgentDataPlaneAnnotation returns true if the Agent Data Plane is enabled via the dedicated `agent.datadoghq.com/adp-enabled` annotation
func HasAgentDataPlaneAnnotation(dda *v2alpha1.DatadogAgent) bool {
	return hasFeatureEnableAnnotation(dda, EnableADPAnnotation)
}
