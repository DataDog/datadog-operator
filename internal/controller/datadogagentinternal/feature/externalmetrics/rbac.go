// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package externalmetrics

import (
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/common"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

func getDCAClusterPolicyRules(useDDM, useWPA bool) []rbacv1.PolicyRule {
	rbacRules := []rbacv1.PolicyRule{
		{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{rbac.ConfigMapsResource},
			ResourceNames: []string{
				common.DatadogCustomMetricsResourceName,
			},
			Verbs: []string{
				rbac.GetVerb,
				rbac.UpdateVerb,
			},
		},
		{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{rbac.ConfigMapsResource},
			ResourceNames: []string{
				common.ExtensionAPIServerAuthResourceName,
			},
			Verbs: []string{
				rbac.GetVerb,
				rbac.ListVerb,
				rbac.WatchVerb,
			},
		},
		{
			APIGroups: []string{rbac.AuthorizationAPIGroup},
			Resources: []string{rbac.SubjectAccessReviewResource},
			Verbs: []string{
				rbac.CreateVerb,
				rbac.GetVerb,
			},
		},
		{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{rbac.EventsResource},
			Verbs:     []string{rbac.CreateVerb},
		},
	}

	if useDDM {
		rbacRules = append(rbacRules, rbacv1.PolicyRule{
			APIGroups: []string{rbac.DatadogAPIGroup},
			Resources: []string{rbac.DatadogMetricsResource},
			Verbs: []string{
				rbac.ListVerb,
				rbac.WatchVerb,
				rbac.CreateVerb,
				rbac.DeleteVerb,
			},
		})

		// Specific update rule for status subresource
		rbacRules = append(rbacRules, rbacv1.PolicyRule{
			APIGroups: []string{rbac.DatadogAPIGroup},
			Resources: []string{rbac.DatadogMetricsStatusResource},
			Verbs:     []string{rbac.UpdateVerb},
		})
	}

	if useWPA {
		rbacRules = append(rbacRules, rbacv1.PolicyRule{
			APIGroups: []string{rbac.DatadogAPIGroup},
			Resources: []string{rbac.WpaResource},
			Verbs: []string{
				rbac.ListVerb,
				rbac.WatchVerb,
				rbac.GetVerb,
			},
		})
	}

	return rbacRules
}

func getAuthDelegatorRoleRef() rbacv1.RoleRef {
	return rbacv1.RoleRef{
		APIGroup: rbac.RbacAPIGroup,
		Kind:     rbac.ClusterRoleKind,
		Name:     "system:auth-delegator",
	}
}

func getExternalMetricsReaderPolicyRules() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		{
			APIGroups: []string{rbac.ExternalMetricsAPIGroup},
			Resources: []string{"*"},
			Verbs: []string{
				rbac.GetVerb,
				rbac.ListVerb,
				rbac.WatchVerb,
			},
		},
	}
}
