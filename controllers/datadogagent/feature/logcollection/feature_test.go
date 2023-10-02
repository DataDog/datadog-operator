// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logcollection

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	v2alpha1test "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1/test"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/test"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func Test_LogCollectionFeature_Configure(t *testing.T) {
	tests := test.FeatureTestSuite{
		{
			Name: "v2alpha1 log collection not enabled",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithLogCollectionEnabled(false).
				BuildWithDefaults(),
			WantConfigure: false,
		},
		{
			Name: "v2alpha1 log collection enabled",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithLogCollectionEnabled(true).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					wantEnvVars := createEnvVars("true", "false", "true")
					assertWants(t, mgrInterface, getWantVolumeMounts(), getWantVolumes(), wantEnvVars)
				},
			),
		},
		{
			Name: "v2alpha1 container collect all enabled",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithLogCollectionEnabled(true).
				WithLogCollectionCollectAll(true).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					wantEnvVars := createEnvVars("true", "true", "true")
					assertWants(t, mgrInterface, getWantVolumeMounts(), getWantVolumes(), wantEnvVars)
				},
			),
		},
		{
			Name: "v2alpha1 container collect using files disabled",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithLogCollectionEnabled(true).
				WithLogCollectionCollectAll(true).
				WithLogCollectionLogCollectionUsingFiles(false).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					wantEnvVars := createEnvVars("true", "true", "false")
					assertWants(t, mgrInterface, getWantVolumeMounts(), getWantVolumes(), wantEnvVars)
				},
			),
		},
		{
			Name: "v2alpha1 open files limit set to custom value",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithLogCollectionEnabled(true).
				WithLogCollectionOpenFilesLimit(250).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					wantEnvVars := createEnvVars("true", "false", "true")
					wantEnvVars = append(wantEnvVars, &corev1.EnvVar{
						Name:  apicommon.DDLogsConfigOpenFilesLimit,
						Value: "250",
					})
					assertWants(t, mgrInterface, getWantVolumeMounts(), getWantVolumes(), wantEnvVars)
				},
			),
		},
		{
			Name: "v2alpha1 custom volumes",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithLogCollectionEnabled(true).
				WithLogCollectionPaths("/custom/pod/logs", "/custom/container/logs", "/custom/symlink", "/custom/temp/storage").
				BuildWithDefaults(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					wantVolumeMounts := []*corev1.VolumeMount{
						{
							Name:      apicommon.PointerVolumeName,
							MountPath: apicommon.PointerVolumePath,
							ReadOnly:  false,
						},
						{
							Name:      apicommon.PodLogVolumeName,
							MountPath: apicommon.PodLogVolumePath,
							ReadOnly:  true,
						},
						{
							Name:      apicommon.ContainerLogVolumeName,
							MountPath: apicommon.ContainerLogVolumePath,
							ReadOnly:  true,
						},
						{
							Name:      apicommon.SymlinkContainerVolumeName,
							MountPath: apicommon.SymlinkContainerVolumePath,
							ReadOnly:  true,
						},
					}
					wantVolumes := []*corev1.Volume{
						{
							Name: apicommon.PointerVolumeName,
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/custom/temp/storage",
								},
							},
						},
						{
							Name: apicommon.PodLogVolumeName,
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/custom/pod/logs",
								},
							},
						},
						{
							Name: apicommon.ContainerLogVolumeName,
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/custom/container/logs",
								},
							},
						},
						{
							Name: apicommon.SymlinkContainerVolumeName,
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/custom/symlink",
								},
							},
						},
					}
					wantEnvVars := createEnvVars("true", "false", "true")
					assertWants(t, mgrInterface, wantVolumeMounts, wantVolumes, wantEnvVars)
				},
			),
		},
	}

	tests.Run(t, buildLogCollectionFeature)

}

func getWantVolumes() []*corev1.Volume {
	wantVolumes := []*corev1.Volume{
		{
			Name: apicommon.PointerVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: apicommon.LogTempStoragePath,
				},
			},
		},
		{
			Name: apicommon.PodLogVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: apicommon.PodLogVolumePath,
				},
			},
		},
		{
			Name: apicommon.ContainerLogVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: apicommon.ContainerLogVolumePath,
				},
			},
		},
		{
			Name: apicommon.SymlinkContainerVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: apicommon.SymlinkContainerVolumePath,
				},
			},
		},
	}
	return wantVolumes
}

func getWantVolumeMounts() []*corev1.VolumeMount {
	wantVolumeMounts := []*corev1.VolumeMount{
		{
			Name:      apicommon.PointerVolumeName,
			MountPath: apicommon.PointerVolumePath,
			ReadOnly:  false,
		},
		{
			Name:      apicommon.PodLogVolumeName,
			MountPath: apicommon.PodLogVolumePath,
			ReadOnly:  true,
		},
		{
			Name:      apicommon.ContainerLogVolumeName,
			MountPath: apicommon.ContainerLogVolumePath,
			ReadOnly:  true,
		},
		{
			Name:      apicommon.SymlinkContainerVolumeName,
			MountPath: apicommon.SymlinkContainerVolumePath,
			ReadOnly:  true,
		},
	}
	return wantVolumeMounts
}

func createEnvVars(logsEnabled, collectAllEnabled, collectUsingFilesEnabled string) []*corev1.EnvVar {
	return []*corev1.EnvVar{
		{
			Name:  apicommon.DDLogsEnabled,
			Value: logsEnabled,
		},
		{
			Name:  apicommon.DDLogsConfigContainerCollectAll,
			Value: collectAllEnabled,
		},
		{
			Name:  apicommon.DDLogsContainerCollectUsingFiles,
			Value: collectUsingFilesEnabled,
		},
	}
}

func assertWants(t testing.TB, mgrInterface feature.PodTemplateManagers, wantVolumeMounts []*corev1.VolumeMount, wantVolumes []*corev1.Volume, wantEnvVars []*corev1.EnvVar) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)
	coreAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.CoreAgentContainerName]
	volumes := mgr.VolumeMgr.Volumes
	agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]

	assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantVolumeMounts), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantVolumeMounts))
	assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))
	assert.True(t, apiutils.IsEqualStruct(agentEnvVars, wantEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, wantEnvVars))
}
