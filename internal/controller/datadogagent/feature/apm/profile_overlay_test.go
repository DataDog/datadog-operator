// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apm

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	v1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/defaults"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	featurefake "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func TestAPMProfileSharedConfigOverlay(t *testing.T) {
	tests := []struct {
		name         string
		dst          *v2alpha1.DatadogAgentSpec
		profile      *v2alpha1.DatadogAgentSpec
		want         *v2alpha1.SingleStepInstrumentation
		wantErr      string
		wantNoApply  bool
		priorProfile bool
	}{
		{
			name: "profile SSI overrides disabled base SSI",
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
			priorProfile: true,
			name:         "map conflict rejects profile",
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
			priorProfile: true,
			name:         "enabled and disabled namespaces reject final overlay",
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
			priorProfile: true,
			name:         "language detection conflict rejects profile",
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
			priorProfile: true,
			name:         "injector image tag conflict rejects profile",
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
			priorProfile: true,
			name:         "injection mode conflict rejects profile",
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
			priorProfile: true,
			name:         "on-demand conflict rejects profile",
			dst: func() *v2alpha1.DatadogAgentSpec {
				spec := testProfileOverlayBaseSpec(true)
				spec.Features.APM.SingleStepInstrumentation.OnDemand = ptr.To(true)
				return spec
			}(),
			profile: testProfileOverlayProfileSpec(&v2alpha1.SingleStepInstrumentation{
				Enabled:  ptr.To(true),
				OnDemand: ptr.To(false),
			}),
			wantErr: "features.apm.instrumentation.onDemand has conflicting values",
		},
		{
			name: "disabled base keeps on-demand opt-out",
			dst: func() *v2alpha1.DatadogAgentSpec {
				spec := testProfileOverlayBaseSpec(false)
				spec.Features.APM.SingleStepInstrumentation.OnDemand = ptr.To(false)
				return spec
			}(),
			profile: testProfileOverlayProfileSpec(&v2alpha1.SingleStepInstrumentation{
				Enabled: ptr.To(true),
			}),
			want: &v2alpha1.SingleStepInstrumentation{
				Injector:          &v2alpha1.InjectorConfig{},
				Enabled:           ptr.To(true),
				OnDemand:          ptr.To(false),
				LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(true)},
			},
		},
		{
			priorProfile: true,
			name:         "prior profile on-demand conflict rejects profile",
			dst: func() *v2alpha1.DatadogAgentSpec {
				spec := testProfileOverlayBaseSpec(false)
				spec.Features.APM.SingleStepInstrumentation.OnDemand = ptr.To(true)
				return spec
			}(),
			profile: testProfileOverlayProfileSpec(&v2alpha1.SingleStepInstrumentation{
				Enabled:  ptr.To(true),
				OnDemand: ptr.To(false),
			}),
			wantErr: "features.apm.instrumentation.onDemand has conflicting values",
		},
		{
			priorProfile: true,
			name:         "targets append in order",
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
			priorProfile: true,
			name:         "same target name with different selector appends",
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
			priorProfile: true,
			name:         "same target and env var name with different values appends",
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
			priorProfile: true,
			name:         "identical target tracer config appends",
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
				Injector:          &v2alpha1.InjectorConfig{},
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
			name: "enabled false preserves explicit namespace config",
			dst:  testProfileOverlayBaseSpec(false),
			profile: testProfileOverlayProfileSpec(&v2alpha1.SingleStepInstrumentation{
				Enabled:           ptr.To(false),
				EnabledNamespaces: []string{"payments"},
			}),
			want: &v2alpha1.SingleStepInstrumentation{Enabled: ptr.To(false), EnabledNamespaces: []string{"payments"}, LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(true)}, Injector: &v2alpha1.InjectorConfig{}},
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
			name: "APM disabled profile can enable SSI",
			dst:  testProfileOverlayBaseSpec(false),
			profile: func() *v2alpha1.DatadogAgentSpec {
				spec := testProfileOverlayProfileSpec(&v2alpha1.SingleStepInstrumentation{
					Enabled: ptr.To(true),
				})
				spec.Features.APM.Enabled = ptr.To(false)
				return spec
			}(),
			want: &v2alpha1.SingleStepInstrumentation{Enabled: ptr.To(true), LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(true)}, Injector: &v2alpha1.InjectorConfig{}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dst := tt.dst.DeepCopy()
			validation := &v2alpha1.DatadogAgentSpec{}
			if tt.priorProfile {
				validation = tt.dst.DeepCopy()
				validation.Features.APM.SingleStepInstrumentation.Enabled = nil
			}
			err := applyAPMOverlayForTest(dst, validation, tt.profile)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			if tt.wantNoApply {
				assert.Equal(t, tt.dst, dst)
				return
			}
			assert.Equal(t, tt.want, dst.Features.APM.SingleStepInstrumentation)
		})
	}
}

