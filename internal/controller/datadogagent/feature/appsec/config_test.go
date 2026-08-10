// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package appsec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// annotationBase returns a Config in which every annotation-backed field holds a
// distinct, non-zero "annotation-derived" value. Merge tests start from this base so
// that a single-field CRD overlay proves both that the CRD wins for that field and
// that no other field is disturbed.
//
// GatewayClasses is intentionally absent: it is CRD-only and has no annotation.
func annotationBase() Config {
	return Config{
		Enabled:                        true,
		AutoDetect:                     ptr.To(true),
		Proxies:                        []string{"istio"},
		ProcessorAddress:               "ann-addr",
		ProcessorPort:                  1111,
		ProcessorServiceName:           "ann-svc",
		ProcessorServiceNamespace:      "ann-ns",
		Mode:                           "sidecar",
		SidecarImage:                   "ann-image",
		SidecarImageTag:                "ann-tag",
		SidecarPort:                    "1000",
		SidecarHealthPort:              "1001",
		SidecarResourcesRequestsCPU:    "100m",
		SidecarResourcesRequestsMemory: "128Mi",
		SidecarResourcesLimitsCPU:      "200m",
		SidecarResourcesLimitsMemory:   "256Mi",
		SidecarBodyParsingSizeLimit:    "1024",
		NginxModuleMountPath:           "/ann-mount",
	}
}

