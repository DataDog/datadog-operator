// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePrivateActionRunnerConfig(t *testing.T) {
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
			name: "config with all fields",
			configData: `private_action_runner:
  enabled: true
  self_enroll: false
  identity_use_k8s_secret: true
  identity_secret_name: custom-secret
  identity_file_path: /path/to/identity
  urn: urn:dd:apps:on-prem-runner:us1:1:runner-xyz
  private_key: my-private-key
  actions_allowlist:
    - com.datadoghq.kubernetes.core.*
    - com.datadoghq.gitlab.*
  task_concurrency: 10
  task_timeout_seconds: 120
  http_timeout_seconds: 45
  http_allowlist:
    - "*.datadoghq.com"
    - "*.github.com"
  http_allow_imds_endpoint: true
  log_file: /var/log/par.log`,
			wantErr: false,
			expectedConfig: &PrivateActionRunnerConfig{
				Enabled:              true,
				SelfEnroll:           false,
				IdentityUseK8sSecret: boolPtr(true),
				IdentitySecretName:   "custom-secret",
				IdentityFilePath:     "/path/to/identity",
				URN:                  "urn:dd:apps:on-prem-runner:us1:1:runner-xyz",
				PrivateKey:           "my-private-key",
				ActionsAllowlist: []string{
					"com.datadoghq.kubernetes.core.*",
					"com.datadoghq.gitlab.*",
				},
				TaskConcurrency:       int32Ptr(10),
				TaskTimeoutSeconds:    int32Ptr(120),
				HTTPTimeoutSeconds:    int32Ptr(45),
				HTTPAllowlist:         []string{"*.datadoghq.com", "*.github.com"},
				HTTPAllowIMDSEndpoint: boolPtr(true),
				LogFile:               "/var/log/par.log",
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
			assert.Equal(t, tt.expectedConfig.IdentityFilePath, config.IdentityFilePath)

			// Check pointers
			if tt.expectedConfig.IdentityUseK8sSecret != nil {
				require.NotNil(t, config.IdentityUseK8sSecret)
				assert.Equal(t, *tt.expectedConfig.IdentityUseK8sSecret, *config.IdentityUseK8sSecret)
			}
			if tt.expectedConfig.TaskConcurrency != nil {
				require.NotNil(t, config.TaskConcurrency)
				assert.Equal(t, *tt.expectedConfig.TaskConcurrency, *config.TaskConcurrency)
			}
			if tt.expectedConfig.TaskTimeoutSeconds != nil {
				require.NotNil(t, config.TaskTimeoutSeconds)
				assert.Equal(t, *tt.expectedConfig.TaskTimeoutSeconds, *config.TaskTimeoutSeconds)
			}
			if tt.expectedConfig.HTTPTimeoutSeconds != nil {
				require.NotNil(t, config.HTTPTimeoutSeconds)
				assert.Equal(t, *tt.expectedConfig.HTTPTimeoutSeconds, *config.HTTPTimeoutSeconds)
			}
			if tt.expectedConfig.HTTPAllowIMDSEndpoint != nil {
				require.NotNil(t, config.HTTPAllowIMDSEndpoint)
				assert.Equal(t, *tt.expectedConfig.HTTPAllowIMDSEndpoint, *config.HTTPAllowIMDSEndpoint)
			}

			// Check slices
			if len(tt.expectedConfig.ActionsAllowlist) > 0 {
				assert.ElementsMatch(t, tt.expectedConfig.ActionsAllowlist, config.ActionsAllowlist)
			}
			if len(tt.expectedConfig.HTTPAllowlist) > 0 {
				assert.ElementsMatch(t, tt.expectedConfig.HTTPAllowlist, config.HTTPAllowlist)
			}

			if tt.expectedConfig.LogFile != "" {
				assert.Equal(t, tt.expectedConfig.LogFile, config.LogFile)
			}
		})
	}
}