func TestAPMProfileSharedConfigOverlaySingletonPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name     string
		dda, dap *bool
		want     *bool
	}{
		{name: "both unset"},
		{name: "DDA set DAP unset", dda: ptr.To(false), want: ptr.To(false)},
		{name: "DDA unset DAP set", dap: ptr.To(false), want: ptr.To(false)},
		{name: "same explicit values", dda: ptr.To(false), dap: ptr.To(false), want: ptr.To(false)},
		{name: "profile overrides explicit DDA value", dda: ptr.To(true), dap: ptr.To(false), want: ptr.To(false)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rawBase := testProfileOverlayBaseSpec(true)
			rawBase.Features.APM.SingleStepInstrumentation.LanguageDetection = &v2alpha1.LanguageDetectionConfig{Enabled: tc.dda}
			dst := rawBase.DeepCopy()
			defaults.DefaultDatadogAgentSpec(dst)
			profile := testProfileOverlayProfileSpec(&v2alpha1.SingleStepInstrumentation{
				Enabled:           ptr.To(true),
				LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: tc.dap},
			})
			err := applyAPMOverlayForTest(dst, &v2alpha1.DatadogAgentSpec{}, profile)
			require.NoError(t, err)
			assert.Equal(t, tc.want, dst.Features.APM.SingleStepInstrumentation.LanguageDetection.Enabled)
		})
	}
}

func TestAPMProfileSharedConfigOverlayDefaultConflicts(t *testing.T) {
	for _, baseSSIEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("base SSI enabled=%t", baseSSIEnabled), func(t *testing.T) {
			rawBase := &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					APM: &v2alpha1.APMFeatureConfig{
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{Enabled: ptr.To(baseSSIEnabled)},
					},
				},
			}
			defaultedBase := rawBase.DeepCopy()
			defaults.DefaultDatadogAgentSpec(defaultedBase)
			require.Equal(t, ptr.To(true), defaultedBase.Features.APM.SingleStepInstrumentation.LanguageDetection.Enabled)

			profile := testProfileOverlayProfileSpec(&v2alpha1.SingleStepInstrumentation{
				Enabled:           ptr.To(true),
				LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(false)},
			})
			require.NoError(t, applyAPMOverlayForTest(defaultedBase, &v2alpha1.DatadogAgentSpec{}, profile))
			assert.Equal(t, ptr.To(false), defaultedBase.Features.APM.SingleStepInstrumentation.LanguageDetection.Enabled)
		})
	}

	t.Run("explicit profile overrides explicit base value", func(t *testing.T) {
		rawBase := testProfileOverlayBaseSpec(true)
		rawBase.Features.APM.SingleStepInstrumentation.LanguageDetection = &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(true)}
		defaultedBase := rawBase.DeepCopy()
		defaults.DefaultDatadogAgentSpec(defaultedBase)

		profile := testProfileOverlayProfileSpec(&v2alpha1.SingleStepInstrumentation{
			Enabled:           ptr.To(true),
			LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(false)},
		})
		err := applyAPMOverlayForTest(defaultedBase, &v2alpha1.DatadogAgentSpec{}, profile)
		require.NoError(t, err)
		assert.Equal(t, ptr.To(false), defaultedBase.Features.APM.SingleStepInstrumentation.LanguageDetection.Enabled)
	})

	t.Run("accepted profile value conflicts", func(t *testing.T) {
		rawBase := testProfileOverlayBaseSpec(false)
		defaultedBase := rawBase.DeepCopy()
		defaults.DefaultDatadogAgentSpec(defaultedBase)
		acceptedProfile := testProfileOverlayProfileSpec(&v2alpha1.SingleStepInstrumentation{
			Enabled:           ptr.To(true),
			LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(true)},
		})
		currentProfile := testProfileOverlayProfileSpec(&v2alpha1.SingleStepInstrumentation{
			Enabled:           ptr.To(true),
			LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(false)},
		})

		shared := &v2alpha1.DatadogAgentSpec{}
		require.NoError(t, applyAPMProfileSharedConfigOverlay(shared, defaultedBase.DeepCopy(), defaultedBase, acceptedProfile))
		err := applyAPMProfileSharedConfigOverlay(shared, defaultedBase.DeepCopy(), defaultedBase, currentProfile)
		require.ErrorContains(t, err, "features.apm.instrumentation.languageDetection.enabled has conflicting values")
	})
}

