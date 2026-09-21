// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package providercaps

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

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
