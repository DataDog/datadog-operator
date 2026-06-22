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

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	featurefake "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
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
			name: "language detection conflict rejects profile",
			dst: func() *v2alpha1.DatadogAgentSpec {
				spec := testProfileOverlayBaseSpec(true)
				spec.Features.APM.SingleStepInstrumentation.LanguageDetection = &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(true)}
				return spec
			}(),
			profile: testProfileOverlayProfileSpec(&v2alpha1.SingleStepInstrumentation{
				Enabled:           ptr.To(true),
				LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(false)},
			}),
			wantErr: "features.apm.instrumentation.languageDetection.enabled has conflicting values",
		},
		{
			name: "injector image tag conflict rejects profile",
			dst: func() *v2alpha1.DatadogAgentSpec {
				spec := testProfileOverlayBaseSpec(true)
				spec.Features.APM.SingleStepInstrumentation.Injector = &v2alpha1.InjectorConfig{ImageTag: "7.66.0"}
				return spec
			}(),
			profile: testProfileOverlayProfileSpec(&v2alpha1.SingleStepInstrumentation{
				Enabled:  ptr.To(true),
				Injector: &v2alpha1.InjectorConfig{ImageTag: "7.67.0"},
			}),
			wantErr: `features.apm.instrumentation.injector.imageTag has conflicting values "7.66.0" and "7.67.0"`,
		},
		{
			name: "injection mode conflict rejects profile",
			dst: func() *v2alpha1.DatadogAgentSpec {
				spec := testProfileOverlayBaseSpec(true)
				spec.Features.APM.SingleStepInstrumentation.InjectionMode = v2alpha1.InjectionModeInitContainer
				return spec
			}(),
			profile: testProfileOverlayProfileSpec(&v2alpha1.SingleStepInstrumentation{
				Enabled:       ptr.To(true),
				InjectionMode: v2alpha1.InjectionModeCSI,
			}),
			wantErr: `features.apm.instrumentation.injectionMode has conflicting values "init_container" and "csi"`,
		},
		{
			name: "targets append in order",
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
						TracerVersions: map[string]string{"java": "1.43.0"},
					},
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
			},
		},
		{
			name: "same target name with different selector appends",
			dst: func() *v2alpha1.DatadogAgentSpec {
				spec := testProfileOverlayBaseSpec(true)
				spec.Features.APM.SingleStepInstrumentation.Targets = []v2alpha1.SSITarget{
					{
						Name: "api",
						PodSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "api"},
						},
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
							MatchLabels: map[string]string{"app": "api-v2"},
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
					},
					{
						Name: "api",
						PodSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "api-v2"},
						},
					},
				},
			},
		},
		{
			name: "same target and env var name with different values appends",
			dst: func() *v2alpha1.DatadogAgentSpec {
				spec := testProfileOverlayBaseSpec(true)
				spec.Features.APM.SingleStepInstrumentation.Targets = []v2alpha1.SSITarget{
					{
						Name: "api",
						PodSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "api"},
						},
						TracerConfigs: []corev1.EnvVar{
							{Name: "DD_TRACE_DEBUG", Value: "true"},
						},
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
						TracerConfigs: []corev1.EnvVar{
							{Name: "DD_TRACE_DEBUG", Value: "false"},
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
						TracerConfigs: []corev1.EnvVar{
							{Name: "DD_TRACE_DEBUG", Value: "true"},
						},
					},
					{
						Name: "api",
						PodSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "api"},
						},
						TracerConfigs: []corev1.EnvVar{
							{Name: "DD_TRACE_DEBUG", Value: "false"},
						},
					},
				},
			},
		},
		{
			name: "identical target tracer config appends",
			dst: func() *v2alpha1.DatadogAgentSpec {
				spec := testProfileOverlayBaseSpec(true)
				spec.Features.APM.SingleStepInstrumentation.Targets = []v2alpha1.SSITarget{
					{
						Name: "api",
						PodSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "api"},
						},
						TracerConfigs: []corev1.EnvVar{
							{Name: "DD_TRACE_DEBUG", Value: "true"},
						},
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
						TracerConfigs: []corev1.EnvVar{
							{Name: "DD_TRACE_DEBUG", Value: "true"},
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
						TracerConfigs: []corev1.EnvVar{
							{Name: "DD_TRACE_DEBUG", Value: "true"},
						},
					},
					{
						Name: "api",
						PodSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "api"},
						},
						TracerConfigs: []corev1.EnvVar{
							{Name: "DD_TRACE_DEBUG", Value: "true"},
						},
					},
				},
			},
		},
		{
			name: "unnamed target-only overlay is preserved",
			dst:  testProfileOverlayBaseSpec(false),
			profile: testProfileOverlayProfileSpec(&v2alpha1.SingleStepInstrumentation{
				Enabled: ptr.To(true),
				Targets: []v2alpha1.SSITarget{
					{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}}},
				},
			}),
			want: &v2alpha1.SingleStepInstrumentation{
				Enabled:           ptr.To(true),
				LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(true)},
				Targets: []v2alpha1.SSITarget{
					{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}}},
				},
			},
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
			name: "enabled false with namespace config is ignored",
			dst:  testProfileOverlayBaseSpec(false),
			profile: testProfileOverlayProfileSpec(&v2alpha1.SingleStepInstrumentation{
				Enabled:           ptr.To(false),
				EnabledNamespaces: []string{"payments"},
			}),
			wantNoApply: true,
		},
		{
			name: "base admission controller disabled with profile SSI enabled rejects profile",
			dst: func() *v2alpha1.DatadogAgentSpec {
				spec := testProfileOverlayBaseSpec(false)
				spec.Features.AdmissionController.Enabled = ptr.To(false)
				return spec
			}(),
			profile: testProfileOverlayProfileSpec(&v2alpha1.SingleStepInstrumentation{
				Enabled: ptr.To(true),
			}),
			wantErr: "features.admissionController.enabled must be true on the base DatadogAgent when APM instrumentation is configured",
		},
		{
			name: "base admission controller disabled with profile APM enabled only is ignored",
			dst: func() *v2alpha1.DatadogAgentSpec {
				spec := testProfileOverlayBaseSpec(false)
				spec.Features.AdmissionController.Enabled = ptr.To(false)
				return spec
			}(),
			profile: func() *v2alpha1.DatadogAgentSpec {
				spec := testProfileOverlayProfileSpec(nil)
				spec.Features.APM.Enabled = ptr.To(true)
				return spec
			}(),
			wantNoApply: true,
		},
		{
			name: "base cluster agent disabled with profile SSI enabled rejects profile",
			dst: func() *v2alpha1.DatadogAgentSpec {
				spec := testProfileOverlayBaseSpec(false)
				spec.Override = map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
					v2alpha1.ClusterAgentComponentName: {Disabled: ptr.To(true)},
				}
				return spec
			}(),
			profile: testProfileOverlayProfileSpec(&v2alpha1.SingleStepInstrumentation{
				Enabled: ptr.To(true),
			}),
			wantErr: "clusterAgent cannot be disabled on the base DatadogAgent when APM instrumentation is configured",
		},
		{
			name: "base cluster agent disabled with profile APM enabled only is ignored",
			dst: func() *v2alpha1.DatadogAgentSpec {
				spec := testProfileOverlayBaseSpec(false)
				spec.Override = map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
					v2alpha1.ClusterAgentComponentName: {Disabled: ptr.To(true)},
				}
				return spec
			}(),
			profile: func() *v2alpha1.DatadogAgentSpec {
				spec := testProfileOverlayProfileSpec(nil)
				spec.Features.APM.Enabled = ptr.To(true)
				return spec
			}(),
			wantNoApply: true,
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
			dst := tt.dst.DeepCopy()
			err := applyAPMProfileSharedConfigOverlay(dst, dst.DeepCopy(), tt.profile)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			if tt.wantNoApply {
				assert.False(t, *dst.Features.APM.SingleStepInstrumentation.Enabled)
				return
			}
			assert.Equal(t, tt.want, dst.Features.APM.SingleStepInstrumentation)
		})
	}
}

