// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package oomkill

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

func Test_oomKillFeature_Configure(t *testing.T) {
	ddav1OOMKillDisabled := v1alpha1.DatadogAgent{
		Spec: v1alpha1.DatadogAgentSpec{
			Agent: v1alpha1.DatadogAgentSpecAgentSpec{
				SystemProbe: &v1alpha1.SystemProbeSpec{
					EnableOOMKill: apiutils.NewBoolPointer(false),
				},
			},
		},
	}

	ddav1OOMKillEnabled := ddav1OOMKillDisabled.DeepCopy()
	{
		ddav1OOMKillEnabled.Spec.Agent.SystemProbe.EnableOOMKill = apiutils.NewBoolPointer(true)
	}

	ddav2OOMKillDisabled := v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				OOMKill: &v2alpha1.OOMKillFeatureConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
			},
		},
	}
	ddav2OOMKillEnabled := ddav2OOMKillDisabled.DeepCopy()
	{
		ddav2OOMKillEnabled.Spec.Features.OOMKill.Enabled = apiutils.NewBoolPointer(true)
	}

	oomKillAgentNodeWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)

		// check volume mounts
		wantVolumeMounts := []corev1.VolumeMount{
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
		}

		systemProbeVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(systemProbeVolumeMounts, wantVolumeMounts), "System Probe volume mounts \ndiff = %s", cmp.Diff(systemProbeVolumeMounts, wantVolumeMounts))

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
		}

		volumes := mgr.VolumeMgr.Volumes
		assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))

		// check env vars
		wantEnvVars := []*corev1.EnvVar{
			{
				Name:  apicommon.DDEnableOOMKillEnvVar,
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
