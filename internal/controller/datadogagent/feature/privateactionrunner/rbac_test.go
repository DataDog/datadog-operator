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
		name                   string
		configData             string
		expectedSecretName     string
		expectedResourcesCount int
	}{
		{
			name: "default identity secret name",
			configData: `private_action_runner:
  enabled: true
  self_enroll: true`,
			expectedSecretName:     "datadog-private-action-runner-identity",
			expectedResourcesCount: 1,
		},
		{
			name: "custom identity secret name",
			configData: `private_action_runner:
  enabled: true
  self_enroll: true
  identity_secret_name: custom-par-secret`,
			expectedSecretName:     "custom-par-secret",
			expectedResourcesCount: 1,
		},
		{
			name: "with URN and private key",
			configData: `private_action_runner:
  enabled: true
  self_enroll: false
  urn: "urn:dd:apps:on-prem-runner:us1:1:runner-abc"
  private_key: "secret-key"
  identity_secret_name: my-identity-secret`,
			expectedSecretName:     "my-identity-secret",
			expectedResourcesCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules := GetRBACPolicyRules(tt.configData)

			assert.Len(t, rules, tt.expectedResourcesCount, "Should have exactly one policy rule")

			rule := rules[0]
			assert.Equal(t, []string{rbac.CoreAPIGroup}, rule.APIGroups, "APIGroups should be core")
			assert.Equal(t, []string{rbac.SecretsResource}, rule.Resources, "Resources should be secrets")
			assert.Equal(t, []string{tt.expectedSecretName}, rule.ResourceNames, "ResourceNames should match expected secret name")
			assert.ElementsMatch(t, []string{rbac.GetVerb, rbac.UpdateVerb, rbac.CreateVerb}, rule.Verbs, "Verbs should include get, update, and create")
		})
	}
}

func TestGetIdentitySecretName(t *testing.T) {
	tests := []struct {
		name         string
		configData   string
		expectedName string
	}{
		{
			name:         "empty config",
			configData:   ``,
			expectedName: "datadog-private-action-runner-identity",
		},
		{
			name: "config without identity_secret_name",
			configData: `private_action_runner:
  enabled: true
  self_enroll: true`,
			expectedName: "datadog-private-action-runner-identity",
		},
		{
			name: "config with custom identity_secret_name",
			configData: `private_action_runner:
  enabled: true
  self_enroll: true
  identity_secret_name: my-custom-secret`,
			expectedName: "my-custom-secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := getIdentitySecretName(tt.configData)
			assert.Equal(t, tt.expectedName, name)
		})
	}
}
