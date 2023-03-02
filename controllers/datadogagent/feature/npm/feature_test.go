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
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/test"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

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
	ddav2NPMEnabled.Spec.Features.NPM.Enabled = apiutils.NewBoolPointer(true)

	ddav2NPMEnabledConfig := ddav2NPMEnabled.DeepCopy()
	ddav2NPMEnabledConfig.Spec.Features.NPM.CollectDNSStats = apiutils.NewBoolPointer(true)
	ddav2NPMEnabledConfig.Spec.Features.NPM.EnableConntrack = apiutils.NewBoolPointer(false)

	npmFeatureEnvVarWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)
		// check env vars
		sysProbeWantEnvVars := []*corev1.EnvVar{
			{
				Name:  apicommon.DDSystemProbeNPMEnabled,
				Value: "true",
			},
			{
				Name:  apicommon.DDSystemProbeEnabled,
				Value: "true",
			},
			{
				Name:  apicommon.DDSystemProbeCollectDNSStatsEnabled,
				Value: "true",
			},
			{
				Name:  apicommon.DDSystemProbeConntrackEnabled,
				Value: "false",
			},
			{
				Name:  apicommon.DDSystemProbeSocket,
				Value: apicommon.DefaultSystemProbeSocketPath,
			},
		}
		systemProbeEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(systemProbeEnvVars, sysProbeWantEnvVars), "4. System Probe envvars \ndiff = %s", cmp.Diff(systemProbeEnvVars, sysProbeWantEnvVars))

	}
	npmAgentNodeWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)

		// check annotations
		wantAnnotations := make(map[string]string)
		wantAnnotations[apicommon.SystemProbeAppArmorAnnotationKey] = apicommon.SystemProbeAppArmorAnnotationValue
		annotations := mgr.AnnotationMgr.Annotations
		assert.True(t, apiutils.IsEqualStruct(annotations, wantAnnotations), "Annotations \ndiff = %s", cmp.Diff(annotations, wantAnnotations))

		// check security context capabilities
		sysProbeCapabilities := mgr.SecurityContextMgr.CapabilitiesByC[apicommonv1.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(sysProbeCapabilities, agent.DefaultCapabilitiesForSystemProbe()), "System Probe security context capabilities \ndiff = %s", cmp.Diff(sysProbeCapabilities, agent.DefaultCapabilitiesForSystemProbe()))

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
				ReadOnly:  false,
			},
		}

		wantProcessAgentVolMounts := append(wantVolumeMounts, corev1.VolumeMount{
			Name:      apicommon.SystemProbeSocketVolumeName,
			MountPath: apicommon.SystemProbeSocketVolumePath,
			ReadOnly:  true,
		})

		wantSystemProbeAgentVolMounts := append(wantVolumeMounts, corev1.VolumeMount{
			Name:      apicommon.SystemProbeSocketVolumeName,
			MountPath: apicommon.SystemProbeSocketVolumePath,
			ReadOnly:  false,
		})

		processAgentMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.ProcessAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(processAgentMounts, wantProcessAgentVolMounts), "Process Agent volume mounts \ndiff = %s", cmp.Diff(processAgentMounts, wantProcessAgentVolMounts))

		sysProbeAgentMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(sysProbeAgentMounts, wantSystemProbeAgentVolMounts), "System Probe volume mounts \ndiff = %s", cmp.Diff(sysProbeAgentMounts, wantSystemProbeAgentVolMounts))

		coreWantVolumeMounts := []corev1.VolumeMount{
			{
				Name:      apicommon.SystemProbeSocketVolumeName,
				MountPath: apicommon.SystemProbeSocketVolumePath,
				ReadOnly:  true,
			},
		}
		coreAgentMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.CoreAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(coreAgentMounts, coreWantVolumeMounts), "Core Agent volume mounts \ndiff = %s", cmp.Diff(coreAgentMounts, coreWantVolumeMounts))

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
		sysProbeWantEnvVars := []*corev1.EnvVar{
			{
				Name:  apicommon.DDSystemProbeNPMEnabled,
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
		npmFeatureEnvVar := []*corev1.EnvVar{
			{
				Name:  apicommon.DDSystemProbeConntrackEnabled,
				Value: "false",
			},
			{
				Name:  apicommon.DDSystemProbeCollectDNSStatsEnabled,
				Value: "false",
			},
		}
		sysProbeWantEnvVarsNPM := append(sysProbeWantEnvVars, npmFeatureEnvVar...)
		systemProbeEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(sysProbeWantEnvVarsNPM, sysProbeWantEnvVarsNPM), "System Probe envvars \ndiff = %s", cmp.Diff(systemProbeEnvVars, sysProbeWantEnvVarsNPM))

		processWantEnvVars := append(sysProbeWantEnvVars, &corev1.EnvVar{
			Name:  apicommon.DDSystemProbeExternal,
			Value: "true",
		})

		processAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.ProcessAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(processAgentEnvVars, processWantEnvVars), "Process Agent envvars \ndiff = %s", cmp.Diff(processAgentEnvVars, processWantEnvVars))
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
			Agent:         test.NewDefaultComponentTest().WithWantFunc(npmAgentNodeWantFunc),
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
			Agent:         test.NewDefaultComponentTest().WithWantFunc(npmAgentNodeWantFunc),
		},
		{
			Name:          "v2alpha1 NPM enabled, conntrack disable, dnsstat enabled",
			DDAv2:         ddav2NPMEnabledConfig,
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(npmFeatureEnvVarWantFunc),
		},
	}

	tests.Run(t, buildNPMFeature)
}