func TestAPMProfileSharedConfigOverlayIsolation(t *testing.T) {
	bools := []*bool{nil, ptr.To(false), ptr.To(true)}
	boolNames := []string{"omitted", "disabled", "enabled"}
	profileModes := []string{"absent", "inherit", "disabled", "enabled", "SSI", "error tracking"}
	profileHostPorts := []*v2alpha1.HostPortConfig{
		nil,
		{Enabled: ptr.To(true)},
		{Enabled: ptr.To(true), Port: ptr.To[int32](8126)},
		{Enabled: ptr.To(true), Port: ptr.To[int32](9126)},
		{Enabled: ptr.To(false), Port: ptr.To[int32](9126)},
	}
	portNames := []string{"omitted", "enabled-port-omitted", "enabled-8126", "enabled-9126", "disabled-9126"}
	for apmIndex, enabled := range bools {
		for hostIndex, hostEnabled := range bools {
			for _, mode := range profileModes {
				for portIndex, hostPort := range profileHostPorts {
					t.Run(fmt.Sprintf("baseAPM=%s/baseHostPort=%s/profile=%s/hostPort=%s", boolNames[apmIndex], boolNames[hostIndex], mode, portNames[portIndex]), func(t *testing.T) {
						raw := testProfileOverlayBaseSpec(false)
						raw.Features.APM.Enabled = enabled
						if hostEnabled != nil {
							raw.Features.APM.HostPortConfig = &v2alpha1.HostPortConfig{Enabled: hostEnabled}
						}
						before := raw.DeepCopy()
						defaults.DefaultDatadogAgentSpec(before)
						profile := testProfileOverlayProfileSpec(nil)
						profile.Global = &v2alpha1.GlobalConfig{ClusterName: ptr.To("profile-only")}
						profile.Features.LogCollection = &v2alpha1.LogCollectionFeatureConfig{Enabled: ptr.To(true)}
						profile.Features.APM.HostPortConfig = hostPort
						profile.Features.APM.UnixDomainSocketConfig = &v2alpha1.UnixDomainSocketConfig{Enabled: ptr.To(false), Path: ptr.To("/profile-only.socket")}
						switch mode {
						case "absent":
							profile.Features.APM = nil
						case "disabled":
							profile.Features.APM.Enabled = ptr.To(false)
						case "enabled":
							profile.Features.APM.Enabled = ptr.To(true)
						case "SSI":
							profile.Features.APM.SingleStepInstrumentation = &v2alpha1.SingleStepInstrumentation{Enabled: ptr.To(true)}
						case "error tracking":
							profile.Features.APM.ErrorTrackingStandalone = &v2alpha1.ErrorTrackingStandalone{Enabled: ptr.To(true)}
						}
						rawBefore, profileBefore := raw.DeepCopy(), profile.DeepCopy()
						dst := before.DeepCopy()
						err := applyAPMOverlayForTest(dst, &v2alpha1.DatadogAgentSpec{}, profile)
						assert.Equal(t, rawBefore, raw, "raw input must remain unchanged")
						assert.Equal(t, profileBefore, profile, "profile input must remain unchanged")
						require.NoError(t, err)
						assert.Equal(t, before.Features.APM.HostPortConfig, dst.Features.APM.HostPortConfig, "node host ports must be preserved")
						// Only shared SSI settings may change on the default spec.
						dst.Features.APM.SingleStepInstrumentation = before.Features.APM.SingleStepInstrumentation.DeepCopy()
						assert.Equal(t, before, dst, "profile-local settings must not reach the default spec")
					})
				}
			}
		}
	}
}