// TestMergeInjectorConfig covers per-field CRD precedence, annotation fallback,
// nil-safety at every nesting level, and the pointer/slice "is set" asymmetry.
func TestMergeInjectorConfig(t *testing.T) {
	tests := []struct {
		name string
		inj  *v2alpha1.AppsecInjectorConfig
		// want mutates a copy of annotationBase() to describe the expected result.
		// A nil mutator means "base returned unchanged".
		want func(*Config)
	}{
		// --- A. Per-field CRD-wins: one row per destination Config field (19 total) ---
		{
			name: "1/19 Enabled: CRD wins over annotation",
			inj:  &v2alpha1.AppsecInjectorConfig{Enabled: ptr.To(false)},
			want: func(c *Config) { c.Enabled = false },
		},
		{
			name: "2/19 AutoDetect: CRD wins over annotation",
			inj:  &v2alpha1.AppsecInjectorConfig{AutoDetect: ptr.To(false)},
			want: func(c *Config) { c.AutoDetect = ptr.To(false) },
		},
		{
			name: "3/19 Proxies: CRD wins over annotation",
			inj:  &v2alpha1.AppsecInjectorConfig{Proxies: []string{"envoy-gateway", "ingress-nginx"}},
			want: func(c *Config) { c.Proxies = []string{"envoy-gateway", "ingress-nginx"} },
		},
		{
			name: "4/19 ProcessorAddress: CRD wins over annotation",
			inj: &v2alpha1.AppsecInjectorConfig{
				Processor: &v2alpha1.AppsecInjectorProcessorConfig{Address: ptr.To("crd-addr")},
			},
			want: func(c *Config) { c.ProcessorAddress = "crd-addr" },
		},
		{
			name: "5/19 ProcessorPort: CRD wins over annotation",
			inj: &v2alpha1.AppsecInjectorConfig{
				Processor: &v2alpha1.AppsecInjectorProcessorConfig{Port: ptr.To(int32(2222))},
			},
			want: func(c *Config) { c.ProcessorPort = 2222 },
		},
		{
			name: "6/19 ProcessorServiceName: CRD wins over annotation",
			inj: &v2alpha1.AppsecInjectorConfig{
				Processor: &v2alpha1.AppsecInjectorProcessorConfig{
					Service: &v2alpha1.AppsecInjectorProcessorServiceConfig{Name: ptr.To("crd-svc")},
				},
			},
			want: func(c *Config) { c.ProcessorServiceName = "crd-svc" },
		},
		{
			name: "7/19 ProcessorServiceNamespace: CRD wins over annotation",
			inj: &v2alpha1.AppsecInjectorConfig{
				Processor: &v2alpha1.AppsecInjectorProcessorConfig{
					Service: &v2alpha1.AppsecInjectorProcessorServiceConfig{Namespace: ptr.To("crd-ns")},
				},
			},
			want: func(c *Config) { c.ProcessorServiceNamespace = "crd-ns" },
		},
		{
			name: "8/19 Mode: CRD wins over annotation",
			inj:  &v2alpha1.AppsecInjectorConfig{Mode: ptr.To("external")},
			want: func(c *Config) { c.Mode = "external" },
		},
		{
			name: "9/19 SidecarImage: CRD wins over annotation",
			inj: &v2alpha1.AppsecInjectorConfig{
				Sidecar: &v2alpha1.AppsecInjectorSidecarConfig{Image: ptr.To("crd-image")},
			},
			want: func(c *Config) { c.SidecarImage = "crd-image" },
		},
		{
			name: "10/19 SidecarImageTag: CRD wins over annotation",
			inj: &v2alpha1.AppsecInjectorConfig{
				Sidecar: &v2alpha1.AppsecInjectorSidecarConfig{ImageTag: ptr.To("crd-tag")},
			},
			want: func(c *Config) { c.SidecarImageTag = "crd-tag" },
		},
		{
			name: "11/19 SidecarPort: CRD wins over annotation",
			inj: &v2alpha1.AppsecInjectorConfig{
				Sidecar: &v2alpha1.AppsecInjectorSidecarConfig{Port: ptr.To(int32(2000))},
			},
			want: func(c *Config) { c.SidecarPort = "2000" },
		},
		{
			name: "12/19 SidecarHealthPort: CRD wins over annotation",
			inj: &v2alpha1.AppsecInjectorConfig{
				Sidecar: &v2alpha1.AppsecInjectorSidecarConfig{HealthPort: ptr.To(int32(2001))},
			},
			want: func(c *Config) { c.SidecarHealthPort = "2001" },
		},
		{
			name: "13/19 SidecarResourcesRequestsCPU: CRD wins over annotation",
			inj: &v2alpha1.AppsecInjectorConfig{
				Sidecar: &v2alpha1.AppsecInjectorSidecarConfig{
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("250m")},
					},
				},
			},
			want: func(c *Config) { c.SidecarResourcesRequestsCPU = "250m" },
		},
		{
			name: "14/19 SidecarResourcesRequestsMemory: CRD wins over annotation",
			inj: &v2alpha1.AppsecInjectorConfig{
				Sidecar: &v2alpha1.AppsecInjectorSidecarConfig{
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("512Mi")},
					},
				},
			},
			want: func(c *Config) { c.SidecarResourcesRequestsMemory = "512Mi" },
		},
		{
			name: "15/19 SidecarResourcesLimitsCPU: CRD wins over annotation",
			inj: &v2alpha1.AppsecInjectorConfig{
				Sidecar: &v2alpha1.AppsecInjectorSidecarConfig{
					Resources: &corev1.ResourceRequirements{
						Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("750m")},
					},
				},
			},
			want: func(c *Config) { c.SidecarResourcesLimitsCPU = "750m" },
		},
		{
			name: "16/19 SidecarResourcesLimitsMemory: CRD wins over annotation",
			inj: &v2alpha1.AppsecInjectorConfig{
				Sidecar: &v2alpha1.AppsecInjectorSidecarConfig{
					Resources: &corev1.ResourceRequirements{
						Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1Gi")},
					},
				},
			},
			want: func(c *Config) { c.SidecarResourcesLimitsMemory = "1Gi" },
		},
		{
			name: "17/19 SidecarBodyParsingSizeLimit: CRD wins over annotation",
			inj: &v2alpha1.AppsecInjectorConfig{
				Sidecar: &v2alpha1.AppsecInjectorSidecarConfig{BodyParsingSizeLimit: ptr.To(int64(4096))},
			},
			want: func(c *Config) { c.SidecarBodyParsingSizeLimit = "4096" },
		},
		{
			name: "18/19 NginxModuleMountPath: CRD wins over annotation",
			inj: &v2alpha1.AppsecInjectorConfig{
				Nginx: &v2alpha1.AppsecInjectorNginxConfig{ModuleMountPath: ptr.To("/crd-mount")},
			},
			want: func(c *Config) { c.NginxModuleMountPath = "/crd-mount" },
		},
		{
			name: "19/19 GatewayClasses: CRD-only value lands",
			inj: &v2alpha1.AppsecInjectorConfig{
				GKE: &v2alpha1.AppsecInjectorGKEConfig{GatewayClasses: []string{"gke-l7-global-external-managed"}},
			},
			want: func(c *Config) { c.GatewayClasses = []string{"gke-l7-global-external-managed"} },
		},

		// --- C. nil injector ---
		{
			name: "C nil injector returns base unchanged",
			inj:  nil,
			want: nil,
		},

		// --- B + D. Annotation fallback and nested-nil safety ---
		{
			name: "B empty injector: every annotation value survives",
			inj:  &v2alpha1.AppsecInjectorConfig{},
			want: nil,
		},
		{
			name: "D nil Processor, Sidecar, Nginx and GKE: no panic, base unchanged",
			inj: &v2alpha1.AppsecInjectorConfig{
				Processor: nil,
				Sidecar:   nil,
				Nginx:     nil,
				GKE:       nil,
			},
			want: nil,
		},
		{
			name: "D empty nested objects: no panic, base unchanged",
			inj: &v2alpha1.AppsecInjectorConfig{
				Processor: &v2alpha1.AppsecInjectorProcessorConfig{},
				Sidecar:   &v2alpha1.AppsecInjectorSidecarConfig{},
				Nginx:     &v2alpha1.AppsecInjectorNginxConfig{},
				GKE:       &v2alpha1.AppsecInjectorGKEConfig{},
			},
			want: nil,
		},
		{
			name: "D non-nil Processor with nil Service: address and port still applied",
			inj: &v2alpha1.AppsecInjectorConfig{
				Processor: &v2alpha1.AppsecInjectorProcessorConfig{
					Address: ptr.To("crd-addr"),
					Port:    ptr.To(int32(8443)),
					Service: nil,
				},
			},
			want: func(c *Config) {
				c.ProcessorAddress = "crd-addr"
				c.ProcessorPort = 8443
			},
		},
		{
			name: "D non-nil Processor with empty Service: no panic, service fields fall back",
			inj: &v2alpha1.AppsecInjectorConfig{
				Processor: &v2alpha1.AppsecInjectorProcessorConfig{
					Service: &v2alpha1.AppsecInjectorProcessorServiceConfig{},
				},
			},
			want: nil,
		},
		{
			name: "D non-nil Sidecar with nil Resources: image still applied",
			inj: &v2alpha1.AppsecInjectorConfig{
				Sidecar: &v2alpha1.AppsecInjectorSidecarConfig{
					Image:     ptr.To("crd-image"),
					Resources: nil,
				},
			},
			want: func(c *Config) { c.SidecarImage = "crd-image" },
		},
		{
			name: "D non-nil Sidecar with empty Resources: no panic, resource fields fall back",
			inj: &v2alpha1.AppsecInjectorConfig{
				Sidecar: &v2alpha1.AppsecInjectorSidecarConfig{
					Resources: &corev1.ResourceRequirements{},
				},
			},
			want: nil,
		},
		{
			name: "D non-nil Nginx with nil ModuleMountPath: falls back to annotation",
			inj: &v2alpha1.AppsecInjectorConfig{
				Nginx: &v2alpha1.AppsecInjectorNginxConfig{ModuleMountPath: nil},
			},
			want: nil,
		},
		{
			name: "D non-nil GKE with nil GatewayClasses: no panic, stays empty",
			inj: &v2alpha1.AppsecInjectorConfig{
				GKE: &v2alpha1.AppsecInjectorGKEConfig{GatewayClasses: nil},
			},
			want: nil,
		},

		// --- E. Non-nil pointer to the empty string counts as SET ---
		{
			name: "E Mode set to empty string blanks the annotation value",
			inj:  &v2alpha1.AppsecInjectorConfig{Mode: ptr.To("")},
			want: func(c *Config) { c.Mode = "" },
		},
		{
			name: "E ProcessorAddress set to empty string blanks the annotation value",
			inj: &v2alpha1.AppsecInjectorConfig{
				Processor: &v2alpha1.AppsecInjectorProcessorConfig{Address: ptr.To("")},
			},
			want: func(c *Config) { c.ProcessorAddress = "" },
		},

		// --- F. Empty slice does NOT clear the base (asymmetric with pointers) ---
		{
			name: "F empty Proxies slice does not clear the annotation value",
			inj:  &v2alpha1.AppsecInjectorConfig{Proxies: []string{}},
			want: nil,
		},
		{
			name: "F nil Proxies slice does not clear the annotation value",
			inj:  &v2alpha1.AppsecInjectorConfig{Proxies: nil},
			want: nil,
		},
		{
			name: "F empty GatewayClasses slice leaves the field empty",
			inj: &v2alpha1.AppsecInjectorConfig{
				GKE: &v2alpha1.AppsecInjectorGKEConfig{GatewayClasses: []string{}},
			},
			want: nil,
		},

		// --- G. Partial resources: only the entries actually present are overridden ---
		{
			name: "G only requests.cpu present: the other three annotation values survive",
			inj: &v2alpha1.AppsecInjectorConfig{
				Sidecar: &v2alpha1.AppsecInjectorSidecarConfig{
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("333m")},
					},
				},
			},
			want: func(c *Config) { c.SidecarResourcesRequestsCPU = "333m" },
		},
		{
			name: "G empty Requests and Limits maps: all four annotation values survive",
			inj: &v2alpha1.AppsecInjectorConfig{
				Sidecar: &v2alpha1.AppsecInjectorSidecarConfig{
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{},
						Limits:   corev1.ResourceList{},
					},
				},
			},
			want: nil,
		},

		// --- H. Claims and non-cpu/memory resource names are ignored ---
		{
			name: "H Claims entry is ignored",
			inj: &v2alpha1.AppsecInjectorConfig{
				Sidecar: &v2alpha1.AppsecInjectorSidecarConfig{
					Resources: &corev1.ResourceRequirements{
						Claims: []corev1.ResourceClaim{{Name: "example-claim"}},
					},
				},
			},
			want: nil,
		},
		{
			name: "H non cpu/memory resource names are ignored",
			inj: &v2alpha1.AppsecInjectorConfig{
				Sidecar: &v2alpha1.AppsecInjectorSidecarConfig{
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceEphemeralStorage: resource.MustParse("2Gi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceEphemeralStorage: resource.MustParse("4Gi"),
						},
						Claims: []corev1.ResourceClaim{{Name: "example-claim"}},
					},
				},
			},
			want: nil,
		},
		{
			name: "H cpu is taken while a sibling non cpu/memory name is dropped",
			inj: &v2alpha1.AppsecInjectorConfig{
				Sidecar: &v2alpha1.AppsecInjectorSidecarConfig{
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:              resource.MustParse("125m"),
							corev1.ResourceEphemeralStorage: resource.MustParse("2Gi"),
						},
					},
				},
			},
			want: func(c *Config) { c.SidecarResourcesRequestsCPU = "125m" },
		},

		// --- Manual QA scenarios recorded in /tmp/appsec_w4_merge.txt ---
		{
			name: "QA happy: base sidecar mode overridden by CRD external mode",
			inj:  &v2alpha1.AppsecInjectorConfig{Mode: ptr.To("external")},
			want: func(c *Config) { c.Mode = "external" },
		},
		{
			name: "QA failure: base sidecar mode preserved when CRD mode is nil",
			inj:  &v2alpha1.AppsecInjectorConfig{Mode: nil},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := annotationBase()
			want := annotationBase()
			if tt.want != nil {
				tt.want(&want)
			}

			var got Config
			require.NotPanics(t, func() {
				got = mergeInjectorConfig(base, tt.inj)
			})

			assert.Equal(t, want, got)
		})
	}
}

