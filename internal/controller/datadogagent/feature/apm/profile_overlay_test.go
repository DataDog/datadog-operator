// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

func TestAPMProfileSharedConfigOverlay(t *testing.T) {
	tests := []struct {
		name        string
		dst         *v2alpha1.DatadogAgentSpec
		profile     *v2alpha1.DatadogAgentSpec
		want        *v2alpha1.SingleStepInstrumentation
		wantErr     string
		wantNoApply bool
	}{
		{
			name: "profile SSI overlays disabled base SSI and ignores synthetic base defaults",
			dst:  testProfileOverlayBaseSpec(false),
			profile: testProfileOverlayProfileSpec(&v2alpha1.SingleStepInstrumentation{
				Enabled:           ptr.To(true),
				EnabledNamespaces: []string{"ml", "training", "ml"},
				LibVersions: map[string]string{
					"java":   "1.43.0",
					"python": "2.14.0",
				},
				LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(false)},
				Injector:          &v2alpha1.InjectorConfig{ImageTag: "7.66.0"},
				InjectionMode:     v2alpha1.InjectionModeInitContainer,
			}),
			want: &v2alpha1.SingleStepInstrumentation{
				Enabled:           ptr.To(true),
				EnabledNamespaces: []string{"ml", "training"},
				LibVersions: map[string]string{
					"java":   "1.43.0",
					"python": "2.14.0",
				},
				LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(false)},
				Injector:          &v2alpha1.InjectorConfig{ImageTag: "7.66.0"},
				InjectionMode:     v2alpha1.InjectionModeInitContainer,
			},
		},
		{
			name: "map conflict rejects profile",
			dst: func() *v2alpha1.DatadogAgentSpec {
				spec := testProfileOverlayBaseSpec(true)
				spec.Features.APM.SingleStepInstrumentation.LibVersions = map[string]string{"java": "1.43.0"}
				return spec
			}(),
			profile: testProfileOverlayProfileSpec(&v2alpha1.SingleStepInstrumentation{
				Enabled:     ptr.To(true),
				LibVersions: map[string]string{"java": "1.44.0"},
			}),
			wantErr: `features.apm.instrumentation.libVersions["java"] has conflicting values "1.43.0" and "1.44.0"`,
		},
		{
			name: "enabled and disabled namespaces reject final overlay",
			dst: func() *v2alpha1.DatadogAgentSpec {
				spec := testProfileOverlayBaseSpec(true)
				spec.Features.APM.SingleStepInstrumentation.EnabledNamespaces = []string{"default"}
				return spec
			}(),
			profile: testProfileOverlayProfileSpec(&v2alpha1.SingleStepInstrumentation{
				Enabled:            ptr.To(true),
				DisabledNamespaces: []string{"kube-system"},
			}),
			wantErr: "features.apm.instrumentation.enabledNamespaces and features.apm.instrumentation.disabledNamespaces cannot both be set",
		},
		{
			name: "targets merge by name",
			dst: func() *v2alpha1.DatadogAgentSpec {
				spec := testProfileOverlayBaseSpec(true)
				spec.Features.APM.SingleStepInstrumentation.Targets = []v2alpha1.SSITarget{
					{
						Name: "api",
						PodSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "api"},
						},
						TracerVersions: map[string]string{"java": "1.43.0"},
					},
				}
				return spec
			}(),
			profile: testProfileOverlayProfileSpec(&v2alpha1.SingleStepInstrumentation{
				Enabled: ptr.To(true),
				Targets: []v2alpha1.SSITarget{
					{
						Name: "api",
						PodSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "api"},
						},
						TracerVersions: map[string]string{"python": "2.14.0"},
						TracerConfigs: []corev1.EnvVar{
							{Name: "DD_TRACE_DEBUG", Value: "true"},
						},
					},
					{
						Name: "worker",
						PodSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "worker"},
						},
					},
				},
			}),
			want: &v2alpha1.SingleStepInstrumentation{
				Enabled:           ptr.To(true),
				LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(true)},
				Injector:          &v2alpha1.InjectorConfig{},
				Targets: []v2alpha1.SSITarget{
					{
						Name: "api",
						PodSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "api"},
						},
						TracerVersions: map[string]string{"java": "1.43.0", "python": "2.14.0"},
						TracerConfigs: []corev1.EnvVar{
							{Name: "DD_TRACE_DEBUG", Value: "true"},
						},
					},
					{
						Name: "worker",
						PodSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "worker"},
						},
					},
				},
			},
		},
		{
			name: "unnamed target-only overlay is ignored",
			dst:  testProfileOverlayBaseSpec(false),
			profile: testProfileOverlayProfileSpec(&v2alpha1.SingleStepInstrumentation{
				Enabled: ptr.To(true),
				Targets: []v2alpha1.SSITarget{
					{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}}},
				},
			}),
			wantNoApply: true,
		},
		{
			name: "targets require supported Cluster Agent version",
			dst: func() *v2alpha1.DatadogAgentSpec {
				spec := testProfileOverlayBaseSpec(false)
				spec.Override = map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
					v2alpha1.ClusterAgentComponentName: {Image: &v2alpha1.AgentImageConfig{Name: "gcr.io/datadoghq/cluster-agent:7.63.0"}},
				}
				return spec
			}(),
			profile: testProfileOverlayProfileSpec(&v2alpha1.SingleStepInstrumentation{
				Enabled: ptr.To(true),
				Targets: []v2alpha1.SSITarget{{Name: "api"}},
			}),
			wantErr: "features.apm.instrumentation.targets requires Cluster Agent version >= 7.64.0-0",
		},
		{
			name: "APM disabled profile with SSI enabled is rejected",
			dst:  testProfileOverlayBaseSpec(false),
			profile: func() *v2alpha1.DatadogAgentSpec {
				spec := testProfileOverlayProfileSpec(&v2alpha1.SingleStepInstrumentation{
					Enabled: ptr.To(true),
				})
				spec.Features.APM.Enabled = ptr.To(false)
				return spec
			}(),
			wantErr: "features.apm.enabled must be true or unset when features.apm.instrumentation.enabled is true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := applyAPMProfileSharedConfigOverlay(tt.dst, tt.dst.DeepCopy(), tt.profile)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			if tt.wantNoApply {
				assert.False(t, *tt.dst.Features.APM.SingleStepInstrumentation.Enabled)
				return
			}
			assert.Equal(t, tt.want, tt.dst.Features.APM.SingleStepInstrumentation)
		})
	}
}

func testProfileOverlayBaseSpec(ssiEnabled bool) *v2alpha1.DatadogAgentSpec {
	return &v2alpha1.DatadogAgentSpec{
		Features: &v2alpha1.DatadogFeatures{
			AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{Enabled: ptr.To(true)},
			APM: &v2alpha1.APMFeatureConfig{
				Enabled: ptr.To(false),
				SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
					Enabled:           ptr.To(ssiEnabled),
					LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(true)},
					Injector:          &v2alpha1.InjectorConfig{},
				},
			},
		},
	}
}

func testProfileOverlayProfileSpec(ssi *v2alpha1.SingleStepInstrumentation) *v2alpha1.DatadogAgentSpec {
	spec := &v2alpha1.DatadogAgentSpec{
		Features: &v2alpha1.DatadogFeatures{
			APM: &v2alpha1.APMFeatureConfig{
				SingleStepInstrumentation: ssi,
			},
		},
	}
	return spec
}
