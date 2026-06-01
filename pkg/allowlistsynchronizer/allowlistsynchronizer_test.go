// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package allowlistsynchronizer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func TestResolveWorkloadAllowlistVersion(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "empty falls back to default", input: "", expected: DefaultWorkloadAllowlistVersion},
		{name: "well-formed override is preserved", input: "v2.5.0", expected: "v2.5.0"},
		{name: "malformed falls back to default (no v prefix)", input: "1.0.5", expected: DefaultWorkloadAllowlistVersion},
		{name: "malformed falls back to default (extra suffix)", input: "v1.0.5-alpha", expected: DefaultWorkloadAllowlistVersion},
		{name: "malformed falls back to default (random)", input: "garbage", expected: DefaultWorkloadAllowlistVersion},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, resolveWorkloadAllowlistVersion(tt.input))
		})
	}
}

func TestDefaultWorkloadAllowlistVersion(t *testing.T) {
	// Sanity check — locks the default to a known value so a silent bump is caught.
	assert.Equal(t, "v1.0.5", DefaultWorkloadAllowlistVersion)
}

func TestApplyAllowlistSynchronizerResource_AllowlistPath(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, SchemeBuilder.AddToScheme(scheme))

	tests := []struct {
		name       string
		version    string
		expectPath string
	}{
		{
			name:       "default version",
			version:    DefaultWorkloadAllowlistVersion,
			expectPath: "Datadog/datadog/datadog-datadog-daemonset-exemption-v1.0.5.yaml",
		},
		{
			name:       "user override",
			version:    "v2.5.0",
			expectPath: "Datadog/datadog/datadog-datadog-daemonset-exemption-v2.5.0.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := fake.NewClientBuilder().WithScheme(scheme).Build()
			require.NoError(t, applyAllowlistSynchronizerResource(c, tt.version, "default-foo"))

			got := &AllowlistSynchronizer{}
			require.NoError(t, c.Get(context.TODO(), client.ObjectKey{Name: "datadog-synchronizer"}, got))
			require.Len(t, got.Spec.AllowlistPaths, 1)
			assert.Equal(t, tt.expectPath, got.Spec.AllowlistPaths[0])
			assert.Empty(t, got.Annotations)
			assert.Equal(t, "datadog-operator", got.Labels["app.kubernetes.io/created-by"])
			assert.Equal(t, "datadog-operator", got.Labels[kubernetes.AppKubernetesManageByLabelKey])
			assert.Equal(t, "datadog-allowlist-synchronizer", got.Labels[kubernetes.AppKubernetesNameLabelKey])
			assert.Equal(t, "default-foo", got.Labels[kubernetes.AppKubernetesPartOfLabelKey])
		})
	}
}

func TestApplyAllowlistSynchronizerResource_UpdatesExistingResource(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, SchemeBuilder.AddToScheme(scheme))

	existing := &AllowlistSynchronizer{
		TypeMeta: metav1.TypeMeta{
			APIVersion: SchemeGroupVersion.String(),
			Kind:       "AllowlistSynchronizer",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "datadog-synchronizer",
			Labels: map[string]string{
				kubernetes.AppKubernetesPartOfLabelKey: "old-owner",
			},
		},
		Spec: AllowlistSynchronizerSpec{
			AllowlistPaths: []string{
				"Datadog/datadog/datadog-datadog-daemonset-exemption-v1.0.1.yaml",
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()

	require.NoError(t, applyAllowlistSynchronizerResource(c, "v1.0.5", "default-foo"))

	got := &AllowlistSynchronizer{}
	require.NoError(t, c.Get(context.TODO(), client.ObjectKey{Name: "datadog-synchronizer"}, got))
	require.Len(t, got.Spec.AllowlistPaths, 1)
	assert.Equal(t, "Datadog/datadog/datadog-datadog-daemonset-exemption-v1.0.5.yaml", got.Spec.AllowlistPaths[0])
	assert.Equal(t, "default-foo", got.Labels[kubernetes.AppKubernetesPartOfLabelKey])
	assert.Equal(t, "datadog-operator", got.Labels[kubernetes.AppKubernetesManageByLabelKey])
	assert.Equal(t, "datadog-allowlist-synchronizer", got.Labels[kubernetes.AppKubernetesNameLabelKey])
}

func TestDefaultCSIWorkloadAllowlistVersion(t *testing.T) {
	assert.Equal(t, "v1.1.0", DefaultCSIWorkloadAllowlistVersion)
}

func TestCSIMatchingAllowlistLabel(t *testing.T) {
	assert.Equal(t, "datadog-datadog-csi-driver-daemonset-exemption-v1.1.0", CSIMatchingAllowlistLabel)
}

func TestResolveCSIWorkloadAllowlistVersion(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "empty falls back to default", input: "", expected: DefaultCSIWorkloadAllowlistVersion},
		{name: "well-formed override is preserved", input: "v2.0.0", expected: "v2.0.0"},
		{name: "malformed falls back to default (no v prefix)", input: "1.1.0", expected: DefaultCSIWorkloadAllowlistVersion},
		{name: "malformed falls back to default (extra suffix)", input: "v1.1.0-beta", expected: DefaultCSIWorkloadAllowlistVersion},
		{name: "malformed falls back to default (random)", input: "garbage", expected: DefaultCSIWorkloadAllowlistVersion},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, resolveCSIWorkloadAllowlistVersion(tt.input))
		})
	}
}

