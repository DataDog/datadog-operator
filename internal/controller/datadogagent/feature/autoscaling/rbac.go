// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autoscaling

import (
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

func getDCAClusterPolicyRules() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		{
			// Access to own CRD
			APIGroups: []string{rbac.DatadogAPIGroup},
			Resources: []string{
				rbac.DatadogPodAutoscalersResource,
				rbac.DatadogPodAutoscalersStatusResource,
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
		{
			// Patching POD to add annotations. TODO: Remove when we have a better way to generate single event
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{
				"pods",
			},
			Verbs: []string{
				rbac.PatchVerb,
			},
		},
		{
			// Patching Deployment to trigger rollout.
			APIGroups: []string{rbac.AppsAPIGroup},
			Resources: []string{
				"deployments",
			},
			Verbs: []string{
				rbac.PatchVerb,
			},
		},
	}
}
