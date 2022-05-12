// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logcollection

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

func Test_LogCollectionFeature_Configure(t *testing.T) {
	// v1alpha1
	ddav1Disabled := &v1alpha1.DatadogAgent{}
	ddav1LogCollectionDisabled := v1alpha1.DatadogAgent{
		Spec: *v1alpha1.DefaultDatadogAgent(ddav1Disabled).DefaultOverride,
	}

	ddav1Enabled := &v1alpha1.DatadogAgent{
		Spec: v1alpha1.DatadogAgentSpec{
			Features: v1alpha1.DatadogFeatures{
				LogCollection: &v1alpha1.LogCollectionConfig{
					Enabled: apiutils.NewBoolPointer(true),
				},
			},
		},
	}
	ddav1LogCollectionEnabled := v1alpha1.DatadogAgent{
		Spec: *v1alpha1.DefaultDatadogAgent(ddav1Enabled).DefaultOverride,
	}

	ddav1ContainerCollectAllEnabled := ddav1LogCollectionEnabled.DeepCopy()
	ddav1ContainerCollectAllEnabled.Spec.Features.LogCollection.LogsConfigContainerCollectAll = apiutils.NewBoolPointer(true)

	ddav1ContainerCollectUsingFilesDisabled := ddav1ContainerCollectAllEnabled.DeepCopy()
	ddav1ContainerCollectUsingFilesDisabled.Spec.Features.LogCollection.ContainerCollectUsingFiles = apiutils.NewBoolPointer(false)

	ddav1CustomOpenFilesLimit := ddav1LogCollectionEnabled.DeepCopy()
	ddav1CustomOpenFilesLimit.Spec.Features.LogCollection.OpenFilesLimit = apiutils.NewInt32Pointer(250)

	ddav1CustomVolumes := ddav1LogCollectionEnabled.DeepCopy()
	ddav1CustomVolumes.Spec.Features.LogCollection.PodLogsPath = apiutils.NewStringPointer("/custom/pod/logs")
	ddav1CustomVolumes.Spec.Features.LogCollection.ContainerLogsPath = apiutils.NewStringPointer("/custom/container/logs")
	ddav1CustomVolumes.Spec.Features.LogCollection.ContainerSymlinksPath = apiutils.NewStringPointer("/custom/symlink")
	ddav1CustomVolumes.Spec.Features.LogCollection.TempStoragePath = apiutils.NewStringPointer("/custom/temp/storage")

	// v2alpha1
	ddav2LogCollectionDisabled := &v2alpha1.DatadogAgent{}
	v2alpha1.DefaultDatadogAgent(ddav2LogCollectionDisabled)

	ddav2LogCollectionEnabled := &v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				LogCollection: &v2alpha1.LogCollectionFeatureConfig{
					Enabled: apiutils.NewBoolPointer(true),
				},
			},
		},
	}
	v2alpha1.DefaultDatadogAgent(ddav2LogCollectionEnabled)

	ddav2ContainerCollectAllEnabled := ddav2LogCollectionEnabled.DeepCopy()
	ddav2ContainerCollectAllEnabled.Spec.Features.LogCollection.ContainerCollectAll = apiutils.NewBoolPointer(true)

	ddav2ContainerCollectUsingFilesDisabled := ddav2ContainerCollectAllEnabled.DeepCopy()
	ddav2ContainerCollectUsingFilesDisabled.Spec.Features.LogCollection.ContainerCollectUsingFiles = apiutils.NewBoolPointer(false)

	ddav2CustomOpenFilesLimit := ddav2LogCollectionEnabled.DeepCopy()
	ddav2CustomOpenFilesLimit.Spec.Features.LogCollection.OpenFilesLimit = apiutils.NewInt32Pointer(250)

	ddav2CustomVolumes := ddav2LogCollectionEnabled.DeepCopy()
	ddav2CustomVolumes.Spec.Features.LogCollection.PodLogsPath = apiutils.NewStringPointer("/custom/pod/logs")
	ddav2CustomVolumes.Spec.Features.LogCollection.ContainerLogsPath = apiutils.NewStringPointer("/custom/container/logs")
	ddav2CustomVolumes.Spec.Features.LogCollection.ContainerSymlinksPath = apiutils.NewStringPointer("/custom/symlink")
	ddav2CustomVolumes.Spec.Features.LogCollection.TempStoragePath = apiutils.NewStringPointer("/custom/temp/storage")

	// volume mounts
	wantVolumeMounts := []corev1.VolumeMount{
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

	// check volumes
	wantVolumes := []corev1.Volume{
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

	tests := test.FeatureTestSuite{
		///////////////////////////
		// v1alpha1.DatadogAgent //
		///////////////////////////
		{
			Name:          "v1alpha1 log collection not enabled",
			DDAv1:         ddav1LogCollectionDisabled.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "v1alpha1 log collection enabled",
			DDAv1:         &ddav1LogCollectionEnabled,
			WantConfigure: true,
			Agent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc: func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMgr.VolumeMountByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantVolumeMounts), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantVolumeMounts))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					wantEnvVars := []*corev1.EnvVar{
						{
							Name:  apicommon.DDLogsEnabled,
							Value: "true",
						},
						{
							Name:  apicommon.DDLogsConfigContainerCollectAll,
							Value: "false",
						},
						{
							Name:  apicommon.DDLogsContainerCollectUsingFiles,
							Value: "true",
						},
					}
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, wantEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, wantEnvVars))
				},
			},
		},
		{
			Name:          "v1alpha1 container collect all enabled",
			DDAv1:         ddav1ContainerCollectAllEnabled,
			WantConfigure: true,
			Agent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc: func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMgr.VolumeMountByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantVolumeMounts), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantVolumeMounts))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					wantEnvVars := []*corev1.EnvVar{
						{
							Name:  apicommon.DDLogsEnabled,
							Value: "true",
						},
						{
							Name:  apicommon.DDLogsConfigContainerCollectAll,
							Value: "true",
						},
						{
							Name:  apicommon.DDLogsContainerCollectUsingFiles,
							Value: "true",
						},
					}
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, wantEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, wantEnvVars))
				},
			},
		},
		{
			Name:          "v1alpha1 container collect using files disabled",
			DDAv1:         ddav1ContainerCollectUsingFilesDisabled,
			WantConfigure: true,
			Agent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc: func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMgr.VolumeMountByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantVolumeMounts), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantVolumeMounts))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					wantEnvVars := []*corev1.EnvVar{
						{
							Name:  apicommon.DDLogsEnabled,
							Value: "true",
						},
						{
							Name:  apicommon.DDLogsConfigContainerCollectAll,
							Value: "true",
						},
						{
							Name:  apicommon.DDLogsContainerCollectUsingFiles,
							Value: "false",
						},
					}
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, wantEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, wantEnvVars))
				},
			},
		},
		{
			Name:          "v1alpha1 open files limit set to custom value",
			DDAv1:         ddav1CustomOpenFilesLimit,
			WantConfigure: true,
			Agent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc: func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMgr.VolumeMountByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantVolumeMounts), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantVolumeMounts))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					wantEnvVars := []*corev1.EnvVar{
						{
							Name:  apicommon.DDLogsEnabled,
							Value: "true",
						},
						{
							Name:  apicommon.DDLogsConfigContainerCollectAll,
							Value: "false",
						},
						{
							Name:  apicommon.DDLogsContainerCollectUsingFiles,
							Value: "true",
						},
						{
							Name:  apicommon.DDLogsConfigOpenFilesLimit,
							Value: "250",
						},
					}
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, wantEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, wantEnvVars))
				},
			},
		},
		{
			Name:          "v1alpha1 custom volumes",
			DDAv1:         ddav1CustomVolumes,
			WantConfigure: true,
			Agent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc: func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMgr.VolumeMountByC[apicommonv1.CoreAgentContainerName]
					wantCustomVolumeMounts := []corev1.VolumeMount{
						{
							Name:      apicommon.PointerVolumeName,
							MountPath: apicommon.PointerVolumePath,
							ReadOnly:  false,
						},
						{
							Name:      apicommon.PodLogVolumeName,
							MountPath: "/custom/pod/logs",
							ReadOnly:  true,
						},
						{
							Name:      apicommon.ContainerLogVolumeName,
							MountPath: "/custom/container/logs",
							ReadOnly:  true,
						},
						{
							Name:      apicommon.SymlinkContainerVolumeName,
							MountPath: "/custom/symlink",
							ReadOnly:  true,
						},
					}
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantCustomVolumeMounts), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantCustomVolumeMounts))
					volumes := mgr.VolumeMgr.Volumes
					wantCustomVolumes := []corev1.Volume{
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
					assert.True(t, apiutils.IsEqualStruct(volumes, wantCustomVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, wantCustomVolumes))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					wantEnvVars := []*corev1.EnvVar{
						{
							Name:  apicommon.DDLogsEnabled,
							Value: "true",
						},
						{
							Name:  apicommon.DDLogsConfigContainerCollectAll,
							Value: "false",
						},
						{
							Name:  apicommon.DDLogsContainerCollectUsingFiles,
							Value: "true",
						},
					}
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, wantEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, wantEnvVars))
				},
			},
		},
		///////////////////////////
		// v2alpha1.DatadogAgent //
		///////////////////////////
		{
			Name:          "v2alpha1 log collection not enabled",
			DDAv2:         ddav2LogCollectionDisabled.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "v2alpha1 log collection enabled",
			DDAv2:         ddav2LogCollectionEnabled,
			WantConfigure: true,
			Agent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc: func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMgr.VolumeMountByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantVolumeMounts), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantVolumeMounts))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					wantEnvVars := []*corev1.EnvVar{
						{
							Name:  apicommon.DDLogsEnabled,
							Value: "true",
						},
						{
							Name:  apicommon.DDLogsConfigContainerCollectAll,
							Value: "false",
						},
						{
							Name:  apicommon.DDLogsContainerCollectUsingFiles,
							Value: "true",
						},
					}
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, wantEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, wantEnvVars))
				},
			},
		},
		{
			Name:          "v2alpha1 container collect all enabled",
			DDAv2:         ddav2ContainerCollectAllEnabled,
			WantConfigure: true,
			Agent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc: func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMgr.VolumeMountByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantVolumeMounts), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantVolumeMounts))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					wantEnvVars := []*corev1.EnvVar{
						{
							Name:  apicommon.DDLogsEnabled,
							Value: "true",
						},
						{
							Name:  apicommon.DDLogsConfigContainerCollectAll,
							Value: "true",
						},
						{
							Name:  apicommon.DDLogsContainerCollectUsingFiles,
							Value: "true",
						},
					}
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, wantEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, wantEnvVars))
				},
			},
		},
		{
			Name:          "v2alpha1 container collect using files disabled",
			DDAv2:         ddav2ContainerCollectUsingFilesDisabled,
			WantConfigure: true,
			Agent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc: func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMgr.VolumeMountByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantVolumeMounts), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantVolumeMounts))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					wantEnvVars := []*corev1.EnvVar{
						{
							Name:  apicommon.DDLogsEnabled,
							Value: "true",
						},
						{
							Name:  apicommon.DDLogsConfigContainerCollectAll,
							Value: "true",
						},
						{
							Name:  apicommon.DDLogsContainerCollectUsingFiles,
							Value: "false",
						},
					}
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, wantEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, wantEnvVars))
				},
			},
		},
		{
			Name:          "v2alpha1 open files limit set to custom value",
			DDAv2:         ddav2CustomOpenFilesLimit,
			WantConfigure: true,
			Agent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc: func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMgr.VolumeMountByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantVolumeMounts), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantVolumeMounts))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					wantEnvVars := []*corev1.EnvVar{
						{
							Name:  apicommon.DDLogsEnabled,
							Value: "true",
						},
						{
							Name:  apicommon.DDLogsConfigContainerCollectAll,
							Value: "false",
						},
						{
							Name:  apicommon.DDLogsContainerCollectUsingFiles,
							Value: "true",
						},
						{
							Name:  apicommon.DDLogsConfigOpenFilesLimit,
							Value: "250",
						},
					}
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, wantEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, wantEnvVars))
				},
			},
		},
		{
			Name:          "v2alpha1 custom volumes",
			DDAv2:         ddav2CustomVolumes,
			WantConfigure: true,
			Agent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc: func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMgr.VolumeMountByC[apicommonv1.CoreAgentContainerName]
					wantVolumeMounts := []corev1.VolumeMount{
						{
							Name:      apicommon.PointerVolumeName,
							MountPath: apicommon.PointerVolumePath,
							ReadOnly:  false,
						},
						{
							Name:      apicommon.PodLogVolumeName,
							MountPath: "/custom/pod/logs",
							ReadOnly:  true,
						},
						{
							Name:      apicommon.ContainerLogVolumeName,
							MountPath: "/custom/container/logs",
							ReadOnly:  true,
						},
						{
							Name:      apicommon.SymlinkContainerVolumeName,
							MountPath: "/custom/symlink",
							ReadOnly:  true,
						},
					}
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantVolumeMounts), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantVolumeMounts))
					volumes := mgr.VolumeMgr.Volumes
					wantVolumes = []corev1.Volume{
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
					assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					wantEnvVars := []*corev1.EnvVar{
						{
							Name:  apicommon.DDLogsEnabled,
							Value: "true",
						},
						{
							Name:  apicommon.DDLogsConfigContainerCollectAll,
							Value: "false",
						},
						{
							Name:  apicommon.DDLogsContainerCollectUsingFiles,
							Value: "true",
						},
					}
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, wantEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, wantEnvVars))
				},
			},
		},
	}

	tests.Run(t, buildLogCollectionFeature)

}
