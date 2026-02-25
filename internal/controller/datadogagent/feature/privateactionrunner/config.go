// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"fmt"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"gopkg.in/yaml.v3"
)

// Environment variable names for Private Action Runner configuration
const (
	DDPAREnabled               = "DD_PRIVATE_ACTION_RUNNER_ENABLED"
	DDPARSelfEnroll            = "DD_PRIVATE_ACTION_RUNNER_SELF_ENROLL"
	DDPARIdentityUseK8sSecret  = "DD_PRIVATE_ACTION_RUNNER_IDENTITY_USE_K8S_SECRET"
	DDPARIdentitySecretName    = "DD_PRIVATE_ACTION_RUNNER_IDENTITY_SECRET_NAME"
	DDPARIdentityFilePath      = "DD_PRIVATE_ACTION_RUNNER_IDENTITY_FILE_PATH"
	DDPARURN                   = "DD_PRIVATE_ACTION_RUNNER_URN"
	DDPARPrivateKey            = "DD_PRIVATE_ACTION_RUNNER_PRIVATE_KEY"
	DDPARActionsAllowlist      = "DD_PRIVATE_ACTION_RUNNER_ACTIONS_ALLOWLIST"
	DDPARTaskConcurrency       = "DD_PRIVATE_ACTION_RUNNER_TASK_CONCURRENCY"
	DDPARTaskTimeoutSeconds    = "DD_PRIVATE_ACTION_RUNNER_TASK_TIMEOUT_SECONDS"
	DDPARHTTPTimeoutSeconds    = "DD_PRIVATE_ACTION_RUNNER_HTTP_TIMEOUT_SECONDS"
	DDPARHTTPAllowlist         = "DD_PRIVATE_ACTION_RUNNER_HTTP_ALLOWLIST"
	DDPARHTTPAllowIMDSEndpoint = "DD_PRIVATE_ACTION_RUNNER_HTTP_ALLOW_IMDS_ENDPOINT"
	DDPARLogFile               = "DD_PRIVATE_ACTION_RUNNER_LOG_FILE"
)

// PrivateActionRunnerConfig represents the parsed configuration from YAML for Private Action Runner
type PrivateActionRunnerConfig struct {
	// Enabled controls whether the Private Action Runner is enabled
	Enabled bool `yaml:"enabled"`

	// SelfEnroll controls whether the runner should automatically enroll with Datadog
	// When true, the runner will automatically register and obtain credentials
	// When false, urn and private_key must be provided
	SelfEnroll bool `yaml:"self_enroll"`

	// IdentityUseK8sSecret controls whether to use a Kubernetes secret for identity storage
	// Defaults to true. When enabled, identity is stored in a K8s secret for persistence.
	IdentityUseK8sSecret *bool `yaml:"identity_use_k8s_secret,omitempty"`

	// IdentitySecretName is the name of the Kubernetes secret to use for identity storage
	// Defaults to "private-action-runner-identity"
	IdentitySecretName string `yaml:"identity_secret_name,omitempty"`

	// IdentityFilePath is the path to a file containing the identity (URN and private key)
	// Alternative to using K8s secrets for identity storage
	IdentityFilePath string `yaml:"identity_file_path,omitempty"`

	// URN is the Unique Resource Name identifying this Private Action Runner instance
	// Format: urn:dd:apps:on-prem-runner:<site>:<org_id>:runner-<runner_id>
	// Required if self_enroll is false
	URN string `yaml:"urn,omitempty"`

	// PrivateKey is the base64-encoded ECDSA private key for authentication
	// Required if self_enroll is false
	PrivateKey string `yaml:"private_key,omitempty"`

	// ActionsAllowlist is a list of action patterns that the runner is allowed to execute
	// Supports wildcard patterns like "com.datadoghq.kubernetes.core.*"
	// Example: ["com.datadoghq.http.request", "com.datadoghq.kubernetes.core.*"]
	ActionsAllowlist []string `yaml:"actions_allowlist,omitempty"`

	// TaskConcurrency controls how many tasks can run concurrently
	// Defaults to 5
	TaskConcurrency *int32 `yaml:"task_concurrency,omitempty"`

	// TaskTimeoutSeconds is the maximum time in seconds a task can run
	// Defaults to 60 seconds
	TaskTimeoutSeconds *int32 `yaml:"task_timeout_seconds,omitempty"`

	// HTTPTimeoutSeconds is the timeout for HTTP requests made by actions
	// Defaults to 30 seconds
	HTTPTimeoutSeconds *int32 `yaml:"http_timeout_seconds,omitempty"`

	// HTTPAllowlist is a list of hostname patterns that HTTP actions can access
	// Supports glob patterns like "*.datadoghq.com"
	// Empty list means all hosts are allowed
	HTTPAllowlist []string `yaml:"http_allowlist,omitempty"`

	// HTTPAllowIMDSEndpoint controls whether HTTP actions can access cloud metadata endpoints
	// (e.g., AWS EC2 instance metadata at 169.254.169.254)
	// Defaults to false for security
	HTTPAllowIMDSEndpoint *bool `yaml:"http_allow_imds_endpoint,omitempty"`

	// LogFile is the path to the log file for Private Action Runner
	// If not specified, logs go to the standard agent log location
	LogFile string `yaml:"log_file,omitempty"`
}

