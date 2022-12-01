// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apm

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
	apmSocketHostPath = "/var/run/datadog"
	apmSocketPath     = "/var/run/datadog/apm.sock"
)

func TestAPMFeature(t *testing.T) {
	tests := test.FeatureTestSuite{
		//////////////////////////
		// v1Alpha1.DatadogAgent
		//////////////////////////
		{
			Name:          "v1alpha1 apm not enabled",
			DDAv1:         newV1Agent(false, false),
			WantConfigure: false,
		},
		{
			Name:          "v1alpha1 apm enabled, use hostport",
			DDAv1:         newV1Agent(true, false),
			WantConfigure: true,
			Agent:         testAgentHostPortOnly(),
		},
		{
			Name:          "v1alpha1 apm enabled, use uds and hostport",
			DDAv1:         newV1Agent(true, true),
			WantConfigure: true,
			Agent:         testAgentHostPortUDS(),
		},

		//////////////////////////
		// v2Alpha1.DatadogAgent
		//////////////////////////
		{
			Name:          "v2alpha1 apm not enabled",
			DDAv2:         newV2Agent(false, false),
			WantConfigure: false,
		},
		{
			Name:          "v2alpha1 apm enabled, use uds",
			DDAv2:         newV2Agent(true, false),
			WantConfigure: true,
			Agent:         testAgentUDSOnly(),
		},
		{
			Name:          "v2alpha1 apm enabled, use uds and host port",
			DDAv2:         newV2Agent(true, true),
			WantConfigure: true,
			Agent:         testAgentHostPortUDS(),
		},
	}

	tests.Run(t, buildAPMFeature)
}

func newV1Agent(enableAPM bool, uds bool) *v1alpha1.DatadogAgent {
	return &v1alpha1.DatadogAgent{
		Spec: v1alpha1.DatadogAgentSpec{
			Agent: v1alpha1.DatadogAgentSpecAgentSpec{
				Apm: &v1alpha1.APMSpec{
					Enabled:  apiutils.NewBoolPointer(enableAPM),
					HostPort: apiutils.NewInt32Pointer(8126),
					UnixDomainSocket: &v1alpha1.APMUnixDomainSocketSpec{
						Enabled:      apiutils.NewBoolPointer(uds),
						HostFilepath: apiutils.NewStringPointer(apmSocketPath),
					},
				},
			},
		},
	}
}

func newV2Agent(enableAPM bool, hostPort bool) *v2alpha1.DatadogAgent {
	return &v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				APM: &v2alpha1.APMFeatureConfig{
					Enabled: apiutils.NewBoolPointer(enableAPM),
					HostPortConfig: &v2alpha1.HostPortConfig{
						Enabled: apiutils.NewBoolPointer(hostPort),
						Port:    apiutils.NewInt32Pointer(8126),
					},
					UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
						Enabled: apiutils.NewBoolPointer(true),
						Path:    apiutils.NewStringPointer(apmSocketPath),
					},
				},
			},
			Global: &v2alpha1.GlobalConfig{},
		},
	}
}

func testAgentHostPortOnly() *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.TraceAgentContainerName]
			expectedAgentEnvs := []*corev1.EnvVar{
				{
					Name:  apicommon.DDAPMEnabled,
					Value: "true",
				},
				{
					Name:  apicommon.DDAPMReceiverPort,
					Value: "8126",
				},
				{
					Name:  apicommon.DDAPMNonLocalTraffic,
					Value: "true",
				},
			}
			assert.True(
				t,
				apiutils.IsEqualStruct(agentEnvs, expectedAgentEnvs),
				"Trace Agent ENVs \ndiff = %s", cmp.Diff(agentEnvs, expectedAgentEnvs),
			)

			agentPorts := mgr.PortMgr.PortsByC[apicommonv1.TraceAgentContainerName]
			expectedPorts := []*corev1.ContainerPort{
				{
					Name:          "traceport",
					ContainerPort: 8126,
					HostPort:      8126,
					Protocol:      corev1.ProtocolTCP,
				},
			}
			assert.True(
				t,
				apiutils.IsEqualStruct(agentPorts, expectedPorts),
				"Trace Agent Ports \ndiff = %s", cmp.Diff(agentPorts, expectedPorts),
			)
		},
	)
}

