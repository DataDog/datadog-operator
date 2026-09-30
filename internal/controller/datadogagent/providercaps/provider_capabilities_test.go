// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package providercaps

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	mergerfake "github.com/DataDog/datadog-operator/internal/controller/datadogagent/merger/fake"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// TestApplyProviderCapabilities_FamilyFallback covers the family-key lookup: a rule
// keyed on "openshift" must apply to every detected openshift-<os_id>, and an exact
// key must still win over it.
func TestApplyProviderCapabilities_FamilyFallback(t *testing.T) {
	const markerEnv = "DD_MARKER"

	envFor := func(value string) ProviderCapabilities {
		return ProviderCapabilities{
			EnvVars: []EnvVarSet{{EnvVar: corev1.EnvVar{Name: markerEnv, Value: value}}},
		}
	}

	tests := []struct {
		name     string
		caps     ProviderCapabilityMap
		provider string
		want     string // "" means the env var must be absent
	}{
		{
			// Detection never yields a bare "openshift", so without the family
			// fallback a platform-wide rule could never fire.
			name:     "family key matches a detected os_id",
			caps:     ProviderCapabilityMap{kubernetes.OpenshiftProvider: envFor("family")},
			provider: "openshift-rhcos",
			want:     "family",
		},
		{
			name:     "family key matches the bare provider too",
			caps:     ProviderCapabilityMap{kubernetes.OpenshiftProvider: envFor("family")},
			provider: kubernetes.OpenshiftProvider,
			want:     "family",
		},
		{
			// A specific os_id can still override the platform-wide rule.
			name: "exact key wins over the family key",
			caps: ProviderCapabilityMap{
				kubernetes.OpenshiftProvider: envFor("family"),
				"openshift-rhcos":            envFor("exact"),
			},
			provider: "openshift-rhcos",
			want:     "exact",
		},
		{
			// The fallback must not leak across providers.
			name:     "non-openshift providers do not collapse",
			caps:     ProviderCapabilityMap{kubernetes.GKECloudProvider: envFor("gke")},
			provider: kubernetes.GKECosProvider,
			want:     "",
		},
		{
			name:     "no matching key leaves the template alone",
			caps:     ProviderCapabilityMap{kubernetes.OpenshiftProvider: envFor("family")},
			provider: kubernetes.EKSCloudProvider,
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})

			ApplyProviderCapabilities(manager, tt.provider, tt.caps)

			var got string
			for _, env := range manager.EnvVarMgr.EnvVarsByC[mergerfake.AllContainers] {
				if env.Name == markerEnv {
					got = env.Value
				}
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestApplyProviderCapabilities_PodLevel covers the pod-scoped fields, which the
// volume and env managers cannot reach.
func TestApplyProviderCapabilities_PodLevel(t *testing.T) {
	spcT := &corev1.SELinuxOptions{User: "system_u", Role: "system_r", Type: "spc_t", Level: "s0"}
	masterToleration := corev1.Toleration{
		Key: "node-role.kubernetes.io/master", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule,
	}

	t.Run("SELinux options are set on an empty security context", func(t *testing.T) {
		manager := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})

		ApplyProviderCapabilities(manager, kubernetes.OpenshiftProvider,
			ProviderCapabilityMap{kubernetes.OpenshiftProvider: {SELinuxOptions: spcT}})

		assert.Equal(t, spcT, manager.PodTemplateSpec().Spec.SecurityContext.SELinuxOptions)
	})

	t.Run("RunAsUser set by the default builder survives", func(t *testing.T) {
		// The regression this field's narrow type exists to prevent: the node agent
		// pod spec carries RunAsUser: 0, and assigning a whole PodSecurityContext
		// would drop it, leaving the Agent unable to read host paths.
		manager := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{SecurityContext: &corev1.PodSecurityContext{RunAsUser: ptr.To(int64(0))}},
		})

		ApplyProviderCapabilities(manager, kubernetes.OpenshiftProvider,
			ProviderCapabilityMap{kubernetes.OpenshiftProvider: {SELinuxOptions: spcT}})

		sc := manager.PodTemplateSpec().Spec.SecurityContext
		assert.Equal(t, spcT, sc.SELinuxOptions)
		require.NotNil(t, sc.RunAsUser, "RunAsUser must not be dropped")
		assert.Equal(t, int64(0), *sc.RunAsUser)
	})

	t.Run("existing SELinux options are not overwritten", func(t *testing.T) {
		existing := &corev1.SELinuxOptions{Type: "container_t"}
		manager := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{SecurityContext: &corev1.PodSecurityContext{SELinuxOptions: existing}},
		})

		ApplyProviderCapabilities(manager, kubernetes.OpenshiftProvider,
			ProviderCapabilityMap{kubernetes.OpenshiftProvider: {SELinuxOptions: spcT}})

		assert.Equal(t, existing, manager.PodTemplateSpec().Spec.SecurityContext.SELinuxOptions)
	})

	t.Run("tolerations are appended alongside existing ones", func(t *testing.T) {
		userToleration := corev1.Toleration{Key: "user-taint", Operator: corev1.TolerationOpExists}
		manager := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{Tolerations: []corev1.Toleration{userToleration}},
		})

		ApplyProviderCapabilities(manager, kubernetes.OpenshiftProvider,
			ProviderCapabilityMap{kubernetes.OpenshiftProvider: {Tolerations: []corev1.Toleration{masterToleration}}})

		assert.Equal(t, []corev1.Toleration{userToleration, masterToleration},
			manager.PodTemplateSpec().Spec.Tolerations)
	})

	t.Run("an already-present toleration is not duplicated", func(t *testing.T) {
		manager := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{Tolerations: []corev1.Toleration{masterToleration}},
		})

		ApplyProviderCapabilities(manager, kubernetes.OpenshiftProvider,
			ProviderCapabilityMap{kubernetes.OpenshiftProvider: {Tolerations: []corev1.Toleration{masterToleration}}})

		assert.Len(t, manager.PodTemplateSpec().Spec.Tolerations, 1)
	})

	t.Run("pod-level fields are untouched for a non-matching provider", func(t *testing.T) {
		manager := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})

		ApplyProviderCapabilities(manager, kubernetes.EKSCloudProvider,
			ProviderCapabilityMap{kubernetes.OpenshiftProvider: {SELinuxOptions: spcT, Tolerations: []corev1.Toleration{masterToleration}}})

		assert.Nil(t, manager.PodTemplateSpec().Spec.SecurityContext)
		assert.Empty(t, manager.PodTemplateSpec().Spec.Tolerations)
	})
}