func TestAPMProfileSharedConfigOverlayLocalAgentServicePortConflict(t *testing.T) {
	for _, firstPort := range []*int32{nil, ptr.To[int32](8126), ptr.To[int32](9126)} {
		for _, secondPort := range []*int32{nil, ptr.To[int32](8126), ptr.To[int32](9126)} {
			base := testProfileOverlayBaseSpec(false)
			base.Features.APM.HostPortConfig = &v2alpha1.HostPortConfig{Enabled: ptr.To(false), Port: ptr.To[int32](10126)}
			shared := &v2alpha1.DatadogAgentSpec{}
			dst := base.DeepCopy()
			var accepted *int32
			for _, port := range []*int32{firstPort, secondPort} {
				profile := testProfileOverlayProfileAPMSpec(8126)
				profile.Features.APM.HostPortConfig.Port = port
				candidate, candidateShared := dst.DeepCopy(), shared.DeepCopy()
				err := applyAPMProfileSharedConfigOverlay(candidateShared, candidate, base, profile)
				if accepted != nil && port != nil && *accepted != *port {
					require.ErrorContains(t, err, `local Agent Service port "traceport" conflicts`)
				} else {
					require.NoError(t, err)
					dst, shared = candidate, candidateShared
					if port != nil {
						accepted = ptr.To(*port)
					}
				}
				assert.Equal(t, base, dst, "Service port selection must not change the default DDAI")
				if accepted != nil {
					assert.Equal(t, accepted, shared.Features.APM.HostPortConfig.Port)
					assert.Nil(t, shared.Features.APM.HostPortConfig.Enabled)
				} else {
					assert.Nil(t, shared.Features, "omitted ports must not create shared configuration")
				}
			}
		}
	}
}

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
			require.NoError(t, applyAPMOverlayForTest(overlayDDA, &v2alpha1.DatadogAgentSpec{}, profile))

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

func testProfileOverlayProfileAPMSpec(port int32) *v2alpha1.DatadogAgentSpec {
	spec := testProfileOverlayProfileSpec(nil)
	spec.Features.APM.Enabled = ptr.To(true)
	spec.Features.APM.HostPortConfig = &v2alpha1.HostPortConfig{
		Enabled: ptr.To(true),
		Port:    ptr.To(port),
	}
	return spec
}

// applyAPMOverlayForTest commits the generated candidate only on success.
func applyAPMOverlayForTest(dst, rawBase, profile *v2alpha1.DatadogAgentSpec) error {
	rawCandidate := rawBase.DeepCopy()
	ddaiCandidate := dst.DeepCopy()
	if err := applyAPMProfileSharedConfigOverlay(rawCandidate, ddaiCandidate, dst, profile); err != nil {
		return err
	}
	*dst = *ddaiCandidate
	return nil
}

// Defaults must not become explicit constraints on profiles, even after an
// earlier accepted profile has inherited them.
func TestAPMProfileSharedConfigOverlayDefaultsBeforeMultipleProfiles(t *testing.T) {
	validationSpec := testProfileOverlayBaseSpec(true)
	validationSpec.Features.APM.Enabled = ptr.To(true)
	validationSpec.Features.APM.SingleStepInstrumentation.LanguageDetection = nil
	defaultDDAISpec := validationSpec.DeepCopy()
	defaults.DefaultDatadogAgentSpec(defaultDDAISpec)
	originalDefaultDDAISpec := defaultDDAISpec.DeepCopy()
	require.Equal(t, ptr.To(true), defaultDDAISpec.Features.APM.SingleStepInstrumentation.LanguageDetection.Enabled)

	omitted := testProfileOverlayProfileSpec(&v2alpha1.SingleStepInstrumentation{Enabled: ptr.To(true)})
	require.NoError(t, applyAPMProfileSharedConfigOverlay(validationSpec, defaultDDAISpec, originalDefaultDDAISpec, omitted))
	require.Nil(t, validationSpec.Features.APM.SingleStepInstrumentation.LanguageDetection)

	explicit := testProfileOverlayProfileSpec(&v2alpha1.SingleStepInstrumentation{
		Enabled: ptr.To(true), LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(false)},
	})
	require.NoError(t, applyAPMProfileSharedConfigOverlay(validationSpec, defaultDDAISpec, originalDefaultDDAISpec, explicit))
	require.NoError(t, applyAPMProfileSharedConfigOverlay(validationSpec, defaultDDAISpec, originalDefaultDDAISpec, omitted))
	assert.Equal(t, ptr.To(false), defaultDDAISpec.Features.APM.SingleStepInstrumentation.LanguageDetection.Enabled)

	conflicting := explicit.DeepCopy()
	conflicting.Features.APM.SingleStepInstrumentation.LanguageDetection.Enabled = ptr.To(true)
	err := applyAPMProfileSharedConfigOverlay(validationSpec.DeepCopy(), defaultDDAISpec.DeepCopy(), originalDefaultDDAISpec, conflicting)
	require.ErrorContains(t, err, "languageDetection.enabled has conflicting values")
}

