// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cspm

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

func Test_cspmFeature_Configure(t *testing.T) {
	ddav1CSPMDisabled := v1alpha1.DatadogAgent{
		Spec: v1alpha1.DatadogAgentSpec{
			Agent: v1alpha1.DatadogAgentSpecAgentSpec{
				Security: &v1alpha1.SecuritySpec{
					Compliance: v1alpha1.ComplianceSpec{
						Enabled: apiutils.NewBoolPointer(false),
					},
				},
			},
		},
	}

	ddav1CSPMEnabled := ddav1CSPMDisabled.DeepCopy()
	{
		ddav1CSPMEnabled.Spec.Agent.Security.Compliance.Enabled = apiutils.NewBoolPointer(true)
		ddav1CSPMEnabled.Spec.Agent.Security.Compliance.ConfigDir = &v1alpha1.ConfigDirSpec{
			ConfigMapName: "custom_test",
			Items: []corev1.KeyToPath{
				{
					Key:  "key1",
					Path: "some/path",
				},
			},
		}
		ddav1CSPMEnabled.Spec.Agent.Security.Compliance.CheckInterval = &metav1.Duration{Duration: 20 * time.Minute}
	}

	ddav2CSPMDisabled := v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				CSPM: &v2alpha1.CSPMFeatureConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
			},
		},
	}
	ddav2CSPMEnabled := ddav2CSPMDisabled.DeepCopy()
	{
		ddav2CSPMEnabled.Spec.Features.CSPM.Enabled = apiutils.NewBoolPointer(true)
		ddav2CSPMEnabled.Spec.Features.CSPM.CustomBenchmarks = &v2alpha1.CustomConfig{
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
		ddav2CSPMEnabled.Spec.Features.CSPM.CheckInterval = &metav1.Duration{Duration: 20 * time.Minute}
	}

	cspmClusterAgentWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)
		dcaEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.ClusterAgentContainerName]

		want := []*corev1.EnvVar{
			{
				Name:  apicommon.DDComplianceConfigEnabled,
				Value: "true",
			},
			{
				Name:  apicommon.DDComplianceConfigCheckInterval,
				Value: "1200000000000",
			},
		}
		assert.True(t, apiutils.IsEqualStruct(dcaEnvVars, want), "DCA envvars \ndiff = %s", cmp.Diff(dcaEnvVars, want))

		wantVolumeMounts := []corev1.VolumeMount{
			{
				Name:      cspmConfigVolumeName,
				MountPath: "/etc/datadog-agent/compliance.d/some/path",
				SubPath:   "some/path",
				ReadOnly:  true,
			},
		}

		volumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.ClusterAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(volumeMounts, wantVolumeMounts), "Cluster Agent volume mounts \ndiff = %s", cmp.Diff(volumeMounts, wantVolumeMounts))

		wantVolumes := []corev1.Volume{
			{
				Name: cspmConfigVolumeName,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "custom_test",
						},
						Items: []corev1.KeyToPath{{Key: "key1", Path: "some/path"}},
					},
				},
			},
		}
		volumes := mgr.VolumeMgr.Volumes
		assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "Cluster Agent volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))
	}

	cspmAgentNodeWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)

		want := []*corev1.EnvVar{
			{
				Name:  apicommon.DDComplianceConfigEnabled,
				Value: "true",
			},
			{
				Name:  apicommon.DDHostRootEnvVar,
				Value: apicommon.HostRootMountPath,
			},
			{
				Name:  apicommon.DDComplianceConfigCheckInterval,
				Value: "1200000000000",
			},
		}
		securityAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.SecurityAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(securityAgentEnvVars, want), "Agent envvars \ndiff = %s", cmp.Diff(securityAgentEnvVars, want))

		// check volume mounts
		wantVolumeMounts := []corev1.VolumeMount{
			{
				Name:      apicommon.SecurityAgentComplianceConfigDirVolumeName,
				MountPath: "/etc/datadog-agent/compliance.d",
				ReadOnly:  true,
			},
			{
				Name:      apicommon.CgroupsVolumeName,
				MountPath: apicommon.CgroupsMountPath,
				ReadOnly:  true,
			},
			{
				Name:      apicommon.PasswdVolumeName,
				MountPath: apicommon.PasswdMountPath,
				ReadOnly:  true,
			},
			{
				Name:      apicommon.ProcdirVolumeName,
				MountPath: apicommon.ProcdirMountPath,
				ReadOnly:  true,
			},
			{
				Name:      apicommon.HostRootVolumeName,
				MountPath: apicommon.HostRootMountPath,
				ReadOnly:  true,
			},
			{
				Name:      apicommon.GroupVolumeName,
				MountPath: apicommon.GroupMountPath,
				ReadOnly:  true,
			},
		}

		securityAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.SecurityAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(securityAgentVolumeMounts, wantVolumeMounts), "Security Agent volume mounts \ndiff = %s", cmp.Diff(securityAgentVolumeMounts, wantVolumeMounts))

		// check volumes
		wantVolumes := []corev1.Volume{
			{
				Name: cspmConfigVolumeName,
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
				Name: apicommon.SecurityAgentComplianceConfigDirVolumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
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
				Name: apicommon.PasswdVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: apicommon.PasswdHostPath,
					},
				},
			},
			{
				Name: apicommon.ProcdirVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: apicommon.ProcdirHostPath,
					},
				},
			},
			{
				Name: apicommon.HostRootVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: apicommon.HostRootHostPath,
					},
				},
			},
			{
				Name: apicommon.GroupVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: apicommon.GroupHostPath,
					},
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
			Name:          "v1alpha1 CSPM not enabled",
			DDAv1:         ddav1CSPMDisabled.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "v1alpha1 CSPM enabled",
			DDAv1:         ddav1CSPMEnabled,
			WantConfigure: true,
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(cspmClusterAgentWantFunc),
			Agent:         test.NewDefaultComponentTest().WithWantFunc(cspmAgentNodeWantFunc),
		},
		//////////////////////////
		// v2Alpha1.DatadogAgent
		//////////////////////////
		{
			Name:          "v2alpha1 CSPM not enabled",
			DDAv2:         ddav2CSPMDisabled.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "v2alpha1 CSPM enabled",
			DDAv2:         ddav2CSPMEnabled,
			WantConfigure: true,
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(cspmClusterAgentWantFunc),
			Agent:         test.NewDefaultComponentTest().WithWantFunc(cspmAgentNodeWantFunc),
		},
	}

	tests.Run(t, buildCSPMFeature)
}