// TestMergeInjectorConfigFullOverlayFromZeroBase proves that every one of the 19
// destination fields can be populated by the CRD alone, with no annotation present.
func TestMergeInjectorConfigFullOverlayFromZeroBase(t *testing.T) {
	inj := &v2alpha1.AppsecInjectorConfig{
		Enabled:    ptr.To(true),
		AutoDetect: ptr.To(false),
		Proxies:    []string{"gke-gateway"},
		Mode:       ptr.To("external"),
		Processor: &v2alpha1.AppsecInjectorProcessorConfig{
			Address: ptr.To("processor.example.com"),
			Port:    ptr.To(int32(8443)),
			Service: &v2alpha1.AppsecInjectorProcessorServiceConfig{
				Name:      ptr.To("appsec-processor"),
				Namespace: ptr.To("datadog"),
			},
		},
		Sidecar: &v2alpha1.AppsecInjectorSidecarConfig{
			Image:                ptr.To("ghcr.io/datadog/callout"),
			ImageTag:             ptr.To("v2.8.2"),
			Port:                 ptr.To(int32(8080)),
			HealthPort:           ptr.To(int32(8081)),
			BodyParsingSizeLimit: ptr.To(int64(1048576)),
			Resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("512Mi"),
				},
			},
		},
		Nginx: &v2alpha1.AppsecInjectorNginxConfig{ModuleMountPath: ptr.To("/modules_mount")},
		GKE: &v2alpha1.AppsecInjectorGKEConfig{
			GatewayClasses: []string{"gke-l7-global-external-managed", "gke-l7-regional-external-managed"},
		},
	}

	want := Config{
		Enabled:                        true,
		AutoDetect:                     ptr.To(false),
		Proxies:                        []string{"gke-gateway"},
		ProcessorAddress:               "processor.example.com",
		ProcessorPort:                  8443,
		ProcessorServiceName:           "appsec-processor",
		ProcessorServiceNamespace:      "datadog",
		Mode:                           "external",
		SidecarImage:                   "ghcr.io/datadog/callout",
		SidecarImageTag:                "v2.8.2",
		SidecarPort:                    "8080",
		SidecarHealthPort:              "8081",
		SidecarResourcesRequestsCPU:    "100m",
		SidecarResourcesRequestsMemory: "128Mi",
		SidecarResourcesLimitsCPU:      "500m",
		SidecarResourcesLimitsMemory:   "512Mi",
		SidecarBodyParsingSizeLimit:    "1048576",
		NginxModuleMountPath:           "/modules_mount",
		GatewayClasses:                 []string{"gke-l7-global-external-managed", "gke-l7-regional-external-managed"},
	}

	assert.Equal(t, want, mergeInjectorConfig(Config{}, inj))
	assert.NoError(t, want.Validate())
}

