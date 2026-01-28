// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package usm

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
)

func Test_usmFeature_Configure(t *testing.T) {

	ddaUSMDisabled := v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				USM: &v2alpha1.USMFeatureConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
			},
		},
	}
	ddaUSMEnabled := ddaUSMDisabled.DeepCopy()
	{
		ddaUSMEnabled.Spec.Features.USM.Enabled = apiutils.NewBoolPointer(true)
	}
	ddaUSMDirectSendEnabled := ddaUSMEnabled.DeepCopy()
	{
		ddaUSMDirectSendEnabled.Spec.Features.NPM = &v2alpha1.NPMFeatureConfig{DirectSend: apiutils.NewBoolPointer(true)}
	}

	usmAgentNodeWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)

		// check annotations
		wantAnnotations := make(map[string]string)
		wantAnnotations[common.SystemProbeAppArmorAnnotationKey] = common.SystemProbeAppArmorAnnotationValue
		annotations := mgr.AnnotationMgr.Annotations
		assert.True(t, apiutils.IsEqualStruct(annotations, wantAnnotations), "Annotations \ndiff = %s", cmp.Diff(annotations, wantAnnotations))

		// check security context capabilities
		sysProbeCapabilities := mgr.SecurityContextMgr.CapabilitiesByC[apicommon.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(sysProbeCapabilities, agent.DefaultCapabilitiesForSystemProbe()), "System Probe security context capabilities \ndiff = %s", cmp.Diff(sysProbeCapabilities, agent.DefaultCapabilitiesForSystemProbe()))

		// check volume mounts
		wantVolumeMounts := []corev1.VolumeMount{
			{
				Name:      common.ProcdirVolumeName,
				MountPath: common.ProcdirMountPath,
				ReadOnly:  true,
			},
			{
				Name:      common.CgroupsVolumeName,
				MountPath: common.CgroupsMountPath,
				ReadOnly:  true,
			},
			{
				Name:      common.DebugfsVolumeName,
				MountPath: common.DebugfsPath,
				ReadOnly:  false,
			},
			{
				Name:      common.SystemProbeSocketVolumeName,
				MountPath: common.SystemProbeSocketVolumePath,
				ReadOnly:  false,
			},
		}

		sysProbeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(sysProbeMounts, wantVolumeMounts), "System Probe volume mounts \ndiff = %s", cmp.Diff(sysProbeMounts, wantVolumeMounts))

		coreWantVolumeMounts := []corev1.VolumeMount{
			{
				Name:      common.SystemProbeSocketVolumeName,
				MountPath: common.SystemProbeSocketVolumePath,
				ReadOnly:  true,
			},
		}
		coreAgentMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.CoreAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(coreAgentMounts, coreWantVolumeMounts), "Core Agent volume mounts \ndiff = %s", cmp.Diff(coreAgentMounts, coreWantVolumeMounts))

		processWantVolumeMounts := []corev1.VolumeMount{
			{
				Name:      common.ProcdirVolumeName,
				MountPath: common.ProcdirMountPath,
				ReadOnly:  true,
			},
			{
				Name:      common.CgroupsVolumeName,
				MountPath: common.CgroupsMountPath,
				ReadOnly:  true,
			},
			{
				Name:      common.DebugfsVolumeName,
				MountPath: common.DebugfsPath,
				ReadOnly:  false,
			},
			{
				Name:      common.SystemProbeSocketVolumeName,
				MountPath: common.SystemProbeSocketVolumePath,
				ReadOnly:  true,
			},
		}
		processAgentMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.ProcessAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(processAgentMounts, processWantVolumeMounts), "Process Agent volume mounts \ndiff = %s", cmp.Diff(processAgentMounts, processWantVolumeMounts))

		// check volumes
		wantVolumes := []corev1.Volume{
			{
				Name: common.ProcdirVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: common.ProcdirHostPath,
					},
				},
			},
			{
				Name: common.CgroupsVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: common.CgroupsHostPath,
					},
				},
			},
			{
				Name: common.DebugfsVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: common.DebugfsPath,
					},
				},
			},
			{
				Name: common.SystemProbeSocketVolumeName,
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
				Name:  DDSystemProbeServiceMonitoringEnabled,
				Value: "true",
			},
			{
				Name:  common.DDSystemProbeEnabled,
				Value: "true",
			},
			{
				Name:  common.DDSystemProbeSocket,
				Value: common.DefaultSystemProbeSocketPath,
			},
			{
				Name:  DDSystemProbeCNMDirectSend,
				Value: "false",
			},
		}

		systemProbeEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(systemProbeEnvVars, wantEnvVars), "System Probe envvars \ndiff = %s", cmp.Diff(systemProbeEnvVars, wantEnvVars))
	}

	usmDirectSendNodeWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)

		for _, c := range mgr.Tpl.Spec.Containers {
			if c.Name == string(apicommon.ProcessAgentContainerName) {
				assert.Fail(t, "process-agent should not have a container")
			}
		}

		processEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.ProcessAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(processEnvVars, nil), "Process agent envvars \ndiff = %s", cmp.Diff(processEnvVars, nil))

		processAgentMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.ProcessAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(processAgentMounts, nil), "Process Agent volume mounts \ndiff = %s", cmp.Diff(processAgentMounts, nil))

		// check env vars
		wantEnvVars := []*corev1.EnvVar{
			{
				Name:  DDSystemProbeServiceMonitoringEnabled,
				Value: "true",
			},
			{
				Name:  common.DDSystemProbeEnabled,
				Value: "true",
			},
			{
				Name:  common.DDSystemProbeSocket,
				Value: common.DefaultSystemProbeSocketPath,
			},
			{
				Name:  DDSystemProbeCNMDirectSend,
				Value: "true",
			},
		}

		systemProbeEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(systemProbeEnvVars, wantEnvVars), "System Probe envvars \ndiff = %s", cmp.Diff(systemProbeEnvVars, wantEnvVars))
	}

	tests := test.FeatureTestSuite{
		{
			Name:          "USM not enabled",
			DDA:           ddaUSMDisabled.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "USM enabled",
			DDA:           ddaUSMEnabled,
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(usmAgentNodeWantFunc),
		},
		{
			Name:          "USM enabled, Direct Send enabled",
			DDA:           ddaUSMDirectSendEnabled,
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(usmDirectSendNodeWantFunc),
		},
	}

	tests.Run(t, buildUSMFeature)
}
