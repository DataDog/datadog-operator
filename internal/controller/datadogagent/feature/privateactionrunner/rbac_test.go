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
			rules := getClusterAgentRBACPolicyRules(tt.identitySecretName)

			assert.Len(t, rules, 1, "Should have exactly one policy rule")

			rule := rules[0]
			assert.Equal(t, []string{rbac.CoreAPIGroup}, rule.APIGroups, "APIGroups should be core")
			assert.Equal(t, []string{rbac.SecretsResource}, rule.Resources, "Resources should be secrets")
			assert.Equal(t, []string{tt.expectedSecretName}, rule.ResourceNames, "ResourceNames should match expected secret name")
			assert.ElementsMatch(t, []string{rbac.GetVerb, rbac.UpdateVerb, rbac.CreateVerb}, rule.Verbs, "Verbs should include get, update, and create")
		})
	}
}
