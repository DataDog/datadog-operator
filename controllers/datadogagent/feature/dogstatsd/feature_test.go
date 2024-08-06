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
	v2alpha1test "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1/test"
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

func Test_DogstatsdFeature_Configure(t *testing.T) {
	tests := test.FeatureTestSuite{
		{
			Name: "dogstatsd udp hostport enabled",
			DDA: v2alpha1test.NewDefaultDatadogAgentBuilder().
				WithDogstatsdHostPortEnabled(true).BuildWithDefaults(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					assertWants(t, mgrInterface, "9", getWantVolumeMounts(), getWantVolumes(), getWantUDPEnvVars(), getWantUDSEnvVars(), getWantHostPorts())
				},
			),
		},
		{
			Name: "udp host network",
			DDA: v2alpha1test.NewDefaultDatadogAgentBuilder().
				WithDogstatsdHostPortEnabled(true).
				WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{
					HostNetwork: apiutils.NewBoolPointer(true),
				}).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					// custom udp envvar
					wantCustomUDPEnvVars := []*corev1.EnvVar{
						{
							Name:  apicommon.DDDogstatsdPort,
							Value: "8125",
						},
						{
							Name:  apicommon.DDDogstatsdNonLocalTraffic,
							Value: "true",
						},
					}

					customPorts := []*corev1.ContainerPort{
						{
							Name:          apicommon.DogstatsdHostPortName,
							HostPort:      8125,
							ContainerPort: 8125,
							Protocol:      corev1.ProtocolUDP,
						},
					}

					assertWants(t, mgrInterface, "9", getWantVolumeMounts(), getWantVolumes(), wantCustomUDPEnvVars, getWantUDSEnvVars(), customPorts)

				},
			),
		},
		{
			Name: "udp host network custom host port",
			DDA: v2alpha1test.NewDefaultDatadogAgentBuilder().
				WithDogstatsdHostPortEnabled(true).
				WithDogstatsdHostPortConfig(1234).
				WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{
					HostNetwork: apiutils.NewBoolPointer(true),
				}).
				BuildWithDefaults(),
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
							ContainerPort: 1234,
							Protocol:      corev1.ProtocolUDP,
						},
					}

					assertWants(t, mgrInterface, "9", getWantVolumeMounts(), getWantVolumes(), wantCustomUDPEnvVars, getWantUDSEnvVars(), customPorts)

				},
			),
		},
		{
			Name: "udp custom host port",
			DDA: v2alpha1test.NewDefaultDatadogAgentBuilder().
				WithDogstatsdHostPortEnabled(true).
				WithDogstatsdHostPortConfig(1234).BuildWithDefaults(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					// custom udp envvar
					wantCustomUDPEnvVars := []*corev1.EnvVar{
						{
							Name:  apicommon.DDDogstatsdPort,
							Value: "8125",
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

					assertWants(t, mgrInterface, "9", getWantVolumeMounts(), getWantVolumes(), wantCustomUDPEnvVars, getWantUDSEnvVars(), customPorts)

				},
			),
		},
		{
			Name: "udp host port enabled no custom host port",
			DDA: v2alpha1test.NewDefaultDatadogAgentBuilder().
				WithDogstatsdHostPortEnabled(true).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					// custom udp envvar
					wantCustomUDPEnvVars := []*corev1.EnvVar{
						{
							Name:  apicommon.DDDogstatsdPort,
							Value: "8125",
						},
						{
							Name:  apicommon.DDDogstatsdNonLocalTraffic,
							Value: "true",
						},
					}

					customPorts := []*corev1.ContainerPort{
						{
							Name:          apicommon.DogstatsdHostPortName,
							HostPort:      8125,
							ContainerPort: apicommon.DefaultDogstatsdPort,
							Protocol:      corev1.ProtocolUDP,
						},
					}

					assertWants(t, mgrInterface, "9", getWantVolumeMounts(), getWantVolumes(), wantCustomUDPEnvVars, getWantUDSEnvVars(), customPorts)

				},
			),
		},
		{
			Name: "udp origin detection enabled",
			DDA: v2alpha1test.NewDefaultDatadogAgentBuilder().
				WithDogstatsdHostPortEnabled(true).
				WithDogstatsdOriginDetectionEnabled(true).BuildWithDefaults(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					customEnvVars := append(getWantUDPEnvVars(), getOriginDetectionEnvVar(), getOriginDetectionClientEnvVar())
					assertWants(t, mgrInterface, "10", getWantVolumeMounts(), getWantVolumes(), customEnvVars, getWantUDSEnvVars(), getWantHostPorts())
				},
			),
		},
		{
			Name: "uds disabled",
			DDA: v2alpha1test.NewDefaultDatadogAgentBuilder().
				WithDogstatsdUnixDomainSocketConfigEnabled(false).BuildWithDefaults(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					assertWants(t, mgrInterface, "11", []*corev1.VolumeMount(nil), []*corev1.Volume{}, nil, nil, getWantContainerPorts())
				},
			),
		},
		{
			Name: "uds custom host filepath",
			DDA: v2alpha1test.NewDefaultDatadogAgentBuilder().
				WithDogstatsdUnixDomainSocketConfigPath(customPath).BuildWithDefaults(),
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
			Name: "uds origin detection",
			DDA: v2alpha1test.NewDefaultDatadogAgentBuilder().
				WithDogstatsdOriginDetectionEnabled(true).BuildWithDefaults(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					assert.True(t, mgr.Tpl.Spec.HostPID, "13. Host PID \ndiff = %s", cmp.Diff(mgr.Tpl.Spec.HostPID, true))
					assertWants(t, mgrInterface, "13", getWantVolumeMounts(), getWantVolumes(), []*corev1.EnvVar{getOriginDetectionEnvVar(), getOriginDetectionClientEnvVar()}, getWantUDSEnvVars(), getWantContainerPorts())
				},
			),
		},
		{
			Name: "mapper profiles",
			DDA: v2alpha1test.NewDefaultDatadogAgentBuilder().
				WithDogstatsdMapperProfiles(customMapperProfilesConf).BuildWithDefaults(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					// mapper profiles envvar
					mapperProfilesEnvVar := corev1.EnvVar{
						Name:  apicommon.DDDogstatsdMapperProfiles,
						Value: customMapperProfilesJSON,
					}

					assertWants(t, mgrInterface, "14", getWantVolumeMounts(), getWantVolumes(), []*corev1.EnvVar{&mapperProfilesEnvVar}, getWantUDSEnvVars(), getWantContainerPorts())
				},
			),
		},
		{
			Name: "udp origin detection enabled, orchestrator tag cardinality",
			DDA: v2alpha1test.NewDefaultDatadogAgentBuilder().
				WithDogstatsdHostPortEnabled(true).
				WithDogstatsdTagCardinality("orchestrator").BuildWithDefaults(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					wantTagCardinalityEnvVar := corev1.EnvVar{
						Name:  apicommon.DDDogstatsdTagCardinality,
						Value: "orchestrator",
					}
					customEnvVars := append(getWantUDPEnvVars(), getOriginDetectionEnvVar(), getOriginDetectionClientEnvVar(), &wantTagCardinalityEnvVar)
					assertWants(t, mgrInterface, "15", getWantVolumeMounts(), getWantVolumes(), customEnvVars, getWantUDSEnvVars(), getWantHostPorts())
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

func getOriginDetectionClientEnvVar() *corev1.EnvVar {
	originDetectionClientEnvVar := corev1.EnvVar{
		Name:  apicommon.DDDogstatsdOriginDetectionClient,
		Value: "true",
	}
	return &originDetectionClientEnvVar
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

func getWantUDSEnvVars() []*corev1.EnvVar {
	wantUDSEnvVars := []*corev1.EnvVar{
		{
			Name:  apicommon.DDDogstatsdSocket,
			Value: apicommon.DogstatsdSocketLocalPath + "/" + apicommon.DogstatsdSocketName,
		},
	}
	return wantUDSEnvVars
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

func assertWants(t testing.TB, mgrInterface feature.PodTemplateManagers, testId string, wantVolumeMounts []*corev1.VolumeMount, wantVolumes []*corev1.Volume, wantEnvVars []*corev1.EnvVar, wantUDSEnvVars []*corev1.EnvVar, wantContainerPorts []*corev1.ContainerPort) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)

	coreAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.CoreAgentContainerName]
	assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantVolumeMounts), "%s. Volume mounts \ndiff = %s", testId, cmp.Diff(coreAgentVolumeMounts, wantVolumeMounts))

	volumes := mgr.VolumeMgr.Volumes
	assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "%s. Volumes \ndiff = %s", testId, cmp.Diff(volumes, wantVolumes))

	agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
	assert.True(t, apiutils.IsEqualStruct(agentEnvVars, wantEnvVars), "%s. Agent Container envvars \ndiff = %s", testId, cmp.Diff(agentEnvVars, wantEnvVars))

	allEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.AllContainers]
	assert.True(t, apiutils.IsEqualStruct(allEnvVars, wantUDSEnvVars), "%s. All Containers envvars \ndiff = %s", testId, cmp.Diff(allEnvVars, wantUDSEnvVars))

	coreAgentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
	assert.True(t, apiutils.IsEqualStruct(coreAgentPorts, wantContainerPorts), "%s. Agent ports \ndiff = %s", testId, cmp.Diff(coreAgentPorts, wantContainerPorts))
}
