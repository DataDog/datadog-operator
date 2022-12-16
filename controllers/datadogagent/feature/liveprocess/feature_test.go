// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package liveprocess

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/test"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func Test_liveProcessFeature_Configure(t *testing.T) {
	ddav1LiveProcessDisabled := v1alpha1.DatadogAgent{
		Spec: v1alpha1.DatadogAgentSpec{
			Agent: v1alpha1.DatadogAgentSpecAgentSpec{
				Process: &v1alpha1.ProcessSpec{
					ProcessCollectionEnabled: apiutils.NewBoolPointer(false),
				},
			},
		},
	}

	ddav1LiveProcessEnabled := ddav1LiveProcessDisabled.DeepCopy()
	{
		ddav1LiveProcessEnabled.Spec.Agent.Process.ProcessCollectionEnabled = apiutils.NewBoolPointer(true)
	}

	ddav2LiveProcessDisabled := v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
			},
		},
	}
	ddav2LiveProcessEnabled := ddav2LiveProcessDisabled.DeepCopy()
	{
		ddav2LiveProcessEnabled.Spec.Features.LiveProcessCollection.Enabled = apiutils.NewBoolPointer(true)
	}

	volumeType := corev1.HostPathUnset

	liveProcessAgentNodeWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)

		// check volume mounts
		wantVolumeMounts := []corev1.VolumeMount{
			{
				Name:      apicommon.PasswdVolumeName,
				MountPath: apicommon.PasswdMountPath,
				ReadOnly:  true,
			},
		}

		processAgentMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.ProcessAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(processAgentMounts, wantVolumeMounts), "Process Agent volume mounts \ndiff = %s", cmp.Diff(processAgentMounts, wantVolumeMounts))

		// check volumes
		wantVolumes := []corev1.Volume{
			{
				Name: apicommon.PasswdVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: apicommon.PasswdHostPath,
						Type: &volumeType,
					},
				},
			},
		}

		volumes := mgr.VolumeMgr.Volumes
		assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))

		// check env vars
		wantEnvVars := []*corev1.EnvVar{
			{
				Name:  apicommon.DDProcessCollectionEnabled,
				Value: "true",
			},
		}

		processAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.ProcessAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(processAgentEnvVars, wantEnvVars), "Process Agent envvars \ndiff = %s", cmp.Diff(processAgentEnvVars, wantEnvVars))
	}

	ddav2LiveProcessEnabledWithScrubStripArgs := ddav2LiveProcessEnabled.DeepCopy()
	{
		ddav2LiveProcessEnabledWithScrubStripArgs.Spec.Features.LiveProcessCollection.ScrubProcessArguments = apiutils.NewBoolPointer(true)
		ddav2LiveProcessEnabledWithScrubStripArgs.Spec.Features.LiveProcessCollection.StripProcessArguments = apiutils.NewBoolPointer(true)
	}

	liveProcessAgentNodeWantFuncWithScrubStripArgs := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)

		// check volume mounts
		wantVolumeMounts := []corev1.VolumeMount{
			{
				Name:      apicommon.PasswdVolumeName,
				MountPath: apicommon.PasswdMountPath,
				ReadOnly:  true,
			},
		}

		processAgentMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.ProcessAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(processAgentMounts, wantVolumeMounts), "Process Agent volume mounts \ndiff = %s", cmp.Diff(processAgentMounts, wantVolumeMounts))

		// check volumes
		wantVolumes := []corev1.Volume{
			{
				Name: apicommon.PasswdVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: apicommon.PasswdHostPath,
						Type: &volumeType,
					},
				},
			},
		}

		volumes := mgr.VolumeMgr.Volumes
		assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))

		// check env vars
		wantEnvVars := []*corev1.EnvVar{
			{
				Name:  apicommon.DDProcessCollectionEnabled,
				Value: "true",
			},
			{
				Name:  apicommon.DDProcessConfigScrubArgs,
				Value: "true",
			},
			{
				Name:  apicommon.DDProcessConfigStripArgs,
				Value: "true",
			},
		}

		processAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.ProcessAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(processAgentEnvVars, wantEnvVars), "Process Agent envvars \ndiff = %s", cmp.Diff(processAgentEnvVars, wantEnvVars))
	}

	tests := test.FeatureTestSuite{
		///////////////////////////
		// v1alpha1.DatadogAgent //
		///////////////////////////
		{
			Name:          "v1alpha1 live process collection not enabled",
			DDAv1:         ddav1LiveProcessDisabled.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "v1alpha1 live process collection enabled",
			DDAv1:         ddav1LiveProcessEnabled,
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(liveProcessAgentNodeWantFunc),
		},
		///////////////////////////
		// v2alpha1.DatadogAgent //
		///////////////////////////
		{
			Name:          "v2alpha1 live process collection not enabled",
			DDAv2:         ddav2LiveProcessDisabled.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "v2alpha1 live process collection enabled",
			DDAv2:         ddav2LiveProcessEnabled,
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(liveProcessAgentNodeWantFunc),
		},
		{
			Name:          "v2alpha1 live process collection enabled with scrub and strip args",
			DDAv2:         ddav2LiveProcessEnabledWithScrubStripArgs,
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(liveProcessAgentNodeWantFuncWithScrubStripArgs),
		},
	}

	tests.Run(t, buildLiveProcessFeature)
}
