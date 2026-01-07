// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package mapper

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-operator/cmd/yaml-mapper/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chartutil"
)

func TestRun(t *testing.T) {
	mapper := NewMapper(MapConfig{
		MappingPath: "mapping_datadog_helm_to_datadogagent_crd.yaml",
		SourcePath:  "../examples/example_source.yaml",
		DestPath:    "../examples/destination.yaml",
	})

	err := mapper.Run()
	require.NoError(t, err)

	// TODO: add validations against the v2alpha1.DatadogAgent struct
}

func TestMergeMapDeep(t *testing.T) {
	tests := []struct {
		name     string
		map1     map[string]interface{}
		map2     map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name: "merge non-overlapping maps",
			map1: map[string]interface{}{
				"key1": "value1",
				"key2": 42,
			},
			map2: map[string]interface{}{
				"key3": "value3",
				"key4": []string{"a", "b"},
			},
			expected: map[string]interface{}{
				"key1": "value1",
				"key2": 42,
				"key3": "value3",
				"key4": []string{"a", "b"},
			},
		},
		{
			name: "merge overlapping maps with simple values (map2 overwrites map1)",
			map1: map[string]interface{}{
				"key1": "value1",
				"key2": 42,
			},
			map2: map[string]interface{}{
				"key1": "newvalue1",
				"key3": "value3",
			},
			expected: map[string]interface{}{
				"key1": "newvalue1",
				"key2": 42,
				"key3": "value3",
			},
		},
		{
			name: "merge nested maps",
			map1: map[string]interface{}{
				"config": map[string]interface{}{
					"database": map[string]interface{}{
						"host": "localhost",
						"port": 5432,
					},
					"cache": map[string]interface{}{
						"enabled": true,
					},
				},
				"version": "1.0",
			},
			map2: map[string]interface{}{
				"config": map[string]interface{}{
					"database": map[string]interface{}{
						"port":     3306,
						"username": "admin",
					},
					"logging": map[string]interface{}{
						"level": "debug",
					},
				},
				"environment": "production",
			},
			expected: map[string]interface{}{
				"config": map[string]interface{}{
					"database": map[string]interface{}{
						"host":     "localhost",
						"port":     3306,
						"username": "admin",
					},
					"cache": map[string]interface{}{
						"enabled": true,
					},
					"logging": map[string]interface{}{
						"level": "debug",
					},
				},
				"version":     "1.0",
				"environment": "production",
			},
		},
		{
			name: "one map is empty",
			map1: map[string]interface{}{
				"key1": "value1",
			},
			map2: map[string]interface{}{},
			expected: map[string]interface{}{
				"key1": "value1",
			},
		},
		{
			name:     "both maps are empty",
			map1:     map[string]interface{}{},
			map2:     map[string]interface{}{},
			expected: map[string]interface{}{},
		},
		{
			name: "mixed value types",
			map1: map[string]interface{}{
				"string":  "text",
				"number":  123,
				"boolean": true,
				"array":   []interface{}{1, 2, 3},
				"nested": map[string]interface{}{
					"inner": "value",
				},
			},
			map2: map[string]interface{}{
				"string": "newtext",
				"float":  3.14,
				"nested": map[string]interface{}{
					"additional": "data",
				},
			},
			expected: map[string]interface{}{
				"string":  "newtext",
				"number":  123,
				"boolean": true,
				"array":   []interface{}{1, 2, 3},
				"float":   3.14,
				"nested": map[string]interface{}{
					"inner":      "value",
					"additional": "data",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			map1Copy := make(map[string]interface{})
			for k, v := range tt.map1 {
				map1Copy[k] = v
			}
			map2Copy := make(map[string]interface{})
			for k, v := range tt.map2 {
				map2Copy[k] = v
			}

			result := utils.MergeMapDeep(map1Copy, map2Copy)
			assert.Equal(t, tt.expected, result)

			assert.Equal(t, tt.expected, map1Copy)
		})
	}
}

func TestInsertAtPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		val      interface{}
		mapName  map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name:    "simple single level path",
			path:    "key",
			val:     "value",
			mapName: map[string]interface{}{},
			expected: map[string]interface{}{
				"key": "value",
			},
		},
		{
			name:    "three level nested path",
			path:    "spec.global.site",
			val:     "datadoghq.com",
			mapName: map[string]interface{}{},
			expected: map[string]interface{}{
				"spec": map[string]interface{}{
					"global": map[string]interface{}{
						"site": "datadoghq.com",
					},
				},
			},
		},
		{
			name:    "deep nested path",
			path:    "spec.override.nodeAgent.containers.agent.resources.limits.memory",
			val:     "512Mi",
			mapName: map[string]interface{}{},
			expected: map[string]interface{}{
				"spec": map[string]interface{}{
					"override": map[string]interface{}{
						"nodeAgent": map[string]interface{}{
							"containers": map[string]interface{}{
								"agent": map[string]interface{}{
									"resources": map[string]interface{}{
										"limits": map[string]interface{}{
											"memory": "512Mi",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "merge with existing map - non-overlapping",
			path: "spec.global.site",
			val:  "datadoghq.com",
			mapName: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": "datadog",
				},
			},
			expected: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": "datadog",
				},
				"spec": map[string]interface{}{
					"global": map[string]interface{}{
						"site": "datadoghq.com",
					},
				},
			},
		},
		{
			name: "merge with existing map - overlapping paths",
			path: "spec.global.logLevel",
			val:  "debug",
			mapName: map[string]interface{}{
				"spec": map[string]interface{}{
					"global": map[string]interface{}{
						"site": "datadoghq.com",
					},
					"features": map[string]interface{}{
						"apm": map[string]interface{}{
							"enabled": true,
						},
					},
				},
			},
			expected: map[string]interface{}{
				"spec": map[string]interface{}{
					"global": map[string]interface{}{
						"site":     "datadoghq.com",
						"logLevel": "debug",
					},
					"features": map[string]interface{}{
						"apm": map[string]interface{}{
							"enabled": true,
						},
					},
				},
			},
		},
		{
			name: "overwrite existing value",
			path: "spec.global.site",
			val:  "datadoghq.eu",
			mapName: map[string]interface{}{
				"spec": map[string]interface{}{
					"global": map[string]interface{}{
						"site": "datadoghq.com",
					},
				},
			},
			expected: map[string]interface{}{
				"spec": map[string]interface{}{
					"global": map[string]interface{}{
						"site": "datadoghq.eu",
					},
				},
			},
		},
		{
			name:    "empty path",
			path:    "",
			val:     "",
			mapName: map[string]interface{}{},
			expected: map[string]interface{}{
				"": "",
			},
		},
		{
			name:    "different value types - integer",
			path:    "spec.override.clusterAgent.replicas",
			val:     3,
			mapName: map[string]interface{}{},
			expected: map[string]interface{}{
				"spec": map[string]interface{}{
					"override": map[string]interface{}{
						"clusterAgent": map[string]interface{}{
							"replicas": 3,
						},
					},
				},
			},
		},
		{
			name:    "different value types - boolean",
			path:    "spec.features.apm.enabled",
			val:     true,
			mapName: map[string]interface{}{},
			expected: map[string]interface{}{
				"spec": map[string]interface{}{
					"features": map[string]interface{}{
						"apm": map[string]interface{}{
							"enabled": true,
						},
					},
				},
			},
		},
		{
			name:    "different value types - slice",
			path:    "spec.global.tags",
			val:     []string{"env:prod", "team:backend"},
			mapName: map[string]interface{}{},
			expected: map[string]interface{}{
				"spec": map[string]interface{}{
					"global": map[string]interface{}{
						"tags": []string{"env:prod", "team:backend"},
					},
				},
			},
		},
		{
			name:    "different value types - map",
			path:    "spec.override.nodeAgent.resources",
			val:     map[string]interface{}{"limits": map[string]interface{}{"memory": "1Gi"}},
			mapName: map[string]interface{}{},
			expected: map[string]interface{}{
				"spec": map[string]interface{}{
					"override": map[string]interface{}{
						"nodeAgent": map[string]interface{}{
							"resources": map[string]interface{}{
								"limits": map[string]interface{}{
									"memory": "1Gi",
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy of the input map to avoid modifying the test data
			mapNameCopy := make(map[string]interface{})
			for k, v := range tt.mapName {
				mapNameCopy[k] = v
			}

			result := utils.InsertAtPath(tt.path, tt.val, mapNameCopy)

			// Verify that the result matches expected
			assert.Equal(t, tt.expected, result)

			// Verify that the function modifies the input map in place
			assert.Equal(t, tt.expected, mapNameCopy)

			// Verify that the returned map is the same object as the input map
			assert.True(t, fmt.Sprintf("%p", result) == fmt.Sprintf("%p", mapNameCopy), "InsertAtPath should return the same map object that was passed in")
		})
	}
}

func TestInsertAtPathEdgeCases(t *testing.T) {
	t.Run("nil_value", func(t *testing.T) {
		mapName := map[string]interface{}{}
		result := utils.InsertAtPath("spec.global.site", nil, mapName)

		expected := map[string]interface{}{
			"spec": map[string]interface{}{
				"global": map[string]interface{}{
					"site": nil,
				},
			},
		}
		assert.Equal(t, expected, result)
	})

	t.Run("path_with_multiple_dots", func(t *testing.T) {
		mapName := map[string]interface{}{}
		result := utils.InsertAtPath("a.b.c.d.e.f", "deep_value", mapName)

		expected := map[string]interface{}{
			"a": map[string]interface{}{
				"b": map[string]interface{}{
					"c": map[string]interface{}{
						"d": map[string]interface{}{
							"e": map[string]interface{}{
								"f": "deep_value",
							},
						},
					},
				},
			},
		}
		assert.Equal(t, expected, result)
	})

	t.Run("path_with_numeric_keys", func(t *testing.T) {
		mapName := map[string]interface{}{}
		result := utils.InsertAtPath("spec.containers.0.name", "agent", mapName)

		expected := map[string]interface{}{
			"spec": map[string]interface{}{
				"containers": map[string]interface{}{
					"0": map[string]interface{}{
						"name": "agent",
					},
				},
			},
		}
		assert.Equal(t, expected, result)
	})
}

func TestMergeOrSet(t *testing.T) {
	tests := []struct {
		name        string
		interim     map[string]interface{}
		key         string
		val         interface{}
		wantInterim map[string]interface{}
	}{
		{
			name:    "simple set",
			interim: map[string]interface{}{},
			key:     "foo.bar",
			val:     "true",
			wantInterim: map[string]interface{}{
				"foo.bar": "true",
			},
		},
		{
			name: "simple override",
			interim: map[string]interface{}{
				"foo.bar": "false",
			},
			key: "foo.bar",
			val: "true",
			wantInterim: map[string]interface{}{
				"foo.bar": "true",
			},
		},
		{
			name: "simple merge",
			interim: map[string]interface{}{
				"foo.bar": "true",
			},
			key: "bar.foo",
			val: "true",
			wantInterim: map[string]interface{}{
				"foo.bar": "true",
				"bar.foo": "true",
			},
		},
		{
			name:    "set map",
			interim: map[string]interface{}{},
			key:     "bar.foo",
			val: map[string]interface{}{
				"foo": "bar",
			},
			wantInterim: map[string]interface{}{
				"bar.foo": map[string]interface{}{
					"foo": "bar",
				},
			},
		},
		{
			name: "merge maps at same key (non-overlapping)",
			interim: map[string]interface{}{
				"spec.global": map[string]interface{}{"site": "datadoghq.com"},
			},
			key: "spec.global",
			val: map[string]interface{}{"logLevel": "debug"},
			wantInterim: map[string]interface{}{
				"spec.global": map[string]interface{}{
					"site":     "datadoghq.com",
					"logLevel": "debug",
				},
			},
		},
		{
			name: "deep-merge nested maps",
			interim: map[string]interface{}{
				"spec.features": map[string]interface{}{
					"apm": map[string]interface{}{"enabled": true},
				},
			},
			key: "spec.features",
			val: map[string]interface{}{
				"apm":  map[string]interface{}{"portEnabled": true},
				"usm":  map[string]interface{}{"enabled": true},
				"apm2": map[string]interface{}{"foo": "bar"},
			},
			wantInterim: map[string]interface{}{
				"spec.features": map[string]interface{}{
					"apm": map[string]interface{}{
						"enabled":     true,
						"portEnabled": true,
					},
					"usm":  map[string]interface{}{"enabled": true},
					"apm2": map[string]interface{}{"foo": "bar"},
				},
			},
		},
		{
			name: "overwrite map with scalar",
			interim: map[string]interface{}{
				"spec.global": map[string]interface{}{"site": "datadoghq.com"},
			},
			key: "spec.global",
			val: "not-a-map-anymore",
			wantInterim: map[string]interface{}{
				"spec.global": "not-a-map-anymore",
			},
		},
		{
			name: "overwrite scalar with map",
			interim: map[string]interface{}{
				"spec.global": "string-value",
			},
			key: "spec.global",
			val: map[string]interface{}{"site": "datadoghq.eu"},
			wantInterim: map[string]interface{}{
				"spec.global": map[string]interface{}{"site": "datadoghq.eu"},
			},
		},
		{
			name: "merge chartutil.Values into map",
			interim: map[string]interface{}{
				"spec.global": map[string]interface{}{"site": "datadoghq.com"},
			},
			key: "spec.global",
			val: chartutil.Values{"logLevel": "info"},
			wantInterim: map[string]interface{}{
				"spec.global": map[string]interface{}{
					"site":     "datadoghq.com",
					"logLevel": "info",
				},
			},
		},
		{
			name: "merge map into chartutil.Values",
			interim: map[string]interface{}{
				"spec.global": chartutil.Values{"site": "datadoghq.com"},
			},
			key: "spec.global",
			val: map[string]interface{}{"logLevel": "warn"},
			wantInterim: map[string]interface{}{
				"spec.global": map[string]interface{}{
					"site":     "datadoghq.com",
					"logLevel": "warn",
				},
			},
		},
		{
			name:        "nil value should be ignored (no set)",
			interim:     map[string]interface{}{},
			key:         "spec.global.site",
			val:         nil,
			wantInterim: map[string]interface{}{},
		},
		{
			name: "nil value should not override existing",
			interim: map[string]interface{}{
				"spec.global.site": "datadoghq.com",
			},
			key: "spec.global.site",
			val: nil,
			wantInterim: map[string]interface{}{
				"spec.global.site": "datadoghq.com",
			},
		},
		{
			name: "deep-merge overlapping nested keys",
			interim: map[string]interface{}{
				"a": map[string]interface{}{
					"b": map[string]interface{}{"x": 1},
				},
			},
			key: "a",
			val: map[string]interface{}{
				"b": map[string]interface{}{"y": 2},
			},
			wantInterim: map[string]interface{}{
				"a": map[string]interface{}{
					"b": map[string]interface{}{"x": 1, "y": 2},
				},
			},
		},
	}
	for _, tt := range tests {
		utils.MergeOrSet(tt.interim, tt.key, tt.val)
		assert.Equal(t, tt.interim, tt.wantInterim)
	}
}

func TestApplyDeprecationRules(t *testing.T) {
	tests := []struct {
		name       string
		sourceVals chartutil.Values
		wantVals   chartutil.Values
	}{
		{
			name: "bool OR: default - deprecated present",
			sourceVals: chartutil.Values{
				"datadog": map[string]interface{}{
					"apm": map[string]interface{}{
						"enabled": true,
					},
				},
			},
			wantVals: chartutil.Values{
				"datadog": map[string]interface{}{
					"apm": map[string]interface{}{
						"portEnabled": true,
					},
				},
			},
		},
		{
			name: "bool OR: both standard and deprecated present",
			sourceVals: chartutil.Values{
				"datadog": map[string]interface{}{
					"apm": map[string]interface{}{
						"enabled":     true,
						"portEnabled": true,
					},
				},
			},
			wantVals: chartutil.Values{
				"datadog": map[string]interface{}{
					"apm": map[string]interface{}{
						"portEnabled": true,
					},
				},
			},
		},
		{
			name: "bool OR: both standard and deprecated present, standard takes precedence",
			sourceVals: chartutil.Values{
				"datadog": map[string]interface{}{
					"apm": map[string]interface{}{
						"enabled":     false,
						"portEnabled": true,
					},
				},
			},
			wantVals: chartutil.Values{
				"datadog": map[string]interface{}{
					"apm": map[string]interface{}{
						"portEnabled": true,
					},
				},
			},
		},
		{
			name: "bool OR: standard false and deprecated true, truthy takes precedence",
			sourceVals: chartutil.Values{
				"datadog": map[string]interface{}{
					"apm": map[string]interface{}{
						"enabled":     true,
						"portEnabled": false,
					},
				},
			},
			wantVals: chartutil.Values{
				"datadog": map[string]interface{}{
					"apm": map[string]interface{}{
						"portEnabled": true,
					},
				},
			},
		},
		{
			name: "bool OR: multiple deprecated candidates - simple",
			sourceVals: chartutil.Values{
				"agents": map[string]interface{}{
					"networkPolicy": map[string]interface{}{
						"create": true,
					},
				},
			},
			wantVals: chartutil.Values{
				"datadog": map[string]interface{}{
					"networkPolicy": map[string]interface{}{
						"create": true,
					},
				},
				"agents": map[string]interface{}{
					"networkPolicy": map[string]interface{}{},
				},
			},
		},
		{
			name: "bool OR: multiple deprecated candidates - complex",
			sourceVals: chartutil.Values{
				"agents": map[string]interface{}{
					"networkPolicy": map[string]interface{}{
						"create": true,
					},
				},
				"clusterAgent": map[string]interface{}{
					"networkPolicy": map[string]interface{}{
						"create": false,
					},
				},
			},
			wantVals: chartutil.Values{
				"datadog": map[string]interface{}{
					"networkPolicy": map[string]interface{}{
						"create": true,
					},
				},
				"agents": map[string]interface{}{
					"networkPolicy": map[string]interface{}{},
				},
				"clusterAgent": map[string]interface{}{
					"networkPolicy": map[string]interface{}{},
				},
			},
		},
		{
			name: "bool OR: multiple deprecated candidates - complex w/extra keys",
			sourceVals: chartutil.Values{
				"agents": map[string]interface{}{
					"networkPolicy": map[string]interface{}{
						"create": true,
						"flavor": "cilium",
					},
				},
				"clusterAgent": map[string]interface{}{
					"networkPolicy": map[string]interface{}{
						"create": false,
						"flavor": "cilium",
						"cilium": map[string]interface{}{
							"dnsSelector": map[string]interface{}{
								"foo": "bar",
							},
						},
					},
				},
			},
			wantVals: chartutil.Values{
				"datadog": map[string]interface{}{
					"networkPolicy": map[string]interface{}{
						"create": true,
					},
				},
				"agents": map[string]interface{}{
					"networkPolicy": map[string]interface{}{
						"flavor": "cilium",
					},
				},
				"clusterAgent": map[string]interface{}{
					"networkPolicy": map[string]interface{}{
						"flavor": "cilium",
						"cilium": map[string]interface{}{
							"dnsSelector": map[string]interface{}{
								"foo": "bar",
							},
						},
					},
				},
			},
		},
		{
			name: "bool OR: multiple deprecated candidates + standard - complex",
			sourceVals: chartutil.Values{
				"datadog": map[string]interface{}{
					"networkPolicy": map[string]interface{}{
						"create": true,
					},
				},
				"agents": map[string]interface{}{
					"networkPolicy": map[string]interface{}{
						"create": false,
						"flavor": "cilium",
					},
				},
				"clusterAgent": map[string]interface{}{
					"networkPolicy": map[string]interface{}{
						"create": false,
						"flavor": "cilium",
						"cilium": map[string]interface{}{
							"dnsSelector": map[string]interface{}{
								"foo": "bar",
							},
						},
					},
				},
			},
			wantVals: chartutil.Values{
				"datadog": map[string]interface{}{
					"networkPolicy": map[string]interface{}{
						"create": true,
					},
				},
				"agents": map[string]interface{}{
					"networkPolicy": map[string]interface{}{
						"flavor": "cilium",
					},
				},
				"clusterAgent": map[string]interface{}{
					"networkPolicy": map[string]interface{}{
						"flavor": "cilium",
						"cilium": map[string]interface{}{
							"dnsSelector": map[string]interface{}{
								"foo": "bar",
							},
						},
					},
				},
			},
		},
		{
			name: "bool OR: multiple deprecated candidates + standard - truthy takes precedence",
			sourceVals: chartutil.Values{
				"datadog": map[string]interface{}{
					"networkPolicy": map[string]interface{}{
						"create": false,
					},
				},
				"agents": map[string]interface{}{
					"networkPolicy": map[string]interface{}{
						"create": true,
						"flavor": "cilium",
					},
				},
				"clusterAgent": map[string]interface{}{
					"networkPolicy": map[string]interface{}{
						"create": false,
						"flavor": "cilium",
						"cilium": map[string]interface{}{
							"dnsSelector": map[string]interface{}{
								"foo": "bar",
							},
						},
					},
				},
			},
			wantVals: chartutil.Values{
				"datadog": map[string]interface{}{
					"networkPolicy": map[string]interface{}{
						"create": true,
					},
				},
				"agents": map[string]interface{}{
					"networkPolicy": map[string]interface{}{
						"flavor": "cilium",
					},
				},
				"clusterAgent": map[string]interface{}{
					"networkPolicy": map[string]interface{}{
						"flavor": "cilium",
						"cilium": map[string]interface{}{
							"dnsSelector": map[string]interface{}{
								"foo": "bar",
							},
						},
					},
				},
			},
		},
		{
			name: "bool negation: default",
			sourceVals: chartutil.Values{
				"datadog": map[string]interface{}{
					"systemProbe": map[string]interface{}{
						"enableDefaultOsReleasePaths": true,
					},
				},
			},
			wantVals: chartutil.Values{
				"datadog": map[string]interface{}{
					"systemProbe":                  map[string]interface{}{},
					"disableDefaultOsReleasePaths": false,
				},
			},
		},
		{
			name: "bool negation: standard false and deprecated false - standard should take precedence",
			sourceVals: chartutil.Values{
				"datadog": map[string]interface{}{
					"systemProbe": map[string]interface{}{
						"enableDefaultOsReleasePaths": false,
					},
					"disableDefaultOsReleasePaths": false,
				},
			},
			wantVals: chartutil.Values{
				"datadog": map[string]interface{}{
					"systemProbe":                  map[string]interface{}{},
					"disableDefaultOsReleasePaths": false,
				},
			},
		},
		{
			name: "bool negation: standard true and deprecated true - standard takes precedence",
			sourceVals: chartutil.Values{
				"datadog": map[string]interface{}{
					"systemProbe": map[string]interface{}{
						"enableDefaultOsReleasePaths": true,
					},
					"disableDefaultOsReleasePaths": true,
				},
			},
			wantVals: chartutil.Values{
				"datadog": map[string]interface{}{
					"systemProbe":                  map[string]interface{}{},
					"disableDefaultOsReleasePaths": true,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualMap := utils.ApplyDeprecationRules(tt.sourceVals)
			assert.Equal(t, tt.wantVals, actualMap)
		})
	}
}

func TestMappingProcessors(t *testing.T) {
	// Test that all mapping processors are properly registered
	t.Run("mapFuncRegistry_dict", func(t *testing.T) {
		expectedFuncs := []string{"mapSecretKeyName", "mapSeccompProfile", "mapSystemProbeAppArmor", "mapLocalServiceName", "mapAppendEnvVar", "mapMergeEnvs", "mapOverrideType", "mapServiceAccountName", "mapHealthPortWithProbes", "mapTraceAgentLivenessProbe", "mapApmPortToContainerPort"}
		mapFuncs := mapFuncRegistry()

		for _, funcName := range expectedFuncs {
			t.Run(funcName+"_exists", func(t *testing.T) {
				runFunc := mapFuncs[funcName]
				assert.NotNil(t, runFunc, "Mapping function %s should be registered", funcName)
			})
		}

		assert.Equal(t, len(expectedFuncs), len(mapFuncs), "Should have exactly %d mapping functions", len(expectedFuncs))
	})

	// Test individual functions through the dictionary
	tests := []struct {
		name         string
		funcName     string
		interim      map[string]interface{}
		newPath      string
		pathVal      interface{}
		mapFuncArgs  []interface{}
		sourceValues chartutil.Values // Helm source values for processors that need them
		expectedMap  map[string]interface{}
	}{
		// mapSecretKeyName tests
		{
			name:     "mapSecretKeyName_apiKey_maps_non_empty",
			funcName: "mapSecretKeyName",
			interim:  map[string]interface{}{},
			newPath:  "spec.global.credentials.apiSecret.secretName",
			pathVal:  "my-api-secret",
			mapFuncArgs: []interface{}{
				map[string]interface{}{
					"keyName":     "api-key",
					"keyNamePath": "spec.global.credentials.apiSecret.keyName",
				},
			},
			expectedMap: map[string]interface{}{
				"spec.global.credentials.apiSecret.secretName": "my-api-secret",
				"spec.global.credentials.apiSecret.keyName":    "api-key",
			},
		},
		{
			name:     "mapSecretKeyName_apiKey_skips_empty",
			funcName: "mapSecretKeyName",
			interim:  map[string]interface{}{},
			newPath:  "spec.global.credentials.apiSecret.secretName",
			pathVal:  "",
			mapFuncArgs: []interface{}{
				map[string]interface{}{
					"keyName":     "api-key",
					"keyNamePath": "spec.global.credentials.apiSecret.keyName",
				},
			},
			expectedMap: map[string]interface{}{},
		},
		{
			name:     "mapSecretKeyName_appKey_maps_non_empty",
			funcName: "mapSecretKeyName",
			interim:  map[string]interface{}{},
			newPath:  "spec.global.credentials.appSecret.secretName",
			pathVal:  "my-app-secret",
			mapFuncArgs: []interface{}{
				map[string]interface{}{
					"keyName":     "app-key",
					"keyNamePath": "spec.global.credentials.appSecret.keyName",
				},
			},
			expectedMap: map[string]interface{}{
				"spec.global.credentials.appSecret.secretName": "my-app-secret",
				"spec.global.credentials.appSecret.keyName":    "app-key",
			},
		},
		{
			name:     "mapSecretKeyName_appKey_skips_empty",
			funcName: "mapSecretKeyName",
			interim:  map[string]interface{}{},
			newPath:  "spec.global.credentials.appSecret.secretName",
			pathVal:  "",
			mapFuncArgs: []interface{}{
				map[string]interface{}{
					"keyName":     "app-key",
					"keyNamePath": "spec.global.credentials.appSecret.keyName",
				},
			},
			expectedMap: map[string]interface{}{},
		},
		{
			name:     "mapSecretKeyName_token_maps_non_empty",
			funcName: "mapSecretKeyName",
			interim:  map[string]interface{}{},
			newPath:  "spec.global.clusterAgentTokenSecret.secretName",
			pathVal:  "my-token-secret",
			mapFuncArgs: []interface{}{
				map[string]interface{}{
					"keyName":     "token",
					"keyNamePath": "spec.global.clusterAgentTokenSecret.keyName",
				},
			},
			expectedMap: map[string]interface{}{
				"spec.global.clusterAgentTokenSecret.secretName": "my-token-secret",
				"spec.global.clusterAgentTokenSecret.keyName":    "token",
			},
		},
		{
			name:     "mapSecretKeyName_token_skips_empty",
			funcName: "mapSecretKeyName",
			interim:  map[string]interface{}{},
			newPath:  "spec.global.clusterAgentTokenSecret.secretName",
			pathVal:  "",
			mapFuncArgs: []interface{}{
				map[string]interface{}{
					"keyName":     "token",
					"keyNamePath": "spec.global.clusterAgentTokenSecret.keyName",
				},
			},
			expectedMap: map[string]interface{}{},
		},
		// mapSeccompProfile tests
		{
			name:     "mapSeccompProfile_localhost",
			funcName: "mapSeccompProfile",
			interim:  map[string]interface{}{},
			newPath:  "spec.override.nodeAgent.containers.system-probe.securityContext.seccompProfile",
			pathVal:  "localhost/system-probe",
			expectedMap: map[string]interface{}{
				"spec.override.nodeAgent.containers.system-probe.securityContext.seccompProfile.type":             "Localhost",
				"spec.override.nodeAgent.containers.system-probe.securityContext.seccompProfile.localhostProfile": "system-probe",
			},
		},
		{
			name:     "mapSeccompProfile_runtime_default",
			funcName: "mapSeccompProfile",
			interim:  map[string]interface{}{},
			newPath:  "spec.override.nodeAgent.containers.system-probe.securityContext.seccompProfile",
			pathVal:  "runtime/default",
			expectedMap: map[string]interface{}{
				"spec.override.nodeAgent.containers.system-probe.securityContext.seccompProfile.type": "RuntimeDefault",
			},
		},
		{
			name:     "mapSeccompProfile_unconfined",
			funcName: "mapSeccompProfile",
			interim:  map[string]interface{}{},
			newPath:  "spec.override.nodeAgent.containers.system-probe.securityContext.seccompProfile",
			pathVal:  "unconfined",
			expectedMap: map[string]interface{}{
				"spec.override.nodeAgent.containers.system-probe.securityContext.seccompProfile.type": "Unconfined",
			},
		},
		// mapSystemProbeAppArmor tests
		{
			name:     "mapSystemProbeAppArmor_no_features_enabled",
			funcName: "mapSystemProbeAppArmor",
			interim: map[string]interface{}{
				"spec.features.cws.enabled": false,
				"spec.features.npm.enabled": false,
			},
			newPath: "spec.override.nodeAgent.containers.system-probe.appArmorProfile",
			pathVal: "unconfined",
			expectedMap: map[string]interface{}{
				"spec.features.cws.enabled": false,
				"spec.features.npm.enabled": false,
			},
		},
		{
			name:     "mapSystemProbeAppArmor_multiple_features_enabled",
			funcName: "mapSystemProbeAppArmor",
			interim: map[string]interface{}{
				"spec.features.cws.enabled":            true,
				"spec.features.npm.enabled":            false,
				"spec.features.tcpQueueLength.enabled": true,
			},
			newPath: "spec.override.nodeAgent.containers.system-probe.appArmorProfile",
			pathVal: "unconfined",
			expectedMap: map[string]interface{}{
				"spec.features.cws.enabled":                                       true,
				"spec.features.npm.enabled":                                       false,
				"spec.features.tcpQueueLength.enabled":                            true,
				"spec.override.nodeAgent.containers.system-probe.appArmorProfile": "unconfined",
			},
		},
		{
			name:     "mapSystemProbeAppArmor_gpu_enabled_privileged",
			funcName: "mapSystemProbeAppArmor",
			interim: map[string]interface{}{
				"spec.features.gpu.enabled":        true,
				"spec.features.gpu.privilegedMode": true,
			},
			newPath: "spec.override.nodeAgent.containers.system-probe.appArmorProfile",
			pathVal: "unconfined",
			expectedMap: map[string]interface{}{
				"spec.features.gpu.enabled":                                       true,
				"spec.features.gpu.privilegedMode":                                true,
				"spec.override.nodeAgent.containers.system-probe.appArmorProfile": "unconfined",
			},
		},
		{
			name:     "mapSystemProbeAppArmor_gpu_enabled_not_privileged",
			funcName: "mapSystemProbeAppArmor",
			interim: map[string]interface{}{
				"spec.features.gpu.enabled":        true,
				"spec.features.gpu.privilegedMode": false,
			},
			newPath: "spec.override.nodeAgent.containers.system-probe.appArmorProfile",
			pathVal: "unconfined",
			expectedMap: map[string]interface{}{
				"spec.features.gpu.enabled":        true,
				"spec.features.gpu.privilegedMode": false,
			},
		},
		{
			name:     "mapSystemProbeAppArmor_empty_apparmor_value",
			funcName: "mapSystemProbeAppArmor",
			interim: map[string]interface{}{
				"spec.features.cws.enabled": true,
			},
			newPath: "spec.override.nodeAgent.containers.system-probe.appArmorProfile",
			pathVal: "",
			expectedMap: map[string]interface{}{
				"spec.features.cws.enabled": true,
			},
		},
		{
			name:     "mapSystemProbeAppArmor_invalid_apparmor_type",
			funcName: "mapSystemProbeAppArmor",
			interim: map[string]interface{}{
				"spec.features.cws.enabled": true,
			},
			newPath: "spec.override.nodeAgent.containers.system-probe.appArmorProfile",
			pathVal: 123,
			expectedMap: map[string]interface{}{
				"spec.features.cws.enabled": true,
			},
		},
		// mapLocalServiceName tests
		{
			name:        "mapLocalServiceName_empty_name",
			funcName:    "mapLocalServiceName",
			interim:     map[string]interface{}{},
			newPath:     "spec.override.clusterAgent.config.external_metrics.local_service_name",
			pathVal:     "",
			expectedMap: map[string]interface{}{},
		},
		{
			name:        "mapLocalServiceName_invalid_type",
			funcName:    "mapLocalServiceName",
			interim:     map[string]interface{}{},
			newPath:     "spec.override.clusterAgent.config.external_metrics.local_service_name",
			pathVal:     123,
			expectedMap: map[string]interface{}{},
		},
		{
			name:     "mapLocalServiceName_overwrite_existing",
			funcName: "mapLocalServiceName",
			interim: map[string]interface{}{
				"spec.override.clusterAgent.config.external_metrics.local_service_name": "old-service",
			},
			newPath: "spec.override.clusterAgent.config.external_metrics.local_service_name",
			pathVal: "new-service",
			expectedMap: map[string]interface{}{
				"spec.override.clusterAgent.config.external_metrics.local_service_name": "new-service",
			},
		},
		{
			name:     "mapAppendEnvVar_add_env_var",
			funcName: "mapAppendEnvVar",
			interim:  map[string]interface{}{},
			newPath:  "spec.override.nodeAgent.containers.agent.env",
			pathVal:  "debug",
			mapFuncArgs: []interface{}{
				map[string]interface{}{
					"name": "DD_LOG_LEVEL",
				},
			},
			expectedMap: map[string]interface{}{
				"spec.override.nodeAgent.containers.agent.env": []interface{}{
					map[string]interface{}{
						"name":  "DD_LOG_LEVEL",
						"value": "debug",
					},
				},
			},
		},
		{
			name:     "mapAppendEnvVar_add_to_existing_env_vars",
			funcName: "mapAppendEnvVar",
			interim: map[string]interface{}{
				"spec.override.nodeAgent.containers.agent.env": []interface{}{
					map[string]interface{}{
						"name":  "EXISTING_VAR",
						"value": "existing_value",
					},
				},
			},
			newPath: "spec.override.nodeAgent.containers.agent.env",
			pathVal: "new_value",
			mapFuncArgs: []interface{}{
				map[string]interface{}{
					"name": "NEW_VAR",
				},
			},
			expectedMap: map[string]interface{}{
				"spec.override.nodeAgent.containers.agent.env": []interface{}{
					map[string]interface{}{
						"name":  "EXISTING_VAR",
						"value": "existing_value",
					},
					map[string]interface{}{
						"name":  "NEW_VAR",
						"value": "new_value",
					},
				},
			},
		},
		{
			name:     "mapAppendEnvVar_valueFrom",
			funcName: "mapAppendEnvVar",
			interim:  map[string]interface{}{},
			newPath:  "spec.override.nodeAgent.env",
			pathVal: map[string]interface{}{
				"valueFrom": map[string]interface{}{
					"fieldRef": map[string]interface{}{
						"fieldPath": "status.hostIP",
					},
				},
			},
			mapFuncArgs: []interface{}{
				map[string]interface{}{
					"name": "DD_KUBERNETES_KUBELET_HOST",
				},
			},
			expectedMap: map[string]interface{}{
				"spec.override.nodeAgent.env": []interface{}{
					map[string]interface{}{
						"name": "DD_KUBERNETES_KUBELET_HOST",
						"valueFrom": map[string]interface{}{
							"fieldRef": map[string]interface{}{
								"fieldPath": "status.hostIP",
							},
						},
					},
				},
			},
		},
		{
			name:     "mapAppendEnvVar_valueFrom_existing_envVars",
			funcName: "mapAppendEnvVar",
			interim: map[string]interface{}{
				"spec.override.nodeAgent.env": []interface{}{
					map[string]interface{}{
						"name":  "EXISTING_VAR",
						"value": "existing_value",
					},
					map[string]interface{}{
						"name":  "EXISTING_VAR_2",
						"value": "existing_value_2",
					},
				},
			},
			newPath: "spec.override.nodeAgent.env",
			pathVal: map[string]interface{}{
				"valueFrom": map[string]interface{}{
					"fieldRef": map[string]interface{}{
						"fieldPath": "status.hostIP",
					},
				},
			},
			mapFuncArgs: []interface{}{
				map[string]interface{}{
					"name": "DD_KUBERNETES_KUBELET_HOST",
				},
			},
			expectedMap: map[string]interface{}{
				"spec.override.nodeAgent.env": []interface{}{
					map[string]interface{}{
						"name":  "EXISTING_VAR",
						"value": "existing_value",
					},
					map[string]interface{}{
						"name":  "EXISTING_VAR_2",
						"value": "existing_value_2",
					},
					map[string]interface{}{
						"name": "DD_KUBERNETES_KUBELET_HOST",
						"valueFrom": map[string]interface{}{
							"fieldRef": map[string]interface{}{
								"fieldPath": "status.hostIP",
							},
						},
					},
				},
			},
		},
		{
			name:     "mapMergeEnvs_add_new_envs",
			funcName: "mapMergeEnvs",
			interim:  map[string]interface{}{},
			newPath:  "spec.override.nodeAgent.containers.agent.env",
			pathVal: []interface{}{
				map[string]interface{}{
					"name":  "VAR1",
					"value": "value1",
				},
			},
			expectedMap: map[string]interface{}{
				"spec.override.nodeAgent.containers.agent.env": []interface{}{
					map[string]interface{}{
						"name":  "VAR1",
						"value": "value1",
					},
				},
			},
		},
		{
			name:     "mapMergeEnvs_add_to_existing_envs",
			funcName: "mapMergeEnvs",
			interim: map[string]interface{}{
				"spec.override.nodeAgent.containers.agent.env": []interface{}{
					map[string]interface{}{
						"name":  "EXISTING_VAR",
						"value": "existing_value",
					},
				},
			},
			newPath: "spec.override.nodeAgent.containers.agent.env",
			pathVal: []interface{}{
				map[string]interface{}{
					"name":  "NEW_VAR",
					"value": "new_value",
				},
			},
			expectedMap: map[string]interface{}{
				"spec.override.nodeAgent.containers.agent.env": []interface{}{
					map[string]interface{}{
						"name":  "EXISTING_VAR",
						"value": "existing_value",
					},
					map[string]interface{}{
						"name":  "NEW_VAR",
						"value": "new_value",
					},
				},
			},
		},
		{
			name:     "mapMergeEnvs_avoid_duplicates",
			funcName: "mapMergeEnvs",
			interim: map[string]interface{}{
				"spec.override.nodeAgent.containers.agent.env": []interface{}{
					map[string]interface{}{
						"name":  "EXISTING_VAR",
						"value": "existing_value",
					},
				},
			},
			newPath: "spec.override.nodeAgent.containers.agent.env",
			pathVal: []interface{}{
				map[string]interface{}{
					"name":  "EXISTING_VAR", // This should not be added again
					"value": "existing_value",
				},
				map[string]interface{}{
					"name":  "NEW_VAR",
					"value": "new_value",
				},
			},
			expectedMap: map[string]interface{}{
				"spec.override.nodeAgent.containers.agent.env": []interface{}{
					map[string]interface{}{
						"name":  "EXISTING_VAR",
						"value": "existing_value", // Keeps the original value
					},
					map[string]interface{}{
						"name":  "NEW_VAR",
						"value": "new_value",
					},
				},
			},
		},
		{
			name:     "mapMergeEnvs_override_duplicates",
			funcName: "mapMergeEnvs",
			interim: map[string]interface{}{
				"spec.override.nodeAgent.containers.agent.env": []interface{}{
					map[string]interface{}{
						"name":  "EXISTING_VAR",
						"value": "existing_value",
					},
				},
			},
			newPath: "spec.override.nodeAgent.containers.agent.env",
			pathVal: []interface{}{
				map[string]interface{}{
					"name":  "EXISTING_VAR", // This should override existing value
					"value": "new_value",
				},
				map[string]interface{}{
					"name":  "NEW_VAR",
					"value": "new_value",
				},
			},
			expectedMap: map[string]interface{}{
				"spec.override.nodeAgent.containers.agent.env": []interface{}{
					map[string]interface{}{
						"name":  "EXISTING_VAR",
						"value": "new_value", // New value overrides previous value
					},
					map[string]interface{}{
						"name":  "NEW_VAR",
						"value": "new_value",
					},
				},
			},
		},
		// mapServiceAccountName tests
		{
			name:     "mapServiceAccountName_rbac_create_false_should_map",
			funcName: "mapServiceAccountName",
			interim: map[string]interface{}{
				"spec.override.clusterAgent.createRbac": false,
			},
			newPath: "spec.override.clusterAgent.serviceAccountName",
			pathVal: "my-custom-sa",
			mapFuncArgs: []interface{}{
				map[string]interface{}{
					"rbacCreatePath": "spec.override.clusterAgent.createRbac",
				},
			},
			expectedMap: map[string]interface{}{
				"spec.override.clusterAgent.createRbac":         false,
				"spec.override.clusterAgent.serviceAccountName": "my-custom-sa",
			},
		},
		{
			name:     "mapServiceAccountName_rbac_create_true_should_not_map",
			funcName: "mapServiceAccountName",
			interim: map[string]interface{}{
				"spec.override.clusterAgent.createRbac": true,
			},
			newPath: "spec.override.clusterAgent.serviceAccountName",
			pathVal: "default",
			mapFuncArgs: []interface{}{
				map[string]interface{}{
					"rbacCreatePath": "spec.override.clusterAgent.createRbac",
				},
			},
			expectedMap: map[string]interface{}{
				"spec.override.clusterAgent.createRbac": true,
				// serviceAccountName should NOT be set when rbac.create is true
			},
		},
		{
			name:     "mapServiceAccountName_rbac_create_not_set_should_not_map",
			funcName: "mapServiceAccountName",
			interim:  map[string]interface{}{},
			newPath:  "spec.override.clusterAgent.serviceAccountName",
			pathVal:  "default",
			mapFuncArgs: []interface{}{
				map[string]interface{}{
					"rbacCreatePath": "spec.override.clusterAgent.createRbac",
				},
			},
			expectedMap: map[string]interface{}{}, // Neither createRbac nor serviceAccountName should be set
		},
		{
			name:     "mapServiceAccountName_nodeAgent_rbac_create_false",
			funcName: "mapServiceAccountName",
			interim: map[string]interface{}{
				"spec.override.nodeAgent.createRbac": false,
			},
			newPath: "spec.override.nodeAgent.serviceAccountName",
			pathVal: "custom-node-agent-sa",
			mapFuncArgs: []interface{}{
				map[string]interface{}{
					"rbacCreatePath": "spec.override.nodeAgent.createRbac",
				},
			},
			expectedMap: map[string]interface{}{
				"spec.override.nodeAgent.createRbac":         false,
				"spec.override.nodeAgent.serviceAccountName": "custom-node-agent-sa",
			},
		},
		// mapOverrideType tests
		{
			name:     "mapOverrideType_slice_to_string",
			funcName: "mapOverrideType",
			interim:  map[string]interface{}{},
			newPath:  "spec.features.foo.bar",
			mapFuncArgs: []interface{}{
				map[string]interface{}{
					"newPath": "spec.features.foo.bar",
					"newType": "string",
				},
			},
			pathVal: []map[string]interface{}{
				{
					"someKey":    "someVal",
					"anotherKey": map[string]interface{}{"foo": true},
				},
			},
			expectedMap: map[string]interface{}{
				"spec.features.foo.bar": `- anotherKey:
    foo: true
  someKey: someVal
`,
			},
		},
		{
			name:     "mapOverrideType_string_to_int",
			funcName: "mapOverrideType",
			interim:  map[string]interface{}{},
			newPath:  "spec.features.foo.bar",
			mapFuncArgs: []interface{}{
				map[string]interface{}{
					"newPath": "spec.features.foo.bar",
					"newType": "int",
				},
			},
			pathVal: "8080",
			expectedMap: map[string]interface{}{
				"spec.features.foo.bar": 8080,
			},
		},
		{
			name:     "mapHealthPortWithProbes_no_probes_defined_in_source",
			funcName: "mapHealthPortWithProbes",
			interim:  map[string]interface{}{},
			newPath:  "spec.override.clusterAgent.containers.cluster-agent.healthPort",
			mapFuncArgs: []interface{}{
				map[string]interface{}{
					"sourcePrefix":  "clusterAgent",
					"containerPath": "spec.override.clusterAgent.containers.cluster-agent",
				},
			},
			pathVal: 9999,
			sourceValues: chartutil.Values{
				// No probe ports defined in Helm source
				"clusterAgent": map[string]interface{}{
					"healthPort": 9999,
				},
			},
			expectedMap: map[string]interface{}{},
		},
		{
			name:     "mapHealthPortWithProbes_all_probes_match",
			funcName: "mapHealthPortWithProbes",
			interim:  map[string]interface{}{},
			newPath:  "spec.override.clusterAgent.containers.cluster-agent.healthPort",
			mapFuncArgs: []interface{}{
				map[string]interface{}{
					"sourcePrefix":  "clusterAgent",
					"containerPath": "spec.override.clusterAgent.containers.cluster-agent",
				},
			},
			pathVal: 9999,
			sourceValues: chartutil.Values{
				"clusterAgent": map[string]interface{}{
					"healthPort": 9999,
					"livenessProbe": map[string]interface{}{
						"httpGet": map[string]interface{}{"port": 9999},
					},
					"readinessProbe": map[string]interface{}{
						"httpGet": map[string]interface{}{"port": 9999},
					},
					"startupProbe": map[string]interface{}{
						"httpGet": map[string]interface{}{"port": 9999},
					},
				},
			},
			expectedMap: map[string]interface{}{
				// All probes match healthPort - map everything
				"spec.override.clusterAgent.containers.cluster-agent.healthPort":                  9999,
				"spec.override.clusterAgent.containers.cluster-agent.livenessProbe.httpGet.port":  9999,
				"spec.override.clusterAgent.containers.cluster-agent.readinessProbe.httpGet.port": 9999,
				"spec.override.clusterAgent.containers.cluster-agent.startupProbe.httpGet.port":   9999,
			},
		},
		{
			name:     "mapHealthPortWithProbes_partial_probes_no_mapping",
			funcName: "mapHealthPortWithProbes",
			interim:  map[string]interface{}{},
			newPath:  "spec.override.nodeAgent.containers.agent.healthPort",
			mapFuncArgs: []interface{}{
				map[string]interface{}{
					"sourcePrefix":  "agents.containers.agent",
					"containerPath": "spec.override.nodeAgent.containers.agent",
				},
			},
			pathVal: float64(8888), // YAML often parses numbers as float64
			sourceValues: chartutil.Values{
				"agents": map[string]interface{}{
					"containers": map[string]interface{}{
						"agent": map[string]interface{}{
							"healthPort": 8888,
							// Only readinessProbe defined - missing livenessProbe and startupProbe
							"readinessProbe": map[string]interface{}{
								"httpGet": map[string]interface{}{
									"port": 8888,
								},
							},
						},
					},
				},
			},
			// No mapping since not all probes are defined
			expectedMap: map[string]interface{}{},
		},
		{
			name:     "mapHealthPortWithProbes_all_probes_defined_one_mismatch",
			funcName: "mapHealthPortWithProbes",
			interim:  map[string]interface{}{},
			newPath:  "spec.override.clusterAgent.containers.cluster-agent.healthPort",
			mapFuncArgs: []interface{}{
				map[string]interface{}{
					"sourcePrefix":  "clusterAgent",
					"containerPath": "spec.override.clusterAgent.containers.cluster-agent",
				},
			},
			pathVal: 8888,
			sourceValues: chartutil.Values{
				"clusterAgent": map[string]interface{}{
					"healthPort": 8888,
					"livenessProbe": map[string]interface{}{
						"httpGet": map[string]interface{}{"port": 8888},
					},
					"readinessProbe": map[string]interface{}{
						"httpGet": map[string]interface{}{"port": 8888},
					},
					"startupProbe": map[string]interface{}{
						// Mismatched port - doesn't match healthPort
						"httpGet": map[string]interface{}{"port": 8877},
					},
				},
			},
			// No mapping since startupProbe.httpGet.port (8877) doesn't match healthPort (8888)
			expectedMap: map[string]interface{}{},
		},
		// mapTraceAgentLivenessProbe tests
		{
			name:        "mapTraceAgentLivenessProbe_no_custom_probe_type_socket_enabled",
			funcName:    "mapTraceAgentLivenessProbe",
			interim:     map[string]interface{}{},
			newPath:     "spec.override.nodeAgent.containers.trace-agent.livenessProbe",
			mapFuncArgs: []interface{}{},
			pathVal: map[string]interface{}{
				"initialDelaySeconds": 15,
				"periodSeconds":       15,
				"timeoutSeconds":      5,
			},
			sourceValues: chartutil.Values{
				"datadog": map[string]interface{}{
					"apm": map[string]interface{}{
						"port":          8126,
						"socketEnabled": true,
					},
				},
			},
			expectedMap: map[string]interface{}{
				// Should add tcpSocket.port from apmPort, plus probe settings
				"spec.override.nodeAgent.containers.trace-agent.livenessProbe.initialDelaySeconds": 15,
				"spec.override.nodeAgent.containers.trace-agent.livenessProbe.periodSeconds":       15,
				"spec.override.nodeAgent.containers.trace-agent.livenessProbe.timeoutSeconds":      5,
				"spec.override.nodeAgent.containers.trace-agent.livenessProbe.tcpSocket.port":      8126,
			},
		},
		{
			name:        "mapTraceAgentLivenessProbe_no_custom_probe_type_port_enabled",
			funcName:    "mapTraceAgentLivenessProbe",
			interim:     map[string]interface{}{},
			newPath:     "spec.override.nodeAgent.containers.trace-agent.livenessProbe",
			mapFuncArgs: []interface{}{},
			pathVal: map[string]interface{}{
				"initialDelaySeconds": 15,
			},
			sourceValues: chartutil.Values{
				"datadog": map[string]interface{}{
					"apm": map[string]interface{}{
						"port":        8126,
						"portEnabled": true,
					},
				},
			},
			expectedMap: map[string]interface{}{
				// Should add tcpSocket.port from apmPort, plus probe settings
				"spec.override.nodeAgent.containers.trace-agent.livenessProbe.initialDelaySeconds": 15,
				"spec.override.nodeAgent.containers.trace-agent.livenessProbe.tcpSocket.port":      8126,
			},
		},
		{
			name:        "mapTraceAgentLivenessProbe_tcpSocket_default_port",
			funcName:    "mapTraceAgentLivenessProbe",
			interim:     map[string]interface{}{},
			newPath:     "spec.override.nodeAgent.containers.trace-agent.livenessProbe",
			mapFuncArgs: []interface{}{},
			pathVal: map[string]interface{}{
				"tcpSocket": map[string]interface{}{
					"port": 8126,
				},
			},
			sourceValues: chartutil.Values{
				"datadog": map[string]interface{}{
					"apm": map[string]interface{}{
						"port": 8126,
					},
				},
			},
			expectedMap: map[string]interface{}{
				// tcpSocket.port matches default - map directly
				"spec.override.nodeAgent.containers.trace-agent.livenessProbe": map[string]interface{}{
					"tcpSocket": map[string]interface{}{
						"port": 8126,
					},
				},
			},
		},
		{
			name:        "mapTraceAgentLivenessProbe_tcpSocket_custom_port_matches_apm",
			funcName:    "mapTraceAgentLivenessProbe",
			interim:     map[string]interface{}{},
			newPath:     "spec.override.nodeAgent.containers.trace-agent.livenessProbe",
			mapFuncArgs: []interface{}{},
			pathVal: map[string]interface{}{
				"tcpSocket": map[string]interface{}{
					"port": 9090,
				},
			},
			sourceValues: chartutil.Values{
				"datadog": map[string]interface{}{
					"apm": map[string]interface{}{
						"port": 9090, // Explicit apm.port matches tcpSocket.port
					},
				},
			},
			expectedMap: map[string]interface{}{
				// tcpSocket.port matches explicit apm.port - map directly
				"spec.override.nodeAgent.containers.trace-agent.livenessProbe": map[string]interface{}{
					"tcpSocket": map[string]interface{}{
						"port": 9090,
					},
				},
			},
		},
		{
			name:        "mapTraceAgentLivenessProbe_tcpSocket_mismatch_no_mapping",
			funcName:    "mapTraceAgentLivenessProbe",
			interim:     map[string]interface{}{},
			newPath:     "spec.override.nodeAgent.containers.trace-agent.livenessProbe",
			mapFuncArgs: []interface{}{},
			pathVal: map[string]interface{}{
				"tcpSocket": map[string]interface{}{
					"port": 9999, // Doesn't match apm.port or default
				},
			},
			sourceValues: chartutil.Values{
				"datadog": map[string]interface{}{
					"apm": map[string]interface{}{
						"port": 8126,
					},
				},
			},
			// No mapping - tcpSocket.port doesn't match apm.port or default
			expectedMap: map[string]interface{}{},
		},
		{
			name:        "mapTraceAgentLivenessProbe_no_tcpSocket_deduce_from_apm_port",
			funcName:    "mapTraceAgentLivenessProbe",
			interim:     map[string]interface{}{},
			newPath:     "spec.override.nodeAgent.containers.trace-agent.livenessProbe",
			mapFuncArgs: []interface{}{},
			pathVal: map[string]interface{}{
				"initialDelaySeconds": 15,
			},
			sourceValues: chartutil.Values{
				"datadog": map[string]interface{}{
					"apm": map[string]interface{}{
						"port": 9000, // Custom apm.port
					},
				},
			},
			expectedMap: map[string]interface{}{
				// tcpSocket.port not set - deduce from apm.port
				"spec.override.nodeAgent.containers.trace-agent.livenessProbe.initialDelaySeconds": 15,
				"spec.override.nodeAgent.containers.trace-agent.livenessProbe.tcpSocket.port":      9000,
			},
		},
		{
			name:        "mapTraceAgentLivenessProbe_no_tcpSocket_use_default",
			funcName:    "mapTraceAgentLivenessProbe",
			interim:     map[string]interface{}{},
			newPath:     "spec.override.nodeAgent.containers.trace-agent.livenessProbe",
			mapFuncArgs: []interface{}{},
			pathVal: map[string]interface{}{
				"initialDelaySeconds": 15,
			},
			sourceValues: chartutil.Values{
				"datadog": map[string]interface{}{
					"apm": map[string]interface{}{}, // port not set - use default 8126
				},
			},
			expectedMap: map[string]interface{}{
				// tcpSocket.port not set and no apm.port - use default 8126
				"spec.override.nodeAgent.containers.trace-agent.livenessProbe.initialDelaySeconds": 15,
				"spec.override.nodeAgent.containers.trace-agent.livenessProbe.tcpSocket.port":      8126,
			},
		},
	}

	mapFuncs := mapFuncRegistry()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapFunc := mapFuncs[tt.funcName]
			require.NotNil(t, mapFunc, "Mapping function %s should exist in registry", tt.funcName)
			// Pass sourceValues (may be nil for processors that don't use it)
			sourceVals := tt.sourceValues
			if sourceVals == nil {
				sourceVals = chartutil.Values{}
			}
			mapFunc(tt.interim, tt.newPath, tt.pathVal, tt.mapFuncArgs, sourceVals)

			assert.Equal(t, tt.expectedMap, tt.interim)
		})
	}

	t.Run("non_existent_function", func(t *testing.T) {
		runFunc := mapFuncRegistry()["nonExistentFunc"]
		assert.Nil(t, runFunc, "Non-existent function should not be in registry")
	})
}
