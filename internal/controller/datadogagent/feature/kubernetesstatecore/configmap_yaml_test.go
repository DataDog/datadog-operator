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
				var config map[string]interface{}
				err := yaml.Unmarshal([]byte(output), &config)
				require.NoError(t, err, "YAML should be valid")

				// Check structure
				instances, ok := config["instances"].([]interface{})
				require.True(t, ok, "instances should be a list")
				require.Len(t, instances, 1, "should have one instance")

				instance, ok := instances[0].(map[string]interface{})
				require.True(t, ok, "instance should be a map")

				customResource, ok := instance["custom_resource"].(map[string]interface{})
				require.True(t, ok, "custom_resource should exist")

				spec, ok := customResource["spec"].(map[string]interface{})
				require.True(t, ok, "spec should exist")

				resources, ok := spec["resources"].([]interface{})
				require.True(t, ok, "resources should be a list")
				require.Len(t, resources, 1, "should have one resource")

				// Check indentation visually
				lines := strings.Split(output, "\n")
				for _, line := range lines {
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
				var config map[string]interface{}
				err := yaml.Unmarshal([]byte(output), &config)
				require.NoError(t, err, "YAML should be valid")

				// Navigate to resources
				instances := config["instances"].([]interface{})
				instance := instances[0].(map[string]interface{})
				customResource := instance["custom_resource"].(map[string]interface{})
				spec := customResource["spec"].(map[string]interface{})
				resources := spec["resources"].([]interface{})

				assert.Len(t, resources, 2, "should have two resources")

				// Check both resources have the correct fields
				for i, res := range resources {
					resource := res.(map[string]interface{})
					gvk, ok := resource["groupVersionKind"].(map[string]interface{})
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
				var config map[string]interface{}
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
	var config map[string]interface{}
	err := yaml.Unmarshal([]byte(output), &config)
	require.NoError(t, err, "YAML should be valid")

	// Navigate to the resource and check metricNamePrefix
	instances := config["instances"].([]interface{})
	instance := instances[0].(map[string]interface{})
	customResource := instance["custom_resource"].(map[string]interface{})
	spec := customResource["spec"].(map[string]interface{})
	resources := spec["resources"].([]interface{})
	resource := resources[0].(map[string]interface{})

	assert.Equal(t, prefix, resource["metricNamePrefix"], "metricNamePrefix should be preserved")
}
