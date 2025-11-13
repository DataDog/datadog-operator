// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cws

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

func Test_cwsFeature_Configure(t *testing.T) {
	ddaCWSDisabled := v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				CWS: &v2alpha1.CWSFeatureConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
				RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
			},
		},
	}
	ddaCWSLiteEnabled := ddaCWSDisabled.DeepCopy()
	{
		ddaCWSLiteEnabled.Spec.Features.CWS.Enabled = apiutils.NewBoolPointer(true)
		ddaCWSLiteEnabled.Spec.Features.CWS.CustomPolicies = &v2alpha1.CustomConfig{
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
		ddaCWSLiteEnabled.Spec.Features.CWS.SyscallMonitorEnabled = apiutils.NewBoolPointer(true)
	}
	ddaCWSFullEnabled := ddaCWSDisabled.DeepCopy()
	{
		ddaCWSFullEnabled.Spec.Features.CWS.Enabled = apiutils.NewBoolPointer(true)
		ddaCWSFullEnabled.Spec.Features.CWS.Network = &v2alpha1.CWSNetworkConfig{
			Enabled: apiutils.NewBoolPointer(true),
		}
		ddaCWSFullEnabled.Spec.Features.CWS.SecurityProfiles = &v2alpha1.CWSSecurityProfilesConfig{
			Enabled: apiutils.NewBoolPointer(true),
		}
		ddaCWSFullEnabled.Spec.Features.CWS.CustomPolicies = &v2alpha1.CustomConfig{
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
		ddaCWSFullEnabled.Spec.Features.CWS.SyscallMonitorEnabled = apiutils.NewBoolPointer(true)
		ddaCWSFullEnabled.Spec.Features.RemoteConfiguration.Enabled = apiutils.NewBoolPointer(true)
	}
	ddaCWSLiteDirectSendEnabled := ddaCWSLiteEnabled.DeepCopy()
	{
		ddaCWSLiteDirectSendEnabled.Spec.Features.CWS.DirectSendFromSystemProbe = apiutils.NewBoolPointer(true)
	}

	tests := test.FeatureTestSuite{
		{
			Name:          "v2alpha1 CWS not enabled",
			DDA:           ddaCWSDisabled.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "v2alpha1 CWS enabled",
			DDA:           ddaCWSLiteEnabled,
			WantConfigure: true,
			Agent:         cwsAgentNodeWantFunc(false, false),
		},
		{
			Name:          "v2alpha1 CWS enabled (with network, security profiles and remote configuration)",
			DDA:           ddaCWSFullEnabled,
			WantConfigure: true,
			Agent:         cwsAgentNodeWantFunc(true, false),
		},
		{
			Name:          "v2alpha1 CWS enabled in direct sender mode",
			DDA:           ddaCWSLiteDirectSendEnabled,
			WantConfigure: true,
			Agent:         cwsAgentNodeWantFunc(false, true),
		},
	}

	tests.Run(t, buildCWSFeature)
}

func cwsAgentNodeWantFunc(withSubFeatures bool, directSendFromSysProbe bool) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			// check security context capabilities
			sysProbeCapabilities := mgr.SecurityContextMgr.CapabilitiesByC[apicommon.SystemProbeContainerName]
			assert.True(t, apiutils.IsEqualStruct(sysProbeCapabilities, agent.DefaultCapabilitiesForSystemProbe()), "System Probe security context capabilities \ndiff = %s", cmp.Diff(sysProbeCapabilities, agent.DefaultCapabilitiesForSystemProbe()))

			securityWant := []*corev1.EnvVar{
				{
					Name:  DDRuntimeSecurityConfigEnabled,
					Value: "true",
				},
				{
					Name:  DDRuntimeSecurityConfigSocket,
					Value: "/var/run/sysprobe/runtime-security.sock",
				},
				{
					Name:  DDRuntimeSecurityConfigSyscallMonitorEnabled,
					Value: "true",
				},
			}
			sysProbeWant := []*corev1.EnvVar{
				{
					Name:  DDRuntimeSecurityConfigEnabled,
					Value: "true",
				},
				{
					Name:  DDRuntimeSecurityConfigSocket,
					Value: "/var/run/sysprobe/runtime-security.sock",
				},
				{
					Name:  DDRuntimeSecurityConfigSyscallMonitorEnabled,
					Value: "true",
				},
			}
			if withSubFeatures {
				sysProbeWant = append(
					sysProbeWant,
					&corev1.EnvVar{
						Name:  DDRuntimeSecurityConfigNetworkEnabled,
						Value: "true",
					},
					&corev1.EnvVar{
						Name:  DDRuntimeSecurityConfigActivityDumpEnabled,
						Value: "true",
					},
					&corev1.EnvVar{
						Name:  DDRuntimeSecurityConfigRemoteConfigurationEnabled,
						Value: "true",
					},
				)
			}
			if directSendFromSysProbe {
				sysProbeWant = append(
					sysProbeWant,
					&corev1.EnvVar{
						Name:  DDRuntimeSecurityConfigDirectSendFromSystemProbe,
						Value: "true",
					},
				)
			}
			sysProbeWant = append(
				sysProbeWant,
				&corev1.EnvVar{
					Name:  DDRuntimeSecurityConfigPoliciesDir,
					Value: securityAgentRuntimePoliciesDirVolumePath,
				},
			)

			securityAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.SecurityAgentContainerName]
			if directSendFromSysProbe {
				assert.True(t, apiutils.IsEqualStruct(securityAgentEnvVars, nil), "Security agent envvars \ndiff = %s", cmp.Diff(securityAgentEnvVars, securityWant))
			} else {
				assert.True(t, apiutils.IsEqualStruct(securityAgentEnvVars, securityWant), "Security agent envvars \ndiff = %s", cmp.Diff(securityAgentEnvVars, securityWant))
			}
			sysProbeEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.SystemProbeContainerName]
			assert.True(t, apiutils.IsEqualStruct(sysProbeEnvVars, sysProbeWant), "System probe envvars \ndiff = %s", cmp.Diff(sysProbeEnvVars, sysProbeWant))

			// check volume mounts
			securityWantVolumeMount := []corev1.VolumeMount{
				{
					Name:      common.SystemProbeSocketVolumeName,
					MountPath: common.SystemProbeSocketVolumePath,
					ReadOnly:  false,
				},
			}
			sysprobeWantVolumeMount := []corev1.VolumeMount{
				{
					Name:      common.DebugfsVolumeName,
					MountPath: common.DebugfsPath,
					ReadOnly:  false,
				},
				{
					Name:      tracefsVolumeName,
					MountPath: tracefsPath,
					ReadOnly:  false,
				},
				{
					Name:      securityfsVolumeName,
					MountPath: securityfsMountPath,
					ReadOnly:  true,
				},
				{
					Name:      common.SystemProbeSocketVolumeName,
					MountPath: common.SystemProbeSocketVolumePath,
					ReadOnly:  false,
				},
				{
					Name:      common.ProcdirVolumeName,
					MountPath: common.ProcdirMountPath,
					ReadOnly:  true,
				},
				{
					Name:      common.PasswdVolumeName,
					MountPath: common.PasswdMountPath,
					ReadOnly:  true,
				},
				{
					Name:      common.GroupVolumeName,
					MountPath: common.GroupMountPath,
					ReadOnly:  true,
				},
				{
					Name:      common.SystemProbeOSReleaseDirVolumeName,
					MountPath: common.SystemProbeOSReleaseDirMountPath,
					ReadOnly:  true,
				},
				{
					Name:      common.CgroupsVolumeName,
					MountPath: common.CgroupsMountPath,
					ReadOnly:  true,
				},
				{
					Name:      common.HostRootVolumeName,
					MountPath: common.HostRootMountPath,
					ReadOnly:  true,
				},
				{
					Name:      securityAgentRuntimePoliciesDirVolumeName,
					MountPath: securityAgentRuntimePoliciesDirVolumePath,
					ReadOnly:  true,
				},
			}

			securityAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.SecurityAgentContainerName]
			if directSendFromSysProbe {
				assert.True(t, apiutils.IsEqualStruct(securityAgentVolumeMounts, nil), "Security Agent volume mounts \ndiff = %s", cmp.Diff(securityAgentVolumeMounts, securityWantVolumeMount))
			} else {
				assert.True(t, apiutils.IsEqualStruct(securityAgentVolumeMounts, securityWantVolumeMount), "Security Agent volume mounts \ndiff = %s", cmp.Diff(securityAgentVolumeMounts, securityWantVolumeMount))
			}
			sysProbeVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.SystemProbeContainerName]
			assert.True(t, apiutils.IsEqualStruct(sysProbeVolumeMounts, sysprobeWantVolumeMount), "System probe volume mounts \ndiff = %s", cmp.Diff(sysProbeVolumeMounts, sysprobeWantVolumeMount))

			// check volumes
			wantVolumes := []corev1.Volume{
				{
					Name: common.DebugfsVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: common.DebugfsPath,
						},
					},
				},
				{
					Name: tracefsVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: tracefsPath,
						},
					},
				},
				{
					Name: securityfsVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: securityfsVolumePath,
						},
					},
				},
				{
					Name: common.SystemProbeSocketVolumeName,
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
				{
					Name: common.ProcdirVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: common.ProcdirHostPath,
						},
					},
				},
				{
					Name: common.PasswdVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: common.PasswdHostPath,
						},
					},
				},
				{
					Name: common.GroupVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: common.GroupHostPath,
						},
					},
				},
				{
					Name: common.SystemProbeOSReleaseDirVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: common.SystemProbeOSReleaseDirVolumePath,
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
					Name: common.HostRootVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: common.HostRootHostPath,
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
					Name: securityAgentRuntimePoliciesDirVolumeName,
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			}

			volumes := mgr.VolumeMgr.Volumes
			assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))

			// check annotations
			customConfig := &v2alpha1.CustomConfig{
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
			hash, err := comparison.GenerateMD5ForSpec(customConfig)
			assert.NoError(t, err)
			wantAnnotations := map[string]string{
				fmt.Sprintf(constants.MD5ChecksumAnnotationKey, feature.CWSIDType): hash,
				common.SystemProbeAppArmorAnnotationKey:                            common.SystemProbeAppArmorAnnotationValue,
			}
			annotations := mgr.AnnotationMgr.Annotations
			assert.True(t, apiutils.IsEqualStruct(annotations, wantAnnotations), "Annotations \ndiff = %s", cmp.Diff(annotations, wantAnnotations))
		},
	)
}