// TestMergeInjectorConfigDoesNotAliasSpecPointers proves the merge deref-and-copies
// AutoDetect instead of storing a pointer into the DatadogAgent spec.
func TestMergeInjectorConfigDoesNotAliasSpecPointers(t *testing.T) {
	autoDetect := true
	inj := &v2alpha1.AppsecInjectorConfig{AutoDetect: &autoDetect}

	got := mergeInjectorConfig(Config{}, inj)
	require.NotNil(t, got.AutoDetect)
	require.True(t, *got.AutoDetect)

	assert.NotSame(t, inj.AutoDetect, got.AutoDetect, "merged config must not alias the spec pointer")

	// Mutating the spec target must not be visible through the merged config.
	autoDetect = false
	assert.True(t, *got.AutoDetect, "merged AutoDetect changed after mutating the spec value")
}

// TestParseAnnotationsSkip covers the CRD-aware parse skip. A malformed annotation on
// a field the CRD sets must NOT error (the CRD wins even over garbage), while the same
// malformed annotation on a field the CRD leaves unset must still error.
func TestParseAnnotationsSkip(t *testing.T) {
	tests := []struct {
		name    string
		ann     map[string]string
		inj     *v2alpha1.AppsecInjectorConfig
		wantErr bool
		// wantCfg asserts on the parsed (pre-merge) config when no error is expected.
		wantCfg *Config
	}{
		// --- I. Parse skip: malformed annotation + CRD sets the same field -> no error ---
		{
			name:    "I enabled: malformed annotation skipped because CRD sets Enabled",
			ann:     map[string]string{AnnotationInjectorEnabled: "not-a-bool"},
			inj:     &v2alpha1.AppsecInjectorConfig{Enabled: ptr.To(true)},
			wantErr: false,
			wantCfg: &Config{},
		},
		{
			name:    "I autoDetect: malformed annotation skipped because CRD sets AutoDetect",
			ann:     map[string]string{AnnotationInjectorAutoDetect: "not-a-bool"},
			inj:     &v2alpha1.AppsecInjectorConfig{AutoDetect: ptr.To(false)},
			wantErr: false,
			wantCfg: &Config{},
		},
		{
			name:    "I proxies: malformed annotation skipped because CRD sets Proxies",
			ann:     map[string]string{AnnotationInjectorProxies: "not-json"},
			inj:     &v2alpha1.AppsecInjectorConfig{Proxies: []string{"istio"}},
			wantErr: false,
			wantCfg: &Config{},
		},
		{
			name: "I processorPort: malformed annotation skipped because CRD sets Processor.Port",
			ann:  map[string]string{AnnotationInjectorProcessorPort: "not-a-number"},
			inj: &v2alpha1.AppsecInjectorConfig{
				Processor: &v2alpha1.AppsecInjectorProcessorConfig{Port: ptr.To(int32(8443))},
			},
			wantErr: false,
			wantCfg: &Config{},
		},
		{
			name: "I all four malformed at once, all four set by the CRD",
			ann: map[string]string{
				AnnotationInjectorEnabled:       "not-a-bool",
				AnnotationInjectorAutoDetect:    "not-a-bool",
				AnnotationInjectorProxies:       "not-json",
				AnnotationInjectorProcessorPort: "not-a-number",
			},
			inj: &v2alpha1.AppsecInjectorConfig{
				Enabled:    ptr.To(true),
				AutoDetect: ptr.To(true),
				Proxies:    []string{"istio"},
				Processor:  &v2alpha1.AppsecInjectorProcessorConfig{Port: ptr.To(int32(8443))},
			},
			wantErr: false,
			wantCfg: &Config{},
		},

		// --- J. Skip mirror: non-nil injector that does NOT set the field -> still errors ---
		{
			name:    "J enabled: injector present but Enabled nil",
			ann:     map[string]string{AnnotationInjectorEnabled: "not-a-bool"},
			inj:     &v2alpha1.AppsecInjectorConfig{Enabled: nil},
			wantErr: true,
		},
		{
			name:    "J enabled: injector sets an unrelated field only",
			ann:     map[string]string{AnnotationInjectorEnabled: "not-a-bool"},
			inj:     &v2alpha1.AppsecInjectorConfig{Mode: ptr.To("external")},
			wantErr: true,
		},
		{
			name:    "J autoDetect: injector present but AutoDetect nil",
			ann:     map[string]string{AnnotationInjectorAutoDetect: "not-a-bool"},
			inj:     &v2alpha1.AppsecInjectorConfig{AutoDetect: nil},
			wantErr: true,
		},
		{
			name:    "J proxies: injector present but Proxies nil",
			ann:     map[string]string{AnnotationInjectorProxies: "not-json"},
			inj:     &v2alpha1.AppsecInjectorConfig{Proxies: nil},
			wantErr: true,
		},
		{
			name:    "J proxies: injector present with an empty Proxies slice",
			ann:     map[string]string{AnnotationInjectorProxies: "not-json"},
			inj:     &v2alpha1.AppsecInjectorConfig{Proxies: []string{}},
			wantErr: true,
		},
		{
			name: "J processorPort: injector present with Processor set but Port nil",
			ann:  map[string]string{AnnotationInjectorProcessorPort: "not-a-number"},
			inj: &v2alpha1.AppsecInjectorConfig{
				Processor: &v2alpha1.AppsecInjectorProcessorConfig{Port: nil},
			},
			wantErr: true,
		},
		{
			name:    "J processorPort: injector present but Processor nil",
			ann:     map[string]string{AnnotationInjectorProcessorPort: "not-a-number"},
			inj:     &v2alpha1.AppsecInjectorConfig{Processor: nil},
			wantErr: true,
		},

		// --- Annotation-only strictness is unchanged when there is no injector ---
		{
			name:    "nil injector: malformed enabled still errors",
			ann:     map[string]string{AnnotationInjectorEnabled: "not-a-bool"},
			inj:     nil,
			wantErr: true,
		},
		{
			name:    "nil injector: malformed autoDetect still errors",
			ann:     map[string]string{AnnotationInjectorAutoDetect: "not-a-bool"},
			inj:     nil,
			wantErr: true,
		},
		{
			name:    "nil injector: malformed proxies still errors",
			ann:     map[string]string{AnnotationInjectorProxies: "not-json"},
			inj:     nil,
			wantErr: true,
		},
		{
			name:    "nil injector: malformed processorPort still errors",
			ann:     map[string]string{AnnotationInjectorProcessorPort: "not-a-number"},
			inj:     nil,
			wantErr: true,
		},

		// --- Well-formed annotations are still parsed when the CRD leaves them unset ---
		{
			name: "annotation fallback: well-formed values parsed with an empty injector",
			ann: map[string]string{
				AnnotationInjectorEnabled:       "true",
				AnnotationInjectorAutoDetect:    "true",
				AnnotationInjectorProxies:       `["istio"]`,
				AnnotationInjectorProcessorPort: "1234",
			},
			inj:     &v2alpha1.AppsecInjectorConfig{},
			wantErr: false,
			wantCfg: &Config{
				Enabled:       true,
				AutoDetect:    ptr.To(true),
				Proxies:       []string{"istio"},
				ProcessorPort: 1234,
			},
		},
		{
			name: "parse skip leaves the field at its zero value for the merge to fill",
			ann: map[string]string{
				AnnotationInjectorEnabled:       "true",
				AnnotationInjectorProcessorPort: "1234",
			},
			inj: &v2alpha1.AppsecInjectorConfig{
				Enabled: ptr.To(false),
			},
			wantErr: false,
			wantCfg: &Config{
				// Enabled was skipped, so it stays false here and is filled by the merge.
				ProcessorPort: 1234,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := parseAnnotations(tt.ann, tt.inj)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			if tt.wantCfg != nil {
				assert.Equal(t, *tt.wantCfg, cfg)
			}
		})
	}
}

