// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secretsbackend

import (
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
	rbacv1 "k8s.io/api/rbac/v1"
)

var secretsBackendGlobalRBACPolicyRules = []rbacv1.PolicyRule{
	{
		APIGroups: []string{rbac.CoreAPIGroup},
		Resources: []string{rbac.SecretsResource},
		Verbs:     []string{rbac.GetVerb},
	},
}

// to do : func to return policy rules based on dda
func getGlobalSecretsPermissions() []rbacv1.PolicyRule {
	rbacRules := []rbacv1.PolicyRule{
		{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{rbac.SecretsResource},
			Verbs:     []string{rbac.GetVerb},
		},
	}

	return rbacRules
}