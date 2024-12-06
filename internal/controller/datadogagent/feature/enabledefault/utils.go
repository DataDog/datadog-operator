// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package enabledefault

import (
	"github.com/DataDog/datadog-operator/api/crds/datadoghq/v2alpha1"
	componentagent "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	componentdca "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/clusteragent"
)

// getDaemonSetNameFromDatadogAgent returns the expected node Agent DS/EDS name based on
// the DDA name and nodeAgent name override
func getDaemonSetNameFromDatadogAgent(dda *v2alpha1.DatadogAgent) string {
	dsName := componentagent.GetAgentName(dda)
	if componentOverride, ok := dda.Spec.Override[v2alpha1.NodeAgentComponentName]; ok {
		if componentOverride.Name != nil && *componentOverride.Name != "" {
			dsName = *componentOverride.Name
		}
	}
	return dsName
}

// getDeploymentNameFromDatadogAgent returns the expected Cluster Agent Deployment name based on
// the DDA name and clusterAgent name override
func getDeploymentNameFromDatadogAgent(dda *v2alpha1.DatadogAgent) string {
	deployName := componentdca.GetClusterAgentName(dda)
	if componentOverride, ok := dda.Spec.Override[v2alpha1.ClusterAgentComponentName]; ok {
		if componentOverride.Name != nil && *componentOverride.Name != "" {
			deployName = *componentOverride.Name
		}
	}
	return deployName
}
