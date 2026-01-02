// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rbac

import (
	"testing"

	"github.com/stretchr/testify/assert"
	rbacv1 "k8s.io/api/rbac/v1"
)

func TestNormalizePolicyRules(t *testing.T) {
	tests := []struct {
		name          string
		inputRules    []rbacv1.PolicyRule
		expectedRules []rbacv1.PolicyRule
	}{
		{
			name:          "nil rules",
			inputRules:    nil,
			expectedRules: nil,
		},
		{
			name:          "empty rules",
			inputRules:    []rbacv1.PolicyRule{},
			expectedRules: nil,
		},
		{
			name: "single rule unchanged",
			inputRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"datadoghq.com"},
					Resources: []string{"datadogmetrics"},
					Verbs:     []string{"list", "watch"},
				},
			},
			expectedRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"datadoghq.com"},
					Resources: []string{"datadogmetrics"},
					Verbs:     []string{"list", "watch"},
				},
			},
		},
		{
			name: "two resources same group same verbs get merged",
			inputRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"datadoghq.com"},
					Resources: []string{"datadogmetrics"},
					Verbs:     []string{"list", "watch"},
				},
				{
					APIGroups: []string{"datadoghq.com"},
					Resources: []string{"watermarkpodautoscalers"},
					Verbs:     []string{"list", "watch"},
				},
			},
			expectedRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"datadoghq.com"},
					Resources: []string{"datadogmetrics", "watermarkpodautoscalers"},
					Verbs:     []string{"list", "watch"},
				},
			},
		},
		{
			name: "two resources same group different verbs stay separate",
			inputRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"datadoghq.com"},
					Resources: []string{"datadogmetrics"},
					Verbs:     []string{"list", "watch"},
				},
				{
					APIGroups: []string{"datadoghq.com"},
					Resources: []string{"watermarkpodautoscalers"},
					Verbs:     []string{"get", "create"},
				},
			},
			expectedRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"datadoghq.com"},
					Resources: []string{"watermarkpodautoscalers"},
					Verbs:     []string{"create", "get"},
				},
				{
					APIGroups: []string{"datadoghq.com"},
					Resources: []string{"datadogmetrics"},
					Verbs:     []string{"list", "watch"},
				},
			},
		},
		{
			name: "multiple groups sorted deterministically",
			inputRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"datadoghq.com"},
					Resources: []string{"datadogmetrics"},
					Verbs:     []string{"list", "watch"},
				},
				{
					APIGroups: []string{""},
					Resources: []string{"secrets"},
					Verbs:     []string{"list", "watch"},
				},
				{
					APIGroups: []string{"apps"},
					Resources: []string{"deployments"},
					Verbs:     []string{"list", "watch"},
				},
			},
			expectedRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"secrets"},
					Verbs:     []string{"list", "watch"},
				},
				{
					APIGroups: []string{"apps"},
					Resources: []string{"deployments"},
					Verbs:     []string{"list", "watch"},
				},
				{
					APIGroups: []string{"datadoghq.com"},
					Resources: []string{"datadogmetrics"},
					Verbs:     []string{"list", "watch"},
				},
			},
		},
		{
			name: "duplicate resources are deduplicated and verbs merged",
			inputRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"datadoghq.com"},
					Resources: []string{"datadogmetrics"},
					Verbs:     []string{"list", "watch"},
				},
				{
					APIGroups: []string{"datadoghq.com"},
					Resources: []string{"datadogmetrics"},
					Verbs:     []string{"get", "create"},
				},
			},
			expectedRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"datadoghq.com"},
					Resources: []string{"datadogmetrics"},
					Verbs:     []string{"create", "get", "list", "watch"},
				},
			},
		},
		{
			name: "complex scenario with mixed verbs and groups",
			inputRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"pods"},
					Verbs:     []string{"get", "list", "watch"},
				},
				{
					APIGroups: []string{""},
					Resources: []string{"services"},
					Verbs:     []string{"get", "list", "watch"},
				},
				{
					APIGroups: []string{""},
					Resources: []string{"configmaps"},
					Verbs:     []string{"list", "watch"},
				},
				{
					APIGroups: []string{"apps"},
					Resources: []string{"deployments"},
					Verbs:     []string{"get", "list"},
				},
				{
					APIGroups: []string{"apps"},
					Resources: []string{"replicasets"},
					Verbs:     []string{"get", "list"},
				},
				{
					APIGroups: []string{"datadoghq.com"},
					Resources: []string{"watermarkpodautoscalers"},
					Verbs:     []string{"list", "watch"},
				},
			},
			expectedRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"pods", "services"},
					Verbs:     []string{"get", "list", "watch"},
				},
				{
					APIGroups: []string{""},
					Resources: []string{"configmaps"},
					Verbs:     []string{"list", "watch"},
				},
				{
					APIGroups: []string{"apps"},
					Resources: []string{"deployments", "replicasets"},
					Verbs:     []string{"get", "list"},
				},
				{
					APIGroups: []string{"datadoghq.com"},
					Resources: []string{"watermarkpodautoscalers"},
					Verbs:     []string{"list", "watch"},
				},
			},
		},
		{
			name: "rule with multiple APIGroups gets split",
			inputRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"", "apps"},
					Resources: []string{"pods"},
					Verbs:     []string{"list", "watch"},
				},
			},
			expectedRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"pods"},
					Verbs:     []string{"list", "watch"},
				},
				{
					APIGroups: []string{"apps"},
					Resources: []string{"pods"},
					Verbs:     []string{"list", "watch"},
				},
			},
		},
		{
			name: "rules with resourceNames are kept separate from those without",
			inputRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"configmaps"},
					Verbs:     []string{"create"},
				},
				{
					APIGroups:     []string{""},
					Resources:     []string{"configmaps"},
					Verbs:         []string{"get", "update"},
					ResourceNames: []string{"datadog-agent-leader-election", "datadog-leader-election"},
				},
				{
					APIGroups:     []string{""},
					Resources:     []string{"configmaps"},
					Verbs:         []string{"get", "update"},
					ResourceNames: []string{"datadog-agent-token", "datadogtoken"},
				},
			},
			expectedRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"configmaps"},
					Verbs:     []string{"create"},
				},
				{
					APIGroups:     []string{""},
					Resources:     []string{"configmaps"},
					Verbs:         []string{"get", "update"},
					ResourceNames: []string{"datadog-agent-leader-election", "datadog-leader-election"},
				},
				{
					APIGroups:     []string{""},
					Resources:     []string{"configmaps"},
					Verbs:         []string{"get", "update"},
					ResourceNames: []string{"datadog-agent-token", "datadogtoken"},
				},
			},
		},
		{
			name: "rules with same resourceNames but different verbs are merged",
			inputRules: []rbacv1.PolicyRule{
				{
					APIGroups:     []string{""},
					Resources:     []string{"secrets"},
					Verbs:         []string{"get"},
					ResourceNames: []string{"my-secret"},
				},
				{
					APIGroups:     []string{""},
					Resources:     []string{"secrets"},
					Verbs:         []string{"update"},
					ResourceNames: []string{"my-secret"},
				},
			},
			expectedRules: []rbacv1.PolicyRule{
				{
					APIGroups:     []string{""},
					Resources:     []string{"secrets"},
					Verbs:         []string{"get", "update"},
					ResourceNames: []string{"my-secret"},
				},
			},
		},
		{
			name: "rules with same resourceNames and verbs are merged",
			inputRules: []rbacv1.PolicyRule{
				{
					APIGroups:     []string{""},
					Resources:     []string{"secrets"},
					Verbs:         []string{"get", "update"},
					ResourceNames: []string{"secret-1"},
				},
				{
					APIGroups:     []string{""},
					Resources:     []string{"secrets"},
					Verbs:         []string{"get", "update"},
					ResourceNames: []string{"secret-1"},
				},
			},
			expectedRules: []rbacv1.PolicyRule{
				{
					APIGroups:     []string{""},
					Resources:     []string{"secrets"},
					Verbs:         []string{"get", "update"},
					ResourceNames: []string{"secret-1"},
				},
			},
		},
		{
			name: "different resources with same resourceNames, apiGroup, and verbs are merged",
			inputRules: []rbacv1.PolicyRule{
				{
					APIGroups:     []string{""},
					Resources:     []string{"secrets"},
					Verbs:         []string{"get"},
					ResourceNames: []string{"my-resource"},
				},
				{
					APIGroups:     []string{""},
					Resources:     []string{"configmaps"},
					Verbs:         []string{"get"},
					ResourceNames: []string{"my-resource"},
				},
			},
			expectedRules: []rbacv1.PolicyRule{
				{
					APIGroups:     []string{""},
					Resources:     []string{"configmaps", "secrets"},
					Verbs:         []string{"get"},
					ResourceNames: []string{"my-resource"},
				},
			},
		},
		{
			name: "resourceNames are sorted deterministically",
			inputRules: []rbacv1.PolicyRule{
				{
					APIGroups:     []string{""},
					Resources:     []string{"configmaps"},
					Verbs:         []string{"get"},
					ResourceNames: []string{"zebra", "apple", "banana"},
				},
				{
					APIGroups:     []string{""},
					Resources:     []string{"configmaps"},
					Verbs:         []string{"get"},
					ResourceNames: []string{"banana", "apple", "zebra"},
				},
			},
			expectedRules: []rbacv1.PolicyRule{
				{
					APIGroups:     []string{""},
					Resources:     []string{"configmaps"},
					Verbs:         []string{"get"},
					ResourceNames: []string{"apple", "banana", "zebra"},
				},
			},
		},
		{
			name: "rule with multiple resources and multiple APIGroups gets expanded",
			inputRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"", "apps"},
					Resources: []string{"pods", "services"},
					Verbs:     []string{"get", "list"},
				},
			},
			expectedRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"pods", "services"},
					Verbs:     []string{"get", "list"},
				},
				{
					APIGroups: []string{"apps"},
					Resources: []string{"pods", "services"},
					Verbs:     []string{"get", "list"},
				},
			},
		},
		{
			name: "verbs in different orders get merged correctly",
			inputRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"pods"},
					Verbs:     []string{"watch", "list", "get"},
				},
				{
					APIGroups: []string{""},
					Resources: []string{"services"},
					Verbs:     []string{"get", "list", "watch"},
				},
			},
			expectedRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"pods", "services"},
					Verbs:     []string{"get", "list", "watch"},
				},
			},
		},
		{
			name: "rules with empty resourceNames list treated same as no resourceNames",
			inputRules: []rbacv1.PolicyRule{
				{
					APIGroups:     []string{""},
					Resources:     []string{"configmaps"},
					Verbs:         []string{"get"},
					ResourceNames: []string{},
				},
				{
					APIGroups: []string{""},
					Resources: []string{"configmaps"},
					Verbs:     []string{"get"},
				},
			},
			expectedRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"configmaps"},
					Verbs:     []string{"get"},
				},
			},
		},
		{
			name: "single rule with multiple resources stays as single rule",
			inputRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"pods", "services", "endpoints"},
					Verbs:     []string{"get", "list"},
				},
			},
			expectedRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"endpoints", "pods", "services"},
					Verbs:     []string{"get", "list"},
				},
			},
		},
		{
			name: "rules with resourceNames only differing in order are consolidated",
			inputRules: []rbacv1.PolicyRule{
				{
					APIGroups:     []string{""},
					Resources:     []string{"secrets"},
					Verbs:         []string{"get"},
					ResourceNames: []string{"secret-a", "secret-b"},
				},
				{
					APIGroups:     []string{""},
					Resources:     []string{"configmaps"},
					Verbs:         []string{"get"},
					ResourceNames: []string{"secret-b", "secret-a"},
				},
			},
			expectedRules: []rbacv1.PolicyRule{
				{
					APIGroups:     []string{""},
					Resources:     []string{"configmaps", "secrets"},
					Verbs:         []string{"get"},
					ResourceNames: []string{"secret-a", "secret-b"},
				},
			},
		},
		{
			name: "complex mix of rules with and without resourceNames",
			inputRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"configmaps"},
					Verbs:     []string{"list", "watch"},
				},
				{
					APIGroups:     []string{""},
					Resources:     []string{"configmaps"},
					Verbs:         []string{"get"},
					ResourceNames: []string{"my-config"},
				},
				{
					APIGroups: []string{""},
					Resources: []string{"secrets"},
					Verbs:     []string{"list", "watch"},
				},
				{
					APIGroups:     []string{""},
					Resources:     []string{"secrets"},
					Verbs:         []string{"get", "update"},
					ResourceNames: []string{"my-secret"},
				},
			},
			expectedRules: []rbacv1.PolicyRule{
				{
					APIGroups:     []string{""},
					Resources:     []string{"configmaps"},
					Verbs:         []string{"get"},
					ResourceNames: []string{"my-config"},
				},
				{
					APIGroups:     []string{""},
					Resources:     []string{"secrets"},
					Verbs:         []string{"get", "update"},
					ResourceNames: []string{"my-secret"},
				},
				{
					APIGroups: []string{""},
					Resources: []string{"configmaps", "secrets"},
					Verbs:     []string{"list", "watch"},
				},
			},
		},
		{
			name: "resources sorted alphabetically within consolidated rules",
			inputRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"services"},
					Verbs:     []string{"get"},
				},
				{
					APIGroups: []string{""},
					Resources: []string{"endpoints"},
					Verbs:     []string{"get"},
				},
				{
					APIGroups: []string{""},
					Resources: []string{"pods"},
					Verbs:     []string{"get"},
				},
			},
			expectedRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"endpoints", "pods", "services"},
					Verbs:     []string{"get"},
				},
			},
		},
		{
			name: "all wildcard rule",
			inputRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"*"},
					Resources: []string{"*"},
					Verbs:     []string{"*"},
				},
			},
			expectedRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"*"},
					Resources: []string{"*"},
					Verbs:     []string{"*"},
				},
			},
		},
		{
			name: "wildcard mixed with specific resources",
			inputRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"pods"},
					Verbs:     []string{"get", "list"},
				},
				{
					APIGroups: []string{""},
					Resources: []string{"*"},
					Verbs:     []string{"get", "list"},
				},
			},
			expectedRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"*", "pods"},
					Verbs:     []string{"get", "list"},
				},
			},
		},
		{
			name: "non-resource URL rules",
			inputRules: []rbacv1.PolicyRule{
				{
					NonResourceURLs: []string{"/healthz"},
					Verbs:           []string{"get"},
				},
				{
					NonResourceURLs: []string{"/metrics"},
					Verbs:           []string{"get"},
				},
			},
			expectedRules: []rbacv1.PolicyRule{
				{
					NonResourceURLs: []string{"/healthz", "/metrics"},
					Verbs:           []string{"get"},
				},
			},
		},
		{
			name: "non-resource URL rules with different verbs stay separate",
			inputRules: []rbacv1.PolicyRule{
				{
					NonResourceURLs: []string{"/healthz"},
					Verbs:           []string{"get"},
				},
				{
					NonResourceURLs: []string{"/metrics"},
					Verbs:           []string{"get", "post"},
				},
			},
			expectedRules: []rbacv1.PolicyRule{
				{
					NonResourceURLs: []string{"/healthz"},
					Verbs:           []string{"get"},
				},
				{
					NonResourceURLs: []string{"/metrics"},
					Verbs:           []string{"get", "post"},
				},
			},
		},
		{
			name: "duplicate non-resource URLs with same verbs are merged",
			inputRules: []rbacv1.PolicyRule{
				{
					NonResourceURLs: []string{"/healthz"},
					Verbs:           []string{"get"},
				},
				{
					NonResourceURLs: []string{"/healthz"},
					Verbs:           []string{"get"},
				},
			},
			expectedRules: []rbacv1.PolicyRule{
				{
					NonResourceURLs: []string{"/healthz"},
					Verbs:           []string{"get"},
				},
			},
		},
		{
			name: "non-resource URLs with same URL but different verbs are merged",
			inputRules: []rbacv1.PolicyRule{
				{
					NonResourceURLs: []string{"/healthz"},
					Verbs:           []string{"get"},
				},
				{
					NonResourceURLs: []string{"/healthz"},
					Verbs:           []string{"post"},
				},
			},
			expectedRules: []rbacv1.PolicyRule{
				{
					NonResourceURLs: []string{"/healthz"},
					Verbs:           []string{"get", "post"},
				},
			},
		},
		{
			name: "mixed resource and non-resource URL rules",
			inputRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"pods"},
					Verbs:     []string{"get", "list"},
				},
				{
					NonResourceURLs: []string{"/healthz"},
					Verbs:           []string{"get"},
				},
				{
					APIGroups: []string{""},
					Resources: []string{"services"},
					Verbs:     []string{"get", "list"},
				},
				{
					NonResourceURLs: []string{"/metrics"},
					Verbs:           []string{"get"},
				},
			},
			expectedRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"pods", "services"},
					Verbs:     []string{"get", "list"},
				},
				{
					NonResourceURLs: []string{"/healthz", "/metrics"},
					Verbs:           []string{"get"},
				},
			},
		},
		{
			name: "non-resource URLs are sorted alphabetically",
			inputRules: []rbacv1.PolicyRule{
				{
					NonResourceURLs: []string{"/version"},
					Verbs:           []string{"get"},
				},
				{
					NonResourceURLs: []string{"/healthz"},
					Verbs:           []string{"get"},
				},
				{
					NonResourceURLs: []string{"/metrics"},
					Verbs:           []string{"get"},
				},
			},
			expectedRules: []rbacv1.PolicyRule{
				{
					NonResourceURLs: []string{"/healthz", "/metrics", "/version"},
					Verbs:           []string{"get"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := NormalizePolicyRules(tt.inputRules)
			assert.Equal(t, tt.expectedRules, actual)
		})
	}
}