func TestApplyCSIAllowlistSynchronizerResource_AllowlistPath(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, SchemeBuilder.AddToScheme(scheme))

	tests := []struct {
		name       string
		version    string
		expectPath string
	}{
		{
			name:       "default version",
			version:    DefaultCSIWorkloadAllowlistVersion,
			expectPath: "Datadog/datadog-csi-driver/datadog-datadog-csi-driver-daemonset-exemption-v1.1.0.yaml",
		},
		{
			name:       "user override",
			version:    "v2.0.0",
			expectPath: "Datadog/datadog-csi-driver/datadog-datadog-csi-driver-daemonset-exemption-v2.0.0.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := fake.NewClientBuilder().WithScheme(scheme).Build()
			require.NoError(t, applyCSIAllowlistSynchronizerResource(c, tt.version, "default-csi"))

			got := &AllowlistSynchronizer{}
			require.NoError(t, c.Get(context.TODO(), client.ObjectKey{Name: "datadog-csi-synchronizer"}, got))
			require.Len(t, got.Spec.AllowlistPaths, 1)
			assert.Equal(t, tt.expectPath, got.Spec.AllowlistPaths[0])
			assert.Empty(t, got.Annotations)
			assert.Equal(t, "datadog-operator", got.Labels["app.kubernetes.io/created-by"])
			assert.Equal(t, "datadog-operator", got.Labels[kubernetes.AppKubernetesManageByLabelKey])
			assert.Equal(t, "datadog-csi-allowlist-synchronizer", got.Labels[kubernetes.AppKubernetesNameLabelKey])
			assert.Equal(t, "default-csi", got.Labels[kubernetes.AppKubernetesPartOfLabelKey])
		})
	}
}

func TestApplyCSIAllowlistSynchronizerResource_UpdatesExistingResource(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, SchemeBuilder.AddToScheme(scheme))

	existing := &AllowlistSynchronizer{
		TypeMeta: metav1.TypeMeta{
			APIVersion: SchemeGroupVersion.String(),
			Kind:       "AllowlistSynchronizer",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "datadog-csi-synchronizer",
			Labels: map[string]string{
				kubernetes.AppKubernetesPartOfLabelKey: "old-owner",
			},
		},
		Spec: AllowlistSynchronizerSpec{
			AllowlistPaths: []string{
				"Datadog/datadog-csi-driver/datadog-datadog-csi-driver-daemonset-exemption-v1.0.1.yaml",
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()

	require.NoError(t, applyCSIAllowlistSynchronizerResource(c, "v1.1.0", "default-csi"))

	got := &AllowlistSynchronizer{}
	require.NoError(t, c.Get(context.TODO(), client.ObjectKey{Name: "datadog-csi-synchronizer"}, got))
	require.Len(t, got.Spec.AllowlistPaths, 1)
	assert.Equal(t, "Datadog/datadog-csi-driver/datadog-datadog-csi-driver-daemonset-exemption-v1.1.0.yaml", got.Spec.AllowlistPaths[0])
	assert.Equal(t, "default-csi", got.Labels[kubernetes.AppKubernetesPartOfLabelKey])
	assert.Equal(t, "datadog-operator", got.Labels[kubernetes.AppKubernetesManageByLabelKey])
	assert.Equal(t, "datadog-csi-allowlist-synchronizer", got.Labels[kubernetes.AppKubernetesNameLabelKey])
}

func TestGetCSIMatchingAllowlistLabelValue(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		expected string
	}{
		{name: "empty uses default", version: "", expected: CSIMatchingAllowlistLabel},
		{name: "default version returns constant", version: "v1.1.0", expected: CSIMatchingAllowlistLabel},
		{name: "custom version computes label", version: "v2.0.0", expected: "datadog-datadog-csi-driver-daemonset-exemption-v2.0.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, GetCSIMatchingAllowlistLabelValue(tt.version))
		})
	}
}
