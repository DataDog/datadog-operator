// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helmcheck

import (
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

// getRBACRules generates the cluster role policy rules required for the Helm Check
func getRBACPolicyRules() []rbacv1.PolicyRule {
	rbacRules := []rbacv1.PolicyRule{
		{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{
				rbac.SecretsResource,
				rbac.ConfigMapsResource,
			},
			Verbs: []string{
				rbac.GetVerb,
				rbac.ListVerb,
				rbac.WatchVerb,
			},
		},
	}
	return rbacRules
}
