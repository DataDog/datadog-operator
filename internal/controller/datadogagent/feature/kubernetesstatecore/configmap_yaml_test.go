// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetesstatecore

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestKsmCheckConfigYAMLFormat(t *testing.T) {
	testCases := []struct {
		name          string
		clusterCheck  bool
		collectorOpts collectorOptions
		validateFunc  func(t *testing.T, output string)
	}{
		{
			name:         "custom resources with proper indentation",
			clusterCheck: true,
			collectorOpts: collectorOptions{
				customResources: []v2alpha1.Resource{
					{
						GroupVersionKind: v2alpha1.GroupVersionKind{
							Group:   "argoproj.io",
							Version: "v1alpha1",
							Kind:    "Application",
						},
					},
				},
			},
			validateFunc: func(t *testing.T, output string) {
				// Check that the YAML is valid
				var config map[string]any
				err := yaml.Unmarshal([]byte(output), &config)
				require.NoError(t, err, "YAML should be valid")

				// Check structure
				instances, ok := config["instances"].([]any)
				require.True(t, ok, "instances should be a list")
				require.Len(t, instances, 1, "should have one instance")

				instance, ok := instances[0].(map[string]any)
				require.True(t, ok, "instance should be a map")

				customResource, ok := instance["custom_resource"].(map[string]any)
				require.True(t, ok, "custom_resource should exist")

				spec, ok := customResource["spec"].(map[string]any)
				require.True(t, ok, "spec should exist")

				resources, ok := spec["resources"].([]any)
				require.True(t, ok, "resources should be a list")
				require.Len(t, resources, 1, "should have one resource")

				// Check indentation visually
				lines := strings.SplitSeq(output, "\n")
				for line := range lines {
					if strings.Contains(line, "custom_resource:") {
						assert.True(t, strings.HasPrefix(line, "    "), "custom_resource should be indented with 4 spaces")
					}
					if strings.Contains(line, "spec:") && strings.Contains(output[:strings.Index(output, line)], "custom_resource") {
						assert.True(t, strings.HasPrefix(line, "      "), "spec should be indented with 6 spaces")
					}
					if strings.Contains(line, "resources:") && strings.Contains(output[:strings.Index(output, line)], "spec:") {
						assert.True(t, strings.HasPrefix(line, "        "), "resources should be indented with 8 spaces")
					}
					if strings.Contains(line, "- metricNamePrefix:") || strings.Contains(line, "- groupVersionKind:") {
						assert.True(t, strings.HasPrefix(line, "          "), "resource items should be indented with 10 spaces")
					}
				}
			},
		},
		{
			name:         "multiple custom resources",
			clusterCheck: true,
			collectorOpts: collectorOptions{
				customResources: []v2alpha1.Resource{
					{
						GroupVersionKind: v2alpha1.GroupVersionKind{
							Group:   "tekton.dev",
							Version: "v1beta1",
							Kind:    "Pipeline",
						},
					},
					{
						GroupVersionKind: v2alpha1.GroupVersionKind{
							Group:   "keda.sh",
							Version: "v1alpha1",
							Kind:    "ScaledObject",
						},
					},
				},
			},
			validateFunc: func(t *testing.T, output string) {
				// Check that the YAML is valid
				var config map[string]any
				err := yaml.Unmarshal([]byte(output), &config)
				require.NoError(t, err, "YAML should be valid")

				// Navigate to resources
				instances := config["instances"].([]any)
				instance := instances[0].(map[string]any)
				customResource := instance["custom_resource"].(map[string]any)
				spec := customResource["spec"].(map[string]any)
				resources := spec["resources"].([]any)

				assert.Len(t, resources, 2, "should have two resources")

				// Check both resources have the correct fields
				for i, res := range resources {
					resource := res.(map[string]any)
					gvk, ok := resource["groupVersionKind"].(map[string]any)
					require.True(t, ok, "resource %d should have groupVersionKind", i)
					assert.NotNil(t, gvk["group"], "resource %d should have group", i)
					assert.NotNil(t, gvk["version"], "resource %d should have version", i)
					assert.NotNil(t, gvk["kind"], "resource %d should have kind", i)
				}
			},
		},
		{
			name:         "combined with VPA",
			clusterCheck: true,
			collectorOpts: collectorOptions{
				enableVPA: true,
				customResources: []v2alpha1.Resource{
					{
						GroupVersionKind: v2alpha1.GroupVersionKind{
							Group:   "fluxcd.io",
							Version: "v2beta1",
							Kind:    "HelmRelease",
						},
					},
				},
			},
			validateFunc: func(t *testing.T, output string) {
				// Check that both VPA and custom resources are present
				assert.Contains(t, output, "- verticalpodautoscalers", "should contain VPA collector")
				assert.Contains(t, output, "custom_resource:", "should contain custom_resource section")

				// Verify YAML is still valid
				var config map[string]any
				err := yaml.Unmarshal([]byte(output), &config)
				require.NoError(t, err, "YAML should be valid with both VPA and custom resources")
			},
		},
		{
			name:         "empty custom resources does not add section",
			clusterCheck: true,
			collectorOpts: collectorOptions{
				customResources: nil,
			},
			validateFunc: func(t *testing.T, output string) {
				assert.NotContains(t, output, "custom_resource:", "should not contain custom_resource section when nil")
			},
		},
		{
			name:         "tags are emitted as a YAML list",
			clusterCheck: false,
			collectorOpts: collectorOptions{
				tags: []string{"env:prod", "team:cont-p"},
			},
			validateFunc: func(t *testing.T, output string) {
				var config map[string]any
				require.NoError(t, yaml.Unmarshal([]byte(output), &config), "YAML should be valid")
				instances := config["instances"].([]any)
				require.Len(t, instances, 1)
				instance := instances[0].(map[string]any)
				tags, ok := instance["tags"].([]any)
				require.True(t, ok, "tags should exist as a list")
				assert.Equal(t, []any{"env:prod", "team:cont-p"}, tags)
			},
		},
		{
			name:          "tags omitted when empty",
			clusterCheck:  false,
			collectorOpts: collectorOptions{},
			validateFunc: func(t *testing.T, output string) {
				assert.NotContains(t, output, "tags:", "tags key should be absent when not configured")
			},
		},
		{
			name:         "tags with YAML-special characters are quoted",
			clusterCheck: false,
			collectorOpts: collectorOptions{
				tags: []string{"env: prod", "path:/tmp", "team:cont-p"},
			},
			validateFunc: func(t *testing.T, output string) {
				var config map[string]any
				require.NoError(t, yaml.Unmarshal([]byte(output), &config), "YAML must remain valid even with space-after-colon and slash in tag values")
				instances := config["instances"].([]any)
				instance := instances[0].(map[string]any)
				tags, ok := instance["tags"].([]any)
				require.True(t, ok, "tags should parse as a list")
				assert.Equal(t, []any{"env: prod", "path:/tmp", "team:cont-p"}, tags, "tag values should round-trip exactly as provided")
			},
		},
		{
			name:         "labels_as_tags emits nested map",
			clusterCheck: false,
			collectorOpts: collectorOptions{
				labelsAsTags: map[string]map[string]string{
					"pod":  {"app": "app"},
					"node": {"zone": "zone", "team": "team"},
				},
			},
			validateFunc: func(t *testing.T, output string) {
				var config map[string]any
				require.NoError(t, yaml.Unmarshal([]byte(output), &config))
				instance := config["instances"].([]any)[0].(map[string]any)
				lat, ok := instance["labels_as_tags"].(map[string]any)
				require.True(t, ok, "labels_as_tags map should exist")
				pod := lat["pod"].(map[string]any)
				assert.Equal(t, "app", pod["app"])
				node := lat["node"].(map[string]any)
				assert.Equal(t, "zone", node["zone"])
				assert.Equal(t, "team", node["team"])
			},
		},
		{
			name:          "labels_as_tags omitted when empty",
			clusterCheck:  false,
			collectorOpts: collectorOptions{},
			validateFunc: func(t *testing.T, output string) {
				assert.NotContains(t, output, "labels_as_tags")
			},
		},
		{
			name:         "annotations_as_tags emits nested map",
			clusterCheck: false,
			collectorOpts: collectorOptions{
				annotationsAsTags: map[string]map[string]string{
					"pod": {"tags_datadoghq_com_version": "version"},
				},
			},
			validateFunc: func(t *testing.T, output string) {
				var config map[string]any
				require.NoError(t, yaml.Unmarshal([]byte(output), &config))
				instance := config["instances"].([]any)[0].(map[string]any)
				aat, ok := instance["annotations_as_tags"].(map[string]any)
				require.True(t, ok, "annotations_as_tags map should exist")
				pod := aat["pod"].(map[string]any)
				assert.Equal(t, "version", pod["tags_datadoghq_com_version"])
			},
		},
		{
			name:          "annotations_as_tags omitted when empty",
			clusterCheck:  false,
			collectorOpts: collectorOptions{},
			validateFunc: func(t *testing.T, output string) {
				assert.NotContains(t, output, "annotations_as_tags")
			},
		},
		{
			name:          "collectSecrets=false drops secrets collector",
			clusterCheck:  false,
			collectorOpts: collectorOptions{collectSecrets: false, collectConfigMaps: true},
			validateFunc: func(t *testing.T, output string) {
				var config map[string]any
				require.NoError(t, yaml.Unmarshal([]byte(output), &config))
				instance := config["instances"].([]any)[0].(map[string]any)
				collectors := instance["collectors"].([]any)
				for _, c := range collectors {
					assert.NotEqual(t, "secrets", c, "secrets collector should be absent")
				}
			},
		},
		{
			name:          "collectConfigMaps=false drops configmaps collector",
			clusterCheck:  false,
			collectorOpts: collectorOptions{collectSecrets: true, collectConfigMaps: false},
			validateFunc: func(t *testing.T, output string) {
				var config map[string]any
				require.NoError(t, yaml.Unmarshal([]byte(output), &config))
				instance := config["instances"].([]any)[0].(map[string]any)
				collectors := instance["collectors"].([]any)
				for _, c := range collectors {
					assert.NotEqual(t, "configmaps", c, "configmaps collector should be absent")
				}
			},
		},
		{
			name:          "both collectSecrets and collectConfigMaps false drops both collectors",
			clusterCheck:  false,
			collectorOpts: collectorOptions{collectSecrets: false, collectConfigMaps: false},
			validateFunc: func(t *testing.T, output string) {
				var config map[string]any
				require.NoError(t, yaml.Unmarshal([]byte(output), &config))
				instance := config["instances"].([]any)[0].(map[string]any)
				collectors := instance["collectors"].([]any)
				for _, c := range collectors {
					assert.NotEqual(t, "secrets", c, "secrets collector should be absent")
					assert.NotEqual(t, "configmaps", c, "configmaps collector should be absent")
				}
			},
		},
		{
			name:          "collectSecrets=true and collectConfigMaps=true include both collectors",
			clusterCheck:  false,
			collectorOpts: collectorOptions{collectSecrets: true, collectConfigMaps: true},
			validateFunc: func(t *testing.T, output string) {
				var config map[string]any
				require.NoError(t, yaml.Unmarshal([]byte(output), &config))
				instance := config["instances"].([]any)[0].(map[string]any)
				collectors := instance["collectors"].([]any)
				assert.Contains(t, collectors, "secrets", "secrets collector should be present when enabled")
				assert.Contains(t, collectors, "configmaps", "configmaps collector should be present when enabled")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			output := ksmCheckConfig(tc.clusterCheck, tc.collectorOpts)
			tc.validateFunc(t, output)
		})
	}
}

func TestKsmCheckConfigCustomResourcesWithMetricNamePrefix(t *testing.T) {
	prefix := "myprefix"
	opts := collectorOptions{
		customResources: []v2alpha1.Resource{
			{
				MetricNamePrefix: &prefix,
				GroupVersionKind: v2alpha1.GroupVersionKind{
					Group:   "cert-manager.io",
					Version: "v1",
					Kind:    "Certificate",
				},
			},
		},
	}

	output := ksmCheckConfig(true, opts)

	// Parse YAML to verify structure
	var config map[string]any
	err := yaml.Unmarshal([]byte(output), &config)
	require.NoError(t, err, "YAML should be valid")

	// Navigate to the resource and check metricNamePrefix
	instances := config["instances"].([]any)
	instance := instances[0].(map[string]any)
	customResource := instance["custom_resource"].(map[string]any)
	spec := customResource["spec"].(map[string]any)
	resources := spec["resources"].([]any)
	resource := resources[0].(map[string]any)

	assert.Equal(t, prefix, resource["metricNamePrefix"], "metricNamePrefix should be preserved")
}
