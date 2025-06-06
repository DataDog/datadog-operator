// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusteragent

import (
	"testing"

	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
	assert "github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
)

func TestGetKubernetesResourceMetadataAsTagsPolicyRules(t *testing.T) {
	labelsAsTags := map[string]map[string]string{
		"pods": {
			"foo": "bar",
			"bar": "bar",
		},
		"deployments.apps": {
			"foo": "baz",
			"bar": "bar",
		},
		"metrics.custom.metrics.group": {
			"foo": "baz",
			"bar": "bar",
		},
	}

	annotationsAsTags := map[string]map[string]string{
		"pods": {
			"foo": "bar",
			"bar": "bar",
		},
		"deployments.apps": {
			"foo": "baz",
			"bar": "bar",
		},
	}

	expectedRules := []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"pods"},
			Verbs: []string{
				rbac.ListVerb,
				rbac.WatchVerb,
			},
		},
		{
			APIGroups: []string{"apps"},
			Resources: []string{"deployments"},
			Verbs: []string{
				rbac.ListVerb,
				rbac.WatchVerb,
			},
		},
		{
			APIGroups: []string{"custom.metrics.group"},
			Resources: []string{"metrics"},
			Verbs: []string{
				rbac.ListVerb,
				rbac.WatchVerb,
			},
		},
	}

	rules := GetKubernetesResourceMetadataAsTagsPolicyRules(labelsAsTags, annotationsAsTags)

	assert.ElementsMatch(t, expectedRules, rules)
}
