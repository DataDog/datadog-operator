// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apm

import (
	"strconv"
	"testing"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
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
	apmSocketHostPath  = apicommon.DogstatsdAPMSocketHostPath + "/" + apicommon.APMSocketName
	apmSocketLocalPath = apicommon.APMSocketVolumeLocalPath + "/" + apicommon.APMSocketName
)

func TestShouldEnableAPM(t *testing.T) {
	tests := []struct {
		name    string
		dda     *v2alpha1.DatadogAgent
		enabled bool
	}{
		{
			// Note that this should not happen since APM is defaulted.
			// This test is just to unitest the function.
			name: "APM nil, SSI nil, all disabled",
			dda: &v2alpha1.DatadogAgent{
				Spec: v2alpha1.DatadogAgentSpec{
					Features: &v2alpha1.DatadogFeatures{
						APM: &v2alpha1.APMFeatureConfig{},
					},
				},
			},
			enabled: false,
		},
		{
			name: "APM false, SSI true, APM and SSI disabled",
			dda: &v2alpha1.DatadogAgent{
				Spec: v2alpha1.DatadogAgentSpec{
					Features: &v2alpha1.DatadogFeatures{
						APM: &v2alpha1.APMFeatureConfig{
							Enabled: apiutils.NewBoolPointer(false),
							SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
								Enabled: apiutils.NewBoolPointer(true),
							},
						},
					},
				},
			},
			enabled: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isEnabled := shouldEnableAPM(tt.dda.Spec.Features.APM)
			assert.Equal(t, tt.enabled, isEnabled)
		})
	}
}

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
			Agent:         testAgentHostPortUDS(apicommonv1.TraceAgentContainerName, 8126, false),
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
				WithAPMHostPortEnabled(false, apiutils.NewInt32Pointer(8126)).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				Build(),
			WantConfigure: true,
			Agent:         testAgentUDSOnly(apicommonv1.TraceAgentContainerName),
		},
		{
			Name: "v2alpha1 apm enabled, use uds with single container strategy",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(false, apiutils.NewInt32Pointer(8126)).
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
				WithAPMHostPortEnabled(true, apiutils.NewInt32Pointer(8126)).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent:         testAgentHostPortUDS(apicommonv1.TraceAgentContainerName, 8126, false),
		},
		{
			Name: "v2alpha1 apm enabled, use uds and host port with single container strategy",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(true, apiutils.NewInt32Pointer(8126)).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				WithSingleContainerStrategy(true).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent:         testAgentHostPortUDS(apicommonv1.UnprivilegedSingleAgentContainerName, 8126, false),
		},
		{
			Name: "v2alpha1 apm enabled, use uds and custom host port",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(true, apiutils.NewInt32Pointer(1234)).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent:         testAgentHostPortUDS(apicommonv1.TraceAgentContainerName, 1234, false),
		},
		{
			Name: "v2alpha1 apm enabled, use uds and custom host port with single container strategy",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(true, apiutils.NewInt32Pointer(1234)).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				WithSingleContainerStrategy(true).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent:         testAgentHostPortUDS(apicommonv1.UnprivilegedSingleAgentContainerName, 1234, false),
		},
		{
			Name: "v2alpha1 apm enabled, use uds and host port enabled but no custom host port",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(true, nil).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent:         testAgentHostPortUDS(apicommonv1.TraceAgentContainerName, 8126, false),
		},
		{
			Name: "v2alpha1 apm enabled, use uds and host port enabled but no custom host port with single container strategy",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(true, nil).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				WithSingleContainerStrategy(true).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent:         testAgentHostPortUDS(apicommonv1.UnprivilegedSingleAgentContainerName, 8126, false),
		},
		{
			Name: "v2alpha1 apm enabled, host port enabled host network",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(true, nil).
				WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{
					HostNetwork: apiutils.NewBoolPointer(true),
				}).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent:         testAgentHostPortUDS(apicommonv1.TraceAgentContainerName, 8126, true),
		},
		{
			Name: "v2alpha1 apm enabled, host port enabled host network with single container strategy",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(true, nil).
				WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{
					HostNetwork: apiutils.NewBoolPointer(true),
				}).
				WithSingleContainerStrategy(true).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent:         testAgentHostPortUDS(apicommonv1.UnprivilegedSingleAgentContainerName, 8126, true),
		},
		{
			Name: "v2alpha1 apm enabled, custom host port host network",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(true, apiutils.NewInt32Pointer(1234)).
				WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{
					HostNetwork: apiutils.NewBoolPointer(true),
				}).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent:         testAgentHostPortUDS(apicommonv1.TraceAgentContainerName, 1234, true),
		},
		{
			Name: "v2alpha1 apm enabled, custom host port host network with single container strategy",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(true, apiutils.NewInt32Pointer(1234)).
				WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{
					HostNetwork: apiutils.NewBoolPointer(true),
				}).
				WithSingleContainerStrategy(true).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent:         testAgentHostPortUDS(apicommonv1.UnprivilegedSingleAgentContainerName, 1234, true),
		},
		{
			Name: "v2alpha1 basic apm single step instrumentation",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(true, apiutils.NewInt32Pointer(8126)).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				WithAdmissionControllerEnabled(true).
				WithAPMSingleStepInstrumentationEnabled(true, nil, nil, nil).
				WithSingleContainerStrategy(false).
				Build(),
			WantConfigure: true,
			ClusterAgent:  testAPMInstrumentation(),
		},
		{
			Name: "v2alpha1 error apm single step instrumentation",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(true, apiutils.NewInt32Pointer(8126)).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				WithAdmissionControllerEnabled(true).
				WithAPMSingleStepInstrumentationEnabled(true,
					nil,
					[]string{"foo", "bar"},
					map[string]string{
						"java": "1.2.4",
					}).
				WithSingleContainerStrategy(false).
				Build(),
			WantConfigure: true,
			ClusterAgent:  testAPMInstrumentationFull(),
		},
		{
			Name: "v2alpha1 step instrumentation precedence",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAPMEnabled(false).
				WithAPMHostPortEnabled(true, apiutils.NewInt32Pointer(8126)).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				WithAPMSingleStepInstrumentationEnabled(true, nil, nil, nil).
				WithAdmissionControllerEnabled(true).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "v2alpha1 step instrumentation w/o AC",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(true, apiutils.NewInt32Pointer(8126)).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				WithAPMSingleStepInstrumentationEnabled(true, nil, nil, nil).
				WithAdmissionControllerEnabled(false).
				WithSingleContainerStrategy(false).
				Build(),
			WantConfigure: true,
			Agent:         testTraceAgentEnabled(apicommonv1.TraceAgentContainerName),
			ClusterAgent:  testAPMInstrumentationDisabledWithAC(),
		},
		{
			Name: "v2alpha1 single step instrumentation namespace specific",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(true, apiutils.NewInt32Pointer(8126)).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				WithAPMSingleStepInstrumentationEnabled(false, []string{"foo", "bar"}, nil, map[string]string{"java": "1.2.4"}).
				WithAdmissionControllerEnabled(true).
				Build(),
			WantConfigure: true,
			ClusterAgent:  testAPMInstrumentationNamespaces(),
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

func testTraceAgentEnabled(containerName apicommonv1.AgentContainerName) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)
			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[containerName]
			expectedAgentEnvs := []*corev1.EnvVar{
				{
					Name:  apicommon.DDAPMEnabled,
					Value: "true",
				},
				{
					Name:  apicommon.DDAPMNonLocalTraffic,
					Value: "true",
				},
				{
					Name:  apicommon.DDAPMReceiverPort,
					Value: "8126",
				},
				{
					Name:  apicommon.DDAPMReceiverSocket,
					Value: apmSocketLocalPath,
				},
			}
			assert.True(
				t,
				apiutils.IsEqualStruct(agentEnvs, expectedAgentEnvs),
				"Trace Agent Env \ndiff = %s", cmp.Diff(agentEnvs, expectedAgentEnvs),
			)
		},
	)
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
					Name:  apicommon.DDAPMNonLocalTraffic,
					Value: "true",
				},
				{
					Name:  apicommon.DDAPMReceiverPort,
					Value: "8126",
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

func testAPMInstrumentationFull() *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.ClusterAgentContainerName]
			expectedAgentEnvs := []*corev1.EnvVar{
				{
					Name:  apicommon.DDAPMInstrumentationEnabled,
					Value: "true",
				},
				{
					Name:  apicommon.DDAPMInstrumentationDisabledNamespaces,
					Value: "[\"foo\",\"bar\"]",
				},
				{
					Name:  apicommon.DDAPMInstrumentationLibVersions,
					Value: "{\"java\":\"1.2.4\"}",
				},
			}
			assert.True(
				t,
				apiutils.IsEqualStruct(agentEnvs, expectedAgentEnvs),
				"Cluster Agent ENVs \ndiff = %s", cmp.Diff(agentEnvs, expectedAgentEnvs),
			)
		},
	)
}

