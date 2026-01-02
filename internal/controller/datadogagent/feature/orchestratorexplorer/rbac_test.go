// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestratorexplorer

import (
	"slices"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

func TestGetRBACPolicyRules(t *testing.T) {
	logger := logr.Discard()

	// Base rules that should always be present
	expectedBaseRules := []rbacv1.PolicyRule{
		{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{rbac.PodsResource, rbac.ServicesResource, rbac.NodesResource, rbac.PersistentVolumesResource, rbac.PersistentVolumeClaimsResource, rbac.ServiceAccountResource, rbac.LimitRangesResource},
			Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
		},
		{
			APIGroups: []string{rbac.AppsAPIGroup},
			Resources: []string{rbac.DeploymentsResource, rbac.ReplicasetsResource, rbac.DaemonsetsResource, rbac.StatefulsetsResource},
			Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
		},
		{
			APIGroups: []string{rbac.BatchAPIGroup},
			Resources: []string{rbac.JobsResource, rbac.CronjobsResource},
			Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
		},
		{
			APIGroups: []string{rbac.DatadogAPIGroup},
			Resources: []string{rbac.Wildcard},
			Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
		},
	}

	tests := []struct {
		name                string
		customResources     []string
		expectedCustomRules []rbacv1.PolicyRule
	}{
		{
			name:                "no custom resources",
			customResources:     []string{},
			expectedCustomRules: []rbacv1.PolicyRule{},
		},
		{
			name:            "with single custom resource",
			customResources: []string{"monitoring.coreos.com/v1/servicemonitors"},
			expectedCustomRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"monitoring.coreos.com"},
					Resources: []string{"servicemonitors"},
					Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
				},
			},
		},
		{
			name:            "with multiple custom resources in same group",
			customResources: []string{"datadoghq.com/v1alpha1/datadogmetrics", "datadoghq.com/v1alpha1/watermarkpodautoscalers"},
			expectedCustomRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"datadoghq.com"},
					Resources: []string{"datadogmetrics"},
					Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
				},
				{
					APIGroups: []string{"datadoghq.com"},
					Resources: []string{"watermarkpodautoscalers"},
					Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
				},
			},
		},
		{
			name:            "with multiple custom resources in different groups",
			customResources: []string{"monitoring.coreos.com/v1/servicemonitors", "cilium.io/v2/ciliumnetworkpolicies"},
			expectedCustomRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"monitoring.coreos.com"},
					Resources: []string{"servicemonitors"},
					Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
				},
				{
					APIGroups: []string{"cilium.io"},
					Resources: []string{"ciliumnetworkpolicies"},
					Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
				},
			},
		},
		{
			name:                "with invalid custom resource format",
			customResources:     []string{"invalid-format"},
			expectedCustomRules: []rbacv1.PolicyRule{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules := getRBACPolicyRules(logger, tt.customResources)

			// check base rules
			for _, expectedRule := range expectedBaseRules {
				found := false
				for _, rule := range rules {
					if slices.Equal(rule.APIGroups, expectedRule.APIGroups) &&
						slices.Equal(rule.Resources, expectedRule.Resources) &&
						slices.Equal(rule.Verbs, expectedRule.Verbs) {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected base rule not found: %+v", expectedRule)
			}

			// check custom rules
			for _, expectedRule := range tt.expectedCustomRules {
				found := false
				for _, rule := range rules {
					if slices.Equal(rule.APIGroups, expectedRule.APIGroups) &&
						slices.Equal(rule.Resources, expectedRule.Resources) &&
						slices.Equal(rule.Verbs, expectedRule.Verbs) {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected custom rule not found: %+v", expectedRule)
			}
		})
	}
}
