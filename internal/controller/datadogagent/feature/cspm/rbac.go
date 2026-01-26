// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cspm

import (
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

// getRBACRules generates the cluster role required for CSPM
func getRBACPolicyRules() []rbacv1.PolicyRule {
	rbacRules := []rbacv1.PolicyRule{
		{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{
				rbac.NamespaceResource,
				rbac.ServiceAccountResource,
			},
			Verbs: []string{rbac.ListVerb},
		},
		{
			APIGroups: []string{rbac.RbacAPIGroup},
			Resources: []string{
				rbac.ClusterRoleBindingResource,
				rbac.RoleBindingResource,
			},
			Verbs: []string{rbac.ListVerb, rbac.WatchVerb},
		},
		{
			APIGroups: []string{rbac.NetworkingAPIGroup},
			Resources: []string{
				rbac.NetworkPolicyResource,
			},
			Verbs: []string{rbac.ListVerb},
		},
	}

	return rbacRules
}
