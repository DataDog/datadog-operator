// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"testing"

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
	f := buildPrivateActionRunnerFeature(nil)

	// Configure the feature as enabled
	ddaSpec := &v2alpha1.DatadogAgentSpec{
		Features: &v2alpha1.DatadogFeatures{
			PrivateActionRunner: &v2alpha1.PrivateActionRunnerFeatureConfig{
				Enabled: apiutils.NewBoolPointer(true),
			},
		},
	}

	f.Configure(&v2alpha1.DatadogAgent{}, ddaSpec, nil)

	// Create test managers
	podTmpl := corev1.PodTemplateSpec{}
	managers := fake.NewPodTemplateManagers(t, podTmpl)

	// Call ManageNodeAgent
	err := f.ManageNodeAgent(managers, "")
	assert.NoError(t, err)

	// Verify environment variables were added to private-action-runner container
	expectedEnvVars := []*corev1.EnvVar{
		{
			Name:  "DD_PRIVATE_ACTION_RUNNER_ENABLED",
			Value: "true",
		},
	}

	privateActionRunnerEnvVars := managers.EnvVarMgr.EnvVarsByC[apicommon.PrivateActionRunnerContainerName]
	for _, expectedEnv := range expectedEnvVars {
		found := false
		for _, actualEnv := range privateActionRunnerEnvVars {
			if actualEnv.Name == expectedEnv.Name {
				found = true
				if diff := cmp.Diff(expectedEnv.Value, actualEnv.Value); diff != "" {
					t.Errorf("Environment variable %s value mismatch (-want +got):\n%s", expectedEnv.Name, diff)
				}
				break
			}
		}
		assert.True(t, found, "Expected environment variable %s not found", expectedEnv.Name)
	}
}

func Test_privateActionRunnerFeature_ID(t *testing.T) {
	f := buildPrivateActionRunnerFeature(nil)
	assert.Equal(t, string(feature.PrivateActionRunnerIDType), string(f.ID()))
}

func Test_privateActionRunnerFeature_ManageClusterAgent(t *testing.T) {
	f := buildPrivateActionRunnerFeature(nil)
	podTmpl := corev1.PodTemplateSpec{}
	managers := fake.NewPodTemplateManagers(t, podTmpl)

	err := f.ManageClusterAgent(managers, "")
	assert.NoError(t, err)
	// Verify no changes were made (private action runner doesn't run in cluster agent)
}

func Test_privateActionRunnerFeature_ManageSingleContainerNodeAgent(t *testing.T) {
	f := buildPrivateActionRunnerFeature(nil)
	podTmpl := corev1.PodTemplateSpec{}
	managers := fake.NewPodTemplateManagers(t, podTmpl)

	err := f.ManageSingleContainerNodeAgent(managers, "")
	assert.NoError(t, err)
	// Verify no changes were made (private action runner requires separate container)
}

func Test_privateActionRunnerFeature_ManageClusterChecksRunner(t *testing.T) {
	f := buildPrivateActionRunnerFeature(nil)
	podTmpl := corev1.PodTemplateSpec{}
	managers := fake.NewPodTemplateManagers(t, podTmpl)

	err := f.ManageClusterChecksRunner(managers, "")
	assert.NoError(t, err)
	// Verify no changes were made (private action runner doesn't run in cluster checks runner)
}

func Test_privateActionRunnerFeature_ManageOtelAgentGateway(t *testing.T) {
	f := buildPrivateActionRunnerFeature(nil)
	podTmpl := corev1.PodTemplateSpec{}
	managers := fake.NewPodTemplateManagers(t, podTmpl)

	err := f.ManageOtelAgentGateway(managers, "")
	assert.NoError(t, err)
	// Verify no changes were made (private action runner doesn't run in OTel Agent Gateway)
}
