// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetesstatecore

import (
	"slices"
	"testing"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
	"github.com/stretchr/testify/assert"
	rbacv1 "k8s.io/api/rbac/v1"
)

func TestGetRBACPolicyRules(t *testing.T) {
	// Base rules that should always be present
	expectedBaseRules := []rbacv1.PolicyRule{
		{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{rbac.ConfigMapsResource, rbac.EndpointsResource, rbac.EventsResource, rbac.LimitRangesResource, rbac.NamespaceResource, rbac.NodesResource, rbac.PersistentVolumeClaimsResource, rbac.PersistentVolumesResource, rbac.PodsResource, rbac.ReplicationControllersResource, rbac.ResourceQuotasResource, rbac.SecretsResource, rbac.ServicesResource},
			Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
		},
		{
			APIGroups: []string{rbac.AppsAPIGroup},
			Resources: []string{rbac.DaemonsetsResource, rbac.DeploymentsResource, rbac.ReplicasetsResource, rbac.StatefulsetsResource, rbac.ControllerRevisionsResource},
			Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
		},
		{
			APIGroups: []string{rbac.BatchAPIGroup},
			Resources: []string{rbac.CronjobsResource, rbac.JobsResource},
			Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
		},
		{
			APIGroups: []string{rbac.AutoscalingK8sIoAPIGroup},
			Resources: []string{rbac.VPAResource},
			Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
		},
	}

	testCases := []struct {
		name               string
		collectorOpts      collectorOptions
		expectedExtraRules []rbacv1.PolicyRule
	}{
		{
			name:               "default options",
			collectorOpts:      collectorOptions{collectSecrets: true, collectConfigMaps: true},
			expectedExtraRules: []rbacv1.PolicyRule{},
		},
		{
			name: "with API services enabled",
			collectorOpts: collectorOptions{
				collectSecrets:    true,
				collectConfigMaps: true,
				enableAPIService:  true,
			},
			expectedExtraRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{rbac.RegistrationAPIGroup},
					Resources: []string{rbac.APIServicesResource},
					Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
				},
			},
		},
		{
			name: "with CRD enabled",
			collectorOpts: collectorOptions{
				collectSecrets:    true,
				collectConfigMaps: true,
				enableCRD:         true,
			},
			expectedExtraRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{rbac.APIExtensionsAPIGroup},
					Resources: []string{rbac.CustomResourceDefinitionsResource},
					Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
				},
			},
		},
		{
			name: "with single custom resource",
			collectorOpts: collectorOptions{
				collectSecrets:    true,
				collectConfigMaps: true,
				customResources: []v2alpha1.Resource{
					{
						GroupVersionKind: v2alpha1.GroupVersionKind{
							Group:   "monitoring.example.com",
							Version: "v1beta1",
							Kind:    "ServiceMonitor",
						},
					},
				},
			},
			expectedExtraRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"monitoring.example.com"},
					Resources: []string{"servicemonitors"},
					Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
				},
			},
		},
		{
			name: "with multiple custom resources in same API group",
			collectorOpts: collectorOptions{
				collectSecrets:    true,
				collectConfigMaps: true,
				customResources: []v2alpha1.Resource{
					{
						GroupVersionKind: v2alpha1.GroupVersionKind{
							Group:   "kafka.strimzi.io",
							Version: "v1beta2",
							Kind:    "Kafka",
						},
					},
					{
						GroupVersionKind: v2alpha1.GroupVersionKind{
							Group:   "kafka.strimzi.io",
							Version: "v1beta2",
							Kind:    "KafkaTopic",
						},
					},
				},
			},
			expectedExtraRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"kafka.strimzi.io"},
					Resources: []string{"kafkas"},
					Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
				},
				{
					APIGroups: []string{"kafka.strimzi.io"},
					Resources: []string{"kafkatopics"},
					Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
				},
			},
		},
		{
			name: "with multiple custom resources in different API groups",
			collectorOpts: collectorOptions{
				collectSecrets:    true,
				collectConfigMaps: true,
				customResources: []v2alpha1.Resource{
					{
						GroupVersionKind: v2alpha1.GroupVersionKind{
							Group:   "tekton.dev",
							Version: "v1beta1",
							Kind:    "Pipeline",
						},
					},
					{
						GroupVersionKind: v2alpha1.GroupVersionKind{
							Group:   "keda.sh",
							Version: "v1alpha1",
							Kind:    "ScaledObject",
						},
					},
				},
			},
			expectedExtraRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"tekton.dev"},
					Resources: []string{"pipelines"},
					Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
				},
				{
					APIGroups: []string{"keda.sh"},
					Resources: []string{"scaledobjects"},
					Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
				},
			},
		},
		{
			name: "with custom resource specifying ResourcePlural",
			collectorOpts: collectorOptions{
				collectSecrets:    true,
				collectConfigMaps: true,
				customResources: []v2alpha1.Resource{
					{
						GroupVersionKind: v2alpha1.GroupVersionKind{
							Group:   "postgresql.cnpg.io",
							Version: "v1",
							Kind:    "Cluster",
						},
						ResourcePlural: "clusters",
					},
				},
			},
			expectedExtraRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"postgresql.cnpg.io"},
					Resources: []string{"clusters"},
					Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
				},
			},
		},
		{
			name: "with duplicate custom resources",
			collectorOpts: collectorOptions{
				collectSecrets:    true,
				collectConfigMaps: true,
				customResources: []v2alpha1.Resource{
					{
						GroupVersionKind: v2alpha1.GroupVersionKind{
							Group:   "cert-manager.io",
							Version: "v1",
							Kind:    "Certificate",
						},
					},
					{
						GroupVersionKind: v2alpha1.GroupVersionKind{
							Group:   "cert-manager.io",
							Version: "v1",
							Kind:    "Certificate",
						},
					},
				},
			},
			expectedExtraRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"cert-manager.io"},
					Resources: []string{"certificates"},
					Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
				},
				{
					APIGroups: []string{"cert-manager.io"},
					Resources: []string{"certificates"},
					Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
				},
			},
		},
		{
			name: "combined options with VPA and custom resources",
			collectorOpts: collectorOptions{
				collectSecrets:    true,
				collectConfigMaps: true,
				enableVPA:         true,
				customResources: []v2alpha1.Resource{
					{
						GroupVersionKind: v2alpha1.GroupVersionKind{
							Group:   "argoproj.io",
							Version: "v1alpha1",
							Kind:    "Application",
						},
					},
				},
			},
			expectedExtraRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"argoproj.io"},
					Resources: []string{"applications"},
					Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
				},
			},
		},
		{
			name: "with wildcard kind",
			collectorOpts: collectorOptions{
				collectSecrets:    true,
				collectConfigMaps: true,
				customResources: []v2alpha1.Resource{
					{
						GroupVersionKind: v2alpha1.GroupVersionKind{
							Group:   "stable.example.com",
							Version: "v1",
							Kind:    "*",
						},
					},
				},
			},
			expectedExtraRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"stable.example.com"},
					Resources: []string{"*"},
					Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
				},
			},
		},
		{
			name:               "collectSecrets=false drops secrets from core rule",
			collectorOpts:      collectorOptions{collectSecrets: false, collectConfigMaps: true},
			expectedExtraRules: []rbacv1.PolicyRule{},
		},
		{
			name:               "collectConfigMaps=false drops configmaps from core rule",
			collectorOpts:      collectorOptions{collectSecrets: true, collectConfigMaps: false},
			expectedExtraRules: []rbacv1.PolicyRule{},
		},
		{
			name:               "both collectSecrets and collectConfigMaps false drops both resources",
			collectorOpts:      collectorOptions{collectSecrets: false, collectConfigMaps: false},
			expectedExtraRules: []rbacv1.PolicyRule{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rules := getRBACPolicyRules(tc.collectorOpts)

			// check base rules — build an adjusted expected core rule based on collector flags
			for _, expectedRule := range expectedBaseRules {
				// For the core API rule, filter out resources that have been disabled
				effectiveRule := expectedRule
				if slices.Contains(expectedRule.APIGroups, rbac.CoreAPIGroup) {
					adjustedResources := slices.Clone(expectedRule.Resources)
					if !tc.collectorOpts.collectSecrets {
						adjustedResources = slices.DeleteFunc(adjustedResources, func(s string) bool { return s == rbac.SecretsResource })
					}
					if !tc.collectorOpts.collectConfigMaps {
						adjustedResources = slices.DeleteFunc(adjustedResources, func(s string) bool { return s == rbac.ConfigMapsResource })
					}
					effectiveRule = rbacv1.PolicyRule{
						APIGroups: expectedRule.APIGroups,
						Resources: adjustedResources,
						Verbs:     expectedRule.Verbs,
					}
				}

				found := false
				for _, rule := range rules {
					if slices.Equal(rule.APIGroups, effectiveRule.APIGroups) &&
						slices.Equal(rule.Resources, effectiveRule.Resources) &&
						slices.Equal(rule.Verbs, effectiveRule.Verbs) {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected base rule not found: %+v", effectiveRule)
			}

			// When collectSecrets=false, verify secrets do not appear in any rule
			if !tc.collectorOpts.collectSecrets {
				for _, rule := range rules {
					for _, r := range rule.Resources {
						assert.NotEqual(t, rbac.SecretsResource, r, "secrets should not appear when collectSecrets=false")
					}
				}
			}

			// When collectConfigMaps=false, verify configmaps do not appear in any rule
			if !tc.collectorOpts.collectConfigMaps {
				for _, rule := range rules {
					for _, r := range rule.Resources {
						assert.NotEqual(t, rbac.ConfigMapsResource, r, "configmaps should not appear when collectConfigMaps=false")
					}
				}
			}

			expectedCount := 13
			if !tc.collectorOpts.collectSecrets {
				expectedCount--
			}
			if !tc.collectorOpts.collectConfigMaps {
				expectedCount--
			}
			for _, rule := range rules {
				if slices.Contains(rule.APIGroups, rbac.CoreAPIGroup) {
					assert.Len(t, rule.Resources, expectedCount, "core API rule should have %d resources", expectedCount)
				}
			}

			// check custom rules
			for _, expectedRule := range tc.expectedExtraRules {
				found := false
				for _, rule := range rules {
					if slices.Equal(rule.APIGroups, expectedRule.APIGroups) &&
						slices.Equal(rule.Resources, expectedRule.Resources) &&
						slices.Equal(rule.Verbs, expectedRule.Verbs) {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected extra rule not found: %+v", expectedRule)
			}
		})
	}
}
