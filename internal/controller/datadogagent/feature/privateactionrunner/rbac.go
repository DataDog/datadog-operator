// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package privateactionrunner

import (
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

const (
	defaultIdentitySecretName = "datadog-private-action-runner-identity"
)

// GetRBACPolicyRules returns the RBAC policy rules for the Private Action Runner
// This creates a Role (not ClusterRole) with permissions on the identity secret used during self enrollment
func GetRBACPolicyRules(configData string) []rbacv1.PolicyRule {
	identitySecretName := getIdentitySecretName(configData)

	return []rbacv1.PolicyRule{
		{
			APIGroups:     []string{rbac.CoreAPIGroup},
			Resources:     []string{rbac.SecretsResource},
			ResourceNames: []string{identitySecretName},
			Verbs: []string{
				rbac.GetVerb,
				rbac.UpdateVerb,
				rbac.CreateVerb,
			},
		},
	}
}

// getIdentitySecretName returns the name of the identity secret from the config data
func getIdentitySecretName(configData string) string {
	config, err := parsePrivateActionRunnerConfig(configData)
	if err != nil || config.IdentitySecretName == "" {
		return defaultIdentitySecretName
	}
	return config.IdentitySecretName
}
