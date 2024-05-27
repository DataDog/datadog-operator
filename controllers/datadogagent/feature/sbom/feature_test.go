// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sbom

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/test"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func Test_sbomFeature_Configure(t *testing.T) {

	sbomDisabled := v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				SBOM: &v2alpha1.SBOMFeatureConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
			},
		},
	}
	sbomEnabled := sbomDisabled.DeepCopy()
	{
		sbomEnabled.Spec.Features.SBOM.Enabled = apiutils.NewBoolPointer(true)
	}

	sbomEnabledContainerImageEnabled := sbomEnabled.DeepCopy()
	{
		sbomEnabledContainerImageEnabled.Spec.Features.SBOM.ContainerImage = &v2alpha1.SBOMContainerImageConfig{Enabled: apiutils.NewBoolPointer(true)}
	}

	sbomEnabledHostEnabled := sbomEnabled.DeepCopy()
	{
		sbomEnabledHostEnabled.Spec.Features.SBOM.Host = &v2alpha1.SBOMHostConfig{Enabled: apiutils.NewBoolPointer(true)}
	}

	sbomNodeAgentWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)

		wantEnvVars := []*corev1.EnvVar{
			{
				Name:  apicommon.DDSBOMEnabled,
				Value: "true",
			},
			{
				Name:  apicommon.DDSBOMContainerImageEnabled,
				Value: "false",
			},
			{
				Name:  apicommon.DDSBOMHostEnabled,
				Value: "false",
			},
		}

		nodeAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.AllContainers]
		assert.True(t, apiutils.IsEqualStruct(nodeAgentEnvVars, wantEnvVars), "Node agent envvars \ndiff = %s", cmp.Diff(nodeAgentEnvVars, wantEnvVars))
	}

	sbomWithContainerImageWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)

		wantEnvVars := []*corev1.EnvVar{
			{
				Name:  apicommon.DDSBOMEnabled,
				Value: "true",
			},
			{
				Name:  apicommon.DDSBOMContainerImageEnabled,
				Value: "true",
			},
			{
				Name:  apicommon.DDSBOMHostEnabled,
				Value: "false",
			},
		}

		nodeAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.AllContainers]
		assert.True(t, apiutils.IsEqualStruct(nodeAgentEnvVars, wantEnvVars), "Node agent envvars \ndiff = %s", cmp.Diff(nodeAgentEnvVars, wantEnvVars))
	}

	sbomWithHostWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)

		wantAllAgentsEnvVars := []*corev1.EnvVar{
			{
				Name:  apicommon.DDSBOMEnabled,
				Value: "true",
			},
			{
				Name:  apicommon.DDSBOMContainerImageEnabled,
				Value: "false",
			},
			{
				Name:  apicommon.DDSBOMHostEnabled,
				Value: "true",
			},
		}

		wantCoreAgentHostEnvVars := []*corev1.EnvVar{
			{
				Name:  apicommon.DDHostRootEnvVar,
				Value: "/host",
			},
		}

		nodeAllAgentsEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.AllContainers]
		nodeCoreAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(nodeCoreAgentEnvVars, wantCoreAgentHostEnvVars), "Node agent envvars \ndiff = %s", cmp.Diff(nodeCoreAgentEnvVars, wantCoreAgentHostEnvVars))
		assert.True(t, apiutils.IsEqualStruct(nodeAllAgentsEnvVars, wantAllAgentsEnvVars), "Node agent envvars \ndiff = %s", cmp.Diff(nodeAllAgentsEnvVars, wantAllAgentsEnvVars))

		wantVolumeMounts := []corev1.VolumeMount{
			{
				Name:      apicommon.SystemProbeOSReleaseDirVolumeName,
				MountPath: apicommon.SystemProbeOSReleaseDirMountPath,
				ReadOnly:  true,
			},
			{
				Name:      apicommon.ApkDirVolumeName,
				MountPath: apicommon.ApkDirMountPath,
				ReadOnly:  true,
			},
			{
				Name:      apicommon.DpkgDirVolumeName,
				MountPath: apicommon.DpkgDirMountPath,
				ReadOnly:  true,
			},
			{
				Name:      apicommon.RpmDirVolumeName,
				MountPath: apicommon.RpmDirMountPath,
				ReadOnly:  true,
			},
			{
				Name:      apicommon.RedhatReleaseVolumeName,
				MountPath: apicommon.RedhatReleaseMountPath,
				ReadOnly:  true,
			},
			{
				Name:      apicommon.FedoraReleaseVolumeName,
				MountPath: apicommon.FedoraReleaseMountPath,
				ReadOnly:  true,
			},
			{
				Name:      apicommon.LsbReleaseVolumeName,
				MountPath: apicommon.LsbReleaseMountPath,
				ReadOnly:  true,
			},
			{
				Name:      apicommon.SystemReleaseVolumeName,
				MountPath: apicommon.SystemReleaseMountPath,
				ReadOnly:  true,
			},
		}

		agentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.CoreAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(agentVolumeMounts, wantVolumeMounts), "Agent volume mounts \ndiff = %s", cmp.Diff(agentVolumeMounts, wantVolumeMounts))

		wantVolumes := []corev1.Volume{
			{
				Name: apicommon.SystemProbeOSReleaseDirVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: apicommon.SystemProbeOSReleaseDirVolumePath,
					},
				},
			},
			{
				Name: apicommon.ApkDirVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: apicommon.ApkDirVolumePath,
					},
				},
			},
			{
				Name: apicommon.DpkgDirVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: apicommon.DpkgDirVolumePath,
					},
				},
			},
			{
				Name: apicommon.RpmDirVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: apicommon.RpmDirVolumePath,
					},
				},
			},
			{
				Name: apicommon.RedhatReleaseVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: apicommon.RedhatReleaseVolumePath,
					},
				},
			},
			{
				Name: apicommon.FedoraReleaseVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: apicommon.FedoraReleaseVolumePath,
					},
				},
			},
			{
				Name: apicommon.LsbReleaseVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: apicommon.LsbReleaseVolumePath,
					},
				},
			},
			{
				Name: apicommon.SystemReleaseVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: apicommon.SystemReleaseVolumePath,
					},
				},
			},
		}

		volumes := mgr.VolumeMgr.Volumes
		assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))
	}

	tests := test.FeatureTestSuite{
		{
			Name:          "SBOM not enabled",
			DDAv2:         sbomDisabled.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "SBOM enabled",
			DDAv2:         sbomEnabled,
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(sbomNodeAgentWantFunc),
		},
		{
			Name:          "SBOM enabled, ContainerImage enabled",
			DDAv2:         sbomEnabledContainerImageEnabled,
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(sbomWithContainerImageWantFunc),
		},
		{
			Name:          "SBOM enabled, Host enabled",
			DDAv2:         sbomEnabledHostEnabled,
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(sbomWithHostWantFunc),
		},
	}

	tests.Run(t, buildSBOMFeature)
}
