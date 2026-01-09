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
func GetDefaultOtelAgentGatewayClusterRolePolicyRules(dda metav1.Object, excludeNonResourceRules bool) []rbacv1.PolicyRule {
	// TODO(OTAGENT-512): add RBAC for OTel load balancing exporter & k8s attributes processor
	return []rbacv1.PolicyRule{}
}
