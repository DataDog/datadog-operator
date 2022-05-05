// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package oomkill

import (
	"testing"

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

func Test_oomKillFeature_Configure(t *testing.T) {
	ddav1OOMKillDisabled := v1alpha1.DatadogAgent{
		Spec: v1alpha1.DatadogAgentSpec{
			Agent: v1alpha1.DatadogAgentSpecAgentSpec{
				SystemProbe: v1alpha1.SystemProbeSpec{
					EnableOOMKill: apiutils.NewBoolPointer(false),
				},
			},
		},
	}

	ddav1OOMKillEnabled := ddav1OOMKillDisabled.DeepCopy()
	{
		ddav1OOMKillEnabled.Spec.Agent.SystemProbe.EnableOOMKill = apiutils.NewBoolPointer(true)
	}

	ddav2OOMKillDisable := v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				oomKillFeat: &v2alpha1.OOMKillFeatureConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
			},
		},
	}
	ddav2OOMKillEnable := ddav2OOMKillDisable.DeepCopy()
	{
		ddav2OOMKillEnable.Spec.Features.OOMKill.Enabled = apiutils.NewBoolPointer(true)
	}

	oomKillAgentNodeWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)

		// check volume mounts
		wantVolumeMounts := []corev1.VolumeMount{
			{
				Name:      modulesVolumeName,
				MountPath: modulesVolumePath,
				ReadOnly:  true,
			},
			{
				Name:      srcVolumeName,
				MountPath: srcVolumePath,
				ReadOnly:  true,
			},
		}

		systemProbeVolumeMounts := mgr.VolumeManager.VolumeMountByC[apicommonv1.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(systemProbeVolumeMounts, wantVolumeMounts), "System Probe volume mounts \ndiff = %s", cmp.Diff(systemProbeVolumeMounts, wantVolumeMounts))

		agentVolumeMounts := mgr.VolumeManager.VolumeMountByC[apicommonv1.CoreAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(agentVolumeMounts, wantVolumeMounts), "Agent volume mounts \ndiff = %s", cmp.Diff(agentVolumeMounts, wantVolumeMounts))

		// check volumes
		wantVolumes := []corev1.Volume{
			{
				Name: modulesVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: modulesVolumePath,
					},
				},
			},
			{
				Name: srcVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: srcVolumePath,
					},
				},
			},
		}

		systemProbeVolumes := mgr.VolumeManager.VolumeByC[apicommonv1.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(systemProbeVolumes, wantVolumes), "System Probe volume \ndiff = %s", cmp.Diff(systemProbeVolumes, wantVolumes))

		agentVolumes := mgr.VolumeManager.VolumeByC[apicommonv1.CoreAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(agentVolumes, wantVolumes), "Agent volume \ndiff = %s", cmp.Diff(agentVolumes, wantVolumes))

		// check env vars
		wantEnvVars := []*corev1.EnvVar{
			{
				Name:  DDEnableOOMKillEnvVar,
				Value: "true",
			},
		}
		agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(agentEnvVars, wantEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, wantEnvVars))

		systemProbeEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(systemProbeEnvVars, wantEnvVars), "System Probe envvars \ndiff = %s", cmp.Diff(systemProbeEnvVars, wantEnvVars))
	}

	tests := test.FeatureTestSuite{
		///////////////////////////
		// v1alpha1.DatadogAgent //
		///////////////////////////
		{
			Name:          "v1alpha1 oom kill not enabled",
			DDAv1:         ddav1OOMKillDisabled.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "v1alpha1 oom kill enabled",
			DDAv1:         ddav1OOMKillEnabled,
			WantConfigure: true,
			Agent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc:   oomKillAgentNodeWantFunc,
			},
		},
		///////////////////////////
		// v2alpha1.DatadogAgent //
		///////////////////////////
		{
			Name:          "v2alpha1 oom kill not enabled",
			DDAv2:         ddav2OOMKillDisabled.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "v2alpha1 oom kill enabled",
			DDAv2:         ddav2OOMKillEnabled,
			WantConfigure: true,
			Agent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc:   oomKillAgentNodeWantFunc,
			},
		},
	}

	tests.Run(t, buildOOMKillFeature)
}
