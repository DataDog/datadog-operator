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
	testCases := []struct {
		name          string
		collectorOpts collectorOptions
		validateFunc  func(t *testing.T, rules []rbacv1.PolicyRule)
	}{
		{
			name:          "default options",
			collectorOpts: collectorOptions{},
			validateFunc: func(t *testing.T, rules []rbacv1.PolicyRule) {
				// Check that basic rules are present
				assert.NotEmpty(t, rules)

				// All rules should have list and watch verbs
				for _, rule := range rules {
					assert.Equal(t, []string{rbac.ListVerb, rbac.WatchVerb}, rule.Verbs)
				}

				// Check for core resources
				hasCore := false
				for _, rule := range rules {
					if slices.Contains(rule.APIGroups, rbac.CoreAPIGroup) {
						hasCore = true
						assert.Contains(t, rule.Resources, rbac.PodsResource)
						assert.Contains(t, rule.Resources, rbac.NodesResource)
						break
					}
				}
				assert.True(t, hasCore, "Should have core API group")
			},
		},
		{
			name: "with API services enabled",
			collectorOpts: collectorOptions{
				enableAPIService: true,
			},
			validateFunc: func(t *testing.T, rules []rbacv1.PolicyRule) {
				// Check for API services rule
				hasAPIServices := false
				for _, rule := range rules {
					if slices.Contains(rule.APIGroups, rbac.RegistrationAPIGroup) {
						hasAPIServices = true
						assert.Contains(t, rule.Resources, rbac.APIServicesResource)
						break
					}
				}
				assert.True(t, hasAPIServices, "Should have API services when enabled")
			},
		},
		{
			name: "with CRD enabled",
			collectorOpts: collectorOptions{
				enableCRD: true,
			},
			validateFunc: func(t *testing.T, rules []rbacv1.PolicyRule) {
				// Check for CRD rule
				hasCRD := false
				for _, rule := range rules {
					if slices.Contains(rule.APIGroups, rbac.APIExtensionsAPIGroup) {
						hasCRD = true
						assert.Contains(t, rule.Resources, rbac.CustomResourceDefinitionsResource)
						break
					}
				}
				assert.True(t, hasCRD, "Should have CRD when enabled")
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
			validateFunc: func(t *testing.T, rules []rbacv1.PolicyRule) {
				// Check for custom resource rule
				hasCustomResource := false
				for _, rule := range rules {
					if slices.Contains(rule.APIGroups, "monitoring.example.com") {
						hasCustomResource = true
						assert.Equal(t, []string{rbac.ListVerb, rbac.WatchVerb}, rule.Verbs)
						assert.Contains(t, rule.Resources, "servicemonitors")
						break
					}
				}
				assert.True(t, hasCustomResource, "Should have custom resource permissions")
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
			validateFunc: func(t *testing.T, rules []rbacv1.PolicyRule) {
				// Should have a single rule for the API group with both resources
				kafkaRuleCount := 0
				for _, rule := range rules {
					if slices.Contains(rule.APIGroups, "kafka.strimzi.io") {
						kafkaRuleCount++
						assert.Equal(t, []string{rbac.ListVerb, rbac.WatchVerb}, rule.Verbs)
						assert.Contains(t, rule.Resources, "kafkas")
						assert.Contains(t, rule.Resources, "kafkatopics")
					}
				}
				assert.Equal(t, 1, kafkaRuleCount, "Should have exactly one rule for kafka.strimzi.io")
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
			validateFunc: func(t *testing.T, rules []rbacv1.PolicyRule) {
				// Should have separate rules for each API group
				hasTekton := false
				hasKeda := false
				for _, rule := range rules {
					if slices.Contains(rule.APIGroups, "tekton.dev") {
						hasTekton = true
						assert.Contains(t, rule.Resources, "pipelines")
					}
					if slices.Contains(rule.APIGroups, "keda.sh") {
						hasKeda = true
						assert.Contains(t, rule.Resources, "scaledobjects")
					}
				}
				assert.True(t, hasTekton, "Should have tekton.dev permissions")
				assert.True(t, hasKeda, "Should have keda.sh permissions")
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
			validateFunc: func(t *testing.T, rules []rbacv1.PolicyRule) {
				// Should use the specified ResourcePlural
				hasPostgres := false
				for _, rule := range rules {
					if slices.Contains(rule.APIGroups, "postgresql.cnpg.io") {
						hasPostgres = true
						assert.Contains(t, rule.Resources, "clusters")
						break
					}
				}
				assert.True(t, hasPostgres, "Should have PostgreSQL cluster permissions")
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
			validateFunc: func(t *testing.T, rules []rbacv1.PolicyRule) {
				// Should not have duplicate resources in the rule
				certManagerRuleCount := 0
				for _, rule := range rules {
					if slices.Contains(rule.APIGroups, "cert-manager.io") {
						certManagerRuleCount++
						// Should only have one "certificates" entry
						certificateCount := 0
						for _, res := range rule.Resources {
							if res == "certificates" {
								certificateCount++
							}
						}
						assert.Equal(t, 1, certificateCount, "Should not have duplicate certificates resource")
					}
				}
				assert.Equal(t, 1, certManagerRuleCount, "Should have exactly one rule for cert-manager.io")
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
			validateFunc: func(t *testing.T, rules []rbacv1.PolicyRule) {
				// Should have both VPA and custom resource permissions
				hasVPA := false
				hasArgo := false
				for _, rule := range rules {
					if slices.Contains(rule.APIGroups, rbac.AutoscalingK8sIoAPIGroup) {
						hasVPA = true
						assert.Contains(t, rule.Resources, rbac.VPAResource)
					}
					if slices.Contains(rule.APIGroups, "argoproj.io") {
						hasArgo = true
						assert.Contains(t, rule.Resources, "applications")
					}
				}
				assert.True(t, hasVPA, "Should have VPA permissions")
				assert.True(t, hasArgo, "Should have Argo Application permissions")
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
			validateFunc: func(t *testing.T, rules []rbacv1.PolicyRule) {
				hasStable := false
				for _, rule := range rules {
					if slices.Contains(rule.APIGroups, "stable.example.com") {
						hasStable = true
						assert.Contains(t, rule.Resources, "*")
						break
					}
				}
				assert.True(t, hasStable, "Should have stable.example.com permissions")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rules := getRBACPolicyRules(tc.collectorOpts)
			tc.validateFunc(t, rules)
		})
	}
}