func TestAPMProfileSharedConfigOverlayDefaultedBaseOnDemand(t *testing.T) {
	for _, baseSSIEnabled := range []bool{false, true} {
		for _, earlyDefault := range []bool{false, true} {
			t.Run(fmt.Sprintf("base SSI enabled=%t/early default=%t", baseSSIEnabled, earlyDefault), func(t *testing.T) {
				raw := &v2alpha1.DatadogAgentSpec{
					Features: &v2alpha1.DatadogFeatures{
						APM: &v2alpha1.APMFeatureConfig{
							SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{Enabled: ptr.To(baseSSIEnabled)},
						},
					},
				}
				dst := raw.DeepCopy()
				defaults.DefaultDatadogAgentSpec(dst)
				if earlyDefault {
					// Also support defaulting onDemand before applying profiles.
					dst.Features.APM.SingleStepInstrumentation.OnDemand = ptr.To(true)
				}
				base := dst.DeepCopy()
				raw = &v2alpha1.DatadogAgentSpec{}
				omitted := testProfileOverlayProfileSpec(&v2alpha1.SingleStepInstrumentation{Enabled: ptr.To(true)})
				require.NoError(t, applyAPMProfileSharedConfigOverlay(raw, dst, base, omitted))
				require.Nil(t, raw.Features.APM.SingleStepInstrumentation.OnDemand)

				profile := testProfileOverlayProfileSpec(&v2alpha1.SingleStepInstrumentation{
					Enabled: ptr.To(true), OnDemand: ptr.To(false),
				})
				require.NoError(t, applyAPMProfileSharedConfigOverlay(raw, dst, base, profile))
				assert.Equal(t, ptr.To(false), dst.Features.APM.SingleStepInstrumentation.OnDemand)
				assert.Equal(t, ptr.To(false), raw.Features.APM.SingleStepInstrumentation.OnDemand)
				assert.Contains(t, renderAPMClusterAgentEnvVars(t, dst), &corev1.EnvVar{Name: DDAPMInstrumentationOnDemand, Value: "false"})
			})
		}
	}
}

