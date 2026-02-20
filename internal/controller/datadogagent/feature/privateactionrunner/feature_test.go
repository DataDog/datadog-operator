// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"encoding/json"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	featureutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func Test_privateActionRunnerFeature_Configure(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		wantFunc    func(t *testing.T, reqComp feature.RequiredComponents)
	}{
		{
			name:        "feature not enabled (no annotation)",
			annotations: nil,
			wantFunc: func(t *testing.T, reqComp feature.RequiredComponents) {
				assert.False(t, reqComp.Agent.IsEnabled())
			},
		},
		{
			name: "feature enabled via annotation",
			annotations: map[string]string{
				featureutils.EnablePrivateActionRunnerAnnotation: "true",
			},
			wantFunc: func(t *testing.T, reqComp feature.RequiredComponents) {
				assert.True(t, reqComp.Agent.IsEnabled())
				assert.Contains(t, reqComp.Agent.Containers, apicommon.CoreAgentContainerName)
				assert.Contains(t, reqComp.Agent.Containers, apicommon.PrivateActionRunnerContainerName)
			},
		},
		{
			name: "feature explicitly disabled via annotation",
			annotations: map[string]string{
				featureutils.EnablePrivateActionRunnerAnnotation: "false",
			},
			wantFunc: func(t *testing.T, reqComp feature.RequiredComponents) {
				assert.False(t, reqComp.Agent.IsEnabled())
				assert.False(t, reqComp.ClusterAgent.IsEnabled())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := buildPrivateActionRunnerFeature(nil)
			dda := &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: tt.annotations,
				},
			}
			reqComp := f.Configure(
				dda,
				&v2alpha1.DatadogAgentSpec{},
				nil,
			)
			tt.wantFunc(t, reqComp)
		})
	}
}

func Test_privateActionRunnerFeature_ManageNodeAgent(t *testing.T) {
	f := buildPrivateActionRunnerFeature(nil)
	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dda",
			Namespace: "default",
			Annotations: map[string]string{
				featureutils.EnablePrivateActionRunnerAnnotation: "true",
				featureutils.PrivateActionRunnerConfigDataAnnotation: `private_action_runner:
	enabled: true
    private_key: some-key
    urn: urn:dd:apps:on-prem-runner:us1:1:runner-abc
    actions_allowlist:
        - com.datadoghq.script.testConnection
        - com.datadoghq.kubernetes.core.listPod`,
			},
		},
	}
	f.Configure(dda, &v2alpha1.DatadogAgentSpec{}, nil)

	// Create test managers
	podTmpl := corev1.PodTemplateSpec{}
	managers := fake.NewPodTemplateManagers(t, podTmpl)

	// Call ManageNodeAgent
	err := f.ManageNodeAgent(managers, "")
	assert.NoError(t, err)

	// Verify volume is mounted
	volumes := managers.VolumeMgr.Volumes
	assert.Len(t, volumes, 1, "Should have exactly one volume")
	vol := volumes[0]
	assert.Equal(t, "privateactionrunner-config", vol.Name, "Volume name should match")
	assert.NotNil(t, vol.VolumeSource.ConfigMap, "Volume should be a ConfigMap volume")
	assert.Equal(t, "test-dda-privateactionrunner", vol.VolumeSource.ConfigMap.Name, "ConfigMap name should match")

	// Verify volume mount
	volumeMounts := managers.VolumeMountMgr.VolumeMountsByC[apicommon.PrivateActionRunnerContainerName]
	assert.Len(t, volumeMounts, 1, "Should have exactly one volume mount")
	mount := volumeMounts[0]
	assert.Equal(t, "privateactionrunner-config", mount.Name, "Mount name should match")
	assert.Equal(t, "/etc/datadog-agent/privateactionrunner.yaml", mount.MountPath, "Mount path should be the hardcoded path")
	assert.Equal(t, "privateactionrunner.yaml", mount.SubPath, "SubPath should mount the file directly")
	assert.True(t, mount.ReadOnly, "Mount should be read-only")

	// Verify hash
	assert.NotEmpty(t, managers.AnnotationMgr.Annotations)
	assert.NotEmpty(t, managers.AnnotationMgr.Annotations["checksum/private_action_runner-custom-config"])
	assert.Equal(t, "7aca0ab8a2cb083533a5552c17a50aa3", managers.AnnotationMgr.Annotations["checksum/private_action_runner-custom-config"])
}

