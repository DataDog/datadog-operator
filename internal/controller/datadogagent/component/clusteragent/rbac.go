// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package clusteragent

import (
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

// GetDefaultClusterAgentRolePolicyRules returns the default policy rules for the Cluster Agent
// Can be used by the Agent if the Cluster Agent is disabled
func GetDefaultClusterAgentRolePolicyRules(dda metav1.Object) []rbacv1.PolicyRule {
	rules := []rbacv1.PolicyRule{}

	rules = append(rules, getLeaderElectionPolicyRuleDCA(dda)...)
	rules = append(rules, rbacv1.PolicyRule{
		APIGroups: []string{rbac.CoreAPIGroup},
		Resources: []string{rbac.ConfigMapsResource},
		ResourceNames: []string{
			common.DatadogClusterIDResourceName,
		},
		Verbs: []string{rbac.GetVerb, rbac.UpdateVerb, rbac.CreateVerb},
	})
	rules = append(rules, rbacv1.PolicyRule{
		APIGroups: []string{rbac.DatadogAPIGroup},
		Resources: []string{rbac.DatadogAgentsResource},
		ResourceNames: []string{
			dda.GetName(),
		},
		Verbs: []string{rbac.GetVerb},
	})
	return rules
}

// getLeaderElectionPolicyRuleDCA returns the policy rules for leader election
func getLeaderElectionPolicyRuleDCA(dda metav1.Object) []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{rbac.ConfigMapsResource},
			ResourceNames: []string{
				common.DatadogLeaderElectionOldResourceName, // Kept for backward compatibility with agent <7.37.0
				utils.GetDatadogLeaderElectionResourceName(dda),
			},
			Verbs: []string{rbac.GetVerb, rbac.UpdateVerb},
		},
		{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{rbac.ConfigMapsResource},
			Verbs:     []string{rbac.CreateVerb},
		},
		{
			APIGroups: []string{rbac.CoordinationAPIGroup},
			Resources: []string{rbac.LeasesResource},
			Verbs:     []string{rbac.CreateVerb},
		},
		{
			APIGroups: []string{rbac.CoordinationAPIGroup},
			Resources: []string{rbac.LeasesResource},
			ResourceNames: []string{
				utils.GetDatadogLeaderElectionResourceName(dda),
			},
			Verbs: []string{rbac.GetVerb, rbac.UpdateVerb},
		},
	}
}

// GetDefaultClusterAgentClusterRolePolicyRules returns the default policy rules for the Cluster Agent
// Can be used by the Agent if the Cluster Agent is disabled
func GetDefaultClusterAgentClusterRolePolicyRules(dda metav1.Object) []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{
				rbac.ServicesResource,
				rbac.EventsResource,
				rbac.EndpointsResource,
				rbac.PodsResource,
				rbac.NodesResource,
				rbac.ComponentStatusesResource,
				rbac.ConfigMapsResource,
				rbac.NamespaceResource,
			},
			Verbs: []string{
				rbac.GetVerb,
				rbac.ListVerb,
				rbac.WatchVerb,
			},
		},
		{
			APIGroups: []string{rbac.OpenShiftQuotaAPIGroup},
			Resources: []string{rbac.ClusterResourceQuotasResource},
			Verbs:     []string{rbac.GetVerb, rbac.ListVerb},
		},
		{
			NonResourceURLs: []string{rbac.VersionURL, rbac.HealthzURL},
			Verbs:           []string{rbac.GetVerb},
		},
		{
			// Horizontal Pod Autoscaling
			APIGroups: []string{rbac.AutoscalingAPIGroup},
			Resources: []string{rbac.HorizontalPodAutoscalersRecource},
			Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
		},
		{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{rbac.NamespaceResource},
			ResourceNames: []string{
				common.KubeSystemResourceName,
			},
			Verbs: []string{rbac.GetVerb},
		},
	}
}

func GetKubernetesResourceMetadataAsTagsPolicyRules(resourcesLabelsAsTags, resourcesAnnotationsAsTags map[string]map[string]string) []rbacv1.PolicyRule {
	// maps group to resource set
	// using map to avoid duplicates
	groupResourceAccumulator := map[string]map[string]struct{}{}

	for groupResource := range resourcesLabelsAsTags {
		gr := schema.ParseGroupResource(groupResource)
		groupResourceAccumulator = appendGroupResource(groupResourceAccumulator, gr.Group, gr.Resource)
	}

	for groupResource := range resourcesAnnotationsAsTags {
		gr := schema.ParseGroupResource(groupResource)
		groupResourceAccumulator = appendGroupResource(groupResourceAccumulator, gr.Group, gr.Resource)
	}

	policyRules := make([]rbacv1.PolicyRule, 0)

	for group, resources := range groupResourceAccumulator {
		for resource := range resources {
			policyRules = append(policyRules, rbacv1.PolicyRule{
				APIGroups: []string{group},
				Resources: []string{resource},
				Verbs: []string{
					rbac.ListVerb,
					rbac.WatchVerb,
				},
			},
			)
		}
	}

	return policyRules
}

func appendGroupResource(groupResourceAccumulator map[string]map[string]struct{}, group string, resource string) map[string]map[string]struct{} {
	if _, exists := groupResourceAccumulator[group]; !exists {
		groupResourceAccumulator[group] = map[string]struct{}{resource: {}}
	} else {
		groupResourceAccumulator[group][resource] = struct{}{}
	}

	return groupResourceAccumulator
}
