// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package npm

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

func createEmptyFakeManager(t testing.TB) feature.PodTemplateManagers {
	mgr := fake.NewPodTemplateManagers(t)
	return mgr
}

func Test_npmFeature_Configure(t *testing.T) {
	ddav1NPMDisabled := v1alpha1.DatadogAgent{
		Spec: v1alpha1.DatadogAgentSpec{
			Features: v1alpha1.DatadogFeatures{
				NetworkMonitoring: &v1alpha1.NetworkMonitoringConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
			},
		},
	}

	ddav1NPMEnabled := ddav1NPMDisabled.DeepCopy()
	{
		ddav1NPMEnabled.Spec.Features.NetworkMonitoring.Enabled = apiutils.NewBoolPointer(true)
	}

	ddav2NPMDisabled := v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				NPM: &v2alpha1.NPMFeatureConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
			},
		},
	}
	ddav2NPMEnabled := ddav2NPMDisabled.DeepCopy()
	{
		ddav2NPMEnabled.Spec.Features.NPM.Enabled = apiutils.NewBoolPointer(true)
	}

	npmAgentNodeWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)

		// check volume mounts
		wantVolumeMounts := []corev1.VolumeMount{
			{
				Name:      apicommon.ProcdirVolumeName,
				MountPath: apicommon.ProcdirMountPath,
				ReadOnly:  true,
			},
			{
				Name:      apicommon.CgroupsVolumeName,
				MountPath: apicommon.CgroupsMountPath,
				ReadOnly:  true,
			},
			{
				Name:      apicommon.DebugfsVolumeName,
				MountPath: apicommon.DebugfsVolumePath,
				ReadOnly:  true,
			},
			{
				Name:      apicommon.SysprobeSocketVolumeName,
				MountPath: apicommon.SysprobeSocketVolumePath,
				ReadOnly:  true,
			},
		}

		processAgentMounts := mgr.VolumeMgr.VolumeMountByC[apicommonv1.ProcessAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(processAgentMounts, wantVolumeMounts), "Process Agent volume mounts \ndiff = %s", cmp.Diff(processAgentMounts, wantVolumeMounts))

		sysProbeAgentMounts := mgr.VolumeMgr.VolumeMountByC[apicommonv1.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(sysProbeAgentMounts, wantVolumeMounts), "System Probe volume mounts \ndiff = %s", cmp.Diff(sysProbeAgentMounts, wantVolumeMounts))

		// check volumes
		wantVolumes := []corev1.Volume{
			{
				Name: apicommon.ProcdirVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: apicommon.ProcdirHostPath,
					},
				},
			},
			{
				Name: apicommon.CgroupsVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: apicommon.CgroupsHostPath,
					},
				},
			},
			{
				Name: apicommon.DebugfsVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: apicommon.DebugfsVolumePath,
					},
				},
			},
			{
				Name: apicommon.SysprobeSocketVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: apicommon.SysprobeSocketVolumePath,
					},
				},
			},
		}

		volumes := mgr.VolumeMgr.Volumes
		assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))

		// check env vars
		wantEnvVars := []*corev1.EnvVar{
			{
				Name:  apicommon.DDSystemProbeNPMEnabledEnvVar,
				Value: "true",
			},
			{
				Name:  apicommon.DDSystemProbeEnabledEnvVar,
				Value: "true",
			},
			{
				Name:  apicommon.DDSystemProbeSocketEnvVar,
				Value: apicommon.DefaultSysprobeSocketPath,
			},
			{
				Name:  apicommon.DDProcessAgentEnabledEnvVar,
				Value: "true",
			},
		}
		processAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.ProcessAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(processAgentEnvVars, wantEnvVars), "Process Agent envvars \ndiff = %s", cmp.Diff(processAgentEnvVars, wantEnvVars))

		systemProbeEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(systemProbeEnvVars, wantEnvVars), "System Probe envvars \ndiff = %s", cmp.Diff(systemProbeEnvVars, wantEnvVars))
	}

	tests := test.FeatureTestSuite{
		///////////////////////////
		// v1alpha1.DatadogAgent //
		///////////////////////////
		{
			Name:          "v1alpha1 NPM not enabled",
			DDAv1:         ddav1NPMDisabled.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "v1alpha1 NPM enabled",
			DDAv1:         ddav1NPMEnabled,
			WantConfigure: true,
			Agent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc:   npmAgentNodeWantFunc,
			},
		},
		// ///////////////////////////
		// // v2alpha1.DatadogAgent //
		// ///////////////////////////
		{
			Name:          "v2alpha1 NPM not enabled",
			DDAv2:         ddav2NPMDisabled.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "v2alpha1 NPM enabled",
			DDAv2:         ddav2NPMEnabled,
			WantConfigure: true,
			Agent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc:   npmAgentNodeWantFunc,
			},
		},
	}

	tests.Run(t, buildNPMFeature)
}