// TestParseAnnotationsDoesNotValidate proves parseAnnotations reports parse failures
// only. Validate-time failures must survive so the merge can override them.
func TestParseAnnotationsDoesNotValidate(t *testing.T) {
	ann := map[string]string{
		AnnotationInjectorEnabled:       "true",
		AnnotationInjectorMode:          "bogus",
		AnnotationInjectorProcessorPort: "70000",
	}

	cfg, err := parseAnnotations(ann, nil)
	require.NoError(t, err, "parseAnnotations must not run Validate")
	assert.Equal(t, "bogus", cfg.Mode)
	assert.Equal(t, 70000, cfg.ProcessorPort)
	assert.Error(t, cfg.Validate(), "the same config must be rejected by Validate")
}

// TestParseAnnotationsFirstErrorIsDeterministic pins that the returned error is the
// first failure in source order, never a random pick from map iteration.
func TestParseAnnotationsFirstErrorIsDeterministic(t *testing.T) {
	tests := []struct {
		name          string
		ann           map[string]string
		wantAnnotName string
	}{
		{
			name: "all four malformed: enabled reported",
			ann: map[string]string{
				AnnotationInjectorEnabled:       "not-a-bool",
				AnnotationInjectorAutoDetect:    "not-a-bool",
				AnnotationInjectorProxies:       "not-json",
				AnnotationInjectorProcessorPort: "not-a-number",
			},
			wantAnnotName: AnnotationInjectorEnabled,
		},
		{
			name: "autoDetect, proxies and port malformed: autoDetect reported",
			ann: map[string]string{
				AnnotationInjectorAutoDetect:    "not-a-bool",
				AnnotationInjectorProxies:       "not-json",
				AnnotationInjectorProcessorPort: "not-a-number",
			},
			wantAnnotName: AnnotationInjectorAutoDetect,
		},
		{
			name: "proxies and port malformed: proxies reported",
			ann: map[string]string{
				AnnotationInjectorProxies:       "not-json",
				AnnotationInjectorProcessorPort: "not-a-number",
			},
			wantAnnotName: AnnotationInjectorProxies,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seen := map[string]struct{}{}
			for range 200 {
				_, err := parseAnnotations(tt.ann, nil)
				require.Error(t, err)
				seen[err.Error()] = struct{}{}
			}

			require.Len(t, seen, 1, "parseAnnotations returned a non-deterministic error: %v", seen)
			for msg := range seen {
				assert.Contains(t, msg, tt.wantAnnotName)
			}
		})
	}
}

