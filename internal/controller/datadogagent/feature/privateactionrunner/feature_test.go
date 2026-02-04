// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
)

func Test_privateActionRunnerFeature_Configure(t *testing.T) {
	tests := []struct {
		name     string
		ddaSpec  *v2alpha1.DatadogAgentSpec
		wantFunc func(t *testing.T, reqComp feature.RequiredComponents)
	}{
		{
			name: "feature not enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{},
			},
			wantFunc: func(t *testing.T, reqComp feature.RequiredComponents) {
				assert.False(t, reqComp.Agent.IsEnabled())
			},
		},
		{
			name: "feature enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					PrivateActionRunner: &v2alpha1.PrivateActionRunnerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(true),
					},
				},
			},
			wantFunc: func(t *testing.T, reqComp feature.RequiredComponents) {
				assert.True(t, reqComp.Agent.IsEnabled())
				assert.Contains(t, reqComp.Agent.Containers, apicommon.CoreAgentContainerName)
				assert.Contains(t, reqComp.Agent.Containers, apicommon.PrivateActionRunnerContainerName)
			},
		},
		{
			name: "feature explicitly disabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					PrivateActionRunner: &v2alpha1.PrivateActionRunnerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(false),
					},
				},
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
			reqComp := f.Configure(
				&v2alpha1.DatadogAgent{},
				tt.ddaSpec,
				nil,
			)
			tt.wantFunc(t, reqComp)
		})
	}
}

func Test_privateActionRunnerFeature_ManageNodeAgent(t *testing.T) {
	tests := []struct {
		name            string
		ddaSpec         *v2alpha1.DatadogAgentSpec
		expectedEnvVars []*corev1.EnvVar
	}{
		{
			name: "basic configuration",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					PrivateActionRunner: &v2alpha1.PrivateActionRunnerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(true),
					},
				},
			},
			expectedEnvVars: nil,
		},
		{
			name: "with self-enrollment enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					PrivateActionRunner: &v2alpha1.PrivateActionRunnerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(true),
						NodeAgent: &v2alpha1.PrivateActionRunnerNodeConfig{
							SelfEnroll: apiutils.NewBoolPointer(true),
						},
					},
				},
			},
			expectedEnvVars: []*corev1.EnvVar{
				{
					Name:  "DD_PRIVATEACTIONRUNNER_SELF_ENROLL",
					Value: "true",
				},
			},
		},
		{
			name: "with actions allowlist",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					PrivateActionRunner: &v2alpha1.PrivateActionRunnerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(true),
						NodeAgent: &v2alpha1.PrivateActionRunnerNodeConfig{
							ActionsAllowlist: []string{
								"com.datadoghq.script.testConnection",
								"com.datadoghq.script.enrichScript",
								"com.datadoghq.script.runPredefinedScript",
								"com.datadoghq.kubernetes.core.listPod",
								"com.datadoghq.kubernetes.core.testConnection",
							},
						},
					},
				},
			},
			expectedEnvVars: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := buildPrivateActionRunnerFeature(nil)
			f.Configure(&v2alpha1.DatadogAgent{}, tt.ddaSpec, nil)

			// Create test managers
			podTmpl := corev1.PodTemplateSpec{}
			managers := fake.NewPodTemplateManagers(t, podTmpl)

			// Call ManageNodeAgent
			err := f.ManageNodeAgent(managers, "")
			assert.NoError(t, err)

			// Verify environment variables
			privateActionRunnerEnvVars := managers.EnvVarMgr.EnvVarsByC[apicommon.PrivateActionRunnerContainerName]
			assert.True(t, apiutils.IsEqualStruct(privateActionRunnerEnvVars, tt.expectedEnvVars),
				"Private action runner envvars \ndiff = %s", cmp.Diff(privateActionRunnerEnvVars, tt.expectedEnvVars))
		})
	}
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

func Test_privateActionRunnerFeature_NodeAgentConfig(t *testing.T) {
	tests := []struct {
		name      string
		ddaSpec   *v2alpha1.DatadogAgentSpec
		wantAgent bool
	}{
		{
			name: "disabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					PrivateActionRunner: &v2alpha1.PrivateActionRunnerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(true),
						NodeAgent: &v2alpha1.PrivateActionRunnerNodeConfig{
							Enabled: apiutils.NewBoolPointer(false),
						},
					},
				},
			},
		},
		{
			name: "node agent enabled if feature is enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					PrivateActionRunner: &v2alpha1.PrivateActionRunnerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(true),
					},
				},
			},
			wantAgent: true,
		},
		{
			name: "explicitly enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					PrivateActionRunner: &v2alpha1.PrivateActionRunnerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(true),
						NodeAgent: &v2alpha1.PrivateActionRunnerNodeConfig{
							Enabled: apiutils.NewBoolPointer(true),
						},
					},
				},
			},
			wantAgent: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := buildPrivateActionRunnerFeature(nil)
			reqComp := f.Configure(&v2alpha1.DatadogAgent{}, tt.ddaSpec, nil)

			if tt.wantAgent {
				assert.True(t, reqComp.Agent.IsEnabled())
				assert.Contains(t, reqComp.Agent.Containers, apicommon.CoreAgentContainerName)
				assert.Contains(t, reqComp.Agent.Containers, apicommon.PrivateActionRunnerContainerName)
			} else {
				assert.False(t, reqComp.Agent.IsEnabled())
			}
		})
	}
}