func Test_privateActionRunnerFeature_ID(t *testing.T) {
	f := buildPrivateActionRunnerFeature(nil)
	assert.Equal(t, string(feature.PrivateActionRunnerIDType), string(f.ID()))
}

func Test_privateActionRunnerFeature_ManageSingleContainerNodeAgent(t *testing.T) {
	f := buildPrivateActionRunnerFeature(nil)
	managers := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})

	err := f.ManageSingleContainerNodeAgent(managers, "")
	assert.NoError(t, err)
}

func Test_privateActionRunnerFeature_ManageClusterChecksRunner(t *testing.T) {
	f := buildPrivateActionRunnerFeature(nil)
	managers := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})

	err := f.ManageClusterChecksRunner(managers, "")
	assert.NoError(t, err)
}

func Test_privateActionRunnerFeature_ManageOtelAgentGateway(t *testing.T) {
	f := buildPrivateActionRunnerFeature(nil)
	managers := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})

	err := f.ManageOtelAgentGateway(managers, "")
	assert.NoError(t, err)
}

func Test_privateActionRunnerFeature_ManageDependencies(t *testing.T) {
	f := buildPrivateActionRunnerFeature(nil)
	err := f.ManageDependencies(nil, "")
	assert.NoError(t, err)
}

func Test_privateActionRunnerFeature_ManageClusterAgent(t *testing.T) {
	f := buildPrivateActionRunnerFeature(nil)
	managers := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})

	err := f.ManageClusterAgent(managers, "")
	assert.NoError(t, err)
}

func Test_buildPrivateActionRunnerFeature_WithLogger(t *testing.T) {
	f := buildPrivateActionRunnerFeature(&feature.Options{
		Logger: logr.Discard(),
	})

	parFeat, ok := f.(*privateActionRunnerFeature)
	assert.True(t, ok)
	assert.NotNil(t, parFeat)
}

func Test_privateActionRunnerFeature_ConfigMapContent(t *testing.T) {
	testScheme := runtime.NewScheme()
	_ = corev1.AddToScheme(testScheme)
	_ = v2alpha1.AddToScheme(testScheme)

	tests := []struct {
		name            string
		annotations     map[string]string
		expectConfigMap bool
		expectedYAML    string
		expectedHash    string
	}{
		{
			name: "feature disabled",
			annotations: map[string]string{
				featureutils.EnablePrivateActionRunnerAnnotation: "false",
			},
			expectConfigMap: false,
		},
		{
			name: "enabled without configdata - uses default",
			annotations: map[string]string{
				featureutils.EnablePrivateActionRunnerAnnotation: "true",
			},
			expectConfigMap: true,
			expectedYAML:    defaultConfigData,
			expectedHash:    "57aedff9cb18bcec9b12a3974ef6fc55",
		},
		{
			name: "enabled with configdata - passes through directly",
			annotations: map[string]string{
				featureutils.EnablePrivateActionRunnerAnnotation: "true",
				featureutils.PrivateActionRunnerConfigDataAnnotation: `private_action_runner:
    private_key: some-key
    urn: urn:dd:apps:on-prem-runner:us1:1:runner-abc
    self_enroll: false
    actions_allowlist:
        - com.datadoghq.script.testConnection
        - com.datadoghq.script.enrichScript`,
			},
			expectConfigMap: true,
			expectedYAML: `private_action_runner:
    private_key: some-key
    urn: urn:dd:apps:on-prem-runner:us1:1:runner-abc
    self_enroll: false
    actions_allowlist:
        - com.datadoghq.script.testConnection
        - com.datadoghq.script.enrichScript`,
			expectedHash: "76f45ac891d62eb42272bbe26f32fb7c",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := buildPrivateActionRunnerFeature(nil)
			dda := &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-dda",
					Namespace:   "default",
					Annotations: tt.annotations,
				},
			}
			f.Configure(dda, &v2alpha1.DatadogAgentSpec{}, nil)

			storeOptions := &store.StoreOptions{
				Scheme: testScheme,
			}
			resourceManagers := feature.NewResourceManagers(store.NewStore(dda, storeOptions))

			err := f.ManageDependencies(resourceManagers, "")
			require.NoError(t, err)

			if !tt.expectConfigMap {
				// Verify no ConfigMap was created
				_, found := resourceManagers.Store().Get(kubernetes.ConfigMapKind, "default", "test-dda-privateactionrunner")
				assert.False(t, found, "ConfigMap should not be created when feature is disabled")
				return
			}

			// Verify ConfigMap was created
			configMapName := "test-dda-privateactionrunner"
			cm, found := resourceManagers.Store().Get(kubernetes.ConfigMapKind, "default", configMapName)
			require.True(t, found, "ConfigMap should be created")
			require.NotNil(t, cm)

			configMap, ok := cm.(*corev1.ConfigMap)
			require.True(t, ok, "Object should be a ConfigMap")
			assert.Equal(t, configMapName, configMap.Name, "ConfigMap name should match")
			assert.Equal(t, "default", configMap.Namespace, "Namespace should match")
			require.Contains(t, configMap.Data, "privateactionrunner.yaml", "ConfigMap must contain privateactionrunner.yaml")

			yamlContent := configMap.Data["privateactionrunner.yaml"]

			// Verify content matches expected
			assert.Equal(t, tt.expectedYAML, yamlContent, "ConfigMap content should match expected output")

			// Verify hash
			assert.NotEmpty(t, configMap.Annotations)
			assert.NotEmpty(t, configMap.Annotations["checksum/private_action_runner-custom-config"])
			assert.Equal(t, tt.expectedHash, configMap.Annotations["checksum/private_action_runner-custom-config"])
		})
	}
}