// TestParseAnnotationsThenMergeThenValidate pins the production ordering: the CRD
// overlay is applied BEFORE Validate, so a CRD value can rescue an annotation that
// would otherwise fail validation.
func TestParseAnnotationsThenMergeThenValidate(t *testing.T) {
	ann := map[string]string{
		AnnotationInjectorEnabled: "true",
		AnnotationInjectorMode:    "bogus",
	}

	// Annotations alone are rejected: "bogus" is not a valid mode.
	annotationOnlyCfg, annotationOnlyErr := parseAnnotations(ann, nil)
	require.NoError(t, annotationOnlyErr, "an invalid mode is a validation failure, not a parse failure")
	require.Error(t, annotationOnlyCfg.Validate(), "annotation-only path must still reject an invalid mode")

	inj := &v2alpha1.AppsecInjectorConfig{
		Enabled: ptr.To(true),
		Mode:    ptr.To("external"),
		Processor: &v2alpha1.AppsecInjectorProcessorConfig{
			Service: &v2alpha1.AppsecInjectorProcessorServiceConfig{Name: ptr.To("appsec-processor")},
		},
	}

	cfg, err := parseAnnotations(ann, inj)
	require.NoError(t, err)

	merged := mergeInjectorConfig(cfg, inj)
	assert.Equal(t, "external", merged.Mode)
	assert.Equal(t, "appsec-processor", merged.ProcessorServiceName)
	assert.True(t, merged.Enabled)

	assert.NoError(t, merged.Validate(), "the CRD overlay must be applied before Validate")
}

