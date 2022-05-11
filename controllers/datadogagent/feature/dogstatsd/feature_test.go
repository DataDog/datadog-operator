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

func Test_DogStatsDFeature_Configure(t *testing.T) {
	customMapperProfilesConf := `- name: 'profile_name'
  prefix: 'profile_prefix'
  mappings:
    - match: 'metric_to_match'
      name: 'mapped_metric_name'
`
	customMapperProfilesJSON := `[{"mappings":[{"match":"metric_to_match","name":"mapped_metric_name"}],"name":"profile_name","prefix":"profile_prefix"}]`

	// v1alpha1
	ddav1Enabled := &v1alpha1.DatadogAgent{}
	ddav1DogStatsDEnabled := v1alpha1.DatadogAgent{
		Spec: *v1alpha1.DefaultDatadogAgent(ddav1Enabled).DefaultOverride,
	}

	ddav1DogStatsDCustomHostPort := ddav1DogStatsDEnabled.DeepCopy()
	ddav1DogStatsDCustomHostPort.Spec.Agent.Config.HostPort = apiutils.NewInt32Pointer(1234)

	ddav1DogStatsDUDPOriginDetection := ddav1DogStatsDEnabled.DeepCopy()
	ddav1DogStatsDUDPOriginDetection.Spec.Agent.Config.Dogstatsd.DogstatsdOriginDetection = apiutils.NewBoolPointer(true)

	ddav1DogStatsDUDSEnabled := ddav1DogStatsDEnabled.DeepCopy()
	ddav1DogStatsDUDSEnabled.Spec.Agent.Config.Dogstatsd.UnixDomainSocket.Enabled = apiutils.NewBoolPointer(true)

	ddav1DogStatsDUDSCustomHostFilepath := ddav1DogStatsDUDSEnabled.DeepCopy()
	ddav1DogStatsDUDSCustomHostFilepath.Spec.Agent.Config.Dogstatsd.UnixDomainSocket.HostFilepath = apiutils.NewStringPointer("/custom/host/filepath")

	ddav1DogStatsDUDSOriginDetection := ddav1DogStatsDUDSEnabled.DeepCopy()
	ddav1DogStatsDUDSOriginDetection.Spec.Agent.Config.Dogstatsd.DogstatsdOriginDetection = apiutils.NewBoolPointer(true)

	ddav1DogStatsDMapperProfiles := ddav1DogStatsDUDSEnabled.DeepCopy()
	ddav1DogStatsDMapperProfiles.Spec.Agent.Config.Dogstatsd.MapperProfiles = &v1alpha1.CustomConfigSpec{ConfigData: &customMapperProfilesConf}

	// v2alpha1
	ddav2DogStatsDDisabled := &v2alpha1.DatadogAgent{}
	v2alpha1.DefaultDatadogAgent(ddav2DogStatsDDisabled)

	ddav2DogStatsDEnabled := ddav2DogStatsDDisabled.DeepCopy()
	ddav2DogStatsDEnabled.Spec.Features.Dogstatsd.HostPortConfig.Enabled = apiutils.NewBoolPointer(true)

	ddav2DogStatsDCustomHostPort := ddav2DogStatsDEnabled.DeepCopy()
	ddav2DogStatsDCustomHostPort.Spec.Features.Dogstatsd.HostPortConfig.Port = apiutils.NewInt32Pointer(1234)

	ddav2DogStatsDUDPOriginDetection := ddav2DogStatsDEnabled.DeepCopy()
	ddav2DogStatsDUDPOriginDetection.Spec.Features.Dogstatsd.OriginDetectionEnabled = apiutils.NewBoolPointer(true)

	ddav2DogStatsDUDSEnabled := ddav2DogStatsDDisabled.DeepCopy()
	ddav2DogStatsDUDSEnabled.Spec.Features.Dogstatsd.UnixDomainSocketConfig.Enabled = apiutils.NewBoolPointer(true)

	ddav2DogStatsDUDSCustomHostFilepath := ddav2DogStatsDUDSEnabled.DeepCopy()
	ddav2DogStatsDUDSCustomHostFilepath.Spec.Features.Dogstatsd.UnixDomainSocketConfig.Path = apiutils.NewStringPointer("/custom/host/filepath")

	ddav2DogStatsDUDSOriginDetection := ddav2DogStatsDUDSEnabled.DeepCopy()
	ddav2DogStatsDUDSOriginDetection.Spec.Features.Dogstatsd.OriginDetectionEnabled = apiutils.NewBoolPointer(true)

	ddav2DogStatsDMapperProfiles := ddav2DogStatsDUDSEnabled.DeepCopy()
	ddav2DogStatsDMapperProfiles.Spec.Features.Dogstatsd.MapperProfiles = &v2alpha1.CustomConfig{ConfigData: &customMapperProfilesConf}

	// v1alpha1 default uds volume mount
	wantVolumeMountsV1 := []corev1.VolumeMount{
		{
			Name:      apicommon.DogStatsDUDSSocketName,
			MountPath: apicommon.DogStatsDUDSHostFilepathV1,
			ReadOnly:  true,
		},
	}
	// v2alpha1 default uds volume mount
	wantVolumeMountsV2 := []corev1.VolumeMount{
		{
			Name:      apicommon.DogStatsDUDSSocketName,
			MountPath: apicommon.DogStatsDUDSHostFilepathV2,
			ReadOnly:  true,
		},
	}

	// v1alpha1 default uds volume
	wantVolumesV1 := []corev1.Volume{
		{
			Name: apicommon.DogStatsDUDSSocketName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: apicommon.DogStatsDUDSHostFilepathV1,
				},
			},
		},
	}

	// v2alpha1 default uds volume
	wantVolumesV2 := []corev1.Volume{
		{
			Name: apicommon.DogStatsDUDSSocketName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: apicommon.DogStatsDUDSHostFilepathV2,
				},
			},
		},
	}

	// default udp envvar
	wantUDPEnvVars := []*corev1.EnvVar{
		{
			Name:  apicommon.DDDogStatsDNonLocalTraffic,
			Value: "true",
		},
	}

	// v1alpha1 default uds envvar
	wantUDSEnvVarsV1 := []*corev1.EnvVar{
		{
			Name:  apicommon.DDDogStatsDSocket,
			Value: apicommon.DogStatsDUDSHostFilepathV1,
		},
	}

	// v2alpha1 default uds envvar
	wantUDSEnvVarsV2 := []*corev1.EnvVar{
		{
			Name:  apicommon.DDDogStatsDSocket,
			Value: apicommon.DogStatsDUDSHostFilepathV2,
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
		Name:  apicommon.DDDogStatsDSocket,
		Value: "/custom/host/filepath",
	}

	// default udp port
	wantPorts := []*corev1.ContainerPort{
		{
			Name:          apicommon.DogStatsDHostPortName,
			HostPort:      apicommon.DogStatsDHostPortHostPort,
			ContainerPort: apicommon.DogStatsDHostPortHostPort,
			Protocol:      corev1.ProtocolUDP,
		},
	}

	tests := test.FeatureTestSuite{
		///////////////////////////
		// v1alpha1.DatadogAgent //
		///////////////////////////
		{
			Name:          "v1alpha1 dogstatsd udp enabled",
			DDAv1:         ddav1DogStatsDEnabled.DeepCopy(),
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
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantPorts), "Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantPorts))
				},
			},
		},
		{
			Name:          "v1alpha1 udp custom host port",
			DDAv1:         ddav1DogStatsDCustomHostPort.DeepCopy(),
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
							Name:          apicommon.DogStatsDHostPortName,
							HostPort:      1234,
							ContainerPort: apicommon.DogStatsDHostPortHostPort,
							Protocol:      corev1.ProtocolUDP,
						},
					}
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, customPorts), "Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, customPorts))
				},
			},
		},
		{
			Name:          "v1alpha1 udp origin detection enabled",
			DDAv1:         ddav1DogStatsDUDPOriginDetection.DeepCopy(),
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
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantPorts), "Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantPorts))
				},
			},
		},
		{
			Name:          "v1alpha1 uds enabled",
			DDAv1:         ddav1DogStatsDUDSEnabled.DeepCopy(),
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
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantPorts), "Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantPorts))
				},
			},
		},
		{
			Name:          "v1alpha1 uds custom host filepath",
			DDAv1:         ddav1DogStatsDUDSCustomHostFilepath,
			WantConfigure: true,
			Agent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc: func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMgr.VolumeMountByC[apicommonv1.CoreAgentContainerName]
					customVolumeMounts := []corev1.VolumeMount{
						{
							Name:      apicommon.DogStatsDUDSSocketName,
							MountPath: "/custom/host/filepath",
							ReadOnly:  true,
						},
					}
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, customVolumeMounts), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, customVolumeMounts))
					volumes := mgr.VolumeMgr.Volumes
					customVolumes := []corev1.Volume{
						{
							Name: apicommon.DogStatsDUDSSocketName,
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
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantPorts), "Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantPorts))
				},
			},
		},
		{
			Name:          "v1alpha1 uds origin detection",
			DDAv1:         ddav1DogStatsDUDSOriginDetection,
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
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantPorts), "Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantPorts))
					assert.True(t, mgr.Tpl.Spec.HostPID, "Host PID \ndiff = %s", cmp.Diff(mgr.Tpl.Spec.HostPID, true))
				},
			},
		},
		{
			Name:          "v1alpha1 mapper profiles",
			DDAv1:         ddav1DogStatsDMapperProfiles,
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
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantPorts), "Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantPorts))
				},
			},
		},
		///////////////////////////
		// v2alpha1.DatadogAgent //
		///////////////////////////
		{
			Name:          "v2alpha1 dogstatsd udp enabled",
			DDAv2:         ddav2DogStatsDEnabled.DeepCopy(),
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
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantPorts), "Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantPorts))
				},
			},
		},
		{
			Name:          "v2alpha1 udp custom host port",
			DDAv2:         ddav2DogStatsDCustomHostPort.DeepCopy(),
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
							Name:          apicommon.DogStatsDHostPortName,
							HostPort:      1234,
							ContainerPort: apicommon.DogStatsDHostPortHostPort,
							Protocol:      corev1.ProtocolUDP,
						},
					}
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, customPorts), "Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, customPorts))
				},
			},
		},
		{
			Name:          "v2alpha1 udp origin detection enabled",
			DDAv2:         ddav2DogStatsDUDPOriginDetection.DeepCopy(),
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
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantPorts), "Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantPorts))
				},
			},
		},
		{
			Name:          "v2alpha1 uds enabled",
			DDAv2:         ddav2DogStatsDUDSEnabled.DeepCopy(),
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
			DDAv2:         ddav2DogStatsDUDSCustomHostFilepath,
			WantConfigure: true,
			Agent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc: func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMgr.VolumeMountByC[apicommonv1.CoreAgentContainerName]
					customVolumeMounts := []corev1.VolumeMount{
						{
							Name:      apicommon.DogStatsDUDSSocketName,
							MountPath: "/custom/host/filepath",
							ReadOnly:  true,
						},
					}
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, customVolumeMounts), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, customVolumeMounts))
					volumes := mgr.VolumeMgr.Volumes
					customVolumes := []corev1.Volume{
						{
							Name: apicommon.DogStatsDUDSSocketName,
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
			DDAv2:         ddav2DogStatsDUDSOriginDetection,
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
			DDAv2:         ddav2DogStatsDMapperProfiles,
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

	tests.Run(t, buildDogStatsDFeature)
}
