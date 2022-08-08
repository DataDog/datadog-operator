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

const (
	v1DogstatsdSocketName = "statsd.sock"
	v1DogstatsdSocketPath = "/var/run/datadog"
	v2DogstatsdSocketPath = "/var/run/datadog/statsd/dsd.socket"
	customVolumePath      = "/custom/host"
	customPath            = "/custom/host/filepath"
)

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
	ddav1DogstatsdUDSCustomHostFilepath.Spec.Agent.Config.Dogstatsd.UnixDomainSocket.HostFilepath = apiutils.NewStringPointer(customPath)

	ddav1DogstatsdUDSOriginDetection := ddav1DogstatsdUDSEnabled.DeepCopy()
	ddav1DogstatsdUDSOriginDetection.Spec.Agent.Config.Dogstatsd.DogstatsdOriginDetection = apiutils.NewBoolPointer(true)

	ddav1DogstatsdMapperProfiles := ddav1DogstatsdUDSEnabled.DeepCopy()
	ddav1DogstatsdMapperProfiles.Spec.Agent.Config.Dogstatsd.MapperProfiles = &v1alpha1.CustomConfigSpec{ConfigData: &customMapperProfilesConf}

	// v2alpha1
	ddav2DogstatsdUDPDisabled := &v2alpha1.DatadogAgent{}
	v2alpha1.DefaultDatadogAgent(ddav2DogstatsdUDPDisabled)

	ddav2DogstatsdHostPortEnabled := ddav2DogstatsdUDPDisabled.DeepCopy()
	ddav2DogstatsdHostPortEnabled.Spec.Features.Dogstatsd.HostPortConfig.Enabled = apiutils.NewBoolPointer(true)
	v2alpha1.DefaultDatadogAgent(ddav2DogstatsdHostPortEnabled)

	ddav2DogstatsdCustomHostPort := ddav2DogstatsdHostPortEnabled.DeepCopy()
	ddav2DogstatsdCustomHostPort.Spec.Features.Dogstatsd.HostPortConfig.Port = apiutils.NewInt32Pointer(1234)
	v2alpha1.DefaultDatadogAgent(ddav2DogstatsdCustomHostPort)

	ddav2DogstatsdUDPOriginDetection := ddav2DogstatsdHostPortEnabled.DeepCopy()
	ddav2DogstatsdUDPOriginDetection.Spec.Features.Dogstatsd.OriginDetectionEnabled = apiutils.NewBoolPointer(true)
	v2alpha1.DefaultDatadogAgent(ddav2DogstatsdUDPOriginDetection)

	ddav2DogstatsdUDSDisabled := ddav2DogstatsdUDPDisabled.DeepCopy()
	ddav2DogstatsdUDSDisabled.Spec.Features.Dogstatsd.UnixDomainSocketConfig.Enabled = apiutils.NewBoolPointer(false)
	v2alpha1.DefaultDatadogAgent(ddav2DogstatsdUDSDisabled)

	ddav2DogstatsdUDSCustomHostFilepath := ddav2DogstatsdUDPDisabled.DeepCopy()
	ddav2DogstatsdUDSCustomHostFilepath.Spec.Features.Dogstatsd.UnixDomainSocketConfig.Path = apiutils.NewStringPointer(customPath)
	v2alpha1.DefaultDatadogAgent(ddav2DogstatsdUDSCustomHostFilepath)

	ddav2DogstatsdUDSOriginDetection := ddav2DogstatsdUDPDisabled.DeepCopy()
	ddav2DogstatsdUDSOriginDetection.Spec.Features.Dogstatsd.OriginDetectionEnabled = apiutils.NewBoolPointer(true)
	v2alpha1.DefaultDatadogAgent(ddav2DogstatsdUDSOriginDetection)

	ddav2DogstatsdMapperProfiles := ddav2DogstatsdUDPDisabled.DeepCopy()
	ddav2DogstatsdMapperProfiles.Spec.Features.Dogstatsd.MapperProfiles = &v2alpha1.CustomConfig{ConfigData: &customMapperProfilesConf}
	v2alpha1.DefaultDatadogAgent(ddav2DogstatsdMapperProfiles)

	// v1alpha1 default uds volume mount
	wantVolumeMountsV1 := []corev1.VolumeMount{
		{
			Name:      apicommon.DogstatsdSocketVolumeName,
			MountPath: v1DogstatsdSocketPath,
			ReadOnly:  true,
		},
	}
	// v2alpha1 default uds volume mount
	wantVolumeMounts := []corev1.VolumeMount{
		{
			Name:      apicommon.DogstatsdSocketVolumeName,
			MountPath: apicommon.DogstatsdSocketVolumePath,
			ReadOnly:  true,
		},
	}

	// v1alpha1 default uds volume
	wantVolumesV1 := []corev1.Volume{
		{
			Name: apicommon.DogstatsdSocketVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: v1DogstatsdSocketPath,
				},
			},
		},
	}

	// v2alpha1 default uds volume
	wantVolumes := []corev1.Volume{
		{
			Name: apicommon.DogstatsdSocketVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: apicommon.DogstatsdSocketVolumePath,
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
			Value: v1DogstatsdSocketPath + "/" + v1DogstatsdSocketName,
		},
	}

	// v2alpha1 default uds envvar
	wantUDSEnvVarsV2 := []*corev1.EnvVar{
		{
			Name:  apicommon.DDDogstatsdSocket,
			Value: v2DogstatsdSocketPath,
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
		Value: customPath,
	}

	// v1alpha1 default udp port
	wantContainerPorts := []*corev1.ContainerPort{
		{
			Name:          apicommon.DogstatsdHostPortName,
			ContainerPort: apicommon.DogstatsdHostPortHostPort,
			Protocol:      corev1.ProtocolUDP,
		},
	}

	// v2alpha1 default udp port
	wantHostPorts := []*corev1.ContainerPort{
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
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, []*corev1.VolumeMount(nil)), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, []*corev1.VolumeMount(nil)))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, []*corev1.Volume{}), "Volumes \ndiff = %s", cmp.Diff(volumes, []*corev1.Volume{}))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, []*corev1.EnvVar(nil)), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, []*corev1.EnvVar(nil)))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantContainerPorts), "Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantContainerPorts))
				},
			),
		},
		{
			Name:          "v1alpha1 udp custom host port",
			DDAv1:         ddav1DogstatsdCustomHostPort,
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, []*corev1.VolumeMount(nil)), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, []*corev1.VolumeMount(nil)))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, []*corev1.Volume{}), "Volumes \ndiff = %s", cmp.Diff(volumes, []*corev1.Volume{}))
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
			),
		},
		{
			Name:          "v1alpha1 udp origin detection enabled",
			DDAv1:         ddav1DogstatsdUDPOriginDetection.DeepCopy(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, []*corev1.VolumeMount(nil)), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, []*corev1.VolumeMount(nil)))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, []*corev1.Volume{}), "Volumes \ndiff = %s", cmp.Diff(volumes, []*corev1.Volume{}))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					customEnvVars := append([]*corev1.EnvVar{}, &originDetectionEnvVar)
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, customEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, customEnvVars))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantContainerPorts), "Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantContainerPorts))
				},
			),
		},
		{
			Name:          "v1alpha1 uds enabled",
			DDAv1:         ddav1DogstatsdUDSEnabled.DeepCopy(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantVolumeMountsV1), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantVolumeMountsV1))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumesV1), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumesV1))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, wantUDSEnvVarsV1), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, wantUDSEnvVarsV1))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantContainerPorts), "Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantContainerPorts))
				},
			),
		},
		{
			Name:          "v1alpha1 uds custom host filepath",
			DDAv1:         ddav1DogstatsdUDSCustomHostFilepath,
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.CoreAgentContainerName]
					customVolumeMounts := []corev1.VolumeMount{
						{
							Name:      apicommon.DogstatsdSocketVolumeName,
							MountPath: customVolumePath,
							ReadOnly:  true,
						},
					}
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, customVolumeMounts), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, customVolumeMounts))
					volumes := mgr.VolumeMgr.Volumes
					customVolumes := []corev1.Volume{
						{
							Name: apicommon.DogstatsdSocketVolumeName,
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: customVolumePath,
								},
							},
						},
					}
					assert.True(t, apiutils.IsEqualStruct(volumes, customVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, customVolumes))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					customEnvVars := append([]*corev1.EnvVar{}, &customFilepathEnvVar)
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, customEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, customEnvVars))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantContainerPorts), "Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantContainerPorts))
				},
			),
		},
		{
			Name:          "v1alpha1 uds origin detection",
			DDAv1:         ddav1DogstatsdUDSOriginDetection,
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantVolumeMountsV1), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantVolumeMountsV1))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumesV1), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumesV1))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					customEnvVars := append(wantUDSEnvVarsV1, &originDetectionEnvVar)
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, customEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, customEnvVars))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantContainerPorts), "Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantContainerPorts))
					assert.True(t, mgr.Tpl.Spec.HostPID, "Host PID \ndiff = %s", cmp.Diff(mgr.Tpl.Spec.HostPID, true))
				},
			),
		},
		{
			Name:          "v1alpha1 mapper profiles",
			DDAv1:         ddav1DogstatsdMapperProfiles,
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantVolumeMountsV1), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantVolumeMountsV1))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumesV1), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumesV1))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					customEnvVars := append(wantUDSEnvVarsV1, &mapperProfilesEnvVar)
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, customEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, customEnvVars))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantContainerPorts), "Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantContainerPorts))
				},
			),
		},
		///////////////////////////
		// v2alpha1.DatadogAgent //
		///////////////////////////
		{
			Name:          "v2alpha1 dogstatsd udp hostport enabled",
			DDAv2:         ddav2DogstatsdHostPortEnabled,
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantVolumeMounts), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantVolumeMounts))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					customEnvVars := append(wantUDPEnvVars, wantUDSEnvVarsV2...)
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, customEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, customEnvVars))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantHostPorts), "Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantHostPorts))
				},
			),
		},
		{
			Name:          "v2alpha1 udp custom host port",
			DDAv2:         ddav2DogstatsdCustomHostPort.DeepCopy(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantVolumeMounts), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantVolumeMounts))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					customEnvVars := append(wantUDPEnvVars, wantUDSEnvVarsV2...)
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, customEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, customEnvVars))
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
			),
		},
		{
			Name:          "v2alpha1 udp origin detection enabled",
			DDAv2:         ddav2DogstatsdUDPOriginDetection.DeepCopy(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantVolumeMounts), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantVolumeMounts))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					customEnvVars := append(wantUDPEnvVars, wantUDSEnvVarsV2...)
					customEnvVars = append(customEnvVars, &originDetectionEnvVar)
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, customEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, customEnvVars))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantHostPorts), "Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantHostPorts))
				},
			),
		},
		{
			Name:          "v2alpha1 uds disabled",
			DDAv2:         ddav2DogstatsdUDSDisabled.DeepCopy(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, []*corev1.Volume(nil)), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, []*corev1.VolumeMount(nil)))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, []*corev1.Volume{}), "Volumes \ndiff = %s", cmp.Diff(volumes, []*corev1.Volume{}))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, agentEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, agentEnvVars))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantContainerPorts), "Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantContainerPorts))
				},
			),
		},
		{
			Name:          "v2alpha1 uds custom host filepath",
			DDAv2:         ddav2DogstatsdUDSCustomHostFilepath,
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.CoreAgentContainerName]
					customVolumeMounts := []corev1.VolumeMount{
						{
							Name:      apicommon.DogstatsdSocketVolumeName,
							MountPath: customVolumePath,
							ReadOnly:  true,
						},
					}
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, customVolumeMounts), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, customVolumeMounts))
					volumes := mgr.VolumeMgr.Volumes
					customVolumes := []corev1.Volume{
						{
							Name: apicommon.DogstatsdSocketVolumeName,
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: customVolumePath,
								},
							},
						},
					}
					assert.True(t, apiutils.IsEqualStruct(volumes, customVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, customVolumes))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					customEnvVars := append([]*corev1.EnvVar{}, &customFilepathEnvVar)
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, customEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, customEnvVars))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantContainerPorts), "Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantContainerPorts))
				},
			),
		},
		{
			Name:          "v2alpha1 uds origin detection",
			DDAv2:         ddav2DogstatsdUDSOriginDetection,
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantVolumeMounts), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantVolumeMounts))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					customEnvVars := append(wantUDSEnvVarsV2, &originDetectionEnvVar)
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, customEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, customEnvVars))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantContainerPorts), "Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantContainerPorts))
					assert.True(t, mgr.Tpl.Spec.HostPID, "Host PID \ndiff = %s", cmp.Diff(mgr.Tpl.Spec.HostPID, true))
				},
			),
		},
		{
			Name:          "v2alpha1 mapper profiles",
			DDAv2:         ddav2DogstatsdMapperProfiles,
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					coreAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantVolumeMounts), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantVolumeMounts))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					customEnvVars := append(wantUDSEnvVarsV2, &mapperProfilesEnvVar)
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, customEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, customEnvVars))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantContainerPorts), "Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantContainerPorts))
				},
			),
		},
	}

	tests.Run(t, buildDogstatsdFeature)
}
