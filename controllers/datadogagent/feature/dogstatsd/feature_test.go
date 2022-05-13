// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dogstatsd

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

func Test_DogstatsdFeature_Configure(t *testing.T) {
	customMapperProfilesConf := `- name: 'profile_name'
  prefix: 'profile_prefix'
  mappings:
    - match: 'metric_to_match'
      name: 'mapped_metric_name'
`
	customMapperProfilesJSON := `[{"mappings":[{"match":"metric_to_match","name":"mapped_metric_name"}],"name":"profile_name","prefix":"profile_prefix"}]`

	// v1alpha1
	ddav1Enabled := &v1alpha1.DatadogAgent{}
	ddav1DogstatsdEnabled := v1alpha1.DatadogAgent{
		Spec: *v1alpha1.DefaultDatadogAgent(ddav1Enabled).DefaultOverride,
	}

	ddav1DogstatsdCustomHostPort := ddav1DogstatsdEnabled.DeepCopy()
	ddav1DogstatsdCustomHostPort.Spec.Agent.Config.HostPort = apiutils.NewInt32Pointer(1234)

	ddav1DogstatsdUDPOriginDetection := ddav1DogstatsdEnabled.DeepCopy()
	ddav1DogstatsdUDPOriginDetection.Spec.Agent.Config.Dogstatsd.DogstatsdOriginDetection = apiutils.NewBoolPointer(true)

	ddav1DogstatsdUDSEnabled := ddav1DogstatsdEnabled.DeepCopy()
	ddav1DogstatsdUDSEnabled.Spec.Agent.Config.Dogstatsd.UnixDomainSocket.Enabled = apiutils.NewBoolPointer(true)

	ddav1DogstatsdUDSCustomHostFilepath := ddav1DogstatsdUDSEnabled.DeepCopy()
	ddav1DogstatsdUDSCustomHostFilepath.Spec.Agent.Config.Dogstatsd.UnixDomainSocket.HostFilepath = apiutils.NewStringPointer("/custom/host/filepath")

	ddav1DogstatsdUDSOriginDetection := ddav1DogstatsdUDSEnabled.DeepCopy()
	ddav1DogstatsdUDSOriginDetection.Spec.Agent.Config.Dogstatsd.DogstatsdOriginDetection = apiutils.NewBoolPointer(true)

	ddav1DogstatsdMapperProfiles := ddav1DogstatsdUDSEnabled.DeepCopy()
	ddav1DogstatsdMapperProfiles.Spec.Agent.Config.Dogstatsd.MapperProfiles = &v1alpha1.CustomConfigSpec{ConfigData: &customMapperProfilesConf}

	// v2alpha1
	ddav2DogstatsdDisabled := &v2alpha1.DatadogAgent{}
	v2alpha1.DefaultDatadogAgent(ddav2DogstatsdDisabled)

	ddav2DogstatsdEnabled := ddav2DogstatsdDisabled.DeepCopy()
	ddav2DogstatsdEnabled.Spec.Features.Dogstatsd.HostPortConfig.Enabled = apiutils.NewBoolPointer(true)
	v2alpha1.DefaultDatadogAgent(ddav2DogstatsdEnabled)

	ddav2DogstatsdCustomHostPort := ddav2DogstatsdEnabled.DeepCopy()
	ddav2DogstatsdCustomHostPort.Spec.Features.Dogstatsd.HostPortConfig.Port = apiutils.NewInt32Pointer(1234)
	v2alpha1.DefaultDatadogAgent(ddav2DogstatsdCustomHostPort)

	ddav2DogstatsdUDPOriginDetection := ddav2DogstatsdEnabled.DeepCopy()
	ddav2DogstatsdUDPOriginDetection.Spec.Features.Dogstatsd.OriginDetectionEnabled = apiutils.NewBoolPointer(true)
	v2alpha1.DefaultDatadogAgent(ddav2DogstatsdUDPOriginDetection)

	ddav2DogstatsdUDSEnabled := ddav2DogstatsdDisabled.DeepCopy()
	ddav2DogstatsdUDSEnabled.Spec.Features.Dogstatsd.UnixDomainSocketConfig.Enabled = apiutils.NewBoolPointer(true)
	v2alpha1.DefaultDatadogAgent(ddav2DogstatsdUDSEnabled)

	ddav2DogstatsdUDSCustomHostFilepath := ddav2DogstatsdUDSEnabled.DeepCopy()
	ddav2DogstatsdUDSCustomHostFilepath.Spec.Features.Dogstatsd.UnixDomainSocketConfig.Path = apiutils.NewStringPointer("/custom/host/filepath")
	v2alpha1.DefaultDatadogAgent(ddav2DogstatsdUDSCustomHostFilepath)

	ddav2DogstatsdUDSOriginDetection := ddav2DogstatsdUDSEnabled.DeepCopy()
	ddav2DogstatsdUDSOriginDetection.Spec.Features.Dogstatsd.OriginDetectionEnabled = apiutils.NewBoolPointer(true)
	v2alpha1.DefaultDatadogAgent(ddav2DogstatsdUDSOriginDetection)

	ddav2DogstatsdMapperProfiles := ddav2DogstatsdUDSEnabled.DeepCopy()
	ddav2DogstatsdMapperProfiles.Spec.Features.Dogstatsd.MapperProfiles = &v2alpha1.CustomConfig{ConfigData: &customMapperProfilesConf}
	v2alpha1.DefaultDatadogAgent(ddav2DogstatsdMapperProfiles)

	// v1alpha1 default uds volume mount
	wantVolumeMountsV1 := []corev1.VolumeMount{
		{
			Name:      apicommon.DogstatsdUDSSocketName,
			MountPath: apicommon.DogstatsdUDSHostFilepathV1,
			ReadOnly:  true,
		},
	}
	// v2alpha1 default uds volume mount
	wantVolumeMountsV2 := []corev1.VolumeMount{
		{
			Name:      apicommon.DogstatsdUDSSocketName,
			MountPath: apicommon.DogstatsdUDSHostFilepathV2,
			ReadOnly:  true,
		},
	}

	// v1alpha1 default uds volume
	wantVolumesV1 := []corev1.Volume{
		{
			Name: apicommon.DogstatsdUDSSocketName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: apicommon.DogstatsdUDSHostFilepathV1,
				},
			},
		},
	}

	// v2alpha1 default uds volume
	wantVolumesV2 := []corev1.Volume{
		{
			Name: apicommon.DogstatsdUDSSocketName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: apicommon.DogstatsdUDSHostFilepathV2,
				},
			},
		},
	}

	// default udp envvar
	wantUDPEnvVars := []*corev1.EnvVar{
		{
			Name:  apicommon.DDDogstatsdNonLocalTraffic,
			Value: "true",
		},
	}

	// v1alpha1 default uds envvar
	wantUDSEnvVarsV1 := []*corev1.EnvVar{
		{
			Name:  apicommon.DDDogstatsdSocket,
			Value: apicommon.DogstatsdUDSHostFilepathV1,
		},
	}

	// v2alpha1 default uds envvar
	wantUDSEnvVarsV2 := []*corev1.EnvVar{
		{
			Name:  apicommon.DDDogstatsdSocket,
			Value: apicommon.DogstatsdUDSHostFilepathV2,
		},
	}

	// origin detection envvar
	originDetectionEnvVar := corev1.EnvVar{
		Name:  apicommon.DDDogstatsdOriginDetection,
		Value: "true",
	}

	// mapper profiles envvar
	mapperProfilesEnvVar := corev1.EnvVar{
		Name:  apicommon.DDDogstatsdMapperProfiles,
		Value: customMapperProfilesJSON,
	}

	// custom uds filepath envvar
	customFilepathEnvVar := corev1.EnvVar{
		Name:  apicommon.DDDogstatsdSocket,
		Value: "/custom/host/filepath",
	}

	// v1alpha1 default udp port
	wantPortsV1 := []*corev1.ContainerPort{
		{
			Name:          apicommon.DogstatsdHostPortName,
			ContainerPort: apicommon.DogstatsdHostPortHostPort,
			Protocol:      corev1.ProtocolUDP,
		},
	}

	// v2alpha1 default udp port
	wantPortsV2 := []*corev1.ContainerPort{
		{
			Name:          apicommon.DogstatsdHostPortName,
			HostPort:      apicommon.DogstatsdHostPortHostPort,
			ContainerPort: apicommon.DogstatsdHostPortHostPort,
			Protocol:      corev1.ProtocolUDP,
		},
	}

	tests := test.FeatureTestSuite{
		///////////////////////////
		// v1alpha1.DatadogAgent //
		///////////////////////////
		{
			Name:          "v1alpha1 dogstatsd udp enabled",
			DDAv1:         &ddav1DogstatsdEnabled,
			WantConfigure: true,
			Agent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc: func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMgr.VolumeMountByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, []*corev1.VolumeMount(nil)), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, []*corev1.VolumeMount(nil)))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, []*corev1.Volume(nil)), "Volumes \ndiff = %s", cmp.Diff(volumes, []*corev1.Volume(nil)))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, wantUDPEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, wantUDPEnvVars))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantPortsV1), "Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantPortsV1))
				},
			},
		},
		{
			Name:          "v1alpha1 udp custom host port",
			DDAv1:         ddav1DogstatsdCustomHostPort,
			WantConfigure: true,
			Agent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc: func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMgr.VolumeMountByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, []*corev1.VolumeMount(nil)), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, []*corev1.VolumeMount(nil)))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, []*corev1.Volume(nil)), "Volumes \ndiff = %s", cmp.Diff(volumes, []*corev1.Volume(nil)))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, wantUDPEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, wantUDPEnvVars))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					customPorts := []*corev1.ContainerPort{
						{
							Name:          apicommon.DogstatsdHostPortName,
							HostPort:      1234,
							ContainerPort: apicommon.DogstatsdHostPortHostPort,
							Protocol:      corev1.ProtocolUDP,
						},
					}
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, customPorts), "Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, customPorts))
				},
			},
		},
		{
			Name:          "v1alpha1 udp origin detection enabled",
			DDAv1:         ddav1DogstatsdUDPOriginDetection.DeepCopy(),
			WantConfigure: true,
			Agent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc: func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMgr.VolumeMountByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, []*corev1.VolumeMount(nil)), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, []*corev1.VolumeMount(nil)))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, []*corev1.Volume(nil)), "Volumes \ndiff = %s", cmp.Diff(volumes, []*corev1.Volume(nil)))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					customEnvVars := append(wantUDPEnvVars, &originDetectionEnvVar)
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, customEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, customEnvVars))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantPortsV1), "Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantPortsV1))
				},
			},
		},
		{
			Name:          "v1alpha1 uds enabled",
			DDAv1:         ddav1DogstatsdUDSEnabled.DeepCopy(),
			WantConfigure: true,
			Agent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc: func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMgr.VolumeMountByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantVolumeMountsV1), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantVolumeMountsV1))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumesV1), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumesV1))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					customEnvVars := append(wantUDPEnvVars, wantUDSEnvVarsV1...)
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, customEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, customEnvVars))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantPortsV1), "Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantPortsV1))
				},
			},
		},
		{
			Name:          "v1alpha1 uds custom host filepath",
			DDAv1:         ddav1DogstatsdUDSCustomHostFilepath,
			WantConfigure: true,
			Agent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc: func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMgr.VolumeMountByC[apicommonv1.CoreAgentContainerName]
					customVolumeMounts := []corev1.VolumeMount{
						{
							Name:      apicommon.DogstatsdUDSSocketName,
							MountPath: "/custom/host/filepath",
							ReadOnly:  true,
						},
					}
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, customVolumeMounts), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, customVolumeMounts))
					volumes := mgr.VolumeMgr.Volumes
					customVolumes := []corev1.Volume{
						{
							Name: apicommon.DogstatsdUDSSocketName,
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/custom/host/filepath",
								},
							},
						},
					}
					assert.True(t, apiutils.IsEqualStruct(volumes, customVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, customVolumes))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					customEnvVars := append(wantUDPEnvVars, &customFilepathEnvVar)
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, customEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, customEnvVars))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantPortsV1), "Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantPortsV1))
				},
			},
		},
		{
			Name:          "v1alpha1 uds origin detection",
			DDAv1:         ddav1DogstatsdUDSOriginDetection,
			WantConfigure: true,
			Agent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc: func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMgr.VolumeMountByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantVolumeMountsV1), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantVolumeMountsV1))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumesV1), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumesV1))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					customEnvVars := append(wantUDPEnvVars, wantUDSEnvVarsV1...)
					customEnvVars = append(customEnvVars, &originDetectionEnvVar)
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, customEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, customEnvVars))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantPortsV1), "Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantPortsV1))
					assert.True(t, mgr.Tpl.Spec.HostPID, "Host PID \ndiff = %s", cmp.Diff(mgr.Tpl.Spec.HostPID, true))
				},
			},
		},
		{
			Name:          "v1alpha1 mapper profiles",
			DDAv1:         ddav1DogstatsdMapperProfiles,
			WantConfigure: true,
			Agent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc: func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMgr.VolumeMountByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantVolumeMountsV1), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantVolumeMountsV1))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumesV1), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumesV1))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					customEnvVars := append(wantUDPEnvVars, wantUDSEnvVarsV1...)
					customEnvVars = append(customEnvVars, &mapperProfilesEnvVar)
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, customEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, customEnvVars))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantPortsV1), "Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantPortsV1))
				},
			},
		},
		///////////////////////////
		// v2alpha1.DatadogAgent //
		///////////////////////////
		{
			Name:          "v2alpha1 dogstatsd udp enabled",
			DDAv2:         ddav2DogstatsdEnabled,
			WantConfigure: true,
			Agent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc: func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMgr.VolumeMountByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, []*corev1.VolumeMount(nil)), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, []*corev1.VolumeMount(nil)))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, []*corev1.Volume(nil)), "Volumes \ndiff = %s", cmp.Diff(volumes, []*corev1.Volume(nil)))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, wantUDPEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, wantUDPEnvVars))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantPortsV2), "Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantPortsV2))
				},
			},
		},
		{
			Name:          "v2alpha1 udp custom host port",
			DDAv2:         ddav2DogstatsdCustomHostPort.DeepCopy(),
			WantConfigure: true,
			Agent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc: func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMgr.VolumeMountByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, []*corev1.VolumeMount(nil)), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, []*corev1.VolumeMount(nil)))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, []*corev1.Volume(nil)), "Volumes \ndiff = %s", cmp.Diff(volumes, []*corev1.Volume(nil)))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, wantUDPEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, wantUDPEnvVars))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					customPorts := []*corev1.ContainerPort{
						{
							Name:          apicommon.DogstatsdHostPortName,
							HostPort:      1234,
							ContainerPort: apicommon.DogstatsdHostPortHostPort,
							Protocol:      corev1.ProtocolUDP,
						},
					}
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, customPorts), "Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, customPorts))
				},
			},
		},
		{
			Name:          "v2alpha1 udp origin detection enabled",
			DDAv2:         ddav2DogstatsdUDPOriginDetection.DeepCopy(),
			WantConfigure: true,
			Agent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc: func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMgr.VolumeMountByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, []*corev1.VolumeMount(nil)), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, []*corev1.VolumeMount(nil)))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, []*corev1.Volume(nil)), "Volumes \ndiff = %s", cmp.Diff(volumes, []*corev1.Volume(nil)))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					customEnvVars := append(wantUDPEnvVars, &originDetectionEnvVar)
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, customEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, customEnvVars))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantPortsV2), "Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantPortsV2))
				},
			},
		},
		{
			Name:          "v2alpha1 uds enabled",
			DDAv2:         ddav2DogstatsdUDSEnabled.DeepCopy(),
			WantConfigure: true,
			Agent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc: func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMgr.VolumeMountByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantVolumeMountsV2), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantVolumeMountsV2))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumesV2), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumesV2))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, agentEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, agentEnvVars))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, []*corev1.ContainerPort(nil)), "Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, []*corev1.ContainerPort(nil)))
				},
			},
		},
		{
			Name:          "v2alpha1 uds custom host filepath",
			DDAv2:         ddav2DogstatsdUDSCustomHostFilepath,
			WantConfigure: true,
			Agent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc: func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMgr.VolumeMountByC[apicommonv1.CoreAgentContainerName]
					customVolumeMounts := []corev1.VolumeMount{
						{
							Name:      apicommon.DogstatsdUDSSocketName,
							MountPath: "/custom/host/filepath",
							ReadOnly:  true,
						},
					}
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, customVolumeMounts), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, customVolumeMounts))
					volumes := mgr.VolumeMgr.Volumes
					customVolumes := []corev1.Volume{
						{
							Name: apicommon.DogstatsdUDSSocketName,
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/custom/host/filepath",
								},
							},
						},
					}
					assert.True(t, apiutils.IsEqualStruct(volumes, customVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, customVolumes))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					customEnvVars := append([]*corev1.EnvVar{}, &customFilepathEnvVar)
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, customEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, customEnvVars))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, []*corev1.ContainerPort(nil)), "Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, []*corev1.ContainerPort(nil)))
				},
			},
		},
		{
			Name:          "v2alpha1 uds origin detection",
			DDAv2:         ddav2DogstatsdUDSOriginDetection,
			WantConfigure: true,
			Agent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc: func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMgr.VolumeMountByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantVolumeMountsV2), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantVolumeMountsV2))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumesV2), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumesV2))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					customEnvVars := append(wantUDSEnvVarsV2, &originDetectionEnvVar)
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, customEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, customEnvVars))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, []*corev1.ContainerPort(nil)), "Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, []*corev1.ContainerPort(nil)))
					assert.True(t, mgr.Tpl.Spec.HostPID, "Host PID \ndiff = %s", cmp.Diff(mgr.Tpl.Spec.HostPID, true))
				},
			},
		},
		{
			Name:          "v2alpha1 mapper profiles",
			DDAv2:         ddav2DogstatsdMapperProfiles,
			WantConfigure: true,
			Agent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc: func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMgr.VolumeMountByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantVolumeMountsV2), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantVolumeMountsV2))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumesV2), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumesV2))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					customEnvVars := append(wantUDSEnvVarsV2, &mapperProfilesEnvVar)
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, customEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, customEnvVars))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, []*corev1.ContainerPort(nil)), "Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, []*corev1.ContainerPort(nil)))
				},
			},
		},
	}

	tests.Run(t, buildDogstatsdFeature)
}