func Test_privateActionRunnerFeature_ConfigureClusterAgent(t *testing.T) {
	tests := []struct {
		name                      string
		annotations               map[string]string
		wantClusterAgentEnabled   bool
		wantNodeAgentEnabled      bool
		expectedClusterConfigData string
	}{
		{
			name:                    "cluster agent not enabled (no annotation)",
			annotations:             nil,
			wantClusterAgentEnabled: false,
			wantNodeAgentEnabled:    false,
		},
		{
			name: "cluster agent enabled via annotation",
			annotations: map[string]string{
				featureutils.EnableClusterAgentPrivateActionRunnerAnnotation: "true",
			},
			wantClusterAgentEnabled:   true,
			wantNodeAgentEnabled:      false,
			expectedClusterConfigData: defaultConfigData,
		},
		{
			name: "cluster agent enabled with custom config",
			annotations: map[string]string{
				featureutils.EnableClusterAgentPrivateActionRunnerAnnotation: "true",
				featureutils.ClusterAgentPrivateActionRunnerConfigDataAnnotation: `private_action_runner:
  enabled: true
  self_enroll: true
  identity_secret_name: my-custom-secret`,
			},
			wantClusterAgentEnabled: true,
			wantNodeAgentEnabled:    false,
			expectedClusterConfigData: `private_action_runner:
  enabled: true
  self_enroll: true
  identity_secret_name: my-custom-secret`,
		},
		{
			name: "cluster agent explicitly disabled",
			annotations: map[string]string{
				featureutils.EnableClusterAgentPrivateActionRunnerAnnotation: "false",
			},
			wantClusterAgentEnabled: false,
			wantNodeAgentEnabled:    false,
		},
		{
			name: "both node and cluster agent enabled",
			annotations: map[string]string{
				featureutils.EnablePrivateActionRunnerAnnotation:             "true",
				featureutils.EnableClusterAgentPrivateActionRunnerAnnotation: "true",
				featureutils.ClusterAgentPrivateActionRunnerConfigDataAnnotation: `private_action_runner:
  enabled: true
  self_enroll: false
  urn: urn:dd:apps:on-prem-runner:us1:1:runner-xyz`,
			},
			wantClusterAgentEnabled: true,
			wantNodeAgentEnabled:    true,
			expectedClusterConfigData: `private_action_runner:
  enabled: true
  self_enroll: false
  urn: urn:dd:apps:on-prem-runner:us1:1:runner-xyz`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := buildPrivateActionRunnerFeature(nil)
			dda := &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: tt.annotations,
				},
			}
			reqComp := f.Configure(dda, &v2alpha1.DatadogAgentSpec{}, nil)

			assert.Equal(t, tt.wantClusterAgentEnabled, reqComp.ClusterAgent.IsEnabled())
			assert.Equal(t, tt.wantNodeAgentEnabled, reqComp.Agent.IsEnabled())

			parFeat, ok := f.(*privateActionRunnerFeature)
			require.True(t, ok)

			// Check if cluster config is set correctly
			if tt.wantClusterAgentEnabled {
				require.NotNil(t, parFeat.clusterConfig, "clusterConfig should not be nil when enabled")
				assert.True(t, parFeat.clusterConfig.Enabled, "clusterConfig.Enabled should be true")

				// If we have expected config data, parse it and validate key fields
				if tt.expectedClusterConfigData != "" {
					expectedConfig, err := parsePrivateActionRunnerConfig(tt.expectedClusterConfigData)
					require.NoError(t, err)
					assert.Equal(t, expectedConfig.Enabled, parFeat.clusterConfig.Enabled)
					assert.Equal(t, expectedConfig.SelfEnroll, parFeat.clusterConfig.SelfEnroll)
					assert.Equal(t, expectedConfig.URN, parFeat.clusterConfig.URN)
					assert.Equal(t, expectedConfig.IdentitySecretName, parFeat.clusterConfig.IdentitySecretName)
				}
			} else {
				assert.Nil(t, parFeat.clusterConfig, "clusterConfig should be nil when not enabled")
			}
		})
	}
}

