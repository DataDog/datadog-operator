// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package oomkill

import (
	"testing"

	"k8s.io/utils/ptr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/providercaps"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func Test_oomKillFeature_Configure(t *testing.T) {
	ddaOOMKillDisabled := v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				OOMKill: &v2alpha1.OOMKillFeatureConfig{
					Enabled: ptr.To(false),
				},
			},
		},
	}
	ddaOOMKillEnabled := ddaOOMKillDisabled.DeepCopy()
	{
		ddaOOMKillEnabled.Spec.Features.OOMKill.Enabled = ptr.To(true)
	}

	oomKillAgentNodeWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)

		// check security context capabilities
		sysProbeCapabilities := mgr.SecurityContextMgr.CapabilitiesByC[apicommon.SystemProbeContainerName]
		assert.True(
			t,
			apiutils.IsEqualStruct(sysProbeCapabilities, agent.DefaultCapabilitiesForSystemProbe()),
			"System Probe security context capabilities \ndiff = %s",
			cmp.Diff(sysProbeCapabilities, agent.DefaultCapabilitiesForSystemProbe()),
		)

		// check volume mounts
		wantCoreAgentVolMounts := []corev1.VolumeMount{
			{
				Name:      common.SystemProbeSocketVolumeName,
				MountPath: common.SystemProbeSocketVolumePath,
				ReadOnly:  true,
			},
		}

		wantSystemProbeVolMounts := []corev1.VolumeMount{
			{
				Name:      common.ModulesVolumeName,
				MountPath: common.ModulesVolumePath,
				ReadOnly:  true,
			},
			{
				Name:      common.SrcVolumeName,
				MountPath: common.SrcVolumePath,
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

		coreAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.CoreAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantCoreAgentVolMounts), "Core agent volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantCoreAgentVolMounts))

		systemProbeVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(systemProbeVolumeMounts, wantSystemProbeVolMounts), "System Probe volume mounts \ndiff = %s", cmp.Diff(systemProbeVolumeMounts, wantSystemProbeVolMounts))

		// check volumes
		wantVolumes := []corev1.Volume{
			{
				Name: common.ModulesVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: common.ModulesVolumePath,
					},
				},
			},
			{
				Name: common.SrcVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: common.SrcVolumePath,
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
				Name:  DDEnableOOMKillEnvVar,
				Value: "true",
			},
			{
				Name:  common.DDSystemProbeSocket,
				Value: common.DefaultSystemProbeSocketPath,
			},
		}
		agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(agentEnvVars, wantEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, wantEnvVars))

		systemProbeEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(systemProbeEnvVars, wantEnvVars), "System Probe envvars \ndiff = %s", cmp.Diff(systemProbeEnvVars, wantEnvVars))
	}

	tests := test.FeatureTestSuite{
		{
			Name:          "oom kill not enabled",
			DDA:           ddaOOMKillDisabled.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "oom kill enabled",
			DDA:           ddaOOMKillEnabled,
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(oomKillAgentNodeWantFunc),
		},
	}

	tests.Run(t, buildOOMKillFeature)
}

func Test_oomKillFeature_NodeAgentProviderCapabilities(t *testing.T) {
	newPodTemplate := func() *corev1.PodTemplateSpec {
		return &corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: string(apicommon.CoreAgentContainerName)},
					{Name: string(apicommon.SystemProbeContainerName)},
				},
			},
		}
	}

	getContainer := func(tmpl *corev1.PodTemplateSpec, name apicommon.AgentContainerName) corev1.Container {
		for _, c := range tmpl.Spec.Containers {
			if c.Name == string(name) {
				return c
			}
		}
		t.Fatalf("container %q not found", name)
		return corev1.Container{}
	}

	volumeNames := func(tmpl *corev1.PodTemplateSpec) []string {
		names := make([]string, 0, len(tmpl.Spec.Volumes))
		for _, v := range tmpl.Spec.Volumes {
			names = append(names, v.Name)
		}
		return names
	}

	t.Run("talos strips src and modules volumes", func(t *testing.T) {
		f := &oomKillFeature{}
		tmpl := newPodTemplate()
		mgr := feature.NewPodTemplateManagers(tmpl)
		require.NoError(t, f.ManageNodeAgent(mgr))

		providercaps.ApplyProviderCapabilities(mgr, kubernetes.TalosProvider, f.NodeAgentProviderCapabilities())

		assert.NotContains(t, volumeNames(tmpl), common.SrcVolumeName)
		assert.NotContains(t, volumeNames(tmpl), common.ModulesVolumeName)
		systemProbeMounts := getContainer(tmpl, apicommon.SystemProbeContainerName).VolumeMounts
		for _, m := range systemProbeMounts {
			assert.NotEqual(t, common.SrcVolumeName, m.Name)
			assert.NotEqual(t, common.ModulesVolumeName, m.Name)
		}
	})

	t.Run("default provider keeps src and modules volumes", func(t *testing.T) {
		f := &oomKillFeature{}
		tmpl := newPodTemplate()
		mgr := feature.NewPodTemplateManagers(tmpl)
		require.NoError(t, f.ManageNodeAgent(mgr))

		providercaps.ApplyProviderCapabilities(mgr, kubernetes.DefaultProvider, f.NodeAgentProviderCapabilities())

		assert.Contains(t, volumeNames(tmpl), common.SrcVolumeName)
		assert.Contains(t, volumeNames(tmpl), common.ModulesVolumeName)
	})
}
