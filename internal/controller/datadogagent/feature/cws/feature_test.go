// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cws

import (
	"fmt"
	"testing"

	"k8s.io/utils/ptr"

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
					Enabled: ptr.To(false),
					Enforcement: &v2alpha1.CWSEnforcementConfig{
						Enabled: ptr.To(false),
					},
				},
				RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
					Enabled: ptr.To(false),
				},
			},
		},
	}
	ddaCWSLiteEnabled := ddaCWSDisabled.DeepCopy()
	{
		ddaCWSLiteEnabled.Spec.Features.CWS.Enabled = ptr.To(true)
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
		ddaCWSLiteEnabled.Spec.Features.CWS.SyscallMonitorEnabled = ptr.To(true)
	}
	ddaCWSFullEnabled := ddaCWSDisabled.DeepCopy()
	{
		ddaCWSFullEnabled.Spec.Features.CWS.Enabled = ptr.To(true)
		ddaCWSFullEnabled.Spec.Features.CWS.Network = &v2alpha1.CWSNetworkConfig{
			Enabled: ptr.To(true),
		}
		ddaCWSFullEnabled.Spec.Features.CWS.SecurityProfiles = &v2alpha1.CWSSecurityProfilesConfig{
			Enabled: ptr.To(true),
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
		ddaCWSFullEnabled.Spec.Features.CWS.SyscallMonitorEnabled = ptr.To(true)
		ddaCWSFullEnabled.Spec.Features.RemoteConfiguration.Enabled = ptr.To(true)
	}
	ddaCWSLiteDirectSendEnabled := ddaCWSLiteEnabled.DeepCopy()
	{
		ddaCWSLiteDirectSendEnabled.Spec.Features.CWS.DirectSendFromSystemProbe = ptr.To(true)
	}

	ddaCWSLiteEnforcementEnabled := ddaCWSLiteEnabled.DeepCopy()
	{
		ddaCWSLiteEnforcementEnabled.Spec.Features.CWS.Enforcement.Enabled = ptr.To(true)
	}

	// Deprecated UseVSock maps to the "Full" VSock mode.
	ddaCWSLiteVSockFull := ddaCWSLiteEnabled.DeepCopy()
	{
		ddaCWSLiteVSockFull.Spec.Global = &v2alpha1.GlobalConfig{
			UseVSock: ptr.To(true),
		}
	}

	ddaCWSLiteVSockSystemProbe := ddaCWSLiteEnabled.DeepCopy()
	{
		ddaCWSLiteVSockSystemProbe.Spec.Global = &v2alpha1.GlobalConfig{
			VSock: &v2alpha1.VSockConfig{
				Enabled: ptr.To(true),
				Mode:    ptr.To(v2alpha1.VSockModeSystemProbe),
			},
		}
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
			Agent:         cwsAgentNodeWantFunc(false, false, false, ""),
		},
		{
			Name:          "v2alpha1 CWS enabled (with network, security profiles and remote configuration)",
			DDA:           ddaCWSFullEnabled,
			WantConfigure: true,
			Agent:         cwsAgentNodeWantFunc(true, false, false, ""),
		},
		{
			Name:          "v2alpha1 CWS enabled in direct sender mode",
			DDA:           ddaCWSLiteDirectSendEnabled,
			WantConfigure: true,
			Agent:         cwsAgentNodeWantFunc(false, true, false, ""),
		},
		{
			Name:          "v2alpha1 CWS enabled with enforcement",
			DDA:           ddaCWSLiteEnforcementEnabled,
			WantConfigure: true,
			Agent:         cwsAgentNodeWantFunc(false, false, true, ""),
		},
		{
			Name:          "v2alpha1 CWS enabled with VSock (Full mode, deprecated useVSock)",
			DDA:           ddaCWSLiteVSockFull,
			WantConfigure: true,
			Agent:         cwsAgentNodeWantFunc(false, false, false, v2alpha1.VSockModeFull),
		},
		{
			Name:          "v2alpha1 CWS enabled with VSock (SystemProbe mode)",
			DDA:           ddaCWSLiteVSockSystemProbe,
			WantConfigure: true,
			Agent:         cwsAgentNodeWantFunc(false, false, false, v2alpha1.VSockModeSystemProbe),
		},
	}

	tests.Run(t, buildCWSFeature)
}

func cwsAgentNodeWantFunc(withSubFeatures bool, directSendFromSysProbe bool, enforcementEnabled bool, vsockMode v2alpha1.VSockMode) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			// check security context capabilities
			sysProbeCapabilities := mgr.SecurityContextMgr.CapabilitiesByC[apicommon.SystemProbeContainerName]

			capabilitiesWant := agent.DefaultCapabilitiesForSystemProbe()
			if enforcementEnabled {
				capabilitiesWant = append(capabilitiesWant, "KILL")
			}
			assert.True(t, apiutils.IsEqualStruct(sysProbeCapabilities, capabilitiesWant), "System Probe security context capabilities \ndiff = %s", cmp.Diff(sysProbeCapabilities, capabilitiesWant))

			// VSock changes the runtime-security socket and event gRPC server depending on the mode:
			//   - Full: all CWS containers use vsock; security-agent hosts the event server.
			//   - SystemProbe: only the system-probe uses vsock and hosts a remote event server
			//     for the micro VM system-probe; the other containers keep the unix socket.
			unixSocket := "/var/run/sysprobe/runtime-security.sock"

			securityWant := []*corev1.EnvVar{
				{
					Name:  DDRuntimeSecurityConfigEnabled,
					Value: "true",
				},
			}
			if vsockMode == v2alpha1.VSockModeFull {
				securityWant = append(securityWant, &corev1.EnvVar{
					Name:  DDRuntimeSecurityConfigEventGRPCServer,
					Value: "security-agent",
				})
			}
			securitySocket := unixSocket
			if vsockMode == v2alpha1.VSockModeFull {
				securitySocket = "vsock:5020"
			}
			securityWant = append(securityWant,
				&corev1.EnvVar{
					Name:  DDRuntimeSecurityConfigSocket,
					Value: securitySocket,
				},
				&corev1.EnvVar{
					Name:  DDRuntimeSecurityConfigSyscallMonitorEnabled,
					Value: "true",
				},
			)
			sysProbeWant := []*corev1.EnvVar{
				{
					Name:  DDRuntimeSecurityConfigEnabled,
					Value: "true",
				},
			}
			switch vsockMode {
			case v2alpha1.VSockModeFull:
				sysProbeWant = append(sysProbeWant,
					&corev1.EnvVar{
						Name:  DDRuntimeSecurityConfigEventGRPCServer,
						Value: "security-agent",
					},
					&corev1.EnvVar{
						Name:  DDRuntimeSecurityConfigSocket,
						Value: "vsock:5020",
					},
				)
			case v2alpha1.VSockModeSystemProbe:
				sysProbeWant = append(sysProbeWant,
					&corev1.EnvVar{
						Name:  DDRuntimeSecurityConfigEventGRPCServer,
						Value: "system-probe",
					},
					&corev1.EnvVar{
						Name:  DDRuntimeSecurityConfigSocket,
						Value: "vsock:5020",
					},
				)
			default:
				sysProbeWant = append(sysProbeWant, &corev1.EnvVar{
					Name:  DDRuntimeSecurityConfigSocket,
					Value: unixSocket,
				})
			}
			sysProbeWant = append(sysProbeWant,
				&corev1.EnvVar{
					Name:  DDRuntimeSecurityConfigSyscallMonitorEnabled,
					Value: "true",
				},
			)
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