// Profile SSI overlays should render the same Cluster Agent config as direct
// base-DDA SSI config. Node Agent config is intentionally out of scope because
// profile-owned APM config renders on the profile DDAI instead of the default
// DDAI.
func TestAPMProfileSharedConfigOverlayMatchesDirectDDAClusterAgentConfig(t *testing.T) {
	tests := []struct {
		name string
		ssi  *v2alpha1.SingleStepInstrumentation
	}{
		{
			name: "enabled namespaces",
			ssi: &v2alpha1.SingleStepInstrumentation{
				Enabled:           ptr.To(true),
				EnabledNamespaces: []string{"payments", "checkout"},
				LibVersions:       map[string]string{"java": "1.43.0", "python": "2.14.0"},
				LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(true)},
				Injector:          &v2alpha1.InjectorConfig{ImageTag: "7.66.0"},
				InjectionMode:     v2alpha1.InjectionModeInitContainer,
				Targets: []v2alpha1.SSITarget{
					{
						Name: "api",
						PodSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "api"},
						},
						TracerVersions: map[string]string{"java": "1.43.0"},
						TracerConfigs: []corev1.EnvVar{
							{Name: "DD_TRACE_DEBUG", Value: "true"},
						},
					},
				},
			},
		},
		{
			name: "disabled namespaces",
			ssi: &v2alpha1.SingleStepInstrumentation{
				Enabled:            ptr.To(true),
				DisabledNamespaces: []string{"kube-system", "datadog"},
				LanguageDetection:  &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(false)},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			directDDA := testProfileOverlayBaseSpec(false)
			directDDA.Features.APM.SingleStepInstrumentation = tt.ssi.DeepCopy()

			overlayDDA := testProfileOverlayBaseSpec(false)
			profile := testProfileOverlayProfileSpec(tt.ssi.DeepCopy())
			require.NoError(t, applyAPMProfileSharedConfigOverlay(overlayDDA, overlayDDA.DeepCopy(), profile))

			assert.Equal(
				t,
				renderAPMClusterAgentEnvVars(t, directDDA),
				renderAPMClusterAgentEnvVars(t, overlayDDA),
			)
		})
	}
}

func renderAPMClusterAgentEnvVars(t testing.TB, spec *v2alpha1.DatadogAgentSpec) []*corev1.EnvVar {
	t.Helper()

	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datadog",
			Namespace: "default",
		},
		Spec: *spec.DeepCopy(),
	}
	feat := buildAPMFeature(nil).(*apmFeature)
	reqComp := feat.Configure(dda, &dda.Spec, nil)
	require.True(t, reqComp.ClusterAgent.IsEnabled())

	mgr := featurefake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})
	require.NoError(t, feat.ManageClusterAgent(mgr))

	return mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName]
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