func TestAPMProfileSharedPortDoesNotEnableNodeAPM(t *testing.T) {
	for _, tc := range []struct {
		name string
		apm  *v2alpha1.APMFeatureConfig
	}{
		{name: "SSI", apm: &v2alpha1.APMFeatureConfig{SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{Enabled: ptr.To(true)}}},
		{name: "standalone error tracking", apm: &v2alpha1.APMFeatureConfig{ErrorTrackingStandalone: &v2alpha1.ErrorTrackingStandalone{Enabled: ptr.To(true)}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			validationSpec := testProfileOverlayBaseSpec(false)
			defaultDDAISpec := validationSpec.DeepCopy()
			defaults.DefaultDatadogAgentSpec(defaultDDAISpec)
			base := defaultDDAISpec.DeepCopy()
			validationSpec = &v2alpha1.DatadogAgentSpec{}
			profile := &v2alpha1.DatadogAgentSpec{Features: &v2alpha1.DatadogFeatures{APM: tc.apm.DeepCopy()}}
			profile.Features.APM.HostPortConfig = &v2alpha1.HostPortConfig{Enabled: ptr.To(true), Port: ptr.To[int32](9126)}

			require.NoError(t, applyAPMProfileSharedConfigOverlay(validationSpec, defaultDDAISpec, base, profile))
			require.NotNil(t, validationSpec.Features.APM.HostPortConfig)
			assert.Equal(t, ptr.To[int32](9126), validationSpec.Features.APM.HostPortConfig.Port)
			assert.Equal(t, base.Features.APM.HostPortConfig, defaultDDAISpec.Features.APM.HostPortConfig)
			assert.Equal(t, ptr.To(false), defaultDDAISpec.Features.APM.Enabled)
			assert.Nil(t, profile.Features.APM.Enabled)

			conflicting := profile.DeepCopy()
			conflicting.Features.APM.HostPortConfig.Port = ptr.To[int32](10126)
			err := applyAPMProfileSharedConfigOverlay(validationSpec.DeepCopy(), defaultDDAISpec.DeepCopy(), base, conflicting)
			require.ErrorContains(t, err, "conflicts with existing port")
		})
	}
}

func TestAPMProfileServicePortDoesNotChangeNodeConfig(t *testing.T) {
	for _, enabled := range []*bool{nil, ptr.To(false), ptr.To(true)} {
		for _, port := range []*int32{nil, ptr.To[int32](8126), ptr.To[int32](10126)} {
			base := testProfileOverlayBaseSpec(false)
			base.Features.APM.HostPortConfig = &v2alpha1.HostPortConfig{Enabled: enabled, Port: port}
			shared := base.DeepCopy()
			validation := &v2alpha1.DatadogAgentSpec{}
			profile := testProfileOverlayProfileAPMSpec(9126)
			beforeBase, beforeProfile := base.DeepCopy(), profile.DeepCopy()
			require.NoError(t, applyAPMProfileSharedConfigOverlay(validation, shared, base, profile))
			assert.Equal(t, base, shared, "a Service port contribution must not change the default node spec")
			require.NotNil(t, validation.Features.APM.HostPortConfig)
			assert.Equal(t, ptr.To[int32](9126), validation.Features.APM.HostPortConfig.Port)
			assert.Nil(t, validation.Features.APM.HostPortConfig.Enabled, "the profile contribution must not invent enablement")
			assert.Equal(t, beforeBase, base)
			assert.Equal(t, beforeProfile, profile)
		}
	}
}

func TestAPMProfileSharedLanguageDetection(t *testing.T) {
	values := []struct {
		name   string
		config *v2alpha1.LanguageDetectionConfig
	}{
		{name: "absent"},
		{name: "empty", config: &v2alpha1.LanguageDetectionConfig{}},
		{name: "false", config: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(false)}},
		{name: "true", config: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(true)}},
	}
	for _, baseValue := range values {
		for _, first := range values {
			for _, second := range values {
				t.Run(baseValue.name+"/"+first.name+"/"+second.name, func(t *testing.T) {
					base := testProfileOverlayBaseSpec(false)
					base.Features.APM.SingleStepInstrumentation.LanguageDetection = baseValue.config.DeepCopy()
					shared := base.DeepCopy()
					validation := &v2alpha1.DatadogAgentSpec{}
					var accepted *bool
					for _, value := range []struct {
						name   string
						config *v2alpha1.LanguageDetectionConfig
					}{first, second} {
						profile := testProfileOverlayProfileSpec(&v2alpha1.SingleStepInstrumentation{LanguageDetection: value.config.DeepCopy()})
						profileBefore, baseBefore := profile.DeepCopy(), base.DeepCopy()
						before := shared.DeepCopy()
						validationBefore := validation.DeepCopy()
						candidate, candidateValidation := shared.DeepCopy(), validation.DeepCopy()
						err := applyAPMProfileSharedConfigOverlay(candidateValidation, candidate, base, profile)
						explicit := value.config != nil && value.config.Enabled != nil
						conflict := explicit && accepted != nil && *accepted != *value.config.Enabled
						if conflict {
							require.ErrorContains(t, err, "languageDetection.enabled has conflicting values")
							assert.Equal(t, before, shared)
							assert.Equal(t, validationBefore, validation)
						} else {
							require.NoError(t, err)
							shared, validation = candidate, candidateValidation
							if explicit {
								accepted = ptr.To(*value.config.Enabled)
							}
						}
						expected := baseValue.config.DeepCopy()
						if accepted != nil {
							expected = &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(*accepted)}
						}
						assert.Equal(t, expected, shared.Features.APM.SingleStepInstrumentation.LanguageDetection)
						raw := validation.Features.APM.SingleStepInstrumentation.LanguageDetection
						if accepted == nil {
							assert.Nil(t, raw)
						} else {
							require.NotNil(t, raw)
							assert.Equal(t, accepted, raw.Enabled)
						}
						assert.Equal(t, profileBefore, profile)
						assert.Equal(t, baseBefore, base)
						assert.Equal(t, base.Override, shared.Override, "shared settings must not introduce component overrides")
					}
				})
			}
		}
	}
}

