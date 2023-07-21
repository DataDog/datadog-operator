// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dogstatsd

import (
	"strconv"
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
	customVolumePath = "/custom/host"
	customPath       = "/custom/host/filepath.sock"
	customSock       = "filepath.sock"
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
			MountPath: apicommon.DogstatsdSocketLocalPath,
			ReadOnly:  false,
		},
	}
	// v2alpha1 default uds volume mount
	wantVolumeMounts := []corev1.VolumeMount{
		{
			Name:      apicommon.DogstatsdSocketVolumeName,
			MountPath: apicommon.DogstatsdSocketLocalPath,
			ReadOnly:  false,
		},
	}

	// v1alpha1 default uds volume
	volType := corev1.HostPathDirectoryOrCreate
	wantVolumesV1 := []corev1.Volume{
		{
			Name: apicommon.DogstatsdSocketVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: apicommon.DogstatsdAPMSocketHostPath,
					Type: &volType,
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
					Path: apicommon.DogstatsdAPMSocketHostPath,
					Type: &volType,
				},
			},
		},
	}

	// default udp envvar
	wantUDPEnvVars := []*corev1.EnvVar{
		{
			Name:  apicommon.DDDogstatsdPort,
			Value: strconv.Itoa(apicommon.DogstatsdHostPortHostPort),
		},
		{
			Name:  apicommon.DDDogstatsdNonLocalTraffic,
			Value: "true",
		},
	}

	// custom udp envvar
	wantCustomUDPEnvVars := []*corev1.EnvVar{
		{
			Name:  apicommon.DDDogstatsdPort,
			Value: "1234",
		},
		{
			Name:  apicommon.DDDogstatsdNonLocalTraffic,
			Value: "true",
		},
	}

	// v1alpha1 default uds envvar
	wantUDSEnvVarsV1 := []*corev1.EnvVar{
		{
			Name:  apicommon.DDDogstatsdSocket,
			Value: apicommon.DogstatsdSocketLocalPath + "/" + "statsd.sock",
		},
	}

	// v2alpha1 default uds envvar
	wantUDSEnvVarsV2 := []*corev1.EnvVar{
		{
			Name:  apicommon.DDDogstatsdSocket,
			Value: apicommon.DogstatsdSocketLocalPath + "/" + apicommon.DogstatsdSocketName,
		},
	}
	customEnvVar := []*corev1.EnvVar{
		{
			Name:  apicommon.DDDogstatsdSocket,
			Value: apicommon.DogstatsdSocketLocalPath + "/" + customSock,
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
		Value: apicommon.DogstatsdSocketLocalPath + "/" + customSock,
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
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, []*corev1.VolumeMount(nil)), "1. Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, []*corev1.VolumeMount(nil)))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, []*corev1.Volume{}), "1. Volumes \ndiff = %s", cmp.Diff(volumes, []*corev1.Volume{}))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, []*corev1.EnvVar(nil)), "1. Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, []*corev1.EnvVar(nil)))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantContainerPorts), "1. Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantContainerPorts))
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
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, []*corev1.VolumeMount(nil)), "2. Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, []*corev1.VolumeMount(nil)))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, []*corev1.Volume{}), "2. Volumes \ndiff = %s", cmp.Diff(volumes, []*corev1.Volume{}))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, wantCustomUDPEnvVars), "2. Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, wantUDPEnvVars))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					customPorts := []*corev1.ContainerPort{
						{
							Name:          apicommon.DogstatsdHostPortName,
							HostPort:      1234,
							ContainerPort: apicommon.DogstatsdHostPortHostPort,
							Protocol:      corev1.ProtocolUDP,
						},
					}
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, customPorts), "2. Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, customPorts))
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
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, []*corev1.VolumeMount(nil)), "3. Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, []*corev1.VolumeMount(nil)))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, []*corev1.Volume{}), "3. Volumes \ndiff = %s", cmp.Diff(volumes, []*corev1.Volume{}))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					customEnvVars := append([]*corev1.EnvVar{}, &originDetectionEnvVar)
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, customEnvVars), "3. Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, customEnvVars))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantContainerPorts), "3. Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantContainerPorts))
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
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantVolumeMountsV1), "4. Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantVolumeMountsV1))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumesV1), "4. Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumesV1))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.AllContainers]
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, wantUDSEnvVarsV1), "4. Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, wantUDSEnvVarsV1))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantContainerPorts), "4. Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantContainerPorts))
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
							MountPath: apicommon.DogstatsdSocketLocalPath,
							ReadOnly:  false,
						},
					}
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, customVolumeMounts), "5. Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, customVolumeMounts))
					volumes := mgr.VolumeMgr.Volumes
					customVolumes := []corev1.Volume{
						{
							Name: apicommon.DogstatsdSocketVolumeName,
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: customVolumePath,
									Type: &volType,
								},
							},
						},
					}
					assert.True(t, apiutils.IsEqualStruct(volumes, customVolumes), "5. Volumes \ndiff = %s", cmp.Diff(volumes, customVolumes))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.AllContainers]
					customEnvVars := append([]*corev1.EnvVar{}, &customFilepathEnvVar)
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, customEnvVars), "5. Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, customEnvVars))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantContainerPorts), "5. Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantContainerPorts))
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
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantVolumeMountsV1), "6. Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantVolumeMountsV1))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumesV1), "6. Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumesV1))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, []*corev1.EnvVar{&originDetectionEnvVar}), "6. Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, []*corev1.EnvVar{&originDetectionEnvVar}))
					allEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.AllContainers]
					assert.True(t, apiutils.IsEqualStruct(allEnvVars, wantUDSEnvVarsV1), "6. All Containers envvars \ndiff = %s", cmp.Diff(agentEnvVars, wantUDSEnvVarsV2))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantContainerPorts), "6. Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantContainerPorts))
					assert.True(t, mgr.Tpl.Spec.HostPID, "6. Host PID \ndiff = %s", cmp.Diff(mgr.Tpl.Spec.HostPID, true))
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
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantVolumeMountsV1), "7. Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantVolumeMountsV1))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumesV1), "7. Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumesV1))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, []*corev1.EnvVar{&mapperProfilesEnvVar}), "7. Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, []*corev1.EnvVar{&mapperProfilesEnvVar}))
					allEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.AllContainers]
					assert.True(t, apiutils.IsEqualStruct(allEnvVars, wantUDSEnvVarsV1), "7. All Containers envvars \ndiff = %s", cmp.Diff(agentEnvVars, wantUDSEnvVarsV2))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantContainerPorts), "7. Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantContainerPorts))
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
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantVolumeMounts), "8. Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantVolumeMounts))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "8. Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, wantUDPEnvVars), "8. Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, wantUDPEnvVars))
					allEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.AllContainers]
					assert.True(t, apiutils.IsEqualStruct(allEnvVars, wantUDSEnvVarsV2), "8. All Containers envvars \ndiff = %s", cmp.Diff(agentEnvVars, wantUDSEnvVarsV2))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantHostPorts), "8. Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantHostPorts))
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
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantVolumeMounts), "9. Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantVolumeMounts))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "9. Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, wantCustomUDPEnvVars), "9. Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, wantUDPEnvVars))
					allEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.AllContainers]
					assert.True(t, apiutils.IsEqualStruct(allEnvVars, wantUDSEnvVarsV2), "9. All Containers envvars \ndiff = %s", cmp.Diff(agentEnvVars, wantUDSEnvVarsV2))

					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					customPorts := []*corev1.ContainerPort{
						{
							Name:          apicommon.DogstatsdHostPortName,
							HostPort:      1234,
							ContainerPort: apicommon.DogstatsdHostPortHostPort,
							Protocol:      corev1.ProtocolUDP,
						},
					}
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, customPorts), "9. Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, customPorts))
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
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantVolumeMounts), "10. Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantVolumeMounts))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "10. Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					customEnvVars := append(wantUDPEnvVars, &originDetectionEnvVar)
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, customEnvVars), "10. Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, customEnvVars))
					allEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.AllContainers]
					assert.True(t, apiutils.IsEqualStruct(allEnvVars, wantUDSEnvVarsV2), "10. All Containers envvars \ndiff = %s", cmp.Diff(agentEnvVars, wantUDSEnvVarsV2))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantHostPorts), "10. Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantHostPorts))
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
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, []*corev1.Volume(nil)), "11. Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, []*corev1.VolumeMount(nil)))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, []*corev1.Volume{}), "11. Volumes \ndiff = %s", cmp.Diff(volumes, []*corev1.Volume{}))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, nil), "11. Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, nil))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantContainerPorts), "11. Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantContainerPorts))
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
							MountPath: apicommon.DogstatsdSocketLocalPath,
							ReadOnly:  false,
						},
					}
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, customVolumeMounts), "12. Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, customVolumeMounts))
					volumes := mgr.VolumeMgr.Volumes
					customVolumes := []corev1.Volume{
						{
							Name: apicommon.DogstatsdSocketVolumeName,
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: customVolumePath,
									Type: &volType,
								},
							},
						},
					}
					assert.True(t, apiutils.IsEqualStruct(volumes, customVolumes), "12. Volumes \ndiff = %s", cmp.Diff(volumes, customVolumes))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.AllContainers]
					customEnvVars := append([]*corev1.EnvVar{}, customEnvVar...)
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, customEnvVars), "12. Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, customEnvVars))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantContainerPorts), "12. Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantContainerPorts))
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
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantVolumeMounts), "13. Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantVolumeMounts))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "13. Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, []*corev1.EnvVar{&originDetectionEnvVar}), "13. Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, []*corev1.EnvVar{&originDetectionEnvVar}))
					allEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.AllContainers]
					assert.True(t, apiutils.IsEqualStruct(allEnvVars, wantUDSEnvVarsV2), "13. All Containers envvars \ndiff = %s", cmp.Diff(allEnvVars, wantUDSEnvVarsV2))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantContainerPorts), "13. Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantContainerPorts))
					assert.True(t, mgr.Tpl.Spec.HostPID, "13. Host PID \ndiff = %s", cmp.Diff(mgr.Tpl.Spec.HostPID, true))
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
					assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantVolumeMounts), "14. Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantVolumeMounts))
					volumes := mgr.VolumeMgr.Volumes
					assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "14. Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(agentEnvVars, []*corev1.EnvVar{&mapperProfilesEnvVar}), "14. Agent Container envvars \ndiff = %s", cmp.Diff(agentEnvVars, []*corev1.EnvVar{&mapperProfilesEnvVar}))
					allEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.AllContainers]
					assert.True(t, apiutils.IsEqualStruct(allEnvVars, wantUDSEnvVarsV2), "14. All Containers envvars \ndiff = %s", cmp.Diff(allEnvVars, wantUDSEnvVarsV2))
					coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantContainerPorts), "14. Agent ports \ndiff = %s", cmp.Diff(coreAgentPorts, wantContainerPorts))
				},
			),
		},
	}

	tests.Run(t, buildDogstatsdFeature)
}
