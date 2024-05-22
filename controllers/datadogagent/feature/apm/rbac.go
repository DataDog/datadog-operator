// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apm

import (
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
	rbacv1 "k8s.io/api/rbac/v1"
)

func getLanguageDetectionRBACPolicyRules() []rbacv1.PolicyRule {
	rbacRules := []rbacv1.PolicyRule{
		{
			APIGroups: []string{rbac.AppsAPIGroup},
			Resources: []string{
				// Currently only deployments are supported
				rbac.DeploymentsResource,
			},
			Verbs: []string{rbac.ListVerb, rbac.WatchVerb, rbac.PatchVerb, rbac.GetVerb},
		},
	}

	return rbacRules
}
