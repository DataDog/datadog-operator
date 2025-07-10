// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package global

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
)

type InstallInfoData struct {
	InstallMethod InstallMethod `yaml:"install_method"`
}

type InstallMethod struct {
	Tool             string `yaml:"tool"`
	ToolVersion      string `yaml:"tool_version"`
	InstallerVersion string `yaml:"installer_version"`
}

func Test_getInstallInfoValue(t *testing.T) {
	tests := []struct {
		name                   string
		toolVersionEnvVarValue string
		expectedToolVersion    string
	}{
		{
			name:                   "Env var empty/unset (os.Getenv returns unset env var as empty string)",
			toolVersionEnvVarValue: "",
			expectedToolVersion:    "unknown",
		},
		{
			name:                   "Env var set",
			toolVersionEnvVarValue: "foo",
			expectedToolVersion:    "foo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(InstallInfoToolVersion, tt.toolVersionEnvVarValue)
			installInfo := InstallInfoData{}

			test := getInstallInfoValue()

			err := yaml.Unmarshal([]byte(test), &installInfo)
			assert.NoError(t, err)

			assert.Equal(t, "datadog-operator", installInfo.InstallMethod.Tool)
			assert.Equal(t, tt.expectedToolVersion, installInfo.InstallMethod.ToolVersion)
			assert.Equal(t, "0.0.0", installInfo.InstallMethod.InstallerVersion)
		})
	}
}

