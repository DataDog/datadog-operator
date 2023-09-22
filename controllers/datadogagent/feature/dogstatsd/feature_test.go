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
	// Identation matters!
	customMapperProfilesConf = `- name: 'profile_name'
  prefix: 'profile_prefix'
  mappings:
    - match: 'metric_to_match'
      name: 'mapped_metric_name'
`
	customMapperProfilesJSON = `[{"mappings":[{"match":"metric_to_match","name":"mapped_metric_name"}],"name":"profile_name","prefix":"profile_prefix"}]`
)

func Test_DogstatsdFeature_ConfigureV2(t *testing.T) {
	tests := test.FeatureTestSuite{
		{
			Name: "v2alpha1 dogstatsd udp hostport enabled",
			DDAv2: newDDABuilder().
				withHostPortEnabled(true).build(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					assertWants(t, mgrInterface, "9", getWantVolumeMounts(), getWantVolumes(), getWantUDPEnvVars(), getWantUDSEnvVarsV2(), getWantHostPorts())
				},
			),
		},
		{
			Name: "v2alpha1 udp custom host port",
			DDAv2: newDDABuilder().
				withHostPortEnabled(true).
				withHostPortConfig(1234).build(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
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

					customPorts := []*corev1.ContainerPort{
						{
							Name:          apicommon.DogstatsdHostPortName,
							HostPort:      1234,
							ContainerPort: apicommon.DogstatsdHostPortHostPort,
							Protocol:      corev1.ProtocolUDP,
						},
					}

					assertWants(t, mgrInterface, "9", getWantVolumeMounts(), getWantVolumes(), wantCustomUDPEnvVars, getWantUDSEnvVarsV2(), customPorts)

				},
			),
		},
		{
			Name: "v2alpha1 udp origin detection enabled",
			DDAv2: newDDABuilder().
				withHostPortEnabled(true).
				withOriginDetectionEnabled(true).build(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					customEnvVars := append(getWantUDPEnvVars(), getOriginDetectionEnvVar())
					assertWants(t, mgrInterface, "10", getWantVolumeMounts(), getWantVolumes(), customEnvVars, getWantUDSEnvVarsV2(), getWantHostPorts())
				},
			),
		},
		{
			Name: "v2alpha1 uds disabled",
			DDAv2: newDDABuilder().
				withUnixDomainSocketConfigEnabled(false).build(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					assertWants(t, mgrInterface, "11", []*corev1.VolumeMount(nil), []*corev1.Volume{}, nil, nil, getWantContainerPorts())
				},
			),
		},
		{
			Name: "v2alpha1 uds custom host filepath",
			DDAv2: newDDABuilder().
				withUnixDomainSocketConfigPath(customPath).build(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					customVolumeMounts := []*corev1.VolumeMount{
						{
							Name:      apicommon.DogstatsdSocketVolumeName,
							MountPath: apicommon.DogstatsdSocketLocalPath,
							ReadOnly:  false,
						},
					}
					customVolumes := []*corev1.Volume{
						{
							Name: apicommon.DogstatsdSocketVolumeName,
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: customVolumePath,
									Type: getVolType(),
								},
							},
						},
					}
					customEnvVars := append([]*corev1.EnvVar{}, getCustomEnvVar()...)

					assertWants(t, mgrInterface, "12", customVolumeMounts, customVolumes, nil, customEnvVars, getWantContainerPorts())
				},
			),
		},
		{
			Name: "v2alpha1 uds origin detection",
			DDAv2: newDDABuilder().
				withOriginDetectionEnabled(true).build(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					assert.True(t, mgr.Tpl.Spec.HostPID, "13. Host PID \ndiff = %s", cmp.Diff(mgr.Tpl.Spec.HostPID, true))
					assertWants(t, mgrInterface, "13", getWantVolumeMounts(), getWantVolumes(), []*corev1.EnvVar{getOriginDetectionEnvVar()}, getWantUDSEnvVarsV2(), getWantContainerPorts())
				},
			),
		},
		{
			Name: "v2alpha1 mapper profiles",
			DDAv2: newDDABuilder().
				withMapperProfiles(customMapperProfilesConf).build(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					// mapper profiles envvar
					mapperProfilesEnvVar := corev1.EnvVar{
						Name:  apicommon.DDDogstatsdMapperProfiles,
						Value: customMapperProfilesJSON,
					}

					assertWants(t, mgrInterface, "14", getWantVolumeMounts(), getWantVolumes(), []*corev1.EnvVar{&mapperProfilesEnvVar}, getWantUDSEnvVarsV2(), getWantContainerPorts())
				},
			),
		},
	}

	tests.Run(t, buildDogstatsdFeature)
}

func getVolType() *corev1.HostPathType {
	volType := corev1.HostPathDirectoryOrCreate
	return &volType
}

func getWantHostPorts() []*corev1.ContainerPort {
	wantHostPorts := []*corev1.ContainerPort{
		{
			Name:          apicommon.DogstatsdHostPortName,
			HostPort:      apicommon.DogstatsdHostPortHostPort,
			ContainerPort: apicommon.DogstatsdHostPortHostPort,
			Protocol:      corev1.ProtocolUDP,
		},
	}
	return wantHostPorts
}

