// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rbac

import (
	"testing"

	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
)

func TestClonePolicyRulesReturnsIndependentRules(t *testing.T) {
	templates := []rbacv1.PolicyRule{
		{
			APIGroups:       []string{CoreAPIGroup},
			Resources:       []string{PodsResource},
			ResourceNames:   []string{"datadog"},
			NonResourceURLs: []string{MetricsURL},
			Verbs:           []string{GetVerb},
		},
	}

	cloned := ClonePolicyRules(templates)
	cloned[0].APIGroups[0] = AppsAPIGroup
	cloned[0].Resources[0] = DeploymentsResource
	cloned[0].ResourceNames[0] = "other"
	cloned[0].NonResourceURLs[0] = HealthzURL
	cloned[0].Verbs[0] = WatchVerb

	require.Equal(t, CoreAPIGroup, templates[0].APIGroups[0])
	require.Equal(t, PodsResource, templates[0].Resources[0])
	require.Equal(t, "datadog", templates[0].ResourceNames[0])
	require.Equal(t, MetricsURL, templates[0].NonResourceURLs[0])
	require.Equal(t, GetVerb, templates[0].Verbs[0])
}