func Test_useSystemProbeCustomSeccomp(t *testing.T) {
	tests := []struct {
		name     string
		dda      *v2alpha1.DatadogAgent
		expected bool
	}{
		{
			name:     "Empty DDA (no override)",
			dda:      &v2alpha1.DatadogAgent{},
			expected: false,
		},
		{
			name: "Override but no custom seccomp for system probe",
			dda: &v2alpha1.DatadogAgent{
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Containers: map[apicommon.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
								apicommon.CoreAgentContainerName: {
									SeccompConfig: &v2alpha1.SeccompConfig{
										CustomProfile: &v2alpha1.CustomConfig{
											ConfigMap: &v2alpha1.ConfigMapConfig{
												Name: "foo",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "Custom seccomp for system probe",
			dda: &v2alpha1.DatadogAgent{
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Containers: map[apicommon.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
								apicommon.SystemProbeContainerName: {
									SeccompConfig: &v2alpha1.SeccompConfig{
										CustomProfile: &v2alpha1.CustomConfig{
											ConfigMap: &v2alpha1.ConfigMapConfig{
												Name: "foo",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "Custom seccomp for system probe, but uses configData",
			dda: &v2alpha1.DatadogAgent{
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Containers: map[apicommon.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
								apicommon.SystemProbeContainerName: {
									SeccompConfig: &v2alpha1.SeccompConfig{
										CustomProfile: &v2alpha1.CustomConfig{
											ConfigData: apiutils.NewStringPointer("foo"),
										},
									},
								},
							},
						},
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := useSystemProbeCustomSeccomp(&tt.dda.Spec)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func Test_setCredentialsFromDDA(t *testing.T) {
	tests := []struct {
		name        string
		dda         metav1.Object
		ddaiFromDDA *v2alpha1.GlobalConfig
		expected    *v2alpha1.GlobalConfig
	}{
		{
			name: "Empty credentials",
			dda: &metav1.ObjectMeta{
				Name: "foo",
			},
			ddaiFromDDA: &v2alpha1.GlobalConfig{
				Credentials: &v2alpha1.DatadogCredentials{},
			},
			expected: &v2alpha1.GlobalConfig{
				Credentials: &v2alpha1.DatadogCredentials{
					APISecret: &v2alpha1.SecretConfig{
						SecretName: "foo-secret",
						KeyName:    "api_key",
					},
				},
			},
		},
		{
			name: "API key set in plain text",
			dda: &metav1.ObjectMeta{
				Name: "foo",
			},
			ddaiFromDDA: &v2alpha1.GlobalConfig{
				Credentials: &v2alpha1.DatadogCredentials{
					APIKey: apiutils.NewStringPointer("api_key"),
				},
			},
			expected: &v2alpha1.GlobalConfig{
				Credentials: &v2alpha1.DatadogCredentials{
					APISecret: &v2alpha1.SecretConfig{
						SecretName: "foo-secret",
						KeyName:    "api_key",
					},
				},
			},
		},
		{
			name: "API key set in plain text and as secret",
			dda: &metav1.ObjectMeta{
				Name: "foo",
			},
			ddaiFromDDA: &v2alpha1.GlobalConfig{
				Credentials: &v2alpha1.DatadogCredentials{
					APIKey: apiutils.NewStringPointer("api_key"),
					APISecret: &v2alpha1.SecretConfig{
						SecretName: "bar",
						KeyName:    "bar_key",
					},
				},
			},
			expected: &v2alpha1.GlobalConfig{
				Credentials: &v2alpha1.DatadogCredentials{
					APISecret: &v2alpha1.SecretConfig{
						SecretName: "bar",
						KeyName:    "bar_key",
					},
				},
			},
		},
		{
			name: "API key set in plain text, app key set in secret",
			dda: &metav1.ObjectMeta{
				Name: "foo",
			},
			ddaiFromDDA: &v2alpha1.GlobalConfig{
				Credentials: &v2alpha1.DatadogCredentials{
					APIKey: apiutils.NewStringPointer("api_key"),
					AppSecret: &v2alpha1.SecretConfig{
						SecretName: "bar",
						KeyName:    "bar_key",
					},
				},
			},
			expected: &v2alpha1.GlobalConfig{
				Credentials: &v2alpha1.DatadogCredentials{
					APISecret: &v2alpha1.SecretConfig{
						SecretName: "foo-secret",
						KeyName:    "api_key",
					},
					AppSecret: &v2alpha1.SecretConfig{
						SecretName: "bar",
						KeyName:    "bar_key",
					},
				},
			},
		},
		{
			name: "API key set in secret, app key set in plain text",
			dda: &metav1.ObjectMeta{
				Name: "foo",
			},
			ddaiFromDDA: &v2alpha1.GlobalConfig{
				Credentials: &v2alpha1.DatadogCredentials{
					APISecret: &v2alpha1.SecretConfig{
						SecretName: "bar",
						KeyName:    "bar_key",
					},
					AppKey: apiutils.NewStringPointer("api_key"),
				},
			},
			expected: &v2alpha1.GlobalConfig{
				Credentials: &v2alpha1.DatadogCredentials{
					APISecret: &v2alpha1.SecretConfig{
						SecretName: "bar",
						KeyName:    "bar_key",
					},
					AppSecret: &v2alpha1.SecretConfig{
						SecretName: "foo-secret",
						KeyName:    "app_key",
					},
				},
			},
		},
		{
			name: "API key and app key set in secret",
			dda: &metav1.ObjectMeta{
				Name: "foo",
			},
			ddaiFromDDA: &v2alpha1.GlobalConfig{
				Credentials: &v2alpha1.DatadogCredentials{
					APISecret: &v2alpha1.SecretConfig{
						SecretName: "bar",
						KeyName:    "bar_key",
					},
					AppSecret: &v2alpha1.SecretConfig{
						SecretName: "bar",
						KeyName:    "app_key",
					},
				},
			},
			expected: &v2alpha1.GlobalConfig{
				Credentials: &v2alpha1.DatadogCredentials{
					APISecret: &v2alpha1.SecretConfig{
						SecretName: "bar",
						KeyName:    "bar_key",
					},
					AppSecret: &v2alpha1.SecretConfig{
						SecretName: "bar",
						KeyName:    "app_key",
					},
				},
			},
		},
		{
			name: "API key and app key set in plain text",
			dda: &metav1.ObjectMeta{
				Name: "foo",
			},
			ddaiFromDDA: &v2alpha1.GlobalConfig{
				Credentials: &v2alpha1.DatadogCredentials{
					APIKey: apiutils.NewStringPointer("api_key"),
					AppKey: apiutils.NewStringPointer("app_key"),
				},
			},
			expected: &v2alpha1.GlobalConfig{
				Credentials: &v2alpha1.DatadogCredentials{
					APISecret: &v2alpha1.SecretConfig{
						SecretName: "foo-secret",
						KeyName:    "api_key",
					},
					AppSecret: &v2alpha1.SecretConfig{
						SecretName: "foo-secret",
						KeyName:    "app_key",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setCredentialsFromDDA(tt.dda, tt.ddaiFromDDA)
			assert.Equal(t, tt.expected, tt.ddaiFromDDA)
		})
	}
}

func Test_setDCATokenFromDDA(t *testing.T) {
	tests := []struct {
		name        string
		dda         metav1.Object
		ddaiFromDDA *v2alpha1.GlobalConfig
		expected    *v2alpha1.GlobalConfig
	}{
		{
			name: "No dca token secret",
			dda: &metav1.ObjectMeta{
				Name: "foo",
			},
			ddaiFromDDA: &v2alpha1.GlobalConfig{},
			expected: &v2alpha1.GlobalConfig{
				ClusterAgentTokenSecret: &v2alpha1.SecretConfig{
					SecretName: "foo-token",
					KeyName:    "token",
				},
			},
		},
		{
			name: "DCA token secret set",
			dda: &metav1.ObjectMeta{
				Name: "foo",
			},
			ddaiFromDDA: &v2alpha1.GlobalConfig{
				ClusterAgentTokenSecret: &v2alpha1.SecretConfig{
					SecretName: "bar",
					KeyName:    "key",
				},
			},
			expected: &v2alpha1.GlobalConfig{
				ClusterAgentTokenSecret: &v2alpha1.SecretConfig{
					SecretName: "bar",
					KeyName:    "key",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setDCATokenFromDDA(tt.dda, tt.ddaiFromDDA)
			assert.Equal(t, tt.expected, tt.ddaiFromDDA)
		})
	}
}
