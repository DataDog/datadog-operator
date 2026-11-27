package otelcollector

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/otelcollector/defaultconfig"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/testutils"
)

func Test_otlpHTTPExporterName(t *testing.T) {
	tests := []struct {
		agentVersion string
		want         string
	}{
		{agentVersion: "7.67.0", want: defaultconfig.DeprecatedOTLPHTTPExporterName},
		{agentVersion: "7.75.5", want: defaultconfig.DeprecatedOTLPHTTPExporterName},
		{agentVersion: "7.75.0-full", want: defaultconfig.DeprecatedOTLPHTTPExporterName},
		{agentVersion: "7.76.0-rc.1", want: defaultconfig.OTLPHTTPExporterName},
		{agentVersion: "7.76.0", want: defaultconfig.OTLPHTTPExporterName},
		{agentVersion: "7.86.0", want: defaultconfig.OTLPHTTPExporterName},
		// Unparseable versions (e.g. custom images) are assumed recent.
		{agentVersion: "latest", want: defaultconfig.OTLPHTTPExporterName},
		{agentVersion: "", want: defaultconfig.OTLPHTTPExporterName},
	}
	for _, tt := range tests {
		t.Run(tt.agentVersion, func(t *testing.T) {
			assert.Equal(t, tt.want, otlpHTTPExporterName(tt.agentVersion))
		})
	}
}

type otelCollectorConfig struct {
	Exporters map[string]any `yaml:"exporters"`
	Service   struct {
		Pipelines map[string]struct {
			Exporters []string `yaml:"exporters"`
		} `yaml:"pipelines"`
	} `yaml:"service"`
}

func Test_defaultGatewayConfigOTLPHTTPExporterName(t *testing.T) {
	tests := []struct {
		name     string
		image    *v2alpha1.AgentImageConfig
		wantName string
	}{
		{name: "default agent image", wantName: defaultconfig.OTLPHTTPExporterName},
		{name: "agent tag 7.75.0", image: &v2alpha1.AgentImageConfig{Tag: "7.75.0"}, wantName: defaultconfig.DeprecatedOTLPHTTPExporterName},
		{name: "agent tag 7.76.0", image: &v2alpha1.AgentImageConfig{Tag: "7.76.0"}, wantName: defaultconfig.OTLPHTTPExporterName},
		{
			name:     "full image name with old tag",
			image:    &v2alpha1.AgentImageConfig{Name: "gcr.io/datadoghq/agent:7.75.0-full"},
			wantName: defaultconfig.DeprecatedOTLPHTTPExporterName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := testutils.NewInitializedDatadogAgentBuilder("ns", "dda").
				WithOTelCollectorEnabled(true).
				WithOTelAgentGatewayEnabled(true)
			if tt.image != nil {
				builder = builder.WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{Image: tt.image})
			}
			dda := builder.Build()

			f := buildOtelCollectorFeature(&feature.Options{})
			f.Configure(dda, &dda.Spec, nil)

			depsStore := store.NewStore(nil, &store.StoreOptions{})
			require.NoError(t, f.ManageDependencies(feature.NewResourceManagers(depsStore)))

			obj, found := depsStore.Get(kubernetes.ConfigMapKind, "ns", "dda-otel-config")
			require.True(t, found, "otel-config ConfigMap not found")
			raw := obj.(*corev1.ConfigMap).Data["otel-config.yaml"]

			var config otelCollectorConfig
			require.NoError(t, yaml.Unmarshal([]byte(raw), &config))

			otherName := defaultconfig.DeprecatedOTLPHTTPExporterName
			if tt.wantName == defaultconfig.DeprecatedOTLPHTTPExporterName {
				otherName = defaultconfig.OTLPHTTPExporterName
			}
			assert.Contains(t, config.Exporters, tt.wantName)
			assert.NotContains(t, config.Exporters, otherName)
			require.NotEmpty(t, config.Service.Pipelines)
			for name, pipeline := range config.Service.Pipelines {
				assert.Contains(t, pipeline.Exporters, tt.wantName, "pipeline %q", name)
				assert.NotContains(t, pipeline.Exporters, otherName, "pipeline %q", name)
			}
		})
	}
}
