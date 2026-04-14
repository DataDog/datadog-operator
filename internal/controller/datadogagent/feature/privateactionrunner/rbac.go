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
func getClusterAgentRBACPolicyRules(identitySecretName string) []rbacv1.PolicyRule {
	if identitySecretName == "" {
		identitySecretName = defaultIdentitySecretName
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

// getK8sRemediationPolicyRules returns the ClusterRole policy rules required for k8s remediation actions.
// The policy rules included are constrained within the maximum set the DCA could have if all features were enabled
func getK8sRemediationPolicyRules() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		// Read to some workload types
		{
			APIGroups: []string{rbac.AppsAPIGroup},
			Resources: []string{rbac.DeploymentsResource, rbac.DaemonsetsResource, rbac.StatefulsetsResource, rbac.ReplicasetsResource},
			Verbs:     []string{rbac.GetVerb, rbac.ListVerb, rbac.WatchVerb},
		},
		{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{rbac.PodsResource, rbac.EventsResource, rbac.ConfigMapsResource},
			Verbs:     []string{rbac.GetVerb, rbac.ListVerb, rbac.WatchVerb},
		},
		// Write deployments (patch/restart)
		{
			APIGroups: []string{rbac.AppsAPIGroup},
			Resources: []string{rbac.DeploymentsResource},
			Verbs:     []string{rbac.PatchVerb},
		},
		// Patch pods
		{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{rbac.PodsResource},
			Verbs:     []string{rbac.PatchVerb},
		},
		// Full write access to configmaps
		{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{rbac.ConfigMapsResource},
			Verbs:     []string{rbac.CreateVerb, rbac.UpdateVerb, rbac.PatchVerb},
		},
		// Write events
		{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{rbac.EventsResource},
			Verbs:     []string{rbac.CreateVerb, rbac.PatchVerb},
		},
	}
}
