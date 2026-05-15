// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autoscaling

import (
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

func getDCAClusterPolicyRules(f *autoscalingFeature) []rbacv1.PolicyRule {
	pr := []rbacv1.PolicyRule{
		{
			// Ability to generate events
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{
				"events",
			},
			Verbs: []string{
				rbac.CreateVerb,
				rbac.PatchVerb,
			},
		},
	}

	if f.workloadEnabled {
		pr = append(pr, []rbacv1.PolicyRule{
			{
				// Access to own CRD
				APIGroups: []string{rbac.DatadogAPIGroup},
				Resources: []string{
					rbac.DatadogPodAutoscalersResource,
					rbac.DatadogPodAutoscalersStatusResource,
					rbac.DatadogPodAutoscalerClusterProfilesResource,
					rbac.DatadogPodAutoscalerClusterProfilesStatusResource,
				},
				Verbs: []string{
					rbac.Wildcard,
				},
			},
			{
				// Scale subresource for all resources
				APIGroups: []string{rbac.Wildcard},
				Resources: []string{
					"*/scale",
				},
				Verbs: []string{
					rbac.GetVerb,
					rbac.UpdateVerb,
				},
			},
			{
				// Patching POD to add annotations. TODO: Remove when we have a better way to generate single event
				APIGroups: []string{rbac.CoreAPIGroup},
				Resources: []string{
					rbac.PodsResource,
				},
				Verbs: []string{
					rbac.PatchVerb,
				},
			},
			{
				// In-place resize: patching pod resources via resize subresource
				APIGroups: []string{rbac.CoreAPIGroup},
				Resources: []string{rbac.PodsResizeResource},
				Verbs:     []string{rbac.PatchVerb},
			},
			{
				APIGroups: []string{rbac.ArgoProjAPIGroup},
				Resources: []string{rbac.Rollout},
				Verbs: []string{
					rbac.GetVerb,
					rbac.ListVerb,
					rbac.WatchVerb,
					rbac.PatchVerb,
				},
			},
			{
				// List/watch for namespaces profiles
				APIGroups: []string{rbac.CoreAPIGroup},
				Resources: []string{rbac.NamespaceResource},
				Verbs: []string{
					rbac.GetVerb,
					rbac.ListVerb,
					rbac.WatchVerb,
				},
			},
		}...,
		)
	}

	if f.workloadEnabled || f.clusterSpotEnabled {
		pr = append(pr, []rbacv1.PolicyRule{
			{
				// Patching workloads to trigger rollout / write spot-disabled-until annotation during on-demand fallback
				APIGroups: []string{rbac.AppsAPIGroup},
				Resources: []string{
					rbac.DeploymentsResource,
					rbac.StatefulsetsResource,
				},
				Verbs: []string{
					rbac.GetVerb,
					rbac.ListVerb,
					rbac.WatchVerb,
					rbac.PatchVerb,
				},
			},
			{
				// Evict pods: in-place resize / pending spot pods during on-demand fallback
				APIGroups: []string{rbac.CoreAPIGroup},
				Resources: []string{rbac.PodsEvictionResource},
				Verbs:     []string{rbac.CreateVerb},
			},
		}...)
	}

	if f.clusterEnabled {
		pr = append(pr, []rbacv1.PolicyRule{
			{
				// Update Karpenter resources
				APIGroups: []string{rbac.KarpenterAPIGroup},
				Resources: []string{rbac.Wildcard},
				Verbs: []string{
					rbac.GetVerb,
					rbac.ListVerb,
					rbac.WatchVerb,
					rbac.CreateVerb,
					rbac.PatchVerb,
					rbac.UpdateVerb,
					rbac.DeleteVerb,
				},
			},
			{
				APIGroups: []string{rbac.KarpenterAWSAPIGroup},
				Resources: []string{rbac.Wildcard},
				Verbs: []string{
					rbac.GetVerb,
					rbac.ListVerb,
				},
			},
			{
				APIGroups: []string{rbac.EKSAPIGroup},
				Resources: []string{rbac.Wildcard},
				Verbs: []string{
					rbac.GetVerb,
					rbac.ListVerb,
				},
			},
		}...,
		)
	}

	return pr
}