func getWantContainerPorts() []*corev1.ContainerPort {
	wantContainerPorts := []*corev1.ContainerPort{
		{
			Name:          apicommon.DogstatsdHostPortName,
			ContainerPort: apicommon.DogstatsdHostPortHostPort,
			Protocol:      corev1.ProtocolUDP,
		},
	}
	return wantContainerPorts
}

func getOriginDetectionEnvVar() *corev1.EnvVar {
	originDetectionEnvVar := corev1.EnvVar{
		Name:  apicommon.DDDogstatsdOriginDetection,
		Value: "true",
	}
	return &originDetectionEnvVar
}

func getCustomEnvVar() []*corev1.EnvVar {
	customEnvVar := []*corev1.EnvVar{
		{
			Name:  apicommon.DDDogstatsdSocket,
			Value: apicommon.DogstatsdSocketLocalPath + "/" + customSock,
		},
	}
	return customEnvVar
}

func getWantUDSEnvVarsV2() []*corev1.EnvVar {
	wantUDSEnvVarsV2 := []*corev1.EnvVar{
		{
			Name:  apicommon.DDDogstatsdSocket,
			Value: apicommon.DogstatsdSocketLocalPath + "/" + apicommon.DogstatsdSocketName,
		},
	}
	return wantUDSEnvVarsV2
}

func getWantUDPEnvVars() []*corev1.EnvVar {
	wantUDPEnvVars := []*corev1.EnvVar{
		{
			Name:  apicommon.DDDogstatsdPort,
			Value: strconv.Itoa(apicommon.DefaultDogstatsdPort),
		},
		{
			Name:  apicommon.DDDogstatsdNonLocalTraffic,
			Value: "true",
		},
	}
	return wantUDPEnvVars
}

func getWantVolumes() []*corev1.Volume {
	volType := corev1.HostPathDirectoryOrCreate
	wantVolumes := []*corev1.Volume{
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
	return wantVolumes
}

func getWantVolumeMounts() []*corev1.VolumeMount {
	wantVolumeMounts := []*corev1.VolumeMount{
		{
			Name:      apicommon.DogstatsdSocketVolumeName,
			MountPath: apicommon.DogstatsdSocketLocalPath,
			ReadOnly:  false,
		},
	}
	return wantVolumeMounts
}

func assertWants(t testing.TB, mgrInterface feature.PodTemplateManagers, testId string, wantVolumeMounts []*corev1.VolumeMount, wantVolumes []*corev1.Volume, wantEnvVars []*corev1.EnvVar, wantUDSEnvVarsV2 []*corev1.EnvVar, wantContainerPorts []*corev1.ContainerPort) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)

	coreAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.CoreAgentContainerName]
	assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantVolumeMounts), "%s. Volume mounts \ndiff = %s", testId, cmp.Diff(coreAgentVolumeMounts, wantVolumeMounts))

	volumes := mgr.VolumeMgr.Volumes
	assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "%s. Volumes \ndiff = %s", testId, cmp.Diff(volumes, wantVolumes))

	agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
	assert.True(t, apiutils.IsEqualStruct(agentEnvVars, wantEnvVars), "%s. Agent Container envvars \ndiff = %s", testId, cmp.Diff(agentEnvVars, wantEnvVars))

	allEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.AllContainers]
	assert.True(t, apiutils.IsEqualStruct(allEnvVars, wantUDSEnvVarsV2), "%s. All Containers envvars \ndiff = %s", testId, cmp.Diff(allEnvVars, wantUDSEnvVarsV2))

	coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
	assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantContainerPorts), "%s. Agent ports \ndiff = %s", testId, cmp.Diff(coreAgentPorts, wantContainerPorts))
}

type DatadogAgentBuilder struct {
	datadogAgent v2alpha1.DatadogAgent
}

func newDDABuilder() *DatadogAgentBuilder {
	dda := &v2alpha1.DatadogAgent{}
	v2alpha1.DefaultDatadogAgent(dda)

	return &DatadogAgentBuilder{
		datadogAgent: *dda,
	}
}

func (builder *DatadogAgentBuilder) withHostPortEnabled(enabled bool) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Features.Dogstatsd.HostPortConfig.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) withHostPortConfig(port int32) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Features.Dogstatsd.HostPortConfig.Port = apiutils.NewInt32Pointer(1234)
	return builder
}

func (builder *DatadogAgentBuilder) withOriginDetectionEnabled(enabled bool) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Features.Dogstatsd.OriginDetectionEnabled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) withUnixDomainSocketConfigEnabled(enabled bool) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Features.Dogstatsd.UnixDomainSocketConfig.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) withUnixDomainSocketConfigPath(customPath string) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Features.Dogstatsd.UnixDomainSocketConfig.Path = apiutils.NewStringPointer(customPath)
	return builder
}

func (builder *DatadogAgentBuilder) withMapperProfiles(customMapperProfilesConf string) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Features.Dogstatsd.MapperProfiles = &v2alpha1.CustomConfig{ConfigData: apiutils.NewStringPointer(customMapperProfilesConf)}
	return builder
}

func (builder *DatadogAgentBuilder) build() *v2alpha1.DatadogAgent {
	v2alpha1.DefaultDatadogAgent(&builder.datadogAgent)
	return &builder.datadogAgent
}
