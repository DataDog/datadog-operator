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
	apmSocketHostPath  = apicommon.DogstatsdAPMSocketHostPath + "/" + apicommon.APMSocketName
	apmSocketLocalPath = apicommon.APMSocketVolumeLocalPath + "/" + apicommon.APMSocketName
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
			Agent:         testAgentHostPortUDS(apicommonv1.TraceAgentContainerName),
		},

		//////////////////////////
		// v2Alpha1.DatadogAgent
		//////////////////////////
		{
			Name: "v2alpha1 apm not enabled",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAPMEnabled(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "v2alpha1 apm not enabled with single container strategy",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAPMEnabled(false).
				WithSingleContainerStrategy(true).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "v2alpha1 apm enabled, use uds",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(false, 8126).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				Build(),
			WantConfigure: true,
			Agent:         testAgentUDSOnly(apicommonv1.TraceAgentContainerName),
		},
		{
			Name: "v2alpha1 apm enabled, use uds with single container strategy",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(false, 8126).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				WithSingleContainerStrategy(true).
				Build(),
			WantConfigure: true,
			Agent:         testAgentUDSOnly(apicommonv1.UnprivilegedSingleAgentContainerName),
		},
		{
			Name: "v2alpha1 apm enabled, use uds and host port",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(true, 8126).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				Build(),
			WantConfigure: true,
			Agent:         testAgentHostPortUDS(apicommonv1.TraceAgentContainerName),
		},
		{
			Name: "v2alpha1 apm enabled, use uds and host port with single container strategy",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(true, 8126).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				WithSingleContainerStrategy(true).
				Build(),
			WantConfigure: true,
			Agent:         testAgentHostPortUDS(apicommonv1.UnprivilegedSingleAgentContainerName),
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
						HostFilepath: apiutils.NewStringPointer(apmSocketHostPath),
					},
				},
			},
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

func testAgentUDSOnly(agentContainerName apicommonv1.AgentContainerName) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[agentContainerName]
			expectedAgentEnvs := []*corev1.EnvVar{
				{
					Name:  apicommon.DDAPMEnabled,
					Value: "true",
				},
				{
					Name:  apicommon.DDAPMReceiverSocket,
					Value: apmSocketLocalPath,
				},
			}
			assert.True(
				t,
				apiutils.IsEqualStruct(agentEnvs, expectedAgentEnvs),
				"Trace Agent ENVs \ndiff = %s", cmp.Diff(agentEnvs, expectedAgentEnvs),
			)

			agentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[agentContainerName]
			expectedVolumeMounts := []*corev1.VolumeMount{
				{
					Name:      apicommon.APMSocketVolumeName,
					MountPath: apicommon.APMSocketVolumeLocalPath,
					ReadOnly:  false,
				},
			}
			assert.True(
				t,
				apiutils.IsEqualStruct(agentVolumeMounts, expectedVolumeMounts),
				"Trace Agent VolumeMounts \ndiff = %s", cmp.Diff(agentVolumeMounts, expectedVolumeMounts),
			)

			agentVolumes := mgr.VolumeMgr.Volumes
			volType := corev1.HostPathDirectoryOrCreate
			expectedVolumes := []*corev1.Volume{
				{
					Name: apicommon.APMSocketVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: apicommon.DogstatsdAPMSocketHostPath,
							Type: &volType,
						},
					},
				},
			}
			assert.True(
				t,
				apiutils.IsEqualStruct(agentVolumes, expectedVolumes),
				"Trace Agent Volumes \ndiff = %s", cmp.Diff(agentVolumes, expectedVolumes),
			)

			agentPorts := mgr.PortMgr.PortsByC[agentContainerName]
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

func testAgentHostPortUDS(agentContainerName apicommonv1.AgentContainerName) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[agentContainerName]
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
					Value: apmSocketLocalPath,
				},
			}
			assert.True(
				t,
				apiutils.IsEqualStruct(agentEnvs, expectedAgentEnvs),
				"Trace Agent ENVs \ndiff = %s", cmp.Diff(agentEnvs, expectedAgentEnvs),
			)

			agentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[agentContainerName]
			expectedVolumeMounts := []*corev1.VolumeMount{
				{
					Name:      apicommon.APMSocketVolumeName,
					MountPath: apicommon.APMSocketVolumeLocalPath,
					ReadOnly:  false,
				},
			}
			assert.True(
				t,
				apiutils.IsEqualStruct(agentVolumeMounts, expectedVolumeMounts),
				"Trace Agent VolumeMounts \ndiff = %s", cmp.Diff(agentVolumeMounts, expectedVolumeMounts),
			)

			agentVolumes := mgr.VolumeMgr.Volumes
			volType := corev1.HostPathDirectoryOrCreate
			expectedVolumes := []*corev1.Volume{
				{
					Name: apicommon.APMSocketVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: apicommon.DogstatsdAPMSocketHostPath,
							Type: &volType,
						},
					},
				},
			}
			assert.True(
				t,
				apiutils.IsEqualStruct(agentVolumes, expectedVolumes),
				"Trace Agent Volumes \ndiff = %s", cmp.Diff(agentVolumes, expectedVolumes),
			)

			agentPorts := mgr.PortMgr.PortsByC[agentContainerName]
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
