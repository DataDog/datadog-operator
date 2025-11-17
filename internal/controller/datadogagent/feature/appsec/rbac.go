// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package appsec

import (
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

// getRBACPolicyRules generates the cluster role permissions required for the AppSec proxy injector feature
func getRBACPolicyRules() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		// Envoy Gateway resources
		{
			APIGroups: []string{"gateway.networking.k8s.io"},
			Resources: []string{
				"gateways",
				"gatewayclasses",
			},
			Verbs: []string{
				rbac.GetVerb,
				rbac.ListVerb,
				rbac.WatchVerb,
				rbac.PatchVerb,
			},
		},
		{
			APIGroups: []string{"gateway.networking.k8s.io"},
			Resources: []string{"referencegrants"},
			Verbs: []string{
				rbac.GetVerb,
				rbac.DeleteVerb,
				rbac.CreateVerb,
				rbac.PatchVerb,
			},
		},
		{
			APIGroups: []string{"gateway.envoyproxy.io"},
			Resources: []string{"envoyextensionpolicies"},
			Verbs: []string{
				rbac.GetVerb,
				rbac.DeleteVerb,
				rbac.CreateVerb,
			},
		},
		// Istio resources
		{
			APIGroups: []string{"networking.istio.io"},
			Resources: []string{"envoyfilters"},
			Verbs: []string{
				rbac.GetVerb,
				rbac.CreateVerb,
				rbac.DeleteVerb,
			},
		},
	}
}
