// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package component

import (
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

// GetAgentName returns the default Agent name based on the DatadogAgent name
func GetAgentName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), constants.DefaultAgentResourceSuffix)
}

// GetDaemonSetNameFromDatadogAgent returns the expected node Agent DS/EDS name based on
// the DDA name and nodeAgent name override
func GetDaemonSetNameFromDatadogAgent(ddaObject metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec) string {
	dsName := GetAgentName(ddaObject)
	if componentOverride, ok := ddaSpec.Override[v2alpha1.NodeAgentComponentName]; ok {
		if componentOverride.Name != nil && *componentOverride.Name != "" {
			dsName = *componentOverride.Name
		}
	}
	return dsName
}

// GetClusterAgentName returns the default Cluster Agent name based on the DatadogAgent name
func GetClusterAgentName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), constants.DefaultClusterAgentResourceSuffix)
}

// GetDeploymentNameFromDatadogAgent returns the expected Cluster Agent Deployment name based on
// the DDA name and clusterAgent name override
func GetDeploymentNameFromDatadogAgent(ddaObject metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec) string {
	deployName := GetClusterAgentName(ddaObject)
	if componentOverride, ok := ddaSpec.Override[v2alpha1.ClusterAgentComponentName]; ok {
		if componentOverride.Name != nil && *componentOverride.Name != "" {
			deployName = *componentOverride.Name
		}
	}
	return deployName
}

func GetEKSControlPlaneMetricsPolicyRule() rbacv1.PolicyRule {
	return rbacv1.PolicyRule{
		APIGroups: []string{rbac.EKSMetricsAPIGroup},
		Resources: []string{
			rbac.EKSKubeControllerManagerMetrics,
			rbac.EKSKubeSchedulerMetrics,
		},
		Verbs: []string{
			rbac.GetVerb,
		},
	}
}
