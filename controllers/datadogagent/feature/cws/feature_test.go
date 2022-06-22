// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cws

import (
	"testing"

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

func createEmptyFakeManager(t testing.TB) feature.PodTemplateManagers {
	mgr := fake.NewPodTemplateManagers(t)
	return mgr
}

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
		ddav2CWSEnabled.Spec.Features.CWS.CustomPolicies = &apicommonv1.ConfigMapConfig{
			Name: "custom_test",
			Items: []corev1.KeyToPath{
				{
					Key:  "key1",
					Path: "some/path",
				},
			},
		}
		ddav2CWSEnabled.Spec.Features.CWS.SyscallMonitorEnabled = apiutils.NewBoolPointer(true)
	}

	cwsAgentNodeWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)

		want := []*corev1.EnvVar{
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
		sysProbeWant := &corev1.EnvVar{
			Name:  apicommon.DDAuthTokenFilePath,
			Value: "/etc/datadog-agent/auth/token",
		}
		securityAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.SecurityAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(securityAgentEnvVars, want), "Security agent envvars \ndiff = %s", cmp.Diff(securityAgentEnvVars, want))
		sysProbeEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(sysProbeEnvVars, append(want, sysProbeWant)), "System probe envvars \ndiff = %s", cmp.Diff(sysProbeEnvVars, append(want, sysProbeWant)))

		// check volume mounts
		wantVolumeMounts := []corev1.VolumeMount{
			{
				Name:      apicommon.SecurityAgentRuntimeCustomPoliciesVolumeName,
				MountPath: apicommon.SecurityAgentRuntimeCustomPoliciesVolumePath,
				SubPath:   "some/path",
				ReadOnly:  true,
			},
			{
				Name:      apicommon.SecurityAgentRuntimePoliciesDirVolumeName,
				MountPath: apicommon.SecurityAgentRuntimePoliciesDirVolumePath,
				ReadOnly:  true,
			},
		}
		securityWantVolumeMount := corev1.VolumeMount{
			Name:      apicommon.SystemProbeSocketVolumeName,
			MountPath: apicommon.SystemProbeSocketVolumePath,
			ReadOnly:  true,
		}

		securityAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.SecurityAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(securityAgentVolumeMounts, append(wantVolumeMounts, securityWantVolumeMount)), "Security Agent volume mounts \ndiff = %s", cmp.Diff(securityAgentVolumeMounts, append(wantVolumeMounts, securityWantVolumeMount)))
		sysProbeVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(sysProbeVolumeMounts, wantVolumeMounts), "System probe volume mounts \ndiff = %s", cmp.Diff(sysProbeVolumeMounts, wantVolumeMounts))

		// check volumes
		wantVolumes := []corev1.Volume{
			{
				Name: apicommon.SecurityAgentRuntimeCustomPoliciesVolumeName,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "custom_test",
						},
					},
				},
			},
			{
				Name: apicommon.SecurityAgentRuntimePoliciesDirVolumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
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
			Agent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc:   cwsAgentNodeWantFunc,
			},
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
			Agent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc:   cwsAgentNodeWantFunc,
			},
		},
	}

	tests.Run(t, buildCWSFeature)
}