func Test_privateActionRunnerFeature_ManageClusterAgentEnvVars(t *testing.T) {
	tests := []struct {
		name              string
		configData        string
		expectedEnvVars   map[string]string
		validateAllowlist bool
		expectedAllowlist []string
	}{
		{
			name: "self-enroll with identity secret",
			configData: `private_action_runner:
  enabled: true
  self_enroll: true
  identity_secret_name: my-par-identity`,
			expectedEnvVars: map[string]string{
				"DD_PRIVATE_ACTION_RUNNER_ENABLED":              "true",
				"DD_PRIVATE_ACTION_RUNNER_SELF_ENROLL":          "true",
				"DD_PRIVATE_ACTION_RUNNER_IDENTITY_SECRET_NAME": "my-par-identity",
			},
			validateAllowlist: false,
		},
		{
			name: "manual enrollment with URN and private key",
			configData: `private_action_runner:
  enabled: true
  self_enroll: false
  urn: urn:dd:apps:on-prem-runner:us1:1:runner-abc
  private_key: my-secret-key
  identity_secret_name: par-secret`,
			expectedEnvVars: map[string]string{
				"DD_PRIVATE_ACTION_RUNNER_ENABLED":              "true",
				"DD_PRIVATE_ACTION_RUNNER_URN":                  "urn:dd:apps:on-prem-runner:us1:1:runner-abc",
				"DD_PRIVATE_ACTION_RUNNER_PRIVATE_KEY":          "my-secret-key",
				"DD_PRIVATE_ACTION_RUNNER_IDENTITY_SECRET_NAME": "par-secret",
			},
			validateAllowlist: false,
		},
		{
			name: "with actions allowlist",
			configData: `private_action_runner:
  enabled: true
  self_enroll: true
  actions_allowlist:
    - com.datadoghq.http.request
    - com.datadoghq.kubernetes.core.listPod
    - com.datadoghq.traceroute`,
			expectedEnvVars: map[string]string{
				"DD_PRIVATE_ACTION_RUNNER_ENABLED":     "true",
				"DD_PRIVATE_ACTION_RUNNER_SELF_ENROLL": "true",
			},
			validateAllowlist: true,
			expectedAllowlist: []string{
				"com.datadoghq.http.request",
				"com.datadoghq.kubernetes.core.listPod",
				"com.datadoghq.traceroute",
			},
		},
		{
			name:       "default config (minimal)",
			configData: defaultConfigData,
			expectedEnvVars: map[string]string{
				"DD_PRIVATE_ACTION_RUNNER_ENABLED": "true",
			},
			validateAllowlist: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := buildPrivateActionRunnerFeature(nil)
			dda := &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dda",
					Namespace: "default",
					Annotations: map[string]string{
						featureutils.EnableClusterAgentPrivateActionRunnerAnnotation:     "true",
						featureutils.ClusterAgentPrivateActionRunnerConfigDataAnnotation: tt.configData,
					},
				},
			}
			f.Configure(dda, &v2alpha1.DatadogAgentSpec{}, nil)

			// Create test managers
			podTmpl := corev1.PodTemplateSpec{}
			managers := fake.NewPodTemplateManagers(t, podTmpl)

			// Call ManageClusterAgent
			err := f.ManageClusterAgent(managers, "")
			assert.NoError(t, err)

			// Verify environment variables
			envVars := managers.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName]
			envVarMap := make(map[string]string)
			for _, env := range envVars {
				envVarMap[env.Name] = env.Value
			}

			for expectedKey, expectedValue := range tt.expectedEnvVars {
				actualValue, found := envVarMap[expectedKey]
				assert.True(t, found, "Expected env var %s not found", expectedKey)
				assert.Equal(t, expectedValue, actualValue, "Env var %s has wrong value", expectedKey)
			}

			// Validate allowlist if specified
			if tt.validateAllowlist {
				allowlistJSON, found := envVarMap["DD_PRIVATE_ACTION_RUNNER_ACTIONS_ALLOWLIST"]
				assert.True(t, found, "Expected DD_PRIVATE_ACTION_RUNNER_ACTIONS_ALLOWLIST not found")

				var allowlist []string
				err := json.Unmarshal([]byte(allowlistJSON), &allowlist)
				assert.NoError(t, err, "Failed to unmarshal allowlist JSON")
				assert.ElementsMatch(t, tt.expectedAllowlist, allowlist, "Allowlist doesn't match expected")
			}
		})
	}
}

