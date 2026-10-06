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
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/defaults"
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
			name: "on-demand conflict rejects profile",
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
				Enabled:           ptr.To(true),
				OnDemand:          ptr.To(false),
				LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(true)},
			},
		},
		{
			name: "disabled base on-demand conflict rejects profile",
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
			err := applyAPMOverlayForTest(dst, tt.dst.DeepCopy(), tt.profile)
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

func TestAPMProfileSharedConfigOverlaySingletonPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name     string
		dda, dap *bool
		want     bool
		wantErr  bool
	}{
		{name: "both unset", want: true},
		{name: "DDA set DAP unset", dda: ptr.To(false), want: false},
		{name: "DDA unset DAP set", dap: ptr.To(false), want: false},
		{name: "same explicit values", dda: ptr.To(false), dap: ptr.To(false), want: false},
		{name: "different explicit values", dda: ptr.To(true), dap: ptr.To(false), wantErr: true},
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
			err := applyAPMOverlayForTest(dst, rawBase, profile)
			if tc.wantErr {
				require.ErrorContains(t, err, "languageDetection.enabled has conflicting values")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, ptr.To(tc.want), dst.Features.APM.SingleStepInstrumentation.LanguageDetection.Enabled)
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
			require.NoError(t, applyAPMOverlayForTest(defaultedBase, rawBase, profile))
			assert.Equal(t, ptr.To(false), defaultedBase.Features.APM.SingleStepInstrumentation.LanguageDetection.Enabled)
		})
	}

	t.Run("explicit base value still conflicts", func(t *testing.T) {
		rawBase := testProfileOverlayBaseSpec(true)
		rawBase.Features.APM.SingleStepInstrumentation.LanguageDetection = &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(true)}
		defaultedBase := rawBase.DeepCopy()
		defaults.DefaultDatadogAgentSpec(defaultedBase)

		profile := testProfileOverlayProfileSpec(&v2alpha1.SingleStepInstrumentation{
			Enabled:           ptr.To(true),
			LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(false)},
		})
		err := applyAPMOverlayForTest(defaultedBase, rawBase, profile)
		require.ErrorContains(t, err, "features.apm.instrumentation.languageDetection.enabled has conflicting values")
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

		shared := rawBase.DeepCopy()
		require.NoError(t, applyAPMProfileSharedConfigOverlay(shared, defaultedBase.DeepCopy(), defaultedBase, acceptedProfile))
		err := applyAPMProfileSharedConfigOverlay(shared, defaultedBase.DeepCopy(), defaultedBase, currentProfile)
		require.ErrorContains(t, err, "features.apm.instrumentation.languageDetection.enabled has conflicting values")
	})
}