func testAPMInstrumentationDisabledWithAC() *test.ComponentTest {
	// Work around to test that the Cluster Agent will not be configured with SSI if the AC is not enabled.
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.ClusterAgentContainerName]

			assert.True(
				t,
				apiutils.IsEqualStruct(agentEnvs, nil),
				"Cluster Agent ENVs \ndiff = %s", cmp.Diff(agentEnvs, nil),
			)
		},
	)
}

func testAPMInstrumentationNamespaces() *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.ClusterAgentContainerName]
			expectedAgentEnvs := []*corev1.EnvVar{
				{
					Name:  apicommon.DDAPMInstrumentationEnabled,
					Value: "false",
				},
				{
					Name:  apicommon.DDAPMInstrumentationEnabledNamespaces,
					Value: "[\"foo\",\"bar\"]",
				},
				{
					Name:  apicommon.DDAPMInstrumentationLibVersions,
					Value: "{\"java\":\"1.2.4\"}",
				},
			}
			assert.True(
				t,
				apiutils.IsEqualStruct(agentEnvs, expectedAgentEnvs),
				"Cluster Agent ENVs \ndiff = %s", cmp.Diff(agentEnvs, expectedAgentEnvs),
			)
		},
	)
}

func testAPMInstrumentation() *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.ClusterAgentContainerName]
			expectedAgentEnvs := []*corev1.EnvVar{
				{
					Name:  apicommon.DDAPMInstrumentationEnabled,
					Value: "true",
				},
			}
			assert.True(
				t,
				apiutils.IsEqualStruct(agentEnvs, expectedAgentEnvs),
				"Cluster Agent ENVs \ndiff = %s", cmp.Diff(agentEnvs, expectedAgentEnvs),
			)
		},
	)
}

func testAgentHostPortUDS(agentContainerName apicommonv1.AgentContainerName, hostPort int32, hostNetwork bool) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			receiverPortValue := int32(8126)
			if hostNetwork {
				receiverPortValue = hostPort
			}

			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[agentContainerName]
			expectedAgentEnvs := []*corev1.EnvVar{
				{
					Name:  apicommon.DDAPMEnabled,
					Value: "true",
				},
				{
					Name:  apicommon.DDAPMNonLocalTraffic,
					Value: "true",
				},
				{
					Name:  apicommon.DDAPMReceiverPort,
					Value: strconv.Itoa(int(receiverPortValue)),
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
					HostPort:      hostPort,
					Protocol:      corev1.ProtocolTCP,
				},
			}
			if hostNetwork {
				expectedPorts[0].ContainerPort = hostPort
			}
			assert.True(
				t,
				apiutils.IsEqualStruct(agentPorts, expectedPorts),
				"Trace Agent Ports \ndiff = %s", cmp.Diff(agentPorts, expectedPorts),
			)
		},
	)
}
