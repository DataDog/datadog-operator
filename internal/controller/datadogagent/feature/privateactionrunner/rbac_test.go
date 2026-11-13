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
	assertPermission(t, rules, rbac.AppsAPIGroup, rbac.ReplicasetsResource, rbac.ListVerb)
	assertPermission(t, rules, rbac.AppsAPIGroup, rbac.StatefulsetsResource, rbac.ListVerb)
	assertPermission(t, rules, rbac.CoreAPIGroup, rbac.PodsResource, rbac.WatchVerb)

	// writes
	assertPermission(t, rules, rbac.AppsAPIGroup, rbac.DeploymentsResource, rbac.PatchVerb)
	assertPermission(t, rules, rbac.AppsAPIGroup, rbac.DeploymentsResource, rbac.UpdateVerb)
	assertPermission(t, rules, rbac.AppsAPIGroup, rbac.StatefulsetsResource, rbac.PatchVerb)
	assertPermission(t, rules, rbac.BatchAPIGroup, rbac.JobsResource, rbac.DeleteVerb)
	assertPermission(t, rules, rbac.BatchAPIGroup, rbac.CronjobsResource, rbac.PatchVerb)
	assertPermission(t, rules, rbac.AutoscalingAPIGroup, rbac.HorizontalPodAutoscalersRecource, rbac.PatchVerb)
	assertPermission(t, rules, rbac.DatadogAPIGroup, rbac.WpaResource, rbac.GetVerb)
	assertPermission(t, rules, rbac.DatadogAPIGroup, rbac.WpaResource, rbac.PatchVerb)
	assertPermission(t, rules, rbac.CoreAPIGroup, rbac.PodsResource, rbac.DeleteVerb)
	assertPermission(t, rules, rbac.CoreAPIGroup, rbac.ConfigMapsResource, rbac.CreateVerb)
	assertPermission(t, rules, rbac.CoreAPIGroup, rbac.EventsResource, rbac.CreateVerb)

	// not granted
	assertNoPermission(t, rules, rbac.AppsAPIGroup, rbac.DeploymentsResource, rbac.DeleteVerb)
	assertNoPermission(t, rules, rbac.AppsAPIGroup, rbac.StatefulsetsResource, rbac.DeleteVerb)
	assertNoPermission(t, rules, rbac.CoreAPIGroup, rbac.PodsResource, rbac.CreateVerb)
	assertNoPermission(t, rules, rbac.CoreAPIGroup, rbac.ConfigMapsResource, rbac.DeleteVerb)
	assertNoPermission(t, rules, rbac.BatchAPIGroup, rbac.JobsResource, rbac.PatchVerb)
	assertNoPermission(t, rules, rbac.BatchAPIGroup, rbac.CronjobsResource, rbac.DeleteVerb)
	assertNoPermission(t, rules, rbac.AutoscalingAPIGroup, rbac.HorizontalPodAutoscalersRecource, rbac.UpdateVerb)
	assertNoPermission(t, rules, rbac.DatadogAPIGroup, rbac.WpaResource, rbac.UpdateVerb)
	for _, verb := range []string{rbac.CreateVerb, rbac.UpdateVerb, rbac.PatchVerb, rbac.DeleteVerb, "deletecollection"} {
		assertNoPermission(t, rules, rbac.AppsAPIGroup, rbac.DaemonsetsResource, verb)
	}
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
			expectedSecretName: "private-action-runner-identity",
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

			assert.Equal(t, []rbacv1.PolicyRule{
				{
					APIGroups:     []string{rbac.CoreAPIGroup},
					Resources:     []string{rbac.SecretsResource},
					ResourceNames: []string{tt.expectedSecretName},
					Verbs:         []string{rbac.GetVerb, rbac.UpdateVerb},
				},
				{
					APIGroups: []string{rbac.CoreAPIGroup},
					Resources: []string{rbac.SecretsResource},
					Verbs:     []string{rbac.CreateVerb},
				},
			}, rules)
		})
	}
}