func TestAPMProfileSharedConfigOverlayLocalAgentServicePortConflict(t *testing.T) {

	for _, tc := range []struct {
		name        string
		basePort    *int32
		profilePort *int32
		wantPort    int32
		wantErr     bool
	}{
		{name: "both unset", wantPort: 8126},
		{name: "DDA set DAP unset", basePort: ptr.To[int32](9126), wantPort: 9126},
		{name: "DDA unset DAP set", profilePort: ptr.To[int32](9126), wantPort: 9126},
		{name: "same explicit values", basePort: ptr.To[int32](9126), profilePort: ptr.To[int32](9126), wantPort: 9126},
		{name: "different explicit values", basePort: ptr.To[int32](8126), profilePort: ptr.To[int32](9126), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rawBase := &v2alpha1.DatadogAgentSpec{}
			if tc.basePort != nil {
				rawBase = testProfileOverlayProfileAPMSpec(*tc.basePort)
			}
			dst := rawBase.DeepCopy()
			defaults.DefaultDatadogAgentSpec(dst)
			profile := testProfileOverlayProfileAPMSpec(8126)
			profile.Features.APM.Enabled = nil
			if tc.profilePort == nil {
				profile.Features.APM.HostPortConfig = nil
			} else {
				profile.Features.APM.HostPortConfig.Port = tc.profilePort
			}
			before := dst.DeepCopy()
			err := applyAPMOverlayForTest(dst, rawBase, profile)
			if tc.wantErr {
				require.ErrorContains(t, err, `local Agent Service port "traceport" conflicts`)
				assert.Equal(t, before, dst)
				return
			}
			require.NoError(t, err)
			port, ok := profileLocalAgentServicePort(dst)
			require.True(t, ok)
			assert.Equal(t, tc.wantPort, port)
		})
	}

	t.Run("defaulted base port already covers profile default port without enabling base host port", func(t *testing.T) {
		base := testProfileOverlayBaseSpec(false)
		base.Features.APM.Enabled = ptr.To(true)
		base.Features.APM.HostPortConfig = &v2alpha1.HostPortConfig{
			Enabled: ptr.To(false),
			Port:    ptr.To(int32(8126)),
		}
		dst := base.DeepCopy()

		require.NoError(t, applyAPMOverlayForTest(dst, base, testProfileOverlayProfileAPMSpec(8126)))
		require.NotNil(t, dst.Features.APM.HostPortConfig)
		assert.False(t, ptr.Deref(dst.Features.APM.HostPortConfig.Enabled, true))
		assert.Equal(t, int32(8126), ptr.Deref(dst.Features.APM.HostPortConfig.Port, 0))
	})

	t.Run("omitted profile port does not conflict with an explicit profile port", func(t *testing.T) {
		rawBase := &v2alpha1.DatadogAgentSpec{}
		dst := rawBase.DeepCopy()
		defaults.DefaultDatadogAgentSpec(dst)
		omitted := testProfileOverlayProfileAPMSpec(8126)
		omitted.Features.APM.HostPortConfig = nil
		explicit := testProfileOverlayProfileAPMSpec(9126)
		shared := rawBase.DeepCopy()
		base := dst.DeepCopy()
		require.NoError(t, applyAPMProfileSharedConfigOverlay(shared, dst, base, omitted))
		require.NoError(t, applyAPMProfileSharedConfigOverlay(shared, dst, base, explicit))
		require.NoError(t, applyAPMProfileSharedConfigOverlay(shared, dst, base, omitted))
		// Both candidates are updated by each accepted overlay.
		assert.Equal(t, ptr.To[int32](9126), shared.Features.APM.HostPortConfig.Port)
		assert.Equal(t, ptr.To[int32](9126), dst.Features.APM.HostPortConfig.Port)
	})

	t.Run("prior profile contributes port, later profile conflicts", func(t *testing.T) {
		base := testProfileOverlayBaseSpec(false)
		shared := base.DeepCopy()
		profile := testProfileOverlayProfileAPMSpec(8126)
		require.NoError(t, applyAPMProfileSharedConfigOverlay(shared, base.DeepCopy(), base, profile))
		require.NoError(t, applyAPMProfileSharedConfigOverlay(shared, base.DeepCopy(), base, profile))
		err := applyAPMProfileSharedConfigOverlay(shared, base.DeepCopy(), base, testProfileOverlayProfileAPMSpec(9126))
		require.ErrorContains(t, err, `local Agent Service port "traceport" conflicts`)
		assert.Equal(t, ptr.To[int32](8126), configuredServicePort(shared.Features.APM))
	})

	t.Run("omitted profile port preserves default node host-port settings", func(t *testing.T) {
		raw := &v2alpha1.DatadogAgentSpec{}
		base := raw.DeepCopy()
		defaults.DefaultDatadogAgentSpec(base)
		profile := testProfileOverlayProfileAPMSpec(8126)
		profile.Features.APM.HostPortConfig = nil
		require.NoError(t, applyAPMOverlayForTest(base, raw, profile))
		assert.False(t, *base.Features.APM.HostPortConfig.Enabled)
		assert.Equal(t, ptr.To[int32](8126), base.Features.APM.HostPortConfig.Port)
	})

	t.Run("explicit default port remains a constraint without enabling default host ports", func(t *testing.T) {
		raw := &v2alpha1.DatadogAgentSpec{}
		base := raw.DeepCopy()
		defaults.DefaultDatadogAgentSpec(base)
		shared := raw.DeepCopy()
		require.NoError(t, applyAPMProfileSharedConfigOverlay(shared, base.DeepCopy(), base, testProfileOverlayProfileAPMSpec(8126)))
		assert.False(t, *shared.Features.APM.HostPortConfig.Enabled)
		err := applyAPMProfileSharedConfigOverlay(shared.DeepCopy(), base.DeepCopy(), base, testProfileOverlayProfileAPMSpec(9126))
		require.ErrorContains(t, err, `local Agent Service port "traceport" conflicts`)
	})

	t.Run("base DDA has host port, profile tries different port", func(t *testing.T) {
		base := testProfileOverlayBaseSpec(false)
		base.Features.APM.Enabled = ptr.To(true)
		base.Features.APM.HostPortConfig = &v2alpha1.HostPortConfig{
			Enabled: ptr.To(true),
			Port:    ptr.To(int32(8126)),
		}
		dst := base.DeepCopy()

		err := applyAPMOverlayForTest(dst, base, testProfileOverlayProfileAPMSpec(9126))
		require.Error(t, err)
		assert.Contains(t, err.Error(), `local Agent Service port "traceport" conflicts`)
	})
}

// Profile SSI settings should produce the same Cluster Agent config as DDA SSI settings.
// Each profile's node Agent config is rendered separately.
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
			require.NoError(t, applyAPMOverlayForTest(overlayDDA, overlayDDA.DeepCopy(), profile))

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

func TestAPMProfileSharedConfigOverlayProfileEnablesAPM(t *testing.T) {
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
			profile := &v2alpha1.DatadogAgentSpec{Features: &v2alpha1.DatadogFeatures{APM: tc.apm.DeepCopy()}}
			profile.Features.APM.HostPortConfig = &v2alpha1.HostPortConfig{Enabled: ptr.To(true), Port: ptr.To[int32](9126)}

			require.NoError(t, applyAPMProfileSharedConfigOverlay(validationSpec, defaultDDAISpec, base, profile))
			require.NotNil(t, validationSpec.Features.APM.HostPortConfig)
			assert.Equal(t, ptr.To[int32](9126), validationSpec.Features.APM.HostPortConfig.Port)
			require.NotNil(t, defaultDDAISpec.Features.APM.HostPortConfig)
			assert.Equal(t, ptr.To[int32](9126), defaultDDAISpec.Features.APM.HostPortConfig.Port)
			assert.Equal(t, ptr.To(false), defaultDDAISpec.Features.APM.Enabled)
			assert.Nil(t, profile.Features.APM.Enabled)

			conflicting := profile.DeepCopy()
			conflicting.Features.APM.HostPortConfig.Port = ptr.To[int32](10126)
			err := applyAPMProfileSharedConfigOverlay(validationSpec.DeepCopy(), defaultDDAISpec.DeepCopy(), base, conflicting)
			require.ErrorContains(t, err, "conflicts with existing port")
		})
	}
}
