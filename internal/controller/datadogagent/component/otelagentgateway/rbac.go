// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package otelagentgateway

import (
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RBAC for OTel Agent Gateway

// GetDefaultOtelAgentGatewayClusterRolePolicyRules returns the default Cluster Role Policy Rules for the OTel Agent Gateway
// These rules support the k8sattributes processor for enriching telemetry with Kubernetes metadata
func GetDefaultOtelAgentGatewayClusterRolePolicyRules(dda metav1.Object, excludeNonResourceRules bool) []rbacv1.PolicyRule {
	policyRule := []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{
				"pods",
				"namespaces",
			},
			Verbs: []string{
				"get",
				"watch",
				"list",
			},
		},
		{
			APIGroups: []string{"apps"},
			Resources: []string{
				"replicasets",
			},
			Verbs: []string{
				"get",
				"list",
				"watch",
			},
		},
		{
			APIGroups: []string{"extensions"},
			Resources: []string{
				"replicasets",
			},
			Verbs: []string{
				"get",
				"list",
				"watch",
			},
		},
	}

	return policyRule
}
