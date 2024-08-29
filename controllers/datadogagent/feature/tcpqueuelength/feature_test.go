// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tcpqueuelength

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/test"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func Test_tcpQueueLengthFeature_Configure(t *testing.T) {

	ddaTCPQLDisabled := v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
			},
		},
	}
	ddaTCPQLEnabled := ddaTCPQLDisabled.DeepCopy()
	{
		ddaTCPQLEnabled.Spec.Features.TCPQueueLength.Enabled = apiutils.NewBoolPointer(true)
	}

	tcpQueueLengthAgentNodeWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)

		// check security context capabilities
		sysProbeCapabilities := mgr.SecurityContextMgr.CapabilitiesByC[apicommonv1.SystemProbeContainerName]
		assert.True(
			t,
			apiutils.IsEqualStruct(sysProbeCapabilities, agent.DefaultCapabilitiesForSystemProbe()),
			"System Probe security context capabilities \ndiff = %s",
			cmp.Diff(sysProbeCapabilities, agent.DefaultCapabilitiesForSystemProbe()),
		)

		// check volume mounts
		wantCoreAgentVolMounts := []corev1.VolumeMount{
			{
				Name:      apicommon.SystemProbeSocketVolumeName,
				MountPath: apicommon.SystemProbeSocketVolumePath,
				ReadOnly:  true,
			},
		}

		wantSystemProbeVolMounts := []corev1.VolumeMount{
			{
				Name:      apicommon.ModulesVolumeName,
				MountPath: apicommon.ModulesVolumePath,
				ReadOnly:  true,
			},
			{
				Name:      apicommon.SrcVolumeName,
				MountPath: apicommon.SrcVolumePath,
				ReadOnly:  true,
			},
			{
				Name:      apicommon.DebugfsVolumeName,
				MountPath: apicommon.DebugfsPath,
				ReadOnly:  false,
			},
			{
				Name:      apicommon.SystemProbeSocketVolumeName,
				MountPath: apicommon.SystemProbeSocketVolumePath,
				ReadOnly:  false,
			},
		}

		coreAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.CoreAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantCoreAgentVolMounts), "Core agent volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantCoreAgentVolMounts))

		systemProbeVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(systemProbeVolumeMounts, wantSystemProbeVolMounts), "System Probe volume mounts \ndiff = %s", cmp.Diff(systemProbeVolumeMounts, wantSystemProbeVolMounts))

		// check volumes
		wantVolumes := []corev1.Volume{
			{
				Name: apicommon.ModulesVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: apicommon.ModulesVolumePath,
					},
				},
			},
			{
				Name: apicommon.SrcVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: apicommon.SrcVolumePath,
					},
				},
			},
			{
				Name: apicommon.DebugfsVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: apicommon.DebugfsPath,
					},
				},
			},
			{
				Name: apicommon.SystemProbeSocketVolumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
		}

		volumes := mgr.VolumeMgr.Volumes
		assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))

		// check env vars
		wantEnvVars := []*corev1.EnvVar{
			{
				Name:  apicommon.DDEnableTCPQueueLengthEnvVar,
				Value: "true",
			},
			{
				Name:  apicommon.DDSystemProbeEnabled,
				Value: "true",
			},
			{
				Name:  apicommon.DDSystemProbeSocket,
				Value: apicommon.DefaultSystemProbeSocketPath,
			},
		}
		agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(agentEnvVars, wantEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, wantEnvVars))

		systemProbeEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(systemProbeEnvVars, wantEnvVars), "System Probe envvars \ndiff = %s", cmp.Diff(systemProbeEnvVars, wantEnvVars))
	}

	tests := test.FeatureTestSuite{
		{
			Name:          "tcp queue length not enabled",
			DDA:           ddaTCPQLDisabled.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "tcp queue length enabled",
			DDA:           ddaTCPQLEnabled,
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(tcpQueueLengthAgentNodeWantFunc),
		},
	}

	tests.Run(t, buildTCPQueueLengthFeature)
}
