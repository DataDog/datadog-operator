// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cilium

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCiliumGroupVersionKinds(t *testing.T) {
	t.Run("returns the CiliumNetworkPolicy kind", func(t *testing.T) {
		gvk := GroupVersionCiliumNetworkPolicyKind()

		require.Equal(t, "cilium.io", gvk.Group)
		require.Equal(t, "v2", gvk.Version)
		require.Equal(t, "CiliumNetworkPolicy", gvk.Kind)
	})

	t.Run("returns the CiliumNetworkPolicyList kind", func(t *testing.T) {
		gvk := GroupVersionCiliumNetworkPolicyListKind()

		require.Equal(t, "cilium.io", gvk.Group)
		require.Equal(t, "v2", gvk.Version)
		require.Equal(t, "CiliumNetworkPolicyList", gvk.Kind)
	})
}

func TestEmptyCiliumUnstructuredPolicy(t *testing.T) {
	policy := EmptyCiliumUnstructuredPolicy()

	require.Empty(t, policy.GetName())
	require.Empty(t, policy.GetNamespace())
	require.Equal(t, GroupVersionCiliumNetworkPolicyKind(), policy.GroupVersionKind())
}

func TestEmptyCiliumUnstructuredListPolicy(t *testing.T) {
	policyList := EmptyCiliumUnstructuredListPolicy()

	require.Empty(t, policyList.Items)
	require.Equal(t, GroupVersionCiliumNetworkPolicyListKind(), policyList.GroupVersionKind())
}