func TestAPMSharedLanguageDetectionComponents(t *testing.T) {
	for _, ssiEnabled := range []*bool{nil, ptr.To(false), ptr.To(true)} {
		ssiName := "omitted"
		if ssiEnabled != nil {
			ssiName = fmt.Sprint(*ssiEnabled)
		}
		for _, enabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("ssi=%s/detection=%v", ssiName, enabled), func(t *testing.T) {
				base := testProfileOverlayBaseSpec(false)
				base.Features.APM.SingleStepInstrumentation.Enabled = ssiEnabled
				base.Features.APM.SingleStepInstrumentation.LanguageDetection.Enabled = ptr.To(!enabled)
				defaults.DefaultDatadogAgentSpec(base)
				shared := base.DeepCopy()
				profile := testProfileOverlayProfileSpec(&v2alpha1.SingleStepInstrumentation{LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(enabled)}})
				require.NoError(t, applyAPMProfileSharedConfigOverlay(&v2alpha1.DatadogAgentSpec{}, shared, base, profile))
				owner := &v1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"}, Spec: *shared}
				f := &apmFeature{}
				required := f.Configure(owner, shared, nil)
				assert.True(t, required.ClusterAgent.IsEnabled())
				detectionEnabled := ptr.Deref(ssiEnabled, false) && enabled
				pods := featurefake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})
				require.NoError(t, f.ManageClusterAgent(pods))
				require.NoError(t, f.ManageNodeAgent(pods))
				assert.Contains(t, pods.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName], &corev1.EnvVar{Name: DDLanguageDetectionReportingEnabled, Value: fmt.Sprint(detectionEnabled)})
				if detectionEnabled {
					assert.Contains(t, pods.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName], &corev1.EnvVar{Name: DDLanguageDetectionEnabled, Value: "true"})
				} else {
					assert.NotContains(t, pods.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName], &corev1.EnvVar{Name: DDLanguageDetectionEnabled, Value: "true"})
				}
				resources := store.NewStore(owner, nil)
				require.NoError(t, f.ManageDependencies(feature.NewResourceManagers(resources)))
				_, found := resources.Get(kubernetes.ClusterRoleBindingKind, "", getRBACResourceName(owner))
				assert.Equal(t, detectionEnabled, found)
				assert.Empty(t, owner.Annotations)
				assert.Equal(t, base.Override, shared.Override)
			})
		}
	}
}

func TestAPMProfileLanguageDetectionRequiresClusterAgent(t *testing.T) {
	for _, enabled := range []*bool{nil, ptr.To(false), ptr.To(true)} {
		base := testProfileOverlayBaseSpec(false)
		base.Override = map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{v2alpha1.ClusterAgentComponentName: {Disabled: ptr.To(true)}}
		shared := base.DeepCopy()
		profile := testProfileOverlayProfileSpec(&v2alpha1.SingleStepInstrumentation{LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: enabled}})
		err := applyAPMOverlayForTest(shared, &v2alpha1.DatadogAgentSpec{}, profile)
		if ptr.Deref(enabled, false) {
			require.ErrorContains(t, err, "clusterAgent cannot be disabled")
			assert.Equal(t, base, shared)
		} else {
			require.NoError(t, err)
		}
	}
}
