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

// TestConfigFromInjector proves that every one of the 19 destination fields can be
// populated by the CRD alone. That is the only way the CRD path ever runs: the CRD and
// the annotations are mutually exclusive sources, so a CRD-configured DatadogAgent never
// inherits an annotation value for a field it leaves unset.
func TestConfigFromInjector(t *testing.T) {
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

	assert.Equal(t, want, configFromInjector(inj))
	assert.NoError(t, want.Validate())
}

// TestConfigFromInjectorDoesNotAliasSpecPointers proves the builder deref-and-copies
// AutoDetect instead of storing a pointer into the DatadogAgent spec.
func TestConfigFromInjectorDoesNotAliasSpecPointers(t *testing.T) {
	autoDetect := true
	inj := &v2alpha1.AppsecInjectorConfig{
		AutoDetect: &autoDetect,
		Proxies:    []string{"istio"},
		GKE:        &v2alpha1.AppsecInjectorGKEConfig{GatewayClasses: []string{"gke-l7-global-external-managed"}},
	}

	got := configFromInjector(inj)
	require.NotNil(t, got.AutoDetect)
	require.True(t, *got.AutoDetect)
	require.Len(t, got.Proxies, 1)
	require.Len(t, got.GatewayClasses, 1)

	assert.NotSame(t, inj.AutoDetect, got.AutoDetect, "config must not alias the spec pointer")
	assert.NotSame(t, &inj.Proxies[0], &got.Proxies[0], "config must not alias the spec slice")
	assert.NotSame(t, &inj.GKE.GatewayClasses[0], &got.GatewayClasses[0], "config must not alias the spec slice")

	// Mutating the spec target must not be visible through the built config.
	autoDetect = false
	inj.Proxies[0] = "ingress-nginx"
	inj.GKE.GatewayClasses[0] = "gke-l7-regional-external-managed"
	assert.True(t, *got.AutoDetect, "AutoDetect changed after mutating the spec value")
	assert.Equal(t, "istio", got.Proxies[0], "Proxies changed after mutating the spec slice")
	assert.Equal(t, "gke-l7-global-external-managed", got.GatewayClasses[0], "GatewayClasses changed after mutating the spec slice")
}

// TestParseAnnotationsDoesNotValidate proves parseAnnotations reports parse failures
// only, leaving semantic rejection to Validate.
func TestParseAnnotationsDoesNotValidate(t *testing.T) {
	ann := map[string]string{
		AnnotationInjectorEnabled:       "true",
		AnnotationInjectorMode:          "bogus",
		AnnotationInjectorProcessorPort: "70000",
	}

	cfg, err := parseAnnotations(ann)
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
				_, err := parseAnnotations(tt.ann)
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
