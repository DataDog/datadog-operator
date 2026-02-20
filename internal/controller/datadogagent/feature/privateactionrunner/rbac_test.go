// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package privateactionrunner

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

func TestGetRBACPolicyRules(t *testing.T) {
	tests := []struct {
		name                   string
		config                 *PrivateActionRunnerConfig
		expectedSecretName     string
		expectedResourcesCount int
	}{
		{
			name: "default identity secret name",
			config: &PrivateActionRunnerConfig{
				Enabled:    true,
				SelfEnroll: true,
			},
			expectedSecretName:     "datadog-private-action-runner-identity",
			expectedResourcesCount: 1,
		},
		{
			name: "custom identity secret name",
			config: &PrivateActionRunnerConfig{
				Enabled:            true,
				SelfEnroll:         true,
				IdentitySecretName: "custom-par-secret",
			},
			expectedSecretName:     "custom-par-secret",
			expectedResourcesCount: 1,
		},
		{
			name: "with URN and private key but self_enroll disabled",
			config: &PrivateActionRunnerConfig{
				Enabled:            true,
				SelfEnroll:         false,
				URN:                "urn:dd:apps:on-prem-runner:us1:1:runner-abc",
				PrivateKey:         "secret-key",
				IdentitySecretName: "my-identity-secret",
			},
			expectedSecretName:     "",
			expectedResourcesCount: 0,
		},
		{
			name: "self_enroll enabled with custom secret name",
			config: &PrivateActionRunnerConfig{
				Enabled:            true,
				SelfEnroll:         true,
				IdentitySecretName: "my-identity-secret",
			},
			expectedSecretName:     "my-identity-secret",
			expectedResourcesCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules := getClusterAgentRBACPolicyRules(tt.config)

			assert.Len(t, rules, tt.expectedResourcesCount, "Should have expected number of policy rules")

			if tt.expectedResourcesCount > 0 {
				rule := rules[0]
				assert.Equal(t, []string{rbac.CoreAPIGroup}, rule.APIGroups, "APIGroups should be core")
				assert.Equal(t, []string{rbac.SecretsResource}, rule.Resources, "Resources should be secrets")
				assert.Equal(t, []string{tt.expectedSecretName}, rule.ResourceNames, "ResourceNames should match expected secret name")
				assert.ElementsMatch(t, []string{rbac.GetVerb, rbac.UpdateVerb, rbac.CreateVerb}, rule.Verbs, "Verbs should include get, update, and create")
			}
		})
	}
}

func TestGetPrivateActionRunnerRbacResourcesName(t *testing.T) {
	tests := []struct {
		name         string
		ownerName    string
		expectedName string
	}{
		{
			name:         "standard owner name",
			ownerName:    "test-dda",
			expectedName: "test-dda-private-action-runner",
		},
		{
			name:         "owner with dashes",
			ownerName:    "my-datadog-agent",
			expectedName: "my-datadog-agent-private-action-runner",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner := &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{
					Name: tt.ownerName,
				},
			}
			name := getPrivateActionRunnerRbacResourcesName(owner)
			assert.Equal(t, tt.expectedName, name)
		})
	}
}