// TestConfigValidateGKEGateway covers the gke-gateway external-mode requirement.
// Listing gke-gateway in proxies forces mode "external", including when mode is unset
// because the empty value means sidecar. Setting gatewayClasses alone does NOT force
// external mode: the agent may discover GKE Gateways through autoDetect.
//
// The rejection message must stay source-neutral. Configuration now reaches Validate
// from the CRD as well as from annotations, so the error names the proxies and mode
// fields instead of an annotation key.
func TestConfigValidateGKEGateway(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name:    "gke-gateway proxy with sidecar mode is rejected",
			config:  Config{Proxies: []string{"gke-gateway"}, Mode: "sidecar"},
			wantErr: true,
		},
		{
			name:    "gke-gateway proxy with unset mode is rejected",
			config:  Config{Proxies: []string{"gke-gateway"}, Mode: ""},
			wantErr: true,
		},
		{
			name: "gke-gateway proxy with external mode is accepted",
			config: Config{
				Enabled:              true,
				Proxies:              []string{"gke-gateway"},
				Mode:                 "external",
				ProcessorServiceName: "x",
			},
			wantErr: false,
		},
		{
			name: "gatewayClasses without the gke-gateway proxy does not require external mode",
			config: Config{
				Enabled:        true,
				GatewayClasses: []string{"gke-l7-global-external-managed"},
				Mode:           "sidecar",
			},
			wantErr: false,
		},
		{
			name: "gke-gateway proxy alongside gatewayClasses still requires external mode",
			config: Config{
				Enabled:        true,
				GatewayClasses: []string{"gke-l7-global-external-managed"},
				Proxies:        []string{"gke-gateway"},
				Mode:           "sidecar",
			},
			wantErr: true,
		},
		{
			name:    "gke-gateway next to another proxy in sidecar mode is rejected",
			config:  Config{Proxies: []string{"istio", "gke-gateway"}, Mode: "sidecar"},
			wantErr: true,
		},
		{
			name:    "other proxies alone are unaffected by the gke-gateway rule",
			config:  Config{Proxies: []string{"istio", "ingress-nginx"}, Mode: "sidecar"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if !tt.wantErr {
				assert.NoError(t, err)
				return
			}

			require.Error(t, err)
			assert.Contains(t, err.Error(), "external", "the error must name the required mode")
			assert.Contains(t, err.Error(), "proxies", "the error must name the proxies field")
			assert.NotContains(t, err.Error(), "agent.datadoghq.com",
				"the error must be source-neutral and never reference an annotation key")
		})
	}
}

