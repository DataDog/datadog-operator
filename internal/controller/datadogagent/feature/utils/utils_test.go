// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	rbacv1 "k8s.io/api/rbac/v1"
)

func TestRBACBuilder(t *testing.T) {
	type resourceWithVerbs struct {
		group    string
		resource string
		verbs    []string
	}

	tests := []struct {
		name          string
		defaultVerbs  []string
		additions     []resourceWithVerbs
		expectedRules []rbacv1.PolicyRule
	}{
		{
			name:          "new builder with no verbs",
			defaultVerbs:  []string{},
			expectedRules: nil,
		},
		{
			name:          "new builder with multiple verbs",
			defaultVerbs:  []string{"list", "watch", "get"},
			additions:     []resourceWithVerbs{},
			expectedRules: nil,
		},
		{
			name:         "single resource with default verbs",
			defaultVerbs: []string{"list", "watch"},
			additions: []resourceWithVerbs{
				{"datadoghq.com", "datadogmetrics", nil},
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
			name:         "single resource with custom verbs",
			defaultVerbs: []string{"list", "watch"},
			additions: []resourceWithVerbs{
				{"datadoghq.com", "datadogmetrics", []string{"get", "create"}},
			},
			expectedRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"datadoghq.com"},
					Resources: []string{"datadogmetrics"},
					Verbs:     []string{"get", "create"},
				},
			},
		},
		{
			name:         "multiple resources same group same verbs",
			defaultVerbs: []string{"list", "watch"},
			additions: []resourceWithVerbs{
				{"datadoghq.com", "datadogmetrics", nil},
				{"datadoghq.com", "watermarkpodautoscalers", nil},
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
			name:         "multiple resources same group different verbs",
			defaultVerbs: []string{"list", "watch"},
			additions: []resourceWithVerbs{
				{"datadoghq.com", "datadogmetrics", nil},
				{"datadoghq.com", "watermarkpodautoscalers", []string{"get", "create"}},
			},
			expectedRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"datadoghq.com"},
					Resources: []string{"watermarkpodautoscalers"},
					Verbs:     []string{"get", "create"},
				},
				{
					APIGroups: []string{"datadoghq.com"},
					Resources: []string{"datadogmetrics"},
					Verbs:     []string{"list", "watch"},
				},
			},
		},
		{
			name:         "multiple groups",
			defaultVerbs: []string{"list", "watch"},
			additions: []resourceWithVerbs{
				{"datadoghq.com", "datadogmetrics", nil},
				{"", "secrets", nil},
			},
			expectedRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"secrets"},
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
			name:         "duplicate resources are deduplicated",
			defaultVerbs: []string{"list", "watch"},
			additions: []resourceWithVerbs{
				{"datadoghq.com", "datadogmetrics", nil},
				{"datadoghq.com", "datadogmetrics", nil}, // duplicate
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
			name:         "duplicate resources with different verbs appends verbs",
			defaultVerbs: []string{"list", "watch"},
			additions: []resourceWithVerbs{
				{"datadoghq.com", "datadogmetrics", nil},
				{"datadoghq.com", "datadogmetrics", []string{"get", "create"}}, // appends
			},
			expectedRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"datadoghq.com"},
					Resources: []string{"datadogmetrics"},
					Verbs:     []string{"list", "watch", "get", "create"},
				},
			},
		},
		{
			name:         "complex scenario with mixed verbs and groups",
			defaultVerbs: []string{"list", "watch"},
			additions: []resourceWithVerbs{
				{"", "pods", []string{"get", "list", "watch"}},
				{"", "services", []string{"get", "list", "watch"}},
				{"", "configmaps", nil},                           // uses default verbs
				{"apps", "deployments", []string{"get", "list"}},  // different verbs
				{"apps", "replicasets", []string{"get", "list"}},  // same verbs as deployments
				{"datadoghq.com", "watermarkpodautoscalers", nil}, // uses default verbs
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
			name:         "builder with no default verbs",
			defaultVerbs: []string{},
			additions: []resourceWithVerbs{
				{"", "pods", []string{"get", "list"}},
				{"", "services", []string{"watch"}},
			},
			expectedRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"pods"},
					Verbs:     []string{"get", "list"},
				},
				{
					APIGroups: []string{""},
					Resources: []string{"services"},
					Verbs:     []string{"watch"},
				},
			},
		},
		{
			name:         "resources with empty verbs get no verbs",
			defaultVerbs: []string{},
			additions: []resourceWithVerbs{
				{"", "pods", nil}, // no default verbs and no custom verbs
			},
			expectedRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"pods"},
					Verbs:     nil,
				},
			},
		},
		{
			name:         "deterministic output - sorted by API groups",
			defaultVerbs: []string{"list", "watch"},
			additions: []resourceWithVerbs{
				{"rbac.authorization.k8s.io", "roles", nil},
				{"apps", "deployments", nil},
				{"datadoghq.com", "datadogmetrics", nil},
			},
			expectedRules: []rbacv1.PolicyRule{
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
				{
					APIGroups: []string{"rbac.authorization.k8s.io"},
					Resources: []string{"roles"},
					Verbs:     []string{"list", "watch"},
				},
			},
		},
		{
			name:         "empty group name",
			defaultVerbs: []string{"list"},
			additions: []resourceWithVerbs{
				{"", "pods", nil},
			},
			expectedRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"pods"},
					Verbs:     []string{"list"},
				},
			},
		},
		{
			name:         "empty resource name",
			defaultVerbs: []string{"list"},
			additions: []resourceWithVerbs{
				{"", "", nil},
			},
			expectedRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{""},
					Verbs:     []string{"list"},
				},
			},
		},
		{
			name:         "empty verbs slice uses default verbs",
			defaultVerbs: []string{"list"},
			additions: []resourceWithVerbs{
				{"", "pods", []string{}},
			},
			expectedRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"pods"},
					Verbs:     []string{"list"}, // should use default verbs
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewRBACBuilder(tt.defaultVerbs...)
			for _, addition := range tt.additions {
				builder = builder.AddGroupKind(addition.group, addition.resource, addition.verbs...)
			}
			rules := builder.Build()
			assert.Equal(t, tt.expectedRules, rules)

		})
	}
}
