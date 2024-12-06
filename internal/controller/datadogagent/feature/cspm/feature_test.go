// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cspm

import (
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/crds/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/crds/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/crds/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

var customConfig = &v2alpha1.CustomConfig{
	ConfigMap: &v2alpha1.ConfigMapConfig{
		Name: "custom_test",
		Items: []corev1.KeyToPath{
			{
				Key:  "key1",
				Path: "some/path",
			},
		},
	},
}

func Test_cspmFeature_Configure(t *testing.T) {
	ddaCSPMDisabled := v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				CSPM: &v2alpha1.CSPMFeatureConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
			},
		},
	}
	ddaCSPMEnabled := ddaCSPMDisabled.DeepCopy()
	{
		ddaCSPMEnabled.Spec.Features.CSPM.Enabled = apiutils.NewBoolPointer(true)
		ddaCSPMEnabled.Spec.Features.CSPM.CustomBenchmarks = &v2alpha1.CustomConfig{
			ConfigMap: &v2alpha1.ConfigMapConfig{
				Name: "custom_test",
				Items: []corev1.KeyToPath{
					{
						Key:  "key1",
						Path: "some/path",
					},
				},
			},
		}
		ddaCSPMEnabled.Spec.Features.CSPM.CheckInterval = &metav1.Duration{Duration: 20 * time.Minute}
		ddaCSPMEnabled.Spec.Features.CSPM.HostBenchmarks = &v2alpha1.CSPMHostBenchmarksConfig{Enabled: apiutils.NewBoolPointer(true)}
	}

	tests := test.FeatureTestSuite{

		{
			Name:          "CSPM not enabled",
			DDA:           ddaCSPMDisabled.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "CSPM enabled",
			DDA:           ddaCSPMEnabled,
			WantConfigure: true,
			ClusterAgent:  cspmClusterAgentWantFunc(),
			Agent:         cspmAgentNodeWantFunc(),
		},
	}

	tests.Run(t, buildCSPMFeature)
}

func cspmClusterAgentWantFunc() *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)
			dcaEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName]

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

			volumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.ClusterAgentContainerName]
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

			// check annotations
			hash, err := comparison.GenerateMD5ForSpec(customConfig)
			assert.NoError(t, err)
			wantAnnotations := map[string]string{
				fmt.Sprintf(apicommon.MD5ChecksumAnnotationKey, feature.CSPMIDType): hash,
			}
			annotations := mgr.AnnotationMgr.Annotations
			assert.True(t, apiutils.IsEqualStruct(annotations, wantAnnotations), "Annotations \ndiff = %s", cmp.Diff(annotations, wantAnnotations))

		},
	)
}

func cspmAgentNodeWantFunc() *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
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
				{
					Name:  apicommon.DDComplianceHostBenchmarksEnabled,
					Value: "true",
				},
			}

			securityAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.SecurityAgentContainerName]
			assert.True(t, apiutils.IsEqualStruct(securityAgentEnvVars, want), "Agent envvars \ndiff = %s", cmp.Diff(securityAgentEnvVars, want))

			// check volume mounts
			wantVolumeMounts := []corev1.VolumeMount{
				{
					Name:      securityAgentComplianceConfigDirVolumeName,
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

			securityAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.SecurityAgentContainerName]
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
					Name: securityAgentComplianceConfigDirVolumeName,
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

			// check annotations
			hash, err := comparison.GenerateMD5ForSpec(customConfig)
			assert.NoError(t, err)
			wantAnnotations := map[string]string{
				fmt.Sprintf(apicommon.MD5ChecksumAnnotationKey, feature.CSPMIDType): hash,
			}
			annotations := mgr.AnnotationMgr.Annotations
			assert.True(t, apiutils.IsEqualStruct(annotations, wantAnnotations), "Annotations \ndiff = %s", cmp.Diff(annotations, wantAnnotations))
		},
	)
}
