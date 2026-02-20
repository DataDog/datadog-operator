// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package privateactionrunner

import (
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

const (
	defaultIdentitySecretName = "datadog-private-action-runner-identity"
)

// getClusterAgentRBACPolicyRules returns the RBAC policy rules for the Private Action Runner
// This creates a Role (not ClusterRole) with permissions on the identity secret used during self enrollment
func getClusterAgentRBACPolicyRules(config *PrivateActionRunnerConfig) []rbacv1.PolicyRule {
	// Only configure the RBAC when self enrollment is enabled
	if config == nil || !config.Enabled || !config.SelfEnroll {
		return nil
	}

	identitySecretName := defaultIdentitySecretName
	if config.IdentitySecretName != "" {
		identitySecretName = config.IdentitySecretName
	}

	return []rbacv1.PolicyRule{
		{
			APIGroups:     []string{rbac.CoreAPIGroup},
			Resources:     []string{rbac.SecretsResource},
			ResourceNames: []string{identitySecretName},
			Verbs:         []string{rbac.GetVerb, rbac.UpdateVerb, rbac.CreateVerb},
		},
	}
}

func getPrivateActionRunnerRbacResourcesName(owner metav1.Object) string {
	return fmt.Sprintf("%s-%s", owner.GetName(), privateActionRunnerSuffix)
}