func Test_parsePrivateActionRunnerConfig(t *testing.T) {
	tests := []struct {
		name           string
		configData     string
		wantErr        bool
		expectedConfig *PrivateActionRunnerConfig
	}{
		{
			name: "valid config with self-enroll",
			configData: `private_action_runner:
  enabled: true
  self_enroll: true
  identity_secret_name: my-secret`,
			wantErr: false,
			expectedConfig: &PrivateActionRunnerConfig{
				Enabled:            true,
				SelfEnroll:         true,
				IdentitySecretName: "my-secret",
			},
		},
		{
			name: "valid config with manual enrollment",
			configData: `private_action_runner:
  enabled: true
  self_enroll: false
  urn: urn:dd:apps:on-prem-runner:us1:1:runner-abc
  private_key: secret-key
  actions_allowlist:
    - com.datadoghq.http.request
    - com.datadoghq.traceroute`,
			wantErr: false,
			expectedConfig: &PrivateActionRunnerConfig{
				Enabled:    true,
				SelfEnroll: false,
				URN:        "urn:dd:apps:on-prem-runner:us1:1:runner-abc",
				PrivateKey: "secret-key",
				ActionsAllowlist: []string{
					"com.datadoghq.http.request",
					"com.datadoghq.traceroute",
				},
			},
		},
		{
			name:       "empty config",
			configData: ``,
			wantErr:    false,
			expectedConfig: &PrivateActionRunnerConfig{
				Enabled: false,
			},
		},
		{
			name:       "missing private_action_runner key",
			configData: `some_other_key: value`,
			wantErr:    false,
			expectedConfig: &PrivateActionRunnerConfig{
				Enabled: false,
			},
		},
		{
			name:       "invalid YAML",
			configData: `private_action_runner:\n  invalid: [unclosed`,
			wantErr:    true,
		},
		{
			name:       "default config",
			configData: defaultConfigData,
			wantErr:    false,
			expectedConfig: &PrivateActionRunnerConfig{
				Enabled: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := parsePrivateActionRunnerConfig(tt.configData)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			require.NotNil(t, config)
			assert.Equal(t, tt.expectedConfig.Enabled, config.Enabled)
			assert.Equal(t, tt.expectedConfig.SelfEnroll, config.SelfEnroll)
			assert.Equal(t, tt.expectedConfig.URN, config.URN)
			assert.Equal(t, tt.expectedConfig.PrivateKey, config.PrivateKey)
			assert.Equal(t, tt.expectedConfig.IdentitySecretName, config.IdentitySecretName)
			if len(tt.expectedConfig.ActionsAllowlist) > 0 {
				assert.ElementsMatch(t, tt.expectedConfig.ActionsAllowlist, config.ActionsAllowlist)
			}
		})
	}
}
