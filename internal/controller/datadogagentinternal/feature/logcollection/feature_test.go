// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logcollection

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/test"
	"github.com/DataDog/datadog-operator/pkg/testutils"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func Test_LogCollectionFeature_Configure(t *testing.T) {
	tests := test.FeatureTestSuite{
		{
			Name: "log collection not enabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithLogCollectionEnabled(false).
				BuildWithDefaults(),
			WantConfigure: false,
		},
		{
			Name: "log collection enabled",
			DDA: testutils.NewDatadogAgentBuilder().
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
			Name: "container collect all enabled",
			DDA: testutils.NewDatadogAgentBuilder().
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
			Name: "container collect using files disabled",
			DDA: testutils.NewDatadogAgentBuilder().
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
			Name: "open files limit set to custom value",
			DDA: testutils.NewDatadogAgentBuilder().
				WithLogCollectionEnabled(true).
				WithLogCollectionOpenFilesLimit(250).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					wantEnvVars := createEnvVars("true", "false", "true")
					wantEnvVars = append(wantEnvVars, &corev1.EnvVar{
						Name:  DDLogsConfigOpenFilesLimit,
						Value: "250",
					})
					assertWants(t, mgrInterface, getWantVolumeMounts(), getWantVolumes(), wantEnvVars)
				},
			),
		},
		{
			Name: "custom volumes",
			DDA: testutils.NewDatadogAgentBuilder().
				WithLogCollectionEnabled(true).
				WithLogCollectionPaths("/custom/pod/logs", "/custom/container/logs", "/custom/symlink", "/custom/temp/storage").
				BuildWithDefaults(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					wantVolumeMounts := []*corev1.VolumeMount{
						{
							Name:      pointerVolumeName,
							MountPath: pointerVolumePath,
							ReadOnly:  false,
						},
						{
							Name:      podLogVolumeName,
							MountPath: podLogVolumePath,
							ReadOnly:  true,
						},
						{
							Name:      containerLogVolumeName,
							MountPath: containerLogVolumePath,
							ReadOnly:  true,
						},
						{
							Name:      symlinkContainerVolumeName,
							MountPath: symlinkContainerVolumePath,
							ReadOnly:  true,
						},
					}
					wantVolumes := []*corev1.Volume{
						{
							Name: pointerVolumeName,
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/custom/temp/storage",
								},
							},
						},
						{
							Name: podLogVolumeName,
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/custom/pod/logs",
								},
							},
						},
						{
							Name: containerLogVolumeName,
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/custom/container/logs",
								},
							},
						},
						{
							Name: symlinkContainerVolumeName,
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
			Name: pointerVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: common.DefaultLogTempStoragePath,
				},
			},
		},
		{
			Name: podLogVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: podLogVolumePath,
				},
			},
		},
		{
			Name: containerLogVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: containerLogVolumePath,
				},
			},
		},
		{
			Name: symlinkContainerVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: symlinkContainerVolumePath,
				},
			},
		},
	}
	return wantVolumes
}

func getWantVolumeMounts() []*corev1.VolumeMount {
	wantVolumeMounts := []*corev1.VolumeMount{
		{
			Name:      pointerVolumeName,
			MountPath: pointerVolumePath,
			ReadOnly:  false,
		},
		{
			Name:      podLogVolumeName,
			MountPath: podLogVolumePath,
			ReadOnly:  true,
		},
		{
			Name:      containerLogVolumeName,
			MountPath: containerLogVolumePath,
			ReadOnly:  true,
		},
		{
			Name:      symlinkContainerVolumeName,
			MountPath: symlinkContainerVolumePath,
			ReadOnly:  true,
		},
	}
	return wantVolumeMounts
}

func createEnvVars(logsEnabled, collectAllEnabled, collectUsingFilesEnabled string) []*corev1.EnvVar {
	return []*corev1.EnvVar{
		{
			Name:  common.DDLogsEnabled,
			Value: logsEnabled,
		},
		{
			Name:  DDLogsConfigContainerCollectAll,
			Value: collectAllEnabled,
		},
		{
			Name:  DDLogsContainerCollectUsingFiles,
			Value: collectUsingFilesEnabled,
		},
	}
}

func assertWants(t testing.TB, mgrInterface feature.PodTemplateManagers, wantVolumeMounts []*corev1.VolumeMount, wantVolumes []*corev1.Volume, wantEnvVars []*corev1.EnvVar) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)
	coreAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.CoreAgentContainerName]
	volumes := mgr.VolumeMgr.Volumes
	agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]

	assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantVolumeMounts), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantVolumeMounts))
	assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))
	assert.True(t, apiutils.IsEqualStruct(agentEnvVars, wantEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, wantEnvVars))
}
