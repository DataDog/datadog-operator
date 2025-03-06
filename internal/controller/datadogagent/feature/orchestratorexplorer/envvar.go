// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestratorexplorer

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"

	apiutils "github.com/DataDog/datadog-operator/api/utils"
)

const (
	DDOrchestratorExplorerEnabled                   = "DD_ORCHESTRATOR_EXPLORER_ENABLED"
	DDOrchestratorExplorerExtraTags                 = "DD_ORCHESTRATOR_EXPLORER_EXTRA_TAGS"
	DDOrchestratorExplorerDDUrl                     = "DD_ORCHESTRATOR_EXPLORER_ORCHESTRATOR_DD_URL"
	DDOrchestratorExplorerAdditionalEndpoints       = "DD_ORCHESTRATOR_ADDITIONAL_ENDPOINTS"
	DDOrchestratorExplorerContainerScrubbingEnabled = "DD_ORCHESTRATOR_EXPLORER_CONTAINER_SCRUBBING_ENABLED"
)

func (f *orchestratorExplorerFeature) getEnvVars() []*corev1.EnvVar {
	envVarsList := []*corev1.EnvVar{
		{
			Name:  DDOrchestratorExplorerEnabled,
			Value: apiutils.BoolToString(&f.enabled),
		},
	}
	if f.enabled {
		envVarsList = append(envVarsList, &corev1.EnvVar{
			Name:  DDOrchestratorExplorerContainerScrubbingEnabled,
			Value: apiutils.BoolToString(&f.scrubContainers),
		})
		if len(f.extraTags) > 0 {
			tags, _ := json.Marshal(f.extraTags)
			envVarsList = append(envVarsList, &corev1.EnvVar{
				Name:  DDOrchestratorExplorerExtraTags,
				Value: string(tags),
			})
		}

		if f.ddURL != "" {
			envVarsList = append(envVarsList, &corev1.EnvVar{
				Name:  DDOrchestratorExplorerDDUrl,
				Value: f.ddURL,
			})
		}
	}

	return envVarsList
}
