// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dataplane

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
	"github.com/DataDog/datadog-operator/pkg/testutils"
)

func Test_dataPlaneFeature(t *testing.T) {
	dataPlaneEnabledEnvVar := &corev1.EnvVar{
		Name:  common.DDDataPlaneEnabled,
		Value: "true",
	}
	dataPlaneDogstatsdEnabledEnvVar := &corev1.EnvVar{
		Name:  common.DDDataPlaneDogstatsdEnabled,
		Value: "true",
	}
	dataPlaneRemoteAgentEnabledEnvVar := &corev1.EnvVar{
		Name:  common.DDDataPlaneRemoteAgentEnabled,
		Value: "true",
	}
	dataPlaneUseNewConfigStreamEndpointEnvVar := &corev1.EnvVar{
		Name:  common.DDDataPlaneUseNewConfigStreamEndpoint,
		Value: "true",
	}

	tests := test.FeatureTestSuite{
		{
			Name: "data plane disabled (default)",
			DDA: testutils.NewDatadogAgentBuilder().
				BuildWithDefaults(),
			WantConfigure: false,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
					assert.NotContains(t, agentEnvVars, dataPlaneEnabledEnvVar, "DD_DATA_PLANE_ENABLED should not be set when Data Plane is not enabled")
					assert.NotContains(t, agentEnvVars, dataPlaneDogstatsdEnabledEnvVar, "DD_DATA_PLANE_DOGSTATSD_ENABLED should not be set when Data Plane is not enabled")
				},
			),
		},
		{
			Name: "data plane enabled by feature default",
			DDA: testutils.NewDatadogAgentBuilder().
				BuildWithDefaults(),
			FeatureOptions: &feature.Options{
				DefaultDataPlaneEnabled: true,
			},
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
					assert.Contains(t, agentEnvVars, dataPlaneEnabledEnvVar, "DD_DATA_PLANE_ENABLED should be set when Data Plane is enabled by the feature default")

					adpEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.AgentDataPlaneContainerName]
					assert.Contains(t, adpEnvVars, dataPlaneRemoteAgentEnabledEnvVar, "DD_DATA_PLANE_REMOTE_AGENT_ENABLED should be set on Agent Data Plane when Data Plane is enabled by the feature default")
					assert.Contains(t, adpEnvVars, dataPlaneUseNewConfigStreamEndpointEnvVar, "DD_DATA_PLANE_USE_NEW_CONFIG_STREAM_ENDPOINT should be set on Agent Data Plane when Data Plane is enabled by the feature default")

					dda := testutils.NewDatadogAgentBuilder().BuildWithDefaults()
					requiredComponents := buildDataPlaneFeature(&feature.Options{DefaultDataPlaneEnabled: true}).Configure(dda, &dda.Spec, dda.Status.RemoteConfigConfiguration)
					assert.Contains(t, requiredComponents.Agent.Containers, apicommon.AgentDataPlaneContainerName, "Agent Data Plane should be a required Agent component when Data Plane is enabled by the feature default")
				},
			),
		},
		{
			Name: "data plane explicitly disabled via CRD overrides feature default",
			DDA: testutils.NewDatadogAgentBuilder().
				WithDataPlaneEnabled(false).
				BuildWithDefaults(),
			FeatureOptions: &feature.Options{
				DefaultDataPlaneEnabled: true,
			},
			WantConfigure: false,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
					assert.NotContains(t, agentEnvVars, dataPlaneEnabledEnvVar, "DD_DATA_PLANE_ENABLED should not be set when the CRD explicitly disables Data Plane")
				},
			),
		},
		{
			Name: "data plane disabled (forced via annotation)",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAnnotations(map[string]string{
					utils.EnableADPAnnotation: "false",
				}).
				BuildWithDefaults(),
			WantConfigure: false,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
					assert.NotContains(t, agentEnvVars, dataPlaneEnabledEnvVar, "DD_DATA_PLANE_ENABLED should not be set when Data Plane is not enabled")
				},
			),
		},
		{
			Name: "data plane enabled via annotation (deprecated)",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAnnotations(map[string]string{
					utils.EnableADPAnnotation: "true",
				}).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
					assert.Contains(t, agentEnvVars, dataPlaneEnabledEnvVar, "DD_DATA_PLANE_ENABLED should be set when Data Plane is enabled via annotation")
				},
			),
		},
		{
			Name: "data plane enabled via CRD",
			DDA: testutils.NewDatadogAgentBuilder().
				WithDataPlaneEnabled(true).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
					assert.Contains(t, agentEnvVars, dataPlaneEnabledEnvVar, "DD_DATA_PLANE_ENABLED should be set when Data Plane is enabled via CRD")
				},
			),
		},
		{
			Name: "data plane CRD takes precedence over annotation (CRD disabled, annotation enabled)",
			DDA: testutils.NewDatadogAgentBuilder().
				WithDataPlaneEnabled(false).
				WithAnnotations(map[string]string{
					utils.EnableADPAnnotation: "true",
				}).
				BuildWithDefaults(),
			WantConfigure: false,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
					assert.NotContains(t, agentEnvVars, dataPlaneEnabledEnvVar, "DD_DATA_PLANE_ENABLED should not be set when CRD explicitly disables Data Plane")
				},
			),
		},
		{
			Name: "data plane with dogstatsd enabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithDataPlaneEnabled(true).
				WithDataPlaneDogstatsdEnabled(true).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
					assert.Contains(t, agentEnvVars, dataPlaneEnabledEnvVar, "DD_DATA_PLANE_ENABLED should be set")
					assert.Contains(t, agentEnvVars, dataPlaneDogstatsdEnabledEnvVar, "DD_DATA_PLANE_DOGSTATSD_ENABLED should be set when Data Plane DogStatsD is enabled")
				},
			),
		},
	}

	tests.Run(t, buildDataPlaneFeature)
}