func testAgentUDSOnly() *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.TraceAgentContainerName]
			expectedAgentEnvs := []*corev1.EnvVar{
				{
					Name:  apicommon.DDAPMEnabled,
					Value: "true",
				},
				{
					Name:  apicommon.DDAPMReceiverSocket,
					Value: apmSocketPath,
				},
			}
			assert.True(
				t,
				apiutils.IsEqualStruct(agentEnvs, expectedAgentEnvs),
				"Trace Agent ENVs \ndiff = %s", cmp.Diff(agentEnvs, expectedAgentEnvs),
			)

			agentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.TraceAgentContainerName]
			expectedVolumeMounts := []*corev1.VolumeMount{
				{
					Name:      apicommon.DogstatsdAPMSocketVolumeName,
					MountPath: apmSocketHostPath,
					ReadOnly:  false,
				},
			}
			assert.True(
				t,
				apiutils.IsEqualStruct(agentVolumeMounts, expectedVolumeMounts),
				"Trace Agent VolumeMounts \ndiff = %s", cmp.Diff(agentVolumeMounts, expectedVolumeMounts),
			)

			agentVolumes := mgr.VolumeMgr.Volumes
			expectedVolumes := []*corev1.Volume{
				{
					Name: apicommon.DogstatsdAPMSocketVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: apmSocketHostPath,
						},
					},
				},
			}
			assert.True(
				t,
				apiutils.IsEqualStruct(agentVolumes, expectedVolumes),
				"Trace Agent Volumes \ndiff = %s", cmp.Diff(agentVolumes, expectedVolumes),
			)

			agentPorts := mgr.PortMgr.PortsByC[apicommonv1.TraceAgentContainerName]
			expectedPorts := []*corev1.ContainerPort{
				{
					Name:          "traceport",
					ContainerPort: 8126,
					Protocol:      corev1.ProtocolTCP,
				},
			}
			assert.True(
				t,
				apiutils.IsEqualStruct(agentPorts, expectedPorts),
				"Trace Agent Ports \ndiff = %s", cmp.Diff(agentPorts, expectedPorts),
			)
		},
	)
}

func testAgentHostPortUDS() *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.TraceAgentContainerName]
			expectedAgentEnvs := []*corev1.EnvVar{
				{
					Name:  apicommon.DDAPMEnabled,
					Value: "true",
				},
				{
					Name:  apicommon.DDAPMReceiverPort,
					Value: "8126",
				},
				{
					Name:  apicommon.DDAPMNonLocalTraffic,
					Value: "true",
				},
				{
					Name:  apicommon.DDAPMReceiverSocket,
					Value: apmSocketPath,
				},
			}
			assert.True(
				t,
				apiutils.IsEqualStruct(agentEnvs, expectedAgentEnvs),
				"Trace Agent ENVs \ndiff = %s", cmp.Diff(agentEnvs, expectedAgentEnvs),
			)

			agentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.TraceAgentContainerName]
			expectedVolumeMounts := []*corev1.VolumeMount{
				{
					Name:      apicommon.DogstatsdAPMSocketVolumeName,
					MountPath: apmSocketHostPath,
					ReadOnly:  false,
				},
			}
			assert.True(
				t,
				apiutils.IsEqualStruct(agentVolumeMounts, expectedVolumeMounts),
				"Trace Agent VolumeMounts \ndiff = %s", cmp.Diff(agentVolumeMounts, expectedVolumeMounts),
			)

			agentVolumes := mgr.VolumeMgr.Volumes
			expectedVolumes := []*corev1.Volume{
				{
					Name: apicommon.DogstatsdAPMSocketVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: apmSocketHostPath,
						},
					},
				},
			}
			assert.True(
				t,
				apiutils.IsEqualStruct(agentVolumes, expectedVolumes),
				"Trace Agent Volumes \ndiff = %s", cmp.Diff(agentVolumes, expectedVolumes),
			)

			agentPorts := mgr.PortMgr.PortsByC[apicommonv1.TraceAgentContainerName]
			expectedPorts := []*corev1.ContainerPort{
				{
					Name:          "traceport",
					ContainerPort: 8126,
					HostPort:      8126,
					Protocol:      corev1.ProtocolTCP,
				},
			}
			assert.True(
				t,
				apiutils.IsEqualStruct(agentPorts, expectedPorts),
				"Trace Agent Ports \ndiff = %s", cmp.Diff(agentPorts, expectedPorts),
			)
		},
	)
}
