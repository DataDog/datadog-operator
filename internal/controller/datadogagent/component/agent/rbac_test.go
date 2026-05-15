// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"testing"

	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

func TestGetDefaultAgentClusterRolePolicyRules(t *testing.T) {
	withNonResourceRules := GetDefaultAgentClusterRolePolicyRules(false, false)
	withoutNonResourceRules := GetDefaultAgentClusterRolePolicyRules(true, false)

	require.Len(t, withNonResourceRules, len(withoutNonResourceRules)+1)
	require.Contains(t, withNonResourceRules, getMetricsEndpointPolicyRule())
	require.NotContains(t, withoutNonResourceRules, getMetricsEndpointPolicyRule())
}

func TestGetKubeletPolicyRule(t *testing.T) {
	require.Equal(t, rbacv1.PolicyRule{
		APIGroups: []string{rbac.CoreAPIGroup},
		Resources: []string{
			rbac.NodeMetricsResource,
			rbac.NodeSpecResource,
			rbac.NodeStats,
			rbac.NodePodsResource,
			rbac.NodeHealthzResource,
			rbac.NodeConfigzResource,
			rbac.NodeLogsResource,
		},
		Verbs: []string{rbac.GetVerb},
	}, getKubeletPolicyRule(true))

	require.Equal(t, rbacv1.PolicyRule{
		APIGroups: []string{rbac.CoreAPIGroup},
		Resources: []string{
			rbac.NodeMetricsResource,
			rbac.NodeSpecResource,
			rbac.NodeProxyResource,
			rbac.NodeStats,
		},
		Verbs: []string{rbac.GetVerb},
	}, getKubeletPolicyRule(false))
}
