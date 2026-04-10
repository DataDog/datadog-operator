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

	// Network CRD collection — maps to orchestrator_explorer.custom_resources.ootb.* config keys
	DDOrchestratorExplorerOOTBGatewayAPI          = "DD_ORCHESTRATOR_EXPLORER_CUSTOM_RESOURCES_OOTB_GATEWAY_API"
	DDOrchestratorExplorerOOTBServiceMesh         = "DD_ORCHESTRATOR_EXPLORER_CUSTOM_RESOURCES_OOTB_SERVICE_MESH"
	DDOrchestratorExplorerOOTBIngressControllers  = "DD_ORCHESTRATOR_EXPLORER_CUSTOM_RESOURCES_OOTB_INGRESS_CONTROLLERS"
)

func (f *orchestratorExplorerFeature) getEnabledEnvVar() *corev1.EnvVar {
	return &corev1.EnvVar{
		Name:  DDOrchestratorExplorerEnabled,
		Value: apiutils.BoolToString(&f.enabled),
	}
}

func (f *orchestratorExplorerFeature) getEnvVars() []*corev1.EnvVar {
	envVarsList := []*corev1.EnvVar{
		{
			Name:  DDOrchestratorExplorerContainerScrubbingEnabled,
			Value: apiutils.BoolToString(&f.scrubContainers),
		},
	}

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

	if f.collectKubernetesNetworkResources {
		trueVal := true
		envVarsList = append(envVarsList,
			&corev1.EnvVar{Name: DDOrchestratorExplorerOOTBGatewayAPI, Value: apiutils.BoolToString(&trueVal)},
			&corev1.EnvVar{Name: DDOrchestratorExplorerOOTBServiceMesh, Value: apiutils.BoolToString(&trueVal)},
			&corev1.EnvVar{Name: DDOrchestratorExplorerOOTBIngressControllers, Value: apiutils.BoolToString(&trueVal)},
		)
	}

	return envVarsList
}
