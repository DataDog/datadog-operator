// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"strconv"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/defaulting"
	"github.com/DataDog/datadog-operator/pkg/utils"

	"github.com/DataDog/datadog-operator/controllers/datadogagent/common"
)

// Process Checks utils

const RunInCoreAgentMinVersion = "7.57.0-0"

func agentSupportsRunInCoreAgent(dda *v2alpha1.DatadogAgent) bool {
	// Agent version must >= 7.53.0 to run feature in core agent
	if nodeAgent, ok := dda.Spec.Override[v2alpha1.NodeAgentComponentName]; ok {
		if nodeAgent.Image != nil {
			return utils.IsAboveMinVersion(common.GetAgentVersionFromImage(*nodeAgent.Image), RunInCoreAgentMinVersion)
		}
	}
	return utils.IsAboveMinVersion(defaulting.AgentLatestVersion, RunInCoreAgentMinVersion)
}

// OverrideRunInCoreAgent determines whether to respect the currentVal based on
// environment variables and the agent version.
func OverrideRunInCoreAgent(dda *v2alpha1.DatadogAgent, currentVal bool) bool {
	if nodeAgent, ok := dda.Spec.Override[v2alpha1.NodeAgentComponentName]; ok {
		for _, env := range nodeAgent.Env {
			if env.Name == apicommon.DDProcessConfigRunInCoreAgent {
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
