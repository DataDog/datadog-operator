// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package otelcollectorgateway

import (
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RBAC for OTel Collector Gateway

// GetDefaultOtelCollectorGatewayClusterRolePolicyRules returns the default Cluster Role Policy Rules for the OTel Collector Gateway
func GetDefaultOtelCollectorGatewayClusterRolePolicyRules(dda metav1.Object, excludeNonResourceRules bool) []rbacv1.PolicyRule {
	policyRule := []rbacv1.PolicyRule{}
	// 	{
	// 		APIGroups: []string{rbac.CoreAPIGroup},
	// 		Resources: []string{
	// 			rbac.PodsResource,
	// 			rbac.NodesResource,
	// 		},
	// 		Verbs: []string{
	// 			rbac.GetVerb,
	// 			rbac.ListVerb,
	// 			rbac.WatchVerb,
	// 		},
	// 	},
	// }

	// if !excludeNonResourceRules {
	// 	policyRule = append(policyRule, rbacv1.PolicyRule{
	// 		NonResourceURLs: []string{
	// 			rbac.MetricsURL,
	// 		},
	// 		Verbs: []string{rbac.GetVerb},
	// 	})
	// }

	return policyRule
}
