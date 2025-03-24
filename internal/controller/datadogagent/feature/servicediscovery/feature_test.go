// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package servicediscovery

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

func Test_serviceDiscoveryFeature_Configure(t *testing.T) {
	ddaServiceDiscoveryDisabled := v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
					Enabled: apiutils.NewBoolPointer(false),
					NetworkStats: &v2alpha1.ServiceDiscoveryNetworkStatsConfig{
						Enabled: apiutils.NewBoolPointer(false),
					},
				},
			},
		},
	}
	ddaServiceDiscoveryEnabledNoNetStats := ddaServiceDiscoveryDisabled.DeepCopy()
	{
		ddaServiceDiscoveryEnabledNoNetStats.Spec.Features.ServiceDiscovery.Enabled = apiutils.NewBoolPointer(true)
	}
	ddaServiceDiscoveryEnabledWithNetStats := ddaServiceDiscoveryEnabledNoNetStats.DeepCopy()
	{
		ddaServiceDiscoveryEnabledWithNetStats.Spec.Features.ServiceDiscovery.NetworkStats.Enabled = apiutils.NewBoolPointer(true)
	}

	tests := test.FeatureTestSuite{
		{
			Name:          "service discovery not enabled",
			DDA:           ddaServiceDiscoveryDisabled.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "service discovery enabled - no network stats",
			DDA:           ddaServiceDiscoveryEnabledNoNetStats,
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(getWantFunc(noNetStats)),
		},
		{
			Name:          "service discovery enabled - with network stats",
			DDA:           ddaServiceDiscoveryEnabledWithNetStats,
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(getWantFunc(withNetStats)),
		},
	}

	tests.Run(t, buildFeature)
}

const (
	noNetStats   = false
	withNetStats = true
)

func getWantFunc(withNetStats bool) func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	return func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
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
				Name:      common.SystemProbeSocketVolumeName,
				MountPath: common.SystemProbeSocketVolumePath,
				ReadOnly:  false,
			},
		}
		if withNetStats {
			wantSystemProbeVolMounts = append(wantSystemProbeVolMounts,
				corev1.VolumeMount{
					Name:      common.DebugfsVolumeName,
					MountPath: common.DebugfsPath,
					ReadOnly:  false,
				}, corev1.VolumeMount{
					Name:      common.ModulesVolumeName,
					MountPath: common.ModulesVolumePath,
					ReadOnly:  true,
				}, corev1.VolumeMount{
					Name:      common.SrcVolumeName,
					MountPath: common.SrcVolumePath,
					ReadOnly:  true,
				})
		}

		coreAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.CoreAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantCoreAgentVolMounts), "Core agent volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantCoreAgentVolMounts))

		systemProbeVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(systemProbeVolumeMounts, wantSystemProbeVolMounts), "System Probe volume mounts \ndiff = %s", cmp.Diff(systemProbeVolumeMounts, wantSystemProbeVolMounts))

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
				Name: common.SystemProbeSocketVolumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
		}
		if withNetStats {
			wantVolumes = append(wantVolumes,
				corev1.Volume{
					Name: common.DebugfsVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: common.DebugfsPath,
						},
					},
				}, corev1.Volume{
					Name: common.ModulesVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: common.ModulesVolumePath,
						},
					},
				}, corev1.Volume{
					Name: common.SrcVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: common.SrcVolumePath,
						},
					},
				})
		}

		volumes := mgr.VolumeMgr.Volumes
		assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))

		// check env vars
		wantAgentEnvVars := []*corev1.EnvVar{
			{
				Name:  DDServiceDiscoveryEnabled,
				Value: "true",
			},
			{
				Name:  common.DDSystemProbeSocket,
				Value: common.DefaultSystemProbeSocketPath,
			},
		}

		// check env vars
		wantSPEnvVars := []*corev1.EnvVar{
			{
				Name:  DDServiceDiscoveryEnabled,
				Value: "true",
			},
			{
				Name:  DDServiceDiscoveryNetworkStatsEnabled,
				Value: boolToString(withNetStats),
			},
			{
				Name:  common.DDSystemProbeSocket,
				Value: common.DefaultSystemProbeSocketPath,
			},
		}

		agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(agentEnvVars, wantAgentEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, wantAgentEnvVars))

		systemProbeEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(systemProbeEnvVars, wantSPEnvVars), "System Probe envvars \ndiff = %s", cmp.Diff(systemProbeEnvVars, wantSPEnvVars))
	}
}

func boolToString(val bool) string {
	if val {
		return "true"
	}
	return "false"
}
