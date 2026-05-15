// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package allowlistsynchronizer

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestComputeAllowlistPaths(t *testing.T) {
	tests := []struct {
		name                 string
		otelCollectorEnabled bool
		expected             []string
	}{
		{
			name:                 "OTel collector disabled",
			otelCollectorEnabled: false,
			expected: []string{
				"Datadog/datadog/datadog-datadog-daemonset-exemption-v1.0.1.yaml",
			},
		},
		{
			name:                 "OTel collector enabled adds v1.0.5",
			otelCollectorEnabled: true,
			expected: []string{
				"Datadog/datadog/datadog-datadog-daemonset-exemption-v1.0.1.yaml",
				"Datadog/datadog/datadog-datadog-daemonset-exemption-v1.0.5.yaml",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ComputeAllowlistPaths(tt.otelCollectorEnabled))
		})
	}
}

func TestNewAllowlistSynchronizer(t *testing.T) {
	paths := ComputeAllowlistPaths(true)
	s := newAllowlistSynchronizer(paths)

	assert.Equal(t, "datadog-synchronizer", s.Name)
	assert.Equal(t, "AllowlistSynchronizer", s.Kind)
	assert.Equal(t, "pre-install,pre-upgrade", s.Annotations["helm.sh/hook"])
	assert.Equal(t, paths, s.Spec.AllowlistPaths)

	// DeepCopyObject returns a distinct value with the same spec, satisfying runtime.Object.
	cp, ok := s.DeepCopyObject().(*AllowlistSynchronizer)
	require.True(t, ok)
	assert.Equal(t, s.Spec.AllowlistPaths, cp.Spec.AllowlistPaths)
}

func TestReconcileAllowlistSynchronizer(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, SchemeBuilder.AddToScheme(scheme))

	t.Run("creates resource when absent", func(t *testing.T) {
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		reconcileAllowlistSynchronizer(k8sClient, true)

		got := &AllowlistSynchronizer{}
		require.NoError(t, k8sClient.Get(context.TODO(), client.ObjectKey{Name: "datadog-synchronizer"}, got))
		assert.Equal(t, []string{allowlistPathV101, allowlistPathV105}, got.Spec.AllowlistPaths)
	})

	t.Run("is a no-op when existing synchronizer already matches desired paths", func(t *testing.T) {
		existing := newAllowlistSynchronizer([]string{allowlistPathV101, allowlistPathV105})
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
		originalRV := existing.ResourceVersion

		reconcileAllowlistSynchronizer(k8sClient, true)

		got := &AllowlistSynchronizer{}
		require.NoError(t, k8sClient.Get(context.TODO(), client.ObjectKey{Name: "datadog-synchronizer"}, got))
		assert.Equal(t, []string{allowlistPathV101, allowlistPathV105}, got.Spec.AllowlistPaths)
		assert.Equal(t, originalRV, got.ResourceVersion, "no update should be issued when paths match")
	})

	t.Run("updates existing synchronizer when paths are stale", func(t *testing.T) {
		// Simulates a synchronizer installed by an older operator version that only
		// referenced v1.0.1; OTel is now enabled, so v1.0.5 must be added.
		existing := newAllowlistSynchronizer([]string{allowlistPathV101})
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
		originalRV := existing.ResourceVersion

		reconcileAllowlistSynchronizer(k8sClient, true)

		got := &AllowlistSynchronizer{}
		require.NoError(t, k8sClient.Get(context.TODO(), client.ObjectKey{Name: "datadog-synchronizer"}, got))
		assert.Equal(t, []string{allowlistPathV101, allowlistPathV105}, got.Spec.AllowlistPaths)
		assert.NotEqual(t, originalRV, got.ResourceVersion, "update should have bumped the resourceVersion")
	})

	t.Run("removes v1.0.5 from existing synchronizer when OTel is disabled", func(t *testing.T) {
		// Covers the reverse direction: an existing resource has v1.0.5 but OTel is
		// no longer enabled, so the path should be dropped.
		existing := newAllowlistSynchronizer([]string{allowlistPathV101, allowlistPathV105})
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()

		reconcileAllowlistSynchronizer(k8sClient, false)

		got := &AllowlistSynchronizer{}
		require.NoError(t, k8sClient.Get(context.TODO(), client.ObjectKey{Name: "datadog-synchronizer"}, got))
		assert.Equal(t, []string{allowlistPathV101}, got.Spec.AllowlistPaths)
	})

	t.Run("returns silently when Update fails", func(t *testing.T) {
		existing := newAllowlistSynchronizer([]string{allowlistPathV101})
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).WithInterceptorFuncs(interceptor.Funcs{
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				return errors.New("update failed")
			},
		}).Build()

		reconcileAllowlistSynchronizer(k8sClient, true)
	})

	t.Run("returns silently on Get error other than NotFound", func(t *testing.T) {
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				return errors.New("transient API error")
			},
		}).Build()

		// Should not panic; should not have created anything.
		reconcileAllowlistSynchronizer(k8sClient, true)

		got := &AllowlistSynchronizer{}
		err := k8sClient.Get(context.TODO(), client.ObjectKey{Name: "datadog-synchronizer"}, got)
		// The interceptor is still installed, so Get returns our injected error.
		assert.Error(t, err)
	})

	t.Run("returns silently when Create reports AlreadyExists", func(t *testing.T) {
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				return apierrors.NewAlreadyExists(schema.GroupResource{Group: "auto.gke.io", Resource: "allowlistsynchronizers"}, "datadog-synchronizer")
			},
		}).Build()

		// Should not panic; the AlreadyExists branch is the early return inside the create error handler.
		reconcileAllowlistSynchronizer(k8sClient, true)
	})

	t.Run("returns silently on other Create errors", func(t *testing.T) {
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				return errors.New("unexpected create error")
			},
		}).Build()

		reconcileAllowlistSynchronizer(k8sClient, false)
	})
}

func TestCreateAllowlistSynchronizerResource(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, SchemeBuilder.AddToScheme(scheme))

	tests := []struct {
		name                 string
		otelCollectorEnabled bool
		expectedPaths        []string
	}{
		{
			name:                 "OTel disabled creates synchronizer with v1.0.1 only",
			otelCollectorEnabled: false,
			expectedPaths:        []string{allowlistPathV101},
		},
		{
			name:                 "OTel enabled creates synchronizer with v1.0.1 and v1.0.5",
			otelCollectorEnabled: true,
			expectedPaths:        []string{allowlistPathV101, allowlistPathV105},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

			err := createAllowlistSynchronizerResource(k8sClient, ComputeAllowlistPaths(tt.otelCollectorEnabled))
			require.NoError(t, err)

			got := &AllowlistSynchronizer{}
			require.NoError(t, k8sClient.Get(context.TODO(), client.ObjectKey{Name: "datadog-synchronizer"}, got))
			assert.Equal(t, tt.expectedPaths, got.Spec.AllowlistPaths)
			assert.Equal(t, "pre-install,pre-upgrade", got.Annotations["helm.sh/hook"])
			assert.Equal(t, "-1", got.Annotations["helm.sh/hook-weight"])
		})
	}
}
