// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package usm

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

func Test_usmFeature_Configure(t *testing.T) {
	ddav1USMDisabled := v1alpha1.DatadogAgent{
		Spec: v1alpha1.DatadogAgentSpec{
			Agent: v1alpha1.DatadogAgentSpecAgentSpec{
				SystemProbe: &v1alpha1.SystemProbeSpec{
					Enabled: apiutils.NewBoolPointer(false),
				},
			},
		},
	}

	ddav1USMEnabled := ddav1USMDisabled.DeepCopy()
	{
		ddav1USMEnabled.Spec.Agent.SystemProbe.Enabled = apiutils.NewBoolPointer(true)
		ddav1USMEnabled.Spec.Agent.SystemProbe.Env = append(
			ddav1USMEnabled.Spec.Agent.SystemProbe.Env,
			corev1.EnvVar{
				Name:  apicommon.DDSystemProbeServiceMonitoringEnabled,
				Value: "true",
			},
		)
	}

	ddav2USMDisabled := v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				USM: &v2alpha1.USMFeatureConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
			},
		},
	}
	ddav2USMEnabled := ddav2USMDisabled.DeepCopy()
	{
		ddav2USMEnabled.Spec.Features.USM.Enabled = apiutils.NewBoolPointer(true)
	}

	usmAgentNodeWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)

		// check annotations
		wantAnnotations := make(map[string]string)
		wantAnnotations[apicommon.SystemProbeAppArmorAnnotationKey] = apicommon.SystemProbeAppArmorAnnotationValue
		annotations := mgr.AnnotationMgr.Annotations
		assert.True(t, apiutils.IsEqualStruct(annotations, wantAnnotations), "Annotations \ndiff = %s", cmp.Diff(annotations, wantAnnotations))

		// check security context capabilities
		wantCapabilities := []corev1.Capability{
			"SYS_ADMIN",
			"SYS_RESOURCE",
			"SYS_PTRACE",
			"NET_ADMIN",
			"NET_BROADCAST",
			"NET_RAW",
			"IPC_LOCK",
			"CHOWN",
		}
		sysProbeCapabilities := mgr.SecurityContextMgr.CapabilitiesByC[apicommonv1.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(sysProbeCapabilities, wantCapabilities), "System Probe security context capabilities \ndiff = %s", cmp.Diff(sysProbeCapabilities, wantCapabilities))

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
				MountPath: apicommon.DebugfsPath,
				ReadOnly:  true,
			},
			{
				Name:      apicommon.SystemProbeSocketVolumeName,
				MountPath: apicommon.SystemProbeSocketVolumePath,
				ReadOnly:  true,
			},
		}

		sysProbeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(sysProbeMounts, wantVolumeMounts), "System Probe volume mounts \ndiff = %s", cmp.Diff(sysProbeMounts, wantVolumeMounts))

		coreWantVolumeMounts := []corev1.VolumeMount{
			{
				Name:      apicommon.SystemProbeSocketVolumeName,
				MountPath: apicommon.SystemProbeSocketVolumePath,
				ReadOnly:  true,
			},
		}
		coreAgentMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.CoreAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(coreAgentMounts, coreWantVolumeMounts), "Core Agent volume mounts \ndiff = %s", cmp.Diff(coreAgentMounts, coreWantVolumeMounts))

		processWantVolumeMounts := []corev1.VolumeMount{
			{
				Name:      apicommon.SystemProbeSocketVolumeName,
				MountPath: apicommon.SystemProbeSocketVolumePath,
				ReadOnly:  true,
			},
		}
		processAgentMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.ProcessAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(processAgentMounts, processWantVolumeMounts), "Process Agent volume mounts \ndiff = %s", cmp.Diff(processAgentMounts, processWantVolumeMounts))

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
				Name:  apicommon.DDSystemProbeServiceMonitoringEnabled,
				Value: "true",
			},
			{
				Name:  apicommon.DDSystemProbeSocket,
				Value: apicommon.DefaultSystemProbeSocketPath,
			},
		}

		systemProbeEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(systemProbeEnvVars, wantEnvVars), "System Probe envvars \ndiff = %s", cmp.Diff(systemProbeEnvVars, wantEnvVars))
	}

	tests := test.FeatureTestSuite{
		///////////////////////////
		// v1alpha1.DatadogAgent //
		///////////////////////////
		{
			Name:          "v1alpha1 USM not enabled",
			DDAv1:         ddav1USMDisabled.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "v1alpha1 USM enabled",
			DDAv1:         ddav1USMEnabled,
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(usmAgentNodeWantFunc),
		},
		// ///////////////////////////
		// // v2alpha1.DatadogAgent //
		// ///////////////////////////
		{
			Name:          "v2alpha1 USM not enabled",
			DDAv2:         ddav2USMDisabled.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "v2alpha1 USM enabled",
			DDAv2:         ddav2USMEnabled,
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(usmAgentNodeWantFunc),
		},
	}

	tests.Run(t, buildUSMFeature)
}
