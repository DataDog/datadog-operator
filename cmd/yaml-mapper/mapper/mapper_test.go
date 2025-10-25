// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).

package mapper

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chartutil"
)

func TestMergeMaps(t *testing.T) {
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

			result := mergeMaps(map1Copy, map2Copy)
			assert.Equal(t, tt.expected, result)

			assert.Equal(t, tt.expected, map1Copy)
		})
	}
}

func TestCustomMapFuncs(t *testing.T) {
	// Test that all custom map functions are properly registered
	t.Run("customMapFuncs_dict", func(t *testing.T) {
		expectedFuncs := []string{"mapApiSecretKey", "mapAppSecretKey", "mapTokenSecretKey", "mapSeccompProfile", "mapSystemProbeAppArmor", "mapLocalServiceName", "mapAppendEnvVar", "mapMergeEnvs", "mapOverrideType"}
		mapFuncs := registry()

		for _, funcName := range expectedFuncs {
			t.Run(funcName+"_exists", func(t *testing.T) {
				runFunc := mapFuncs[funcName]
				assert.NotNil(t, runFunc, "Custom map function %s should be registered", funcName)
			})
		}

		assert.Equal(t, len(expectedFuncs), len(mapFuncs), "Should have exactly %d custom map functions", len(expectedFuncs))
	})

	// Test individual functions through the dictionary
	tests := []struct {
		name        string
		funcName    string
		interim     map[string]interface{}
		newPath     string
		pathVal     interface{}
		mapFuncArgs []interface{}
		expectedMap map[string]interface{}
	}{
		// mapApiSecretKey tests
		{
			name:     "mapApiSecretKey_empty_map",
			funcName: "mapApiSecretKey",
			interim:  map[string]interface{}{},
			newPath:  "spec.global.credentials.apiSecret.secretName",
			pathVal:  "my-api-secret",
			expectedMap: map[string]interface{}{
				"spec.global.credentials.apiSecret.secretName": "my-api-secret",
				"spec.global.credentials.apiSecret.keyName":    "api-key",
			},
		},
		{
			name:     "mapApiSecretKey_existing_map",
			funcName: "mapApiSecretKey",
			interim: map[string]interface{}{
				"spec.global.site":      "datadoghq.com",
				"spec.agent.image.name": "datadog/agent",
			},
			newPath: "spec.global.credentials.apiSecret.secretName",
			pathVal: "datadog-api-secret",
			expectedMap: map[string]interface{}{
				"spec.global.site":                             "datadoghq.com",
				"spec.agent.image.name":                        "datadog/agent",
				"spec.global.credentials.apiSecret.secretName": "datadog-api-secret",
				"spec.global.credentials.apiSecret.keyName":    "api-key",
			},
		},
		{
			name:     "mapApiSecretKey_overwrite",
			funcName: "mapApiSecretKey",
			interim: map[string]interface{}{
				"spec.global.credentials.apiSecret.secretName": "old-secret",
				"spec.global.credentials.apiSecret.keyName":    "old-key",
			},
			newPath: "spec.global.credentials.apiSecret.secretName",
			pathVal: "new-api-secret",
			expectedMap: map[string]interface{}{
				"spec.global.credentials.apiSecret.secretName": "new-api-secret",
				"spec.global.credentials.apiSecret.keyName":    "api-key",
			},
		},
		// mapAppSecretKey tests
		{
			name:     "mapAppSecretKey_empty_map",
			funcName: "mapAppSecretKey",
			interim:  map[string]interface{}{},
			newPath:  "spec.global.credentials.appSecret.secretName",
			pathVal:  "my-app-secret",
			expectedMap: map[string]interface{}{
				"spec.global.credentials.appSecret.secretName": "my-app-secret",
				"spec.global.credentials.appSecret.keyName":    "app-key",
			},
		},
		{
			name:     "mapAppSecretKey_with_existing_api_secret",
			funcName: "mapAppSecretKey",
			interim: map[string]interface{}{
				"spec.global.credentials.apiSecret.secretName": "api-secret",
				"spec.global.credentials.apiSecret.keyName":    "api-key",
			},
			newPath: "spec.global.credentials.appSecret.secretName",
			pathVal: "datadog-app-secret",
			expectedMap: map[string]interface{}{
				"spec.global.credentials.apiSecret.secretName": "api-secret",
				"spec.global.credentials.apiSecret.keyName":    "api-key",
				"spec.global.credentials.appSecret.secretName": "datadog-app-secret",
				"spec.global.credentials.appSecret.keyName":    "app-key",
			},
		},
		{
			name:     "mapAppSecretKey_overwrite",
			funcName: "mapAppSecretKey",
			interim: map[string]interface{}{
				"spec.global.credentials.appSecret.secretName": "old-app-secret",
				"spec.global.credentials.appSecret.keyName":    "old-app-key",
			},
			newPath: "spec.global.credentials.appSecret.secretName",
			pathVal: "new-app-secret",
			expectedMap: map[string]interface{}{
				"spec.global.credentials.appSecret.secretName": "new-app-secret",
				"spec.global.credentials.appSecret.keyName":    "app-key",
			},
		},
		// mapTokenSecretKey tests
		{
			name:     "mapTokenSecretKey_empty_map",
			funcName: "mapTokenSecretKey",
			interim:  map[string]interface{}{},
			newPath:  "spec.global.clusterAgentTokenSecret.secretName",
			pathVal:  "my-token-secret",
			expectedMap: map[string]interface{}{
				"spec.global.clusterAgentTokenSecret.secretName": "my-token-secret",
				"spec.global.clusterAgentTokenSecret.keyName":    "token",
			},
		},
		{
			name:     "mapTokenSecretKey_with_existing_secrets",
			funcName: "mapTokenSecretKey",
			interim: map[string]interface{}{
				"spec.global.credentials.apiSecret.secretName": "api-secret",
				"spec.global.credentials.appSecret.secretName": "app-secret",
			},
			newPath: "spec.global.clusterAgentTokenSecret.secretName",
			pathVal: "cluster-agent-token",
			expectedMap: map[string]interface{}{
				"spec.global.credentials.apiSecret.secretName":   "api-secret",
				"spec.global.credentials.appSecret.secretName":   "app-secret",
				"spec.global.clusterAgentTokenSecret.secretName": "cluster-agent-token",
				"spec.global.clusterAgentTokenSecret.keyName":    "token",
			},
		},
		{
			name:     "mapTokenSecretKey_overwrite",
			funcName: "mapTokenSecretKey",
			interim: map[string]interface{}{
				"spec.global.clusterAgentTokenSecret.secretName": "old-token-secret",
				"spec.global.clusterAgentTokenSecret.keyName":    "old-token",
			},
			newPath: "spec.global.clusterAgentTokenSecret.secretName",
			pathVal: "new-token-secret",
			expectedMap: map[string]interface{}{
				"spec.global.clusterAgentTokenSecret.secretName": "new-token-secret",
				"spec.global.clusterAgentTokenSecret.keyName":    "token",
			},
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
	}

	customFuncs := registry()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			customFunc := customFuncs[tt.funcName]
			require.NotNil(t, customFunc, "Custom function %s should exist in registry", tt.funcName)
			customFunc(tt.interim, tt.newPath, tt.pathVal, tt.mapFuncArgs)

			assert.Equal(t, tt.expectedMap, tt.interim)
		})
	}

	t.Run("non_existent_function", func(t *testing.T) {
		runFunc := registry()["nonExistentFunc"]
		assert.Nil(t, runFunc, "Non-existent function should not be in registry")
	})
}

func TestMakeTable(t *testing.T) {
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

			result := makeTable(tt.path, tt.val, mapNameCopy)

			// Verify that the result matches expected
			assert.Equal(t, tt.expected, result)

			// Verify that the function modifies the input map in place
			assert.Equal(t, tt.expected, mapNameCopy)

			// Verify that the returned map is the same object as the input map
			assert.True(t, fmt.Sprintf("%p", result) == fmt.Sprintf("%p", mapNameCopy), "makeTable should return the same map object that was passed in")
		})
	}
}

func TestMakeTableEdgeCases(t *testing.T) {
	t.Run("nil_value", func(t *testing.T) {
		mapName := map[string]interface{}{}
		result := makeTable("spec.global.site", nil, mapName)

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
		result := makeTable("a.b.c.d.e.f", "deep_value", mapName)

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
		result := makeTable("spec.containers.0.name", "agent", mapName)

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

func TestFoldDeprecated(t *testing.T) {
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
			actualMap := foldDeprecated(tt.sourceVals)
			assert.Equal(t, tt.wantVals, actualMap)
		})
	}
}

// TODO: add test for setInterim()
