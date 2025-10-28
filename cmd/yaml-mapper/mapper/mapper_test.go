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
	"helm.sh/helm/v3/pkg/chartutil"
)

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
