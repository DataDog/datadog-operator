// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flightrecorder

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	"github.com/DataDog/datadog-operator/pkg/testutils"
)

func Test_flightRecorderFeature(t *testing.T) {
	flightRecorderEnabledEnvVar := &corev1.EnvVar{
		Name:  common.DDFlightRecorderEnabled,
		Value: "true",
	}
	flightRecorderSocketPathEnvVar := &corev1.EnvVar{
		Name:  common.DDFlightRecorderSocketPath,
		Value: flightRecorderSocketFile,
	}

	tests := test.FeatureTestSuite{
		{
			Name: "flightrecorder disabled (default)",
			DDA: testutils.NewDatadogAgentBuilder().
				BuildWithDefaults(),
			WantConfigure: false,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
					assert.NotContains(t, agentEnvVars, flightRecorderEnabledEnvVar, "DD_FLIGHTRECORDER_ENABLED should not be set when FlightRecorder is not enabled")
				},
			),
		},
		{
			Name: "flightrecorder enabled via env var",
			DDA: testutils.NewDatadogAgentBuilder().
				WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{
					Env: []corev1.EnvVar{
						{Name: common.DDExperimentalFlightRecorderEnabled, Value: "true"},
					},
				}).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)

					// Check env vars on core agent and trace agent
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
					assert.Contains(t, agentEnvVars, flightRecorderEnabledEnvVar, "DD_FLIGHTRECORDER_ENABLED should be set on core agent")
					assert.Contains(t, agentEnvVars, flightRecorderSocketPathEnvVar, "DD_FLIGHTRECORDER_SOCKET_PATH should be set on core agent")

					traceAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.TraceAgentContainerName]
					assert.Contains(t, traceAgentEnvVars, flightRecorderEnabledEnvVar, "DD_FLIGHTRECORDER_ENABLED should be set on trace agent")
					assert.Contains(t, traceAgentEnvVars, flightRecorderSocketPathEnvVar, "DD_FLIGHTRECORDER_SOCKET_PATH should be set on trace agent")

					// Check volumes
					expectedSocketVol := &corev1.Volume{
						Name: flightRecorderSocketVolumeName,
						VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
					}
					expectedDataVol := &corev1.Volume{
						Name: flightRecorderDataVolumeName,
						VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
					}
					assert.Contains(t, mgr.VolumeMgr.Volumes, expectedSocketVol, "socket volume should be added")
					assert.Contains(t, mgr.VolumeMgr.Volumes, expectedDataVol, "data volume should be added")

					// Check volume mounts on core agent (socket only)
					expectedSocketMount := &corev1.VolumeMount{
						Name:      flightRecorderSocketVolumeName,
						MountPath: flightRecorderSocketPath,
					}
					coreAgentMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.CoreAgentContainerName]
					assert.Contains(t, coreAgentMounts, expectedSocketMount, "core agent should have socket volume mount")

					// Check volume mounts on trace agent (socket)
					traceAgentMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.TraceAgentContainerName]
					assert.Contains(t, traceAgentMounts, expectedSocketMount, "trace agent should have socket volume mount")

					// Check volume mounts on flightrecorder container (socket + data)
					expectedDataMount := &corev1.VolumeMount{
						Name:      flightRecorderDataVolumeName,
						MountPath: flightRecorderDataPath,
					}
					frMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.FlightRecorderContainerName]
					assert.Contains(t, frMounts, expectedSocketMount, "flightrecorder should have socket volume mount")
					assert.Contains(t, frMounts, expectedDataMount, "flightrecorder should have data volume mount")
				},
			),
		},
		{
			Name: "flightrecorder explicitly disabled (env var not set)",
			DDA: testutils.NewDatadogAgentBuilder().
				WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{
					Env: []corev1.EnvVar{
						{Name: common.DDExperimentalFlightRecorderEnabled, Value: "false"},
					},
				}).
				BuildWithDefaults(),
			WantConfigure: false,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
					assert.NotContains(t, agentEnvVars, flightRecorderEnabledEnvVar, "DD_FLIGHTRECORDER_ENABLED should not be set when FlightRecorder is disabled")
				},
			),
		},
	}

	tests.Run(t, buildFlightRecorderFeature)
}