func TestPrivateActionRunnerConfig_ToEnvVars(t *testing.T) {
	tests := []struct {
		name            string
		config          *PrivateActionRunnerConfig
		expectedEnvVars map[string]string
		notExpectedVars []string
	}{
		{
			name:            "nil config",
			config:          nil,
			expectedEnvVars: map[string]string{},
		},
		{
			name: "minimal config - enabled true",
			config: &PrivateActionRunnerConfig{
				Enabled: true,
			},
			expectedEnvVars: map[string]string{
				DDPAREnabled:              "true",
				DDPARSelfEnroll:           "false",
				DDPARIdentityUseK8sSecret: "true",
			},
			notExpectedVars: []string{
				DDPARURN,
				DDPARPrivateKey,
				DDPARIdentitySecretName,
				DDPARActionsAllowlist,
			},
		},
		{
			name: "self-enroll with custom secret name",
			config: &PrivateActionRunnerConfig{
				Enabled:            true,
				SelfEnroll:         true,
				IdentitySecretName: "my-custom-secret",
			},
			expectedEnvVars: map[string]string{
				DDPAREnabled:              "true",
				DDPARSelfEnroll:           "true",
				DDPARIdentityUseK8sSecret: "true",
				DDPARIdentitySecretName:   "my-custom-secret",
			},
			notExpectedVars: []string{
				DDPARURN,
				DDPARPrivateKey,
			},
		},
		{
			name: "manual enrollment with URN and private key",
			config: &PrivateActionRunnerConfig{
				Enabled:            true,
				SelfEnroll:         false,
				URN:                "urn:dd:apps:on-prem-runner:us1:1:runner-abc",
				PrivateKey:         "base64-encoded-key",
				IdentitySecretName: "par-identity",
			},
			expectedEnvVars: map[string]string{
				DDPAREnabled:              "true",
				DDPARSelfEnroll:           "false",
				DDPARIdentityUseK8sSecret: "true",
				DDPARURN:                  "urn:dd:apps:on-prem-runner:us1:1:runner-abc",
				DDPARPrivateKey:           "base64-encoded-key",
				DDPARIdentitySecretName:   "par-identity",
			},
		},
		{
			name: "with actions allowlist",
			config: &PrivateActionRunnerConfig{
				Enabled:    true,
				SelfEnroll: true,
				ActionsAllowlist: []string{
					"com.datadoghq.http.request",
					"com.datadoghq.kubernetes.core.*",
					"com.datadoghq.gitlab.*",
				},
			},
			expectedEnvVars: map[string]string{
				DDPAREnabled:              "true",
				DDPARSelfEnroll:           "true",
				DDPARIdentityUseK8sSecret: "true",
				DDPARActionsAllowlist:     "com.datadoghq.http.request,com.datadoghq.kubernetes.core.*,com.datadoghq.gitlab.*",
			},
		},
		{
			name: "with task configuration",
			config: &PrivateActionRunnerConfig{
				Enabled:            true,
				SelfEnroll:         true,
				TaskConcurrency:    int32Ptr(10),
				TaskTimeoutSeconds: int32Ptr(120),
			},
			expectedEnvVars: map[string]string{
				DDPAREnabled:              "true",
				DDPARSelfEnroll:           "true",
				DDPARIdentityUseK8sSecret: "true",
				DDPARTaskConcurrency:      "10",
				DDPARTaskTimeoutSeconds:   "120",
			},
		},
		{
			name: "with HTTP configuration",
			config: &PrivateActionRunnerConfig{
				Enabled:            true,
				SelfEnroll:         false,
				HTTPTimeoutSeconds: int32Ptr(45),
				HTTPAllowlist: []string{
					"*.datadoghq.com",
					"*.github.com",
				},
				HTTPAllowIMDSEndpoint: boolPtr(true),
			},
			expectedEnvVars: map[string]string{
				DDPAREnabled:               "true",
				DDPARSelfEnroll:            "false",
				DDPARIdentityUseK8sSecret:  "true",
				DDPARHTTPTimeoutSeconds:    "45",
				DDPARHTTPAllowlist:         "*.datadoghq.com,*.github.com",
				DDPARHTTPAllowIMDSEndpoint: "true",
			},
		},
		{
			name: "identity use k8s secret explicitly false",
			config: &PrivateActionRunnerConfig{
				Enabled:              true,
				SelfEnroll:           false,
				IdentityUseK8sSecret: boolPtr(false),
				IdentityFilePath:     "/path/to/identity.json",
			},
			expectedEnvVars: map[string]string{
				DDPAREnabled:              "true",
				DDPARSelfEnroll:           "false",
				DDPARIdentityUseK8sSecret: "false",
				DDPARIdentityFilePath:     "/path/to/identity.json",
			},
		},
		{
			name: "complete configuration",
			config: &PrivateActionRunnerConfig{
				Enabled:               true,
				SelfEnroll:            false,
				IdentityUseK8sSecret:  boolPtr(true),
				IdentitySecretName:    "custom-secret",
				URN:                   "urn:dd:apps:on-prem-runner:us1:1:runner-xyz",
				PrivateKey:            "my-key",
				ActionsAllowlist:      []string{"com.datadoghq.*"},
				TaskConcurrency:       int32Ptr(15),
				TaskTimeoutSeconds:    int32Ptr(180),
				HTTPTimeoutSeconds:    int32Ptr(60),
				HTTPAllowlist:         []string{"*.example.com"},
				HTTPAllowIMDSEndpoint: boolPtr(false),
				LogFile:               "/var/log/par.log",
			},
			expectedEnvVars: map[string]string{
				DDPAREnabled:               "true",
				DDPARSelfEnroll:            "false",
				DDPARIdentityUseK8sSecret:  "true",
				DDPARIdentitySecretName:    "custom-secret",
				DDPARURN:                   "urn:dd:apps:on-prem-runner:us1:1:runner-xyz",
				DDPARPrivateKey:            "my-key",
				DDPARActionsAllowlist:      "com.datadoghq.*",
				DDPARTaskConcurrency:       "15",
				DDPARTaskTimeoutSeconds:    "180",
				DDPARHTTPTimeoutSeconds:    "60",
				DDPARHTTPAllowlist:         "*.example.com",
				DDPARHTTPAllowIMDSEndpoint: "false",
				DDPARLogFile:               "/var/log/par.log",
			},
		},
		{
			name: "disabled config should not set enabled env var",
			config: &PrivateActionRunnerConfig{
				Enabled:    false,
				SelfEnroll: true,
			},
			expectedEnvVars: map[string]string{
				DDPARSelfEnroll:           "true",
				DDPARIdentityUseK8sSecret: "true",
			},
			notExpectedVars: []string{
				DDPAREnabled,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envVars := tt.config.ToEnvVars()

			if tt.config == nil {
				assert.Nil(t, envVars)
				return
			}

			// Convert to map for easier validation
			envVarMap := make(map[string]string)
			for _, ev := range envVars {
				envVarMap[ev.Name] = ev.Value
			}

			// Check expected env vars
			for expectedKey, expectedValue := range tt.expectedEnvVars {
				actualValue, found := envVarMap[expectedKey]
				assert.True(t, found, "Expected env var %s not found", expectedKey)
				assert.Equal(t, expectedValue, actualValue, "Env var %s has wrong value", expectedKey)
			}

			// Check that unexpected env vars are not present
			for _, unexpectedKey := range tt.notExpectedVars {
				_, found := envVarMap[unexpectedKey]
				assert.False(t, found, "Unexpected env var %s should not be set", unexpectedKey)
			}
		})
	}
}

// Helper functions for creating pointers
func boolPtr(b bool) *bool {
	return &b
}

func int32Ptr(i int32) *int32 {
	return &i
}
