// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package privateactionrunner

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

func TestGetRBACPolicyRules(t *testing.T) {
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
			rules := getClusterAgentRBACPolicyRules(tt.identitySecretName, false)

			assert.Len(t, rules, 1, "Should have exactly one policy rule")

			rule := rules[0]
			assert.Equal(t, []string{rbac.CoreAPIGroup}, rule.APIGroups, "APIGroups should be core")
			assert.Equal(t, []string{rbac.SecretsResource}, rule.Resources, "Resources should be secrets")
			assert.Equal(t, []string{tt.expectedSecretName}, rule.ResourceNames, "ResourceNames should match expected secret name")
			assert.ElementsMatch(t, []string{rbac.GetVerb, rbac.UpdateVerb, rbac.CreateVerb}, rule.Verbs, "Verbs should include get, update, and create")
		})
	}

	t.Run("k8s remediation enabled adds extra rules", func(t *testing.T) {
		rules := getClusterAgentRBACPolicyRules("my-secret", true)
		assert.Len(t, rules, 4, "Should have exactly four policy rules")

		// Second rule: get/watch/patch/create on deployments (apps group)
		deploymentsRule := rules[1]
		assert.Equal(t, []string{rbac.AppsAPIGroup}, deploymentsRule.APIGroups)
		assert.Equal(t, []string{rbac.DeploymentsResource}, deploymentsRule.Resources)
		assert.ElementsMatch(t, []string{rbac.GetVerb, rbac.WatchVerb, rbac.PatchVerb, rbac.CreateVerb}, deploymentsRule.Verbs)

		// Third rule: get/watch/patch/create on pods, configmaps, events (core group)
		coreRule := rules[2]
		assert.Equal(t, []string{rbac.CoreAPIGroup}, coreRule.APIGroups)
		assert.ElementsMatch(t, []string{rbac.PodsResource, rbac.ConfigMapsResource, rbac.EventsResource}, coreRule.Resources)
		assert.ElementsMatch(t, []string{rbac.GetVerb, rbac.WatchVerb, rbac.PatchVerb, rbac.CreateVerb}, coreRule.Verbs)

		// Fourth rule: update on configmaps
		configMapUpdateRule := rules[3]
		assert.Equal(t, []string{rbac.CoreAPIGroup}, configMapUpdateRule.APIGroups)
		assert.Equal(t, []string{rbac.ConfigMapsResource}, configMapUpdateRule.Resources)
		assert.Equal(t, []string{rbac.UpdateVerb}, configMapUpdateRule.Verbs)
	})

	t.Run("k8s remediation enabled with default secret name", func(t *testing.T) {
		rules := getClusterAgentRBACPolicyRules("", true)
		assert.Len(t, rules, 4, "Should have exactly four policy rules")
		assert.Equal(t, []string{"datadog-private-action-runner-identity"}, rules[0].ResourceNames)
	})
}
