// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package npm

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func Test_npmFeature_Configure(t *testing.T) {
	ddaNPMDisabled := v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				NPM: &v2alpha1.NPMFeatureConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
			},
		},
	}
	ddaNPMEnabled := ddaNPMDisabled.DeepCopy()
	ddaNPMEnabled.Spec.Features.NPM.Enabled = apiutils.NewBoolPointer(true)

	ddaNPMEnabledConfig := ddaNPMEnabled.DeepCopy()
	ddaNPMEnabledConfig.Spec.Features.NPM.CollectDNSStats = apiutils.NewBoolPointer(true)
	ddaNPMEnabledConfig.Spec.Features.NPM.EnableConntrack = apiutils.NewBoolPointer(false)

	npmFeatureEnvVarWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)
		// check env vars
		sysProbeWantEnvVars := []*corev1.EnvVar{
			{
				Name:  DDSystemProbeNPMEnabled,
				Value: "true",
			},
			{
				Name:  v2alpha1.DDSystemProbeEnabled,
				Value: "true",
			},
			{
				Name:  v2alpha1.DDSystemProbeSocket,
				Value: v2alpha1.DefaultSystemProbeSocketPath,
			},
			{
				Name:  DDSystemProbeCollectDNSStatsEnabled,
				Value: "true",
			},
			{
				Name:  DDSystemProbeConntrackEnabled,
				Value: "false",
			},
		}
		systemProbeEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(systemProbeEnvVars, sysProbeWantEnvVars), "4. System Probe envvars \ndiff = %s", cmp.Diff(systemProbeEnvVars, sysProbeWantEnvVars))

	}
	npmAgentNodeWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)

		// check annotations
		wantAnnotations := make(map[string]string)
		wantAnnotations[v2alpha1.SystemProbeAppArmorAnnotationKey] = v2alpha1.SystemProbeAppArmorAnnotationValue
		annotations := mgr.AnnotationMgr.Annotations
		assert.True(t, apiutils.IsEqualStruct(annotations, wantAnnotations), "Annotations \ndiff = %s", cmp.Diff(annotations, wantAnnotations))

		// check security context capabilities
		sysProbeCapabilities := mgr.SecurityContextMgr.CapabilitiesByC[apicommon.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(sysProbeCapabilities, agent.DefaultCapabilitiesForSystemProbe()), "System Probe security context capabilities \ndiff = %s", cmp.Diff(sysProbeCapabilities, agent.DefaultCapabilitiesForSystemProbe()))

		// check volume mounts
		wantVolumeMounts := []corev1.VolumeMount{
			{
				Name:      v2alpha1.ProcdirVolumeName,
				MountPath: v2alpha1.ProcdirMountPath,
				ReadOnly:  true,
			},
			{
				Name:      v2alpha1.CgroupsVolumeName,
				MountPath: v2alpha1.CgroupsMountPath,
				ReadOnly:  true,
			},
			{
				Name:      v2alpha1.DebugfsVolumeName,
				MountPath: v2alpha1.DebugfsPath,
				ReadOnly:  false,
			},
		}

		wantProcessAgentVolMounts := append(wantVolumeMounts, corev1.VolumeMount{
			Name:      v2alpha1.SystemProbeSocketVolumeName,
			MountPath: v2alpha1.SystemProbeSocketVolumePath,
			ReadOnly:  true,
		})

		wantSystemProbeAgentVolMounts := append(wantVolumeMounts, corev1.VolumeMount{
			Name:      v2alpha1.SystemProbeSocketVolumeName,
			MountPath: v2alpha1.SystemProbeSocketVolumePath,
			ReadOnly:  false,
		})

		processAgentMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.ProcessAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(processAgentMounts, wantProcessAgentVolMounts), "Process Agent volume mounts \ndiff = %s", cmp.Diff(processAgentMounts, wantProcessAgentVolMounts))

		sysProbeAgentMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(sysProbeAgentMounts, wantSystemProbeAgentVolMounts), "System Probe volume mounts \ndiff = %s", cmp.Diff(sysProbeAgentMounts, wantSystemProbeAgentVolMounts))

		coreWantVolumeMounts := []corev1.VolumeMount{
			{
				Name:      v2alpha1.SystemProbeSocketVolumeName,
				MountPath: v2alpha1.SystemProbeSocketVolumePath,
				ReadOnly:  true,
			},
		}
		coreAgentMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.CoreAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(coreAgentMounts, coreWantVolumeMounts), "Core Agent volume mounts \ndiff = %s", cmp.Diff(coreAgentMounts, coreWantVolumeMounts))

		// check volumes
		wantVolumes := []corev1.Volume{
			{
				Name: v2alpha1.ProcdirVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: v2alpha1.ProcdirHostPath,
					},
				},
			},
			{
				Name: v2alpha1.CgroupsVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: v2alpha1.CgroupsHostPath,
					},
				},
			},
			{
				Name: v2alpha1.DebugfsVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: v2alpha1.DebugfsPath,
					},
				},
			},
			{
				Name: v2alpha1.SystemProbeSocketVolumeName,
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
				Name:  DDSystemProbeNPMEnabled,
				Value: "true",
			},
			{
				Name:  v2alpha1.DDSystemProbeEnabled,
				Value: "true",
			},
			{
				Name:  v2alpha1.DDSystemProbeSocket,
				Value: v2alpha1.DefaultSystemProbeSocketPath,
			},
		}
		npmFeatureEnvVar := []*corev1.EnvVar{
			{
				Name:  DDSystemProbeConntrackEnabled,
				Value: "false",
			},
			{
				Name:  DDSystemProbeCollectDNSStatsEnabled,
				Value: "false",
			},
		}
		sysProbeWantEnvVarsNPM := append(sysProbeWantEnvVars, npmFeatureEnvVar...)
		systemProbeEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(sysProbeWantEnvVarsNPM, sysProbeWantEnvVarsNPM), "System Probe envvars \ndiff = %s", cmp.Diff(systemProbeEnvVars, sysProbeWantEnvVarsNPM))

		processWantEnvVars := append(sysProbeWantEnvVars, &corev1.EnvVar{
			Name:  v2alpha1.DDSystemProbeExternal,
			Value: "true",
		})

		processAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.ProcessAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(processAgentEnvVars, processWantEnvVars), "Process Agent envvars \ndiff = %s", cmp.Diff(processAgentEnvVars, processWantEnvVars))
	}

	tests := test.FeatureTestSuite{
		{
			Name:          "NPM not enabled",
			DDA:           ddaNPMDisabled.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "NPM enabled",
			DDA:           ddaNPMEnabled,
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(npmAgentNodeWantFunc),
		},
		{
			Name:          "NPM enabled, conntrack disable, dnsstat enabled",
			DDA:           ddaNPMEnabledConfig,
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(npmFeatureEnvVarWantFunc),
		},
	}

	tests.Run(t, buildNPMFeature)
}
