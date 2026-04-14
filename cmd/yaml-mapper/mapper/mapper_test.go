// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package mapper

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-operator/cmd/yaml-mapper/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chartutil"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name          string
		sourcePath    string
		destPath      string
		expectedError string
	}{
		{
			name:          "source_values_no_mapping_errors",
			sourcePath:    "testdata/values_no_errors.yaml",
			destPath:      "testdata/dda_no_errors.yaml",
			expectedError: "",
		},
		{
			name:          "source_values_with_mapping_errors",
			sourcePath:    "testdata/values_errors.yaml",
			destPath:      "testdata/dda_errors.yaml",
			expectedError: "mapping completed with 4 error(s): the mapped DDA may contain misconfigurations",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapper := NewMapper(MapConfig{
				MappingPath: "mapping_datadog_helm_to_datadogagent_crd.yaml",
				SourcePath:  tt.sourcePath,
				DestPath:    tt.destPath,
			})

			err := mapper.Run()
			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestRunConsecutive verifies that running the mapper consecutively
// with different YAML inputs does not pollute state between runs.
func TestRunConsecutive(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name         string
		namespace    string
		inputValues  string
		expectedDDA  map[string]any
		missingPaths []string
	}{
		{
			name: "run_1: no namespace",
			inputValues: `nameOverride: "first-dda-name"
datadog:
  site: "datadoghq.com"
`,
			namespace: "namespace-one",
			expectedDDA: map[string]any{
				"metadata.name":      "first-dda-name",
				"metadata.namespace": "namespace-one",
				"spec.global.site":   "datadoghq.com",
			},
		},
		{
			name: "run_2: namespace",
			inputValues: `nameOverride: "second-dda-name"
datadog:
  site: "datadoghq.eu"
`,
			expectedDDA: map[string]any{
				"metadata.name":    "second-dda-name",
				"spec.global.site": "datadoghq.eu",
			},
			missingPaths: []string{"metadata.namespace"},
		},
		{
			name: "run_3: namespace and overrides",
			inputValues: `nameOverride: "third-dda-name"
datadog:
  site: "us5.datadoghq.com"
  logLevel: "debug"
`,
			namespace: "namespace-three",
			expectedDDA: map[string]any{
				"metadata.name":        "third-dda-name",
				"metadata.namespace":   "namespace-three",
				"spec.global.site":     "us5.datadoghq.com",
				"spec.global.logLevel": "debug",
			},
		},
	}

	for i, tt := range tests {
		valuesPath := filepath.Join(tempDir, fmt.Sprintf("values-%d.yaml", i+1))
		ddaPath := filepath.Join(tempDir, fmt.Sprintf("dda-%d.yaml", i+1))

		writeTestFile(t, valuesPath, tt.inputValues)

		mapper := NewMapper(MapConfig{
			MappingPath: "mapping_datadog_helm_to_datadogagent_crd.yaml",
			SourcePath:  valuesPath,
			DestPath:    ddaPath,
			Namespace:   tt.namespace,
		})
		err := mapper.Run()
		require.NoError(t, err, "run %s failed", tt.name)

		dda, err := chartutil.ReadValuesFile(ddaPath)
		require.NoError(t, err, "run %s failed to read output", tt.name)
		assertValues(t, dda, tt.expectedDDA)
		for _, missingPath := range tt.missingPaths {
			assertMissingPath(t, dda, missingPath, "run %s should not contain %s", tt.name, missingPath)
		}
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
}

func assertValues(t *testing.T, values chartutil.Values, expected map[string]any) {
	t.Helper()
	for path, want := range expected {
		got, err := values.PathValue(path)
		require.NoError(t, err, "expected path %q to exist", path)
		assert.Equal(t, want, got, "unexpected value at path %q", path)
	}
}

func assertMissingPath(t *testing.T, values chartutil.Values, path string, msgAndArgs ...any) {
	t.Helper()
	_, err := values.PathValue(path)
	assert.Error(t, err, msgAndArgs...)
}

func TestMergeMapDeep(t *testing.T) {
	tests := []struct {
		name     string
		map1     map[string]any
		map2     map[string]any
		expected map[string]any
	}{
		{
			name: "merge non-overlapping maps",
			map1: map[string]any{
				"key1": "value1",
				"key2": 42,
			},
			map2: map[string]any{
				"key3": "value3",
				"key4": []string{"a", "b"},
			},
			expected: map[string]any{
				"key1": "value1",
				"key2": 42,
				"key3": "value3",
				"key4": []string{"a", "b"},
			},
		},
		{
			name: "merge overlapping maps with simple values (map2 overwrites map1)",
			map1: map[string]any{
				"key1": "value1",
				"key2": 42,
			},
			map2: map[string]any{
				"key1": "newvalue1",
				"key3": "value3",
			},
			expected: map[string]any{
				"key1": "newvalue1",
				"key2": 42,
				"key3": "value3",
			},
		},
		{
			name: "merge nested maps",
			map1: map[string]any{
				"config": map[string]any{
					"database": map[string]any{
						"host": "localhost",
						"port": 5432,
					},
					"cache": map[string]any{
						"enabled": true,
					},
				},
				"version": "1.0",
			},
			map2: map[string]any{
				"config": map[string]any{
					"database": map[string]any{
						"port":     3306,
						"username": "admin",
					},
					"logging": map[string]any{
						"level": "debug",
					},
				},
				"environment": "production",
			},
			expected: map[string]any{
				"config": map[string]any{
					"database": map[string]any{
						"host":     "localhost",
						"port":     3306,
						"username": "admin",
					},
					"cache": map[string]any{
						"enabled": true,
					},
					"logging": map[string]any{
						"level": "debug",
					},
				},
				"version":     "1.0",
				"environment": "production",
			},
		},
		{
			name: "one map is empty",
			map1: map[string]any{
				"key1": "value1",
			},
			map2: map[string]any{},
			expected: map[string]any{
				"key1": "value1",
			},
		},
		{
			name:     "both maps are empty",
			map1:     map[string]any{},
			map2:     map[string]any{},
			expected: map[string]any{},
		},
		{
			name: "mixed value types",
			map1: map[string]any{
				"string":  "text",
				"number":  123,
				"boolean": true,
				"array":   []any{1, 2, 3},
				"nested": map[string]any{
					"inner": "value",
				},
			},
			map2: map[string]any{
				"string": "newtext",
				"float":  3.14,
				"nested": map[string]any{
					"additional": "data",
				},
			},
			expected: map[string]any{
				"string":  "newtext",
				"number":  123,
				"boolean": true,
				"array":   []any{1, 2, 3},
				"float":   3.14,
				"nested": map[string]any{
					"inner":      "value",
					"additional": "data",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			map1Copy := make(map[string]any)
			maps.Copy(map1Copy, tt.map1)
			map2Copy := make(map[string]any)
			maps.Copy(map2Copy, tt.map2)

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
		val      any
		mapName  map[string]any
		expected map[string]any
	}{
		{
			name:    "simple single level path",
			path:    "key",
			val:     "value",
			mapName: map[string]any{},
			expected: map[string]any{
				"key": "value",
			},
		},
		{
			name:    "three level nested path",
			path:    "spec.global.site",
			val:     "datadoghq.com",
			mapName: map[string]any{},
			expected: map[string]any{
				"spec": map[string]any{
					"global": map[string]any{
						"site": "datadoghq.com",
					},
				},
			},
		},
		{
			name:    "deep nested path",
			path:    "spec.override.nodeAgent.containers.agent.resources.limits.memory",
			val:     "512Mi",
			mapName: map[string]any{},
			expected: map[string]any{
				"spec": map[string]any{
					"override": map[string]any{
						"nodeAgent": map[string]any{
							"containers": map[string]any{
								"agent": map[string]any{
									"resources": map[string]any{
										"limits": map[string]any{
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
			mapName: map[string]any{
				"metadata": map[string]any{
					"name": "datadog",
				},
			},
			expected: map[string]any{
				"metadata": map[string]any{
					"name": "datadog",
				},
				"spec": map[string]any{
					"global": map[string]any{
						"site": "datadoghq.com",
					},
				},
			},
		},
		{
			name: "merge with existing map - overlapping paths",
			path: "spec.global.logLevel",
			val:  "debug",
			mapName: map[string]any{
				"spec": map[string]any{
					"global": map[string]any{
						"site": "datadoghq.com",
					},
					"features": map[string]any{
						"apm": map[string]any{
							"enabled": true,
						},
					},
				},
			},
			expected: map[string]any{
				"spec": map[string]any{
					"global": map[string]any{
						"site":     "datadoghq.com",
						"logLevel": "debug",
					},
					"features": map[string]any{
						"apm": map[string]any{
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
			mapName: map[string]any{
				"spec": map[string]any{
					"global": map[string]any{
						"site": "datadoghq.com",
					},
				},
			},
			expected: map[string]any{
				"spec": map[string]any{
					"global": map[string]any{
						"site": "datadoghq.eu",
					},
				},
			},
		},
		{
			name:    "empty path",
			path:    "",
			val:     "",
			mapName: map[string]any{},
			expected: map[string]any{
				"": "",
			},
		},
		{
			name:    "different value types - integer",
			path:    "spec.override.clusterAgent.replicas",
			val:     3,
			mapName: map[string]any{},
			expected: map[string]any{
				"spec": map[string]any{
					"override": map[string]any{
						"clusterAgent": map[string]any{
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
			mapName: map[string]any{},
			expected: map[string]any{
				"spec": map[string]any{
					"features": map[string]any{
						"apm": map[string]any{
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
			mapName: map[string]any{},
			expected: map[string]any{
				"spec": map[string]any{
					"global": map[string]any{
						"tags": []string{"env:prod", "team:backend"},
					},
				},
			},
		},
		{
			name:    "different value types - map",
			path:    "spec.override.nodeAgent.resources",
			val:     map[string]any{"limits": map[string]any{"memory": "1Gi"}},
			mapName: map[string]any{},
			expected: map[string]any{
				"spec": map[string]any{
					"override": map[string]any{
						"nodeAgent": map[string]any{
							"resources": map[string]any{
								"limits": map[string]any{
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
			mapNameCopy := make(map[string]any)
			maps.Copy(mapNameCopy, tt.mapName)

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
		mapName := map[string]any{}
		result := utils.InsertAtPath("spec.global.site", nil, mapName)

		expected := map[string]any{
			"spec": map[string]any{
				"global": map[string]any{
					"site": nil,
				},
			},
		}
		assert.Equal(t, expected, result)
	})

	t.Run("path_with_multiple_dots", func(t *testing.T) {
		mapName := map[string]any{}
		result := utils.InsertAtPath("a.b.c.d.e.f", "deep_value", mapName)

		expected := map[string]any{
			"a": map[string]any{
				"b": map[string]any{
					"c": map[string]any{
						"d": map[string]any{
							"e": map[string]any{
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
		mapName := map[string]any{}
		result := utils.InsertAtPath("spec.containers.0.name", "agent", mapName)

		expected := map[string]any{
			"spec": map[string]any{
				"containers": map[string]any{
					"0": map[string]any{
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
		interim     map[string]any
		key         string
		val         any
		wantInterim map[string]any
	}{
		{
			name:    "simple set",
			interim: map[string]any{},
			key:     "foo.bar",
			val:     "true",
			wantInterim: map[string]any{
				"foo.bar": "true",
			},
		},
		{
			name: "simple override",
			interim: map[string]any{
				"foo.bar": "false",
			},
			key: "foo.bar",
			val: "true",
			wantInterim: map[string]any{
				"foo.bar": "true",
			},
		},
		{
			name: "simple merge",
			interim: map[string]any{
				"foo.bar": "true",
			},
			key: "bar.foo",
			val: "true",
			wantInterim: map[string]any{
				"foo.bar": "true",
				"bar.foo": "true",
			},
		},
		{
			name:    "set map",
			interim: map[string]any{},
			key:     "bar.foo",
			val: map[string]any{
				"foo": "bar",
			},
			wantInterim: map[string]any{
				"bar.foo": map[string]any{
					"foo": "bar",
				},
			},
		},
		{
			name: "merge maps at same key (non-overlapping)",
			interim: map[string]any{
				"spec.global": map[string]any{"site": "datadoghq.com"},
			},
			key: "spec.global",
			val: map[string]any{"logLevel": "debug"},
			wantInterim: map[string]any{
				"spec.global": map[string]any{
					"site":     "datadoghq.com",
					"logLevel": "debug",
				},
			},
		},
		{
			name: "deep-merge nested maps",
			interim: map[string]any{
				"spec.features": map[string]any{
					"apm": map[string]any{"enabled": true},
				},
			},
			key: "spec.features",
			val: map[string]any{
				"apm":  map[string]any{"portEnabled": true},
				"usm":  map[string]any{"enabled": true},
				"apm2": map[string]any{"foo": "bar"},
			},
			wantInterim: map[string]any{
				"spec.features": map[string]any{
					"apm": map[string]any{
						"enabled":     true,
						"portEnabled": true,
					},
					"usm":  map[string]any{"enabled": true},
					"apm2": map[string]any{"foo": "bar"},
				},
			},
		},
		{
			name: "overwrite map with scalar",
			interim: map[string]any{
				"spec.global": map[string]any{"site": "datadoghq.com"},
			},
			key: "spec.global",
			val: "not-a-map-anymore",
			wantInterim: map[string]any{
				"spec.global": "not-a-map-anymore",
			},
		},
		{
			name: "overwrite scalar with map",
			interim: map[string]any{
				"spec.global": "string-value",
			},
			key: "spec.global",
			val: map[string]any{"site": "datadoghq.eu"},
			wantInterim: map[string]any{
				"spec.global": map[string]any{"site": "datadoghq.eu"},
			},
		},
		{
			name: "merge chartutil.Values into map",
			interim: map[string]any{
				"spec.global": map[string]any{"site": "datadoghq.com"},
			},
			key: "spec.global",
			val: chartutil.Values{"logLevel": "info"},
			wantInterim: map[string]any{
				"spec.global": map[string]any{
					"site":     "datadoghq.com",
					"logLevel": "info",
				},
			},
		},
		{
			name: "merge map into chartutil.Values",
			interim: map[string]any{
				"spec.global": chartutil.Values{"site": "datadoghq.com"},
			},
			key: "spec.global",
			val: map[string]any{"logLevel": "warn"},
			wantInterim: map[string]any{
				"spec.global": map[string]any{
					"site":     "datadoghq.com",
					"logLevel": "warn",
				},
			},
		},
		{
			name:        "nil value should be ignored (no set)",
			interim:     map[string]any{},
			key:         "spec.global.site",
			val:         nil,
			wantInterim: map[string]any{},
		},
		{
			name: "nil value should not override existing",
			interim: map[string]any{
				"spec.global.site": "datadoghq.com",
			},
			key: "spec.global.site",
			val: nil,
			wantInterim: map[string]any{
				"spec.global.site": "datadoghq.com",
			},
		},
		{
			name: "deep-merge overlapping nested keys",
			interim: map[string]any{
				"a": map[string]any{
					"b": map[string]any{"x": 1},
				},
			},
			key: "a",
			val: map[string]any{
				"b": map[string]any{"y": 2},
			},
			wantInterim: map[string]any{
				"a": map[string]any{
					"b": map[string]any{"x": 1, "y": 2},
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
				"datadog": map[string]any{
					"apm": map[string]any{
						"enabled": true,
					},
				},
			},
			wantVals: chartutil.Values{
				"datadog": map[string]any{
					"apm": map[string]any{
						"portEnabled": true,
					},
				},
			},
		},
		{
			name: "bool OR: both standard and deprecated present",
			sourceVals: chartutil.Values{
				"datadog": map[string]any{
					"apm": map[string]any{
						"enabled":     true,
						"portEnabled": true,
					},
				},
			},
			wantVals: chartutil.Values{
				"datadog": map[string]any{
					"apm": map[string]any{
						"portEnabled": true,
					},
				},
			},
		},
		{
			name: "bool OR: both standard and deprecated present, standard takes precedence",
			sourceVals: chartutil.Values{
				"datadog": map[string]any{
					"apm": map[string]any{
						"enabled":     false,
						"portEnabled": true,
					},
				},
			},
			wantVals: chartutil.Values{
				"datadog": map[string]any{
					"apm": map[string]any{
						"portEnabled": true,
					},
				},
			},
		},
		{
			name: "bool OR: standard false and deprecated true, truthy takes precedence",
			sourceVals: chartutil.Values{
				"datadog": map[string]any{
					"apm": map[string]any{
						"enabled":     true,
						"portEnabled": false,
					},
				},
			},
			wantVals: chartutil.Values{
				"datadog": map[string]any{
					"apm": map[string]any{
						"portEnabled": true,
					},
				},
			},
		},
		{
			name: "bool OR: multiple deprecated candidates - simple",
			sourceVals: chartutil.Values{
				"agents": map[string]any{
					"networkPolicy": map[string]any{
						"create": true,
					},
				},
			},
			wantVals: chartutil.Values{
				"datadog": map[string]any{
					"networkPolicy": map[string]any{
						"create": true,
					},
				},
				"agents": map[string]any{
					"networkPolicy": map[string]any{},
				},
			},
		},
		{
			name: "bool OR: multiple deprecated candidates - complex",
			sourceVals: chartutil.Values{
				"agents": map[string]any{
					"networkPolicy": map[string]any{
						"create": true,
					},
				},
				"clusterAgent": map[string]any{
					"networkPolicy": map[string]any{
						"create": false,
					},
				},
			},
			wantVals: chartutil.Values{
				"datadog": map[string]any{
					"networkPolicy": map[string]any{
						"create": true,
					},
				},
				"agents": map[string]any{
					"networkPolicy": map[string]any{},
				},
				"clusterAgent": map[string]any{
					"networkPolicy": map[string]any{},
				},
			},
		},
		{
			name: "bool OR: multiple deprecated candidates - complex w/extra keys",
			sourceVals: chartutil.Values{
				"agents": map[string]any{
					"networkPolicy": map[string]any{
						"create": true,
						"flavor": "cilium",
					},
				},
				"clusterAgent": map[string]any{
					"networkPolicy": map[string]any{
						"create": false,
						"flavor": "cilium",
						"cilium": map[string]any{
							"dnsSelector": map[string]any{
								"foo": "bar",
							},
						},
					},
				},
			},
			wantVals: chartutil.Values{
				"datadog": map[string]any{
					"networkPolicy": map[string]any{
						"create": true,
					},
				},
				"agents": map[string]any{
					"networkPolicy": map[string]any{
						"flavor": "cilium",
					},
				},
				"clusterAgent": map[string]any{
					"networkPolicy": map[string]any{
						"flavor": "cilium",
						"cilium": map[string]any{
							"dnsSelector": map[string]any{
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
				"datadog": map[string]any{
					"networkPolicy": map[string]any{
						"create": true,
					},
				},
				"agents": map[string]any{
					"networkPolicy": map[string]any{
						"create": false,
						"flavor": "cilium",
					},
				},
				"clusterAgent": map[string]any{
					"networkPolicy": map[string]any{
						"create": false,
						"flavor": "cilium",
						"cilium": map[string]any{
							"dnsSelector": map[string]any{
								"foo": "bar",
							},
						},
					},
				},
			},
			wantVals: chartutil.Values{
				"datadog": map[string]any{
					"networkPolicy": map[string]any{
						"create": true,
					},
				},
				"agents": map[string]any{
					"networkPolicy": map[string]any{
						"flavor": "cilium",
					},
				},
				"clusterAgent": map[string]any{
					"networkPolicy": map[string]any{
						"flavor": "cilium",
						"cilium": map[string]any{
							"dnsSelector": map[string]any{
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
				"datadog": map[string]any{
					"networkPolicy": map[string]any{
						"create": false,
					},
				},
				"agents": map[string]any{
					"networkPolicy": map[string]any{
						"create": true,
						"flavor": "cilium",
					},
				},
				"clusterAgent": map[string]any{
					"networkPolicy": map[string]any{
						"create": false,
						"flavor": "cilium",
						"cilium": map[string]any{
							"dnsSelector": map[string]any{
								"foo": "bar",
							},
						},
					},
				},
			},
			wantVals: chartutil.Values{
				"datadog": map[string]any{
					"networkPolicy": map[string]any{
						"create": true,
					},
				},
				"agents": map[string]any{
					"networkPolicy": map[string]any{
						"flavor": "cilium",
					},
				},
				"clusterAgent": map[string]any{
					"networkPolicy": map[string]any{
						"flavor": "cilium",
						"cilium": map[string]any{
							"dnsSelector": map[string]any{
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
				"datadog": map[string]any{
					"systemProbe": map[string]any{
						"enableDefaultOsReleasePaths": true,
					},
				},
			},
			wantVals: chartutil.Values{
				"datadog": map[string]any{
					"systemProbe":                  map[string]any{},
					"disableDefaultOsReleasePaths": false,
				},
			},
		},
		{
			name: "bool negation: standard false and deprecated false - standard should take precedence",
			sourceVals: chartutil.Values{
				"datadog": map[string]any{
					"systemProbe": map[string]any{
						"enableDefaultOsReleasePaths": false,
					},
					"disableDefaultOsReleasePaths": false,
				},
			},
			wantVals: chartutil.Values{
				"datadog": map[string]any{
					"systemProbe":                  map[string]any{},
					"disableDefaultOsReleasePaths": false,
				},
			},
		},
		{
			name: "bool negation: standard true and deprecated true - standard takes precedence",
			sourceVals: chartutil.Values{
				"datadog": map[string]any{
					"systemProbe": map[string]any{
						"enableDefaultOsReleasePaths": true,
					},
					"disableDefaultOsReleasePaths": true,
				},
			},
			wantVals: chartutil.Values{
				"datadog": map[string]any{
					"systemProbe":                  map[string]any{},
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
		expectedFuncs := []string{"mapSecretKeyName", "mapSeccompProfile", "mapSystemProbeAppArmor", "mapLocalServiceName", "mapAppendEnvVar", "mapMergeEnvs", "mapOverrideType"}
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
		name        string
		funcName    string
		interim     map[string]any
		newPath     string
		pathVal     any
		mapFuncArgs []any
		expectedMap map[string]any
	}{
		// mapSecretKeyName tests
		{
			name:     "mapSecretKeyName_apiSecret_empty_map",
			funcName: "mapSecretKeyName",
			interim:  map[string]any{},
			newPath:  "spec.global.credentials.apiSecret.secretName",
			pathVal:  "my-api-secret",
			mapFuncArgs: []any{
				map[string]any{
					"keyName":     "api-key",
					"keyNamePath": "spec.global.credentials.apiSecret.keyName",
				},
			},
			expectedMap: map[string]any{
				"spec.global.credentials.apiSecret.secretName": "my-api-secret",
				"spec.global.credentials.apiSecret.keyName":    "api-key",
			},
		},
		{
			name:     "mapSecretKeyName_apiSecret_existing_map",
			funcName: "mapSecretKeyName",
			interim: map[string]any{
				"spec.global.site":      "datadoghq.com",
				"spec.agent.image.name": "datadog/agent",
			},
			newPath: "spec.global.credentials.apiSecret.secretName",
			pathVal: "datadog-api-secret",
			mapFuncArgs: []any{
				map[string]any{
					"keyName":     "api-key",
					"keyNamePath": "spec.global.credentials.apiSecret.keyName",
				},
			},
			expectedMap: map[string]any{
				"spec.global.site":                             "datadoghq.com",
				"spec.agent.image.name":                        "datadog/agent",
				"spec.global.credentials.apiSecret.secretName": "datadog-api-secret",
				"spec.global.credentials.apiSecret.keyName":    "api-key",
			},
		},
		{
			name:     "mapSecretKeyName_apiSecret_overwrite",
			funcName: "mapSecretKeyName",
			interim: map[string]any{
				"spec.global.credentials.apiSecret.secretName": "old-secret",
				"spec.global.credentials.apiSecret.keyName":    "old-key",
			},
			newPath: "spec.global.credentials.apiSecret.secretName",
			pathVal: "new-api-secret",
			mapFuncArgs: []any{
				map[string]any{
					"keyName":     "api-key",
					"keyNamePath": "spec.global.credentials.apiSecret.keyName",
				},
			},
			expectedMap: map[string]any{
				"spec.global.credentials.apiSecret.secretName": "new-api-secret",
				"spec.global.credentials.apiSecret.keyName":    "api-key",
			},
		},
		{
			name:     "mapSecretKeyName_appSecret_empty_map",
			funcName: "mapSecretKeyName",
			interim:  map[string]any{},
			newPath:  "spec.global.credentials.appSecret.secretName",
			pathVal:  "my-app-secret",
			mapFuncArgs: []any{
				map[string]any{
					"keyName":     "app-key",
					"keyNamePath": "spec.global.credentials.appSecret.keyName",
				},
			},
			expectedMap: map[string]any{
				"spec.global.credentials.appSecret.secretName": "my-app-secret",
				"spec.global.credentials.appSecret.keyName":    "app-key",
			},
		},
		{
			name:     "mapSecretKeyName_app_secret_with_existing_api_secret",
			funcName: "mapSecretKeyName",
			interim: map[string]any{
				"spec.global.credentials.apiSecret.secretName": "api-secret",
				"spec.global.credentials.apiSecret.keyName":    "api-key",
			},
			newPath: "spec.global.credentials.appSecret.secretName",
			pathVal: "datadog-app-secret",
			mapFuncArgs: []any{
				map[string]any{
					"keyName":     "app-key",
					"keyNamePath": "spec.global.credentials.appSecret.keyName",
				},
			},
			expectedMap: map[string]any{
				"spec.global.credentials.apiSecret.secretName": "api-secret",
				"spec.global.credentials.apiSecret.keyName":    "api-key",
				"spec.global.credentials.appSecret.secretName": "datadog-app-secret",
				"spec.global.credentials.appSecret.keyName":    "app-key",
			},
		},
		{
			name:     "mapSecretKeyName_appSecret_overwrite",
			funcName: "mapSecretKeyName",
			interim: map[string]any{
				"spec.global.credentials.appSecret.secretName": "old-app-secret",
				"spec.global.credentials.appSecret.keyName":    "old-app-key",
			},
			newPath: "spec.global.credentials.appSecret.secretName",
			pathVal: "new-app-secret",
			mapFuncArgs: []any{
				map[string]any{
					"keyName":     "app-key",
					"keyNamePath": "spec.global.credentials.appSecret.keyName",
				},
			},
			expectedMap: map[string]any{
				"spec.global.credentials.appSecret.secretName": "new-app-secret",
				"spec.global.credentials.appSecret.keyName":    "app-key",
			},
		},
		{
			name:     "mapSecretKeyName_tokenSecret_empty_map",
			funcName: "mapSecretKeyName",
			interim:  map[string]any{},
			newPath:  "spec.global.clusterAgentTokenSecret.secretName",
			pathVal:  "my-token-secret",
			mapFuncArgs: []any{
				map[string]any{
					"keyName":     "token",
					"keyNamePath": "spec.global.clusterAgentTokenSecret.keyName",
				},
			},
			expectedMap: map[string]any{
				"spec.global.clusterAgentTokenSecret.secretName": "my-token-secret",
				"spec.global.clusterAgentTokenSecret.keyName":    "token",
			},
		},
		{
			name:     "mapSecretKeyName_tokenSecret_with_existing_secrets",
			funcName: "mapSecretKeyName",
			interim: map[string]any{
				"spec.global.credentials.apiSecret.secretName": "api-secret",
				"spec.global.credentials.appSecret.secretName": "app-secret",
			},
			newPath: "spec.global.clusterAgentTokenSecret.secretName",
			mapFuncArgs: []any{
				map[string]any{
					"keyName":     "token",
					"keyNamePath": "spec.global.clusterAgentTokenSecret.keyName",
				},
			},
			pathVal: "cluster-agent-token",
			expectedMap: map[string]any{
				"spec.global.credentials.apiSecret.secretName":   "api-secret",
				"spec.global.credentials.appSecret.secretName":   "app-secret",
				"spec.global.clusterAgentTokenSecret.secretName": "cluster-agent-token",
				"spec.global.clusterAgentTokenSecret.keyName":    "token",
			},
		},
		{
			name:     "mapSecretKeyName_tokenSecret_Key_overwrite",
			funcName: "mapSecretKeyName",
			interim: map[string]any{
				"spec.global.clusterAgentTokenSecret.secretName": "old-token-secret",
				"spec.global.clusterAgentTokenSecret.keyName":    "old-token",
			},
			newPath: "spec.global.clusterAgentTokenSecret.secretName",
			pathVal: "new-token-secret",
			mapFuncArgs: []any{
				map[string]any{
					"keyName":     "token",
					"keyNamePath": "spec.global.clusterAgentTokenSecret.keyName",
				},
			},
			expectedMap: map[string]any{
				"spec.global.clusterAgentTokenSecret.secretName": "new-token-secret",
				"spec.global.clusterAgentTokenSecret.keyName":    "token",
			},
		},
		// mapSeccompProfile tests
		{
			name:     "mapSeccompProfile_localhost",
			funcName: "mapSeccompProfile",
			interim:  map[string]any{},
			newPath:  "spec.override.nodeAgent.containers.system-probe.securityContext.seccompProfile",
			pathVal:  "localhost/system-probe",
			expectedMap: map[string]any{
				"spec.override.nodeAgent.containers.system-probe.securityContext.seccompProfile.type":             "Localhost",
				"spec.override.nodeAgent.containers.system-probe.securityContext.seccompProfile.localhostProfile": "system-probe",
			},
		},
		{
			name:     "mapSeccompProfile_runtime_default",
			funcName: "mapSeccompProfile",
			interim:  map[string]any{},
			newPath:  "spec.override.nodeAgent.containers.system-probe.securityContext.seccompProfile",
			pathVal:  "runtime/default",
			expectedMap: map[string]any{
				"spec.override.nodeAgent.containers.system-probe.securityContext.seccompProfile.type": "RuntimeDefault",
			},
		},
		{
			name:     "mapSeccompProfile_unconfined",
			funcName: "mapSeccompProfile",
			interim:  map[string]any{},
			newPath:  "spec.override.nodeAgent.containers.system-probe.securityContext.seccompProfile",
			pathVal:  "unconfined",
			expectedMap: map[string]any{
				"spec.override.nodeAgent.containers.system-probe.securityContext.seccompProfile.type": "Unconfined",
			},
		},
		// mapSystemProbeAppArmor tests
		{
			name:     "mapSystemProbeAppArmor_no_features_enabled",
			funcName: "mapSystemProbeAppArmor",
			interim: map[string]any{
				"spec.features.cws.enabled": false,
				"spec.features.npm.enabled": false,
			},
			newPath: "spec.override.nodeAgent.containers.system-probe.appArmorProfile",
			pathVal: "unconfined",
			expectedMap: map[string]any{
				"spec.features.cws.enabled": false,
				"spec.features.npm.enabled": false,
			},
		},
		{
			name:     "mapSystemProbeAppArmor_multiple_features_enabled",
			funcName: "mapSystemProbeAppArmor",
			interim: map[string]any{
				"spec.features.cws.enabled":            true,
				"spec.features.npm.enabled":            false,
				"spec.features.tcpQueueLength.enabled": true,
			},
			newPath: "spec.override.nodeAgent.containers.system-probe.appArmorProfile",
			pathVal: "unconfined",
			expectedMap: map[string]any{
				"spec.features.cws.enabled":                                       true,
				"spec.features.npm.enabled":                                       false,
				"spec.features.tcpQueueLength.enabled":                            true,
				"spec.override.nodeAgent.containers.system-probe.appArmorProfile": "unconfined",
			},
		},
		{
			name:     "mapSystemProbeAppArmor_gpu_enabled_privileged",
			funcName: "mapSystemProbeAppArmor",
			interim: map[string]any{
				"spec.features.gpu.enabled":        true,
				"spec.features.gpu.privilegedMode": true,
			},
			newPath: "spec.override.nodeAgent.containers.system-probe.appArmorProfile",
			pathVal: "unconfined",
			expectedMap: map[string]any{
				"spec.features.gpu.enabled":                                       true,
				"spec.features.gpu.privilegedMode":                                true,
				"spec.override.nodeAgent.containers.system-probe.appArmorProfile": "unconfined",
			},
		},
		{
			name:     "mapSystemProbeAppArmor_gpu_enabled_not_privileged",
			funcName: "mapSystemProbeAppArmor",
			interim: map[string]any{
				"spec.features.gpu.enabled":        true,
				"spec.features.gpu.privilegedMode": false,
			},
			newPath: "spec.override.nodeAgent.containers.system-probe.appArmorProfile",
			pathVal: "unconfined",
			expectedMap: map[string]any{
				"spec.features.gpu.enabled":        true,
				"spec.features.gpu.privilegedMode": false,
			},
		},
		{
			name:     "mapSystemProbeAppArmor_empty_apparmor_value",
			funcName: "mapSystemProbeAppArmor",
			interim: map[string]any{
				"spec.features.cws.enabled": true,
			},
			newPath: "spec.override.nodeAgent.containers.system-probe.appArmorProfile",
			pathVal: "",
			expectedMap: map[string]any{
				"spec.features.cws.enabled": true,
			},
		},
		{
			name:     "mapSystemProbeAppArmor_invalid_apparmor_type",
			funcName: "mapSystemProbeAppArmor",
			interim: map[string]any{
				"spec.features.cws.enabled": true,
			},
			newPath: "spec.override.nodeAgent.containers.system-probe.appArmorProfile",
			pathVal: 123,
			expectedMap: map[string]any{
				"spec.features.cws.enabled": true,
			},
		},
		// mapLocalServiceName tests
		{
			name:        "mapLocalServiceName_empty_name",
			funcName:    "mapLocalServiceName",
			interim:     map[string]any{},
			newPath:     "spec.override.clusterAgent.config.external_metrics.local_service_name",
			pathVal:     "",
			expectedMap: map[string]any{},
		},
		{
			name:        "mapLocalServiceName_invalid_type",
			funcName:    "mapLocalServiceName",
			interim:     map[string]any{},
			newPath:     "spec.override.clusterAgent.config.external_metrics.local_service_name",
			pathVal:     123,
			expectedMap: map[string]any{},
		},
		{
			name:     "mapLocalServiceName_overwrite_existing",
			funcName: "mapLocalServiceName",
			interim: map[string]any{
				"spec.override.clusterAgent.config.external_metrics.local_service_name": "old-service",
			},
			newPath: "spec.override.clusterAgent.config.external_metrics.local_service_name",
			pathVal: "new-service",
			expectedMap: map[string]any{
				"spec.override.clusterAgent.config.external_metrics.local_service_name": "new-service",
			},
		},
		{
			name:     "mapAppendEnvVar_add_env_var",
			funcName: "mapAppendEnvVar",
			interim:  map[string]any{},
			newPath:  "spec.override.nodeAgent.containers.agent.env",
			pathVal:  "debug",
			mapFuncArgs: []any{
				map[string]any{
					"name": "DD_LOG_LEVEL",
				},
			},
			expectedMap: map[string]any{
				"spec.override.nodeAgent.containers.agent.env": []any{
					map[string]any{
						"name":  "DD_LOG_LEVEL",
						"value": "debug",
					},
				},
			},
		},
		{
			name:     "mapAppendEnvVar_add_to_existing_env_vars",
			funcName: "mapAppendEnvVar",
			interim: map[string]any{
				"spec.override.nodeAgent.containers.agent.env": []any{
					map[string]any{
						"name":  "EXISTING_VAR",
						"value": "existing_value",
					},
				},
			},
			newPath: "spec.override.nodeAgent.containers.agent.env",
			pathVal: "new_value",
			mapFuncArgs: []any{
				map[string]any{
					"name": "NEW_VAR",
				},
			},
			expectedMap: map[string]any{
				"spec.override.nodeAgent.containers.agent.env": []any{
					map[string]any{
						"name":  "EXISTING_VAR",
						"value": "existing_value",
					},
					map[string]any{
						"name":  "NEW_VAR",
						"value": "new_value",
					},
				},
			},
		},
		{
			name:     "mapAppendEnvVar_valueFrom",
			funcName: "mapAppendEnvVar",
			interim:  map[string]any{},
			newPath:  "spec.override.nodeAgent.env",
			pathVal: map[string]any{
				"valueFrom": map[string]any{
					"fieldRef": map[string]any{
						"fieldPath": "status.hostIP",
					},
				},
			},
			mapFuncArgs: []any{
				map[string]any{
					"name": "DD_KUBERNETES_KUBELET_HOST",
				},
			},
			expectedMap: map[string]any{
				"spec.override.nodeAgent.env": []any{
					map[string]any{
						"name": "DD_KUBERNETES_KUBELET_HOST",
						"valueFrom": map[string]any{
							"fieldRef": map[string]any{
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
			interim: map[string]any{
				"spec.override.nodeAgent.env": []any{
					map[string]any{
						"name":  "EXISTING_VAR",
						"value": "existing_value",
					},
					map[string]any{
						"name":  "EXISTING_VAR_2",
						"value": "existing_value_2",
					},
				},
			},
			newPath: "spec.override.nodeAgent.env",
			pathVal: map[string]any{
				"valueFrom": map[string]any{
					"fieldRef": map[string]any{
						"fieldPath": "status.hostIP",
					},
				},
			},
			mapFuncArgs: []any{
				map[string]any{
					"name": "DD_KUBERNETES_KUBELET_HOST",
				},
			},
			expectedMap: map[string]any{
				"spec.override.nodeAgent.env": []any{
					map[string]any{
						"name":  "EXISTING_VAR",
						"value": "existing_value",
					},
					map[string]any{
						"name":  "EXISTING_VAR_2",
						"value": "existing_value_2",
					},
					map[string]any{
						"name": "DD_KUBERNETES_KUBELET_HOST",
						"valueFrom": map[string]any{
							"fieldRef": map[string]any{
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
			interim:  map[string]any{},
			newPath:  "spec.override.nodeAgent.containers.agent.env",
			pathVal: []any{
				map[string]any{
					"name":  "VAR1",
					"value": "value1",
				},
			},
			expectedMap: map[string]any{
				"spec.override.nodeAgent.containers.agent.env": []any{
					map[string]any{
						"name":  "VAR1",
						"value": "value1",
					},
				},
			},
		},
		{
			name:     "mapMergeEnvs_add_to_existing_envs",
			funcName: "mapMergeEnvs",
			interim: map[string]any{
				"spec.override.nodeAgent.containers.agent.env": []any{
					map[string]any{
						"name":  "EXISTING_VAR",
						"value": "existing_value",
					},
				},
			},
			newPath: "spec.override.nodeAgent.containers.agent.env",
			pathVal: []any{
				map[string]any{
					"name":  "NEW_VAR",
					"value": "new_value",
				},
			},
			expectedMap: map[string]any{
				"spec.override.nodeAgent.containers.agent.env": []any{
					map[string]any{
						"name":  "EXISTING_VAR",
						"value": "existing_value",
					},
					map[string]any{
						"name":  "NEW_VAR",
						"value": "new_value",
					},
				},
			},
		},
		{
			name:     "mapMergeEnvs_avoid_duplicates",
			funcName: "mapMergeEnvs",
			interim: map[string]any{
				"spec.override.nodeAgent.containers.agent.env": []any{
					map[string]any{
						"name":  "EXISTING_VAR",
						"value": "existing_value",
					},
				},
			},
			newPath: "spec.override.nodeAgent.containers.agent.env",
			pathVal: []any{
				map[string]any{
					"name":  "EXISTING_VAR", // This should not be added again
					"value": "existing_value",
				},
				map[string]any{
					"name":  "NEW_VAR",
					"value": "new_value",
				},
			},
			expectedMap: map[string]any{
				"spec.override.nodeAgent.containers.agent.env": []any{
					map[string]any{
						"name":  "EXISTING_VAR",
						"value": "existing_value", // Keeps the original value
					},
					map[string]any{
						"name":  "NEW_VAR",
						"value": "new_value",
					},
				},
			},
		},
		{
			name:     "mapMergeEnvs_override_duplicates",
			funcName: "mapMergeEnvs",
			interim: map[string]any{
				"spec.override.nodeAgent.containers.agent.env": []any{
					map[string]any{
						"name":  "EXISTING_VAR",
						"value": "existing_value",
					},
				},
			},
			newPath: "spec.override.nodeAgent.containers.agent.env",
			pathVal: []any{
				map[string]any{
					"name":  "EXISTING_VAR", // This should override existing value
					"value": "new_value",
				},
				map[string]any{
					"name":  "NEW_VAR",
					"value": "new_value",
				},
			},
			expectedMap: map[string]any{
				"spec.override.nodeAgent.containers.agent.env": []any{
					map[string]any{
						"name":  "EXISTING_VAR",
						"value": "new_value", // New value overrides previous value
					},
					map[string]any{
						"name":  "NEW_VAR",
						"value": "new_value",
					},
				},
			},
		},
		{
			name:     "mapOverrideType_slice_to_string",
			funcName: "mapOverrideType",
			interim:  map[string]any{},
			newPath:  "spec.features.foo.bar",
			mapFuncArgs: []any{
				map[string]any{
					"newPath": "spec.features.foo.bar",
					"newType": "string",
				},
			},
			pathVal: []map[string]any{
				{
					"someKey":    "someVal",
					"anotherKey": map[string]any{"foo": true},
				},
			},
			expectedMap: map[string]any{
				"spec.features.foo.bar": `- anotherKey:
    foo: true
  someKey: someVal
`,
			},
		},
		{
			name:     "mapOverrideType_string_to_int",
			funcName: "mapOverrideType",
			interim:  map[string]any{},
			newPath:  "spec.features.foo.bar",
			mapFuncArgs: []any{
				map[string]any{
					"newPath": "spec.features.foo.bar",
					"newType": "int",
				},
			},
			pathVal: "8080",
			expectedMap: map[string]any{
				"spec.features.foo.bar": 8080,
			},
		},
	}

	mapFuncs := mapFuncRegistry()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapFunc := mapFuncs[tt.funcName]
			require.NotNil(t, mapFunc, "Mapping function %s should exist in registry", tt.funcName)
			mapFunc(tt.interim, tt.newPath, tt.pathVal, tt.mapFuncArgs)

			assert.Equal(t, tt.expectedMap, tt.interim)
		})
	}

	t.Run("non_existent_function", func(t *testing.T) {
		runFunc := mapFuncRegistry()["nonExistentFunc"]
		assert.Nil(t, runFunc, "Non-existent function should not be in registry")
	})
}