// TestRequiresGKESupport pins the truth table the GKE cluster-agent version gate and
// the DD_APPSEC_PROXY_GKE_GATEWAY_CLASSES emission both branch on. Either trigger is
// sufficient on its own, and an empty or nil gatewayClasses list is not a trigger.
func TestRequiresGKESupport(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		want   bool
	}{
		{
			name:   "gke-gateway proxy only",
			config: Config{Proxies: []string{"gke-gateway"}, Mode: "external"},
			want:   true,
		},
		{
			name:   "gatewayClasses only",
			config: Config{GatewayClasses: []string{"gke-l7-global-external-managed"}},
			want:   true,
		},
		{
			name: "both the gke-gateway proxy and gatewayClasses",
			config: Config{
				Proxies:        []string{"gke-gateway"},
				GatewayClasses: []string{"gke-l7-global-external-managed"},
				Mode:           "external",
			},
			want: true,
		},
		{
			name:   "gke-gateway among several proxies",
			config: Config{Proxies: []string{"istio", "gke-gateway"}, Mode: "external"},
			want:   true,
		},
		{
			name:   "neither: zero config",
			config: Config{},
			want:   false,
		},
		{
			name:   "neither: other proxies only",
			config: Config{Proxies: []string{"istio", "ingress-nginx"}},
			want:   false,
		},
		{
			name:   "empty GatewayClasses slice is not a trigger",
			config: Config{GatewayClasses: []string{}},
			want:   false,
		},
		{
			name:   "nil GatewayClasses slice is not a trigger",
			config: Config{GatewayClasses: nil},
			want:   false,
		},
		{
			name:   "empty GatewayClasses slice next to a non-GKE proxy is not a trigger",
			config: Config{Proxies: []string{"istio"}, GatewayClasses: []string{}},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.config.requiresGKESupport())
		})
	}
}

// TestGKEGatewayQAScenarios pins the two manual QA scenarios recorded in
// /tmp/appsec_w4_validate.txt. The happy scenario is the deliberate asymmetry:
// gatewayClasses alone requires GKE support without requiring external mode.
func TestGKEGatewayQAScenarios(t *testing.T) {
	t.Run("QA happy: gatewayClasses alone is legal in sidecar mode and still requires GKE support", func(t *testing.T) {
		cfg := Config{
			Enabled:        true,
			GatewayClasses: []string{"gke-l7-global-external-managed"},
			Mode:           "sidecar",
		}

		assert.NoError(t, cfg.Validate(), "gatewayClasses alone must not require external mode")
		assert.True(t, cfg.requiresGKESupport(), "gatewayClasses alone must require GKE support")
	})

	t.Run("QA failure: gke-gateway proxy in sidecar mode is rejected", func(t *testing.T) {
		cfg := Config{Proxies: []string{"gke-gateway"}, Mode: "sidecar"}

		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "external")
		t.Logf("rejection message: %s", err.Error())
	})
}
