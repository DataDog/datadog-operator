// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestratorexplorer

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/api/crds/datadoghq/common"
	apiutils "github.com/DataDog/datadog-operator/api/crds/utils"
)

func (f *orchestratorExplorerFeature) getEnvVars() []*corev1.EnvVar {
	envVarsList := []*corev1.EnvVar{
		{
			Name:  apicommon.DDOrchestratorExplorerEnabled,
			Value: "true",
		},
		{
			Name:  apicommon.DDOrchestratorExplorerContainerScrubbingEnabled,
			Value: apiutils.BoolToString(&f.scrubContainers),
		},
	}

	if len(f.extraTags) > 0 {
		tags, _ := json.Marshal(f.extraTags)
		envVarsList = append(envVarsList, &corev1.EnvVar{
			Name:  apicommon.DDOrchestratorExplorerExtraTags,
			Value: string(tags),
		})
	}

	if f.ddURL != "" {
		envVarsList = append(envVarsList, &corev1.EnvVar{
			Name:  apicommon.DDOrchestratorExplorerDDUrl,
			Value: f.ddURL,
		})
	}

	return envVarsList
}
