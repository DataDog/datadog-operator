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

// getClusterAgentRBACPolicyRules returns the RBAC policy rules for the Private Action Runner
// This creates a Role (not ClusterRole) with permissions on the identity secret used during self enrollment
func getClusterAgentRBACPolicyRules(identitySecretName string, enableK8sRemediation bool) []rbacv1.PolicyRule {
	if identitySecretName == "" {
		identitySecretName = defaultIdentitySecretName
	}

	baseRules := []rbacv1.PolicyRule{
		{
			APIGroups:     []string{rbac.CoreAPIGroup},
			Resources:     []string{rbac.SecretsResource},
			ResourceNames: []string{identitySecretName},
			Verbs:         []string{rbac.GetVerb, rbac.UpdateVerb, rbac.CreateVerb},
		},
	}

	if enableK8sRemediation {
		baseRules = append(baseRules, rbacv1.PolicyRule{
			APIGroups: []string{rbac.AppsAPIGroup},
			Resources: []string{rbac.DeploymentsResource},
			Verbs:     []string{rbac.GetVerb, rbac.WatchVerb, rbac.PatchVerb, rbac.CreateVerb},
		})
		baseRules = append(baseRules, rbacv1.PolicyRule{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{rbac.PodsResource, rbac.ConfigMapsResource, rbac.EventsResource},
			Verbs:     []string{rbac.GetVerb, rbac.WatchVerb, rbac.PatchVerb, rbac.CreateVerb},
		})
		baseRules = append(baseRules, rbacv1.PolicyRule{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{rbac.ConfigMapsResource},
			Verbs:     []string{rbac.UpdateVerb},
		})
	}

	return baseRules
}
