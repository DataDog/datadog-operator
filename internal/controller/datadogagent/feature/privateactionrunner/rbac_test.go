// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package privateactionrunner

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

func hasPermission(rules []rbacv1.PolicyRule, apiGroup, resource, verb string) bool {
	for _, rule := range rules {
		if slices.Contains(rule.APIGroups, apiGroup) &&
			slices.Contains(rule.Resources, resource) &&
			slices.Contains(rule.Verbs, verb) {
			return true
		}
	}
	return false
}

func assertPermission(t *testing.T, rules []rbacv1.PolicyRule, apiGroup, resource, verb string) {
	t.Helper()
	if !hasPermission(rules, apiGroup, resource, verb) {
		t.Errorf("expected %s %s/%s to be granted but it was not", verb, apiGroup, resource)
	}
}

func assertNoPermission(t *testing.T, rules []rbacv1.PolicyRule, apiGroup, resource, verb string) {
	t.Helper()
	if hasPermission(rules, apiGroup, resource, verb) {
		t.Errorf("expected %s %s/%s NOT to be granted but it was", verb, apiGroup, resource)
	}
}

func TestGetK8sRemediationPolicyRules(t *testing.T) {
	rules := getK8sRemediationPolicyRules()

	// reads
	assertPermission(t, rules, rbac.AppsAPIGroup, rbac.DeploymentsResource, rbac.GetVerb)
	assertPermission(t, rules, rbac.AppsAPIGroup, rbac.StatefulsetsResource, rbac.ListVerb)
	assertPermission(t, rules, rbac.CoreAPIGroup, rbac.PodsResource, rbac.WatchVerb)

	// writes
	assertPermission(t, rules, rbac.AppsAPIGroup, rbac.DeploymentsResource, rbac.PatchVerb)
	assertPermission(t, rules, rbac.AppsAPIGroup, rbac.StatefulsetsResource, rbac.PatchVerb)
	assertPermission(t, rules, rbac.CoreAPIGroup, rbac.PodsResource, rbac.DeleteVerb)
	assertPermission(t, rules, rbac.CoreAPIGroup, rbac.ConfigMapsResource, rbac.CreateVerb)
	assertPermission(t, rules, rbac.CoreAPIGroup, rbac.EventsResource, rbac.CreateVerb)

	// not granted
	assertNoPermission(t, rules, rbac.AppsAPIGroup, rbac.DeploymentsResource, rbac.DeleteVerb)
	assertNoPermission(t, rules, rbac.AppsAPIGroup, rbac.StatefulsetsResource, rbac.DeleteVerb)
	assertNoPermission(t, rules, rbac.CoreAPIGroup, rbac.PodsResource, rbac.CreateVerb)
	assertNoPermission(t, rules, rbac.CoreAPIGroup, rbac.ConfigMapsResource, rbac.DeleteVerb)
}

func TestGetClusterAgentRBACPolicyRules(t *testing.T) {
	tests := []struct {
		name               string
		identitySecretName string
		expectedSecretName string
	}{
		{
			name:               "default identity secret name when empty string provided",
			identitySecretName: "",
			expectedSecretName: "datadog-private-action-runner-identity",
		},
		{
			name:               "custom identity secret name",
			identitySecretName: "custom-par-secret",
			expectedSecretName: "custom-par-secret",
		},
		{
			name:               "another custom secret name",
			identitySecretName: "my-identity-secret",
			expectedSecretName: "my-identity-secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules := getClusterAgentRBACPolicyRules(tt.identitySecretName)

			assert.Len(t, rules, 1, "Should have exactly one policy rule")

			rule := rules[0]
			assert.Equal(t, []string{rbac.CoreAPIGroup}, rule.APIGroups)
			assert.Equal(t, []string{rbac.SecretsResource}, rule.Resources)
			assert.Equal(t, []string{tt.expectedSecretName}, rule.ResourceNames)
			assert.ElementsMatch(t, []string{rbac.GetVerb, rbac.UpdateVerb, rbac.CreateVerb}, rule.Verbs)
		})
	}
}
