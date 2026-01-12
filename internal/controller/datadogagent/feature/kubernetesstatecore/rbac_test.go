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
			collectorOpts:      collectorOptions{},
			expectedExtraRules: []rbacv1.PolicyRule{},
		},
		{
			name: "with API services enabled",
			collectorOpts: collectorOptions{
				enableAPIService: true,
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
				enableCRD: true,
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
				enableVPA: true,
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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rules := getRBACPolicyRules(tc.collectorOpts)

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
