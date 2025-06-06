// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package global

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
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
		ddai     *v1alpha1.DatadogAgentInternal
		expected bool
	}{
		{
			name:     "Empty DDA (no override)",
			ddai:     &v1alpha1.DatadogAgentInternal{},
			expected: false,
		},
		{
			name: "Override but no custom seccomp for system probe",
			ddai: &v1alpha1.DatadogAgentInternal{
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
			ddai: &v1alpha1.DatadogAgentInternal{
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
			name: "Custom seccomp for system probe, but uses configData (not supported)",
			ddai: &v1alpha1.DatadogAgentInternal{
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
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := useSystemProbeCustomSeccomp(tt.ddai)
			assert.Equal(t, tt.expected, actual)
		})
	}
}
