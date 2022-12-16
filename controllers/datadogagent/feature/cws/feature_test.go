// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cws

import (
	"testing"

	"github.com/DataDog/datadog-operator/controllers/datadogagent/component/agent"
	corev1 "k8s.io/api/core/v1"

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
)

func Test_cwsFeature_Configure(t *testing.T) {
	ddav1CWSDisabled := v1alpha1.DatadogAgent{
		Spec: v1alpha1.DatadogAgentSpec{
			Agent: v1alpha1.DatadogAgentSpecAgentSpec{
				Security: &v1alpha1.SecuritySpec{
					Runtime: v1alpha1.RuntimeSecuritySpec{
						Enabled: apiutils.NewBoolPointer(false),
					},
				},
			},
		},
	}

	ddav1CWSEnabled := ddav1CWSDisabled.DeepCopy()
	{
		ddav1CWSEnabled.Spec.Agent.Security.Runtime.Enabled = apiutils.NewBoolPointer(true)
		ddav1CWSEnabled.Spec.Agent.Security.Runtime.PoliciesDir = &v1alpha1.ConfigDirSpec{
			ConfigMapName: "custom_test",
			Items: []corev1.KeyToPath{
				{
					Key:  "key1",
					Path: "some/path",
				},
			},
		}
		ddav1CWSEnabled.Spec.Agent.Security.Runtime.SyscallMonitor = &v1alpha1.SyscallMonitorSpec{
			Enabled: apiutils.NewBoolPointer(true),
		}
	}

	ddav2CWSDisabled := v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				CWS: &v2alpha1.CWSFeatureConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
			},
		},
	}
	ddav2CWSEnabled := ddav2CWSDisabled.DeepCopy()
	{
		ddav2CWSEnabled.Spec.Features.CWS.Enabled = apiutils.NewBoolPointer(true)
		ddav2CWSEnabled.Spec.Features.CWS.CustomPolicies = &v2alpha1.CustomConfig{
			ConfigMap: &apicommonv1.ConfigMapConfig{
				Name: "custom_test",
				Items: []corev1.KeyToPath{
					{
						Key:  "key1",
						Path: "some/path",
					},
				},
			},
		}
		ddav2CWSEnabled.Spec.Features.CWS.SyscallMonitorEnabled = apiutils.NewBoolPointer(true)
	}

	cwsAgentNodeWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)

		// check security context capabilities
		sysProbeCapabilities := mgr.SecurityContextMgr.CapabilitiesByC[apicommonv1.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(sysProbeCapabilities, agent.DefaultCapabilitiesForSystemProbe()), "System Probe security context capabilities \ndiff = %s", cmp.Diff(sysProbeCapabilities, agent.DefaultCapabilitiesForSystemProbe()))

		securityWant := []*corev1.EnvVar{
			{
				Name:  apicommon.DDRuntimeSecurityConfigEnabled,
				Value: "true",
			},
			{
				Name:  apicommon.DDRuntimeSecurityConfigSocket,
				Value: "/var/run/sysprobe/runtime-security.sock",
			},
			{
				Name:  apicommon.DDRuntimeSecurityConfigSyscallMonitorEnabled,
				Value: "true",
			},
			{
				Name:  apicommon.DDHostRootEnvVar,
				Value: apicommon.HostRootMountPath,
			},
			{
				Name:  apicommon.DDRuntimeSecurityConfigPoliciesDir,
				Value: apicommon.SecurityAgentRuntimePoliciesDirVolumePath,
			},
		}
		sysProbeWant := []*corev1.EnvVar{
			{
				Name:  apicommon.DDRuntimeSecurityConfigEnabled,
				Value: "true",
			},
			{
				Name:  apicommon.DDRuntimeSecurityConfigSocket,
				Value: "/var/run/sysprobe/runtime-security.sock",
			},
			{
				Name:  apicommon.DDRuntimeSecurityConfigSyscallMonitorEnabled,
				Value: "true",
			},
			{
				Name:  apicommon.DDRuntimeSecurityConfigPoliciesDir,
				Value: apicommon.SecurityAgentRuntimePoliciesDirVolumePath,
			},
		}
		securityAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.SecurityAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(securityAgentEnvVars, securityWant), "Security agent envvars \ndiff = %s", cmp.Diff(securityAgentEnvVars, securityWant))
		sysProbeEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(sysProbeEnvVars, sysProbeWant), "System probe envvars \ndiff = %s", cmp.Diff(sysProbeEnvVars, sysProbeWant))

		// check volume mounts
		securityWantVolumeMount := []corev1.VolumeMount{
			{
				Name:      apicommon.SystemProbeSocketVolumeName,
				MountPath: apicommon.SystemProbeSocketVolumePath,
				ReadOnly:  true,
			},
			{
				Name:      apicommon.HostRootVolumeName,
				MountPath: apicommon.HostRootMountPath,
				ReadOnly:  true,
			},
			{
				Name:      apicommon.SecurityAgentRuntimePoliciesDirVolumeName,
				MountPath: apicommon.SecurityAgentRuntimePoliciesDirVolumePath,
				ReadOnly:  true,
			},
		}
		sysprobeWantVolumeMount := []corev1.VolumeMount{
			{
				Name:      apicommon.DebugfsVolumeName,
				MountPath: apicommon.DebugfsPath,
				ReadOnly:  false,
			},
			{
				Name:      apicommon.SecurityfsVolumeName,
				MountPath: apicommon.SecurityfsMountPath,
				ReadOnly:  true,
			},
			{
				Name:      apicommon.SystemProbeSocketVolumeName,
				MountPath: apicommon.SystemProbeSocketVolumePath,
				ReadOnly:  false,
			},
			{
				Name:      apicommon.ProcdirVolumeName,
				MountPath: apicommon.ProcdirMountPath,
				ReadOnly:  true,
			},
			{
				Name:      apicommon.PasswdVolumeName,
				MountPath: apicommon.PasswdMountPath,
				ReadOnly:  true,
			},
			{
				Name:      apicommon.GroupVolumeName,
				MountPath: apicommon.GroupMountPath,
				ReadOnly:  true,
			},
			{
				Name:      apicommon.SystemProbeOSReleaseDirVolumeName,
				MountPath: apicommon.SystemProbeOSReleaseDirMountPath,
				ReadOnly:  true,
			},
			{
				Name:      apicommon.SecurityAgentRuntimePoliciesDirVolumeName,
				MountPath: apicommon.SecurityAgentRuntimePoliciesDirVolumePath,
				ReadOnly:  true,
			},
		}

		securityAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.SecurityAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(securityAgentVolumeMounts, securityWantVolumeMount), "Security Agent volume mounts \ndiff = %s", cmp.Diff(securityAgentVolumeMounts, securityWantVolumeMount))
		sysProbeVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(sysProbeVolumeMounts, sysprobeWantVolumeMount), "System probe volume mounts \ndiff = %s", cmp.Diff(sysProbeVolumeMounts, sysprobeWantVolumeMount))

		// check volumes
		volumeType := corev1.HostPathUnset
		wantVolumes := []corev1.Volume{
			{
				Name: apicommon.DebugfsVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: apicommon.DebugfsPath,
						Type: &volumeType,
					},
				},
			},
			{
				Name: apicommon.SecurityfsVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: apicommon.SecurityfsVolumePath,
						Type: &volumeType,
					},
				},
			},
			{
				Name: apicommon.SystemProbeSocketVolumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
			{
				Name: apicommon.ProcdirVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: apicommon.ProcdirHostPath,
						Type: &volumeType,
					},
				},
			},
			{
				Name: apicommon.PasswdVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: apicommon.PasswdHostPath,
						Type: &volumeType,
					},
				},
			},
			{
				Name: apicommon.GroupVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: apicommon.GroupHostPath,
						Type: &volumeType,
					},
				},
			},
			{
				Name: apicommon.SystemProbeOSReleaseDirVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: apicommon.SystemProbeOSReleaseDirVolumePath,
						Type: &volumeType,
					},
				},
			},
			{
				Name: apicommon.HostRootVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: apicommon.HostRootHostPath,
						Type: &volumeType,
					},
				},
			},
			{
				Name: cwsConfigVolumeName,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "custom_test",
						},
						Items: []corev1.KeyToPath{{Key: "key1", Path: "some/path"}},
					},
				},
			},
			{
				Name: apicommon.SecurityAgentRuntimePoliciesDirVolumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
		}

		volumes := mgr.VolumeMgr.Volumes
		assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))
	}

	tests := test.FeatureTestSuite{
		//////////////////////////
		// v1Alpha1.DatadogAgent
		//////////////////////////
		{
			Name:          "v1alpha1 CWS not enabled",
			DDAv1:         ddav1CWSDisabled.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "v1alpha1 CWS enabled",
			DDAv1:         ddav1CWSEnabled,
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(cwsAgentNodeWantFunc),
		},
		//////////////////////////
		// v2Alpha1.DatadogAgent
		//////////////////////////
		{
			Name:          "v2alpha1 CWS not enabled",
			DDAv2:         ddav2CWSDisabled.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "v2alpha1 CWS enabled",
			DDAv2:         ddav2CWSEnabled,
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(cwsAgentNodeWantFunc),
		},
	}

	tests.Run(t, buildCWSFeature)
}