func parsePrivateActionRunnerConfig(configData string) (*PrivateActionRunnerConfig, error) {
	config := struct {
		PrivateActionRunner *PrivateActionRunnerConfig `yaml:"private_action_runner"`
	}{
		PrivateActionRunner: &PrivateActionRunnerConfig{},
	}
	if err := yaml.Unmarshal([]byte(configData), &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config data: %w", err)
	}
	return config.PrivateActionRunner, nil
}

// ToEnvVars converts the PrivateActionRunnerConfig to a list of environment variables
func (c *PrivateActionRunnerConfig) ToEnvVars() []*corev1.EnvVar {
	if c == nil {
		return nil
	}

	envVars := make([]*corev1.EnvVar, 0)

	if c.Enabled {
		envVars = append(envVars, &corev1.EnvVar{
			Name:  DDPAREnabled,
			Value: "true",
		})
	}

	envVars = append(envVars, &corev1.EnvVar{
		Name:  DDPARSelfEnroll,
		Value: strconv.FormatBool(c.SelfEnroll),
	})

	identityUseK8sSecret := c.IdentityUseK8sSecret == nil || *c.IdentityUseK8sSecret
	envVars = append(envVars, &corev1.EnvVar{
		Name:  DDPARIdentityUseK8sSecret,
		Value: strconv.FormatBool(identityUseK8sSecret),
	})

	if c.IdentitySecretName != "" {
		envVars = append(envVars, &corev1.EnvVar{
			Name:  DDPARIdentitySecretName,
			Value: c.IdentitySecretName,
		})
	}

	if c.IdentityFilePath != "" {
		envVars = append(envVars, &corev1.EnvVar{
			Name:  DDPARIdentityFilePath,
			Value: c.IdentityFilePath,
		})
	}

	if c.URN != "" {
		envVars = append(envVars, &corev1.EnvVar{
			Name:  DDPARURN,
			Value: c.URN,
		})
	}

	if c.PrivateKey != "" {
		envVars = append(envVars, &corev1.EnvVar{
			Name:  DDPARPrivateKey,
			Value: c.PrivateKey,
		})
	}

	if len(c.ActionsAllowlist) > 0 {
		envVars = append(envVars, &corev1.EnvVar{
			Name:  DDPARActionsAllowlist,
			Value: strings.Join(c.ActionsAllowlist, ","),
		})
	}

	if c.TaskConcurrency != nil {
		envVars = append(envVars, &corev1.EnvVar{
			Name:  DDPARTaskConcurrency,
			Value: strconv.FormatInt(int64(*c.TaskConcurrency), 10),
		})
	}

	if c.TaskTimeoutSeconds != nil {
		envVars = append(envVars, &corev1.EnvVar{
			Name:  DDPARTaskTimeoutSeconds,
			Value: strconv.FormatInt(int64(*c.TaskTimeoutSeconds), 10),
		})
	}

	if c.HTTPTimeoutSeconds != nil {
		envVars = append(envVars, &corev1.EnvVar{
			Name:  DDPARHTTPTimeoutSeconds,
			Value: strconv.FormatInt(int64(*c.HTTPTimeoutSeconds), 10),
		})
	}

	if len(c.HTTPAllowlist) > 0 {
		envVars = append(envVars, &corev1.EnvVar{
			Name:  DDPARHTTPAllowlist,
			Value: strings.Join(c.HTTPAllowlist, ","),
		})
	}

	if c.HTTPAllowIMDSEndpoint != nil {
		envVars = append(envVars, &corev1.EnvVar{
			Name:  DDPARHTTPAllowIMDSEndpoint,
			Value: strconv.FormatBool(*c.HTTPAllowIMDSEndpoint),
		})
	}

	if c.LogFile != "" {
		envVars = append(envVars, &corev1.EnvVar{
			Name:  DDPARLogFile,
			Value: c.LogFile,
		})
	}

	return envVars
}
