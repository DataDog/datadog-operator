// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apm

import (
	"strconv"
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/testutils"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	apmSocketHostPath  = common.DogstatsdAPMSocketHostPath + "/" + common.APMSocketName
	apmSocketLocalPath = apmSocketVolumeLocalPath + "/" + common.APMSocketName
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
		{
			Name: "apm not enabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAPMEnabled(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "apm not enabled with single container strategy",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAPMEnabled(false).
				WithSingleContainerStrategy(true).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "apm enabled, use uds",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(false, apiutils.NewInt32Pointer(8126)).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				Build(),
			WantConfigure: true,
			Agent:         testAgentUDSOnly(apicommon.TraceAgentContainerName),
		},
		{
			Name: "apm enabled, use uds with single container strategy",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(false, apiutils.NewInt32Pointer(8126)).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				WithSingleContainerStrategy(true).
				Build(),
			WantConfigure: true,
			Agent:         testAgentUDSOnly(apicommon.UnprivilegedSingleAgentContainerName),
		},
		{
			Name: "apm enabled, use uds and host port",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(true, apiutils.NewInt32Pointer(8126)).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent:         testAgentHostPortUDS(apicommon.TraceAgentContainerName, 8126, false),
		},
		{
			Name: "apm enabled, use uds and host port with single container strategy",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(true, apiutils.NewInt32Pointer(8126)).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				WithSingleContainerStrategy(true).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent:         testAgentHostPortUDS(apicommon.UnprivilegedSingleAgentContainerName, 8126, false),
		},
		{
			Name: "apm enabled, use uds and custom host port",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(true, apiutils.NewInt32Pointer(1234)).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent:         testAgentHostPortUDS(apicommon.TraceAgentContainerName, 1234, false),
		},
		{
			Name: "apm enabled, use uds and custom host port with single container strategy",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(true, apiutils.NewInt32Pointer(1234)).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				WithSingleContainerStrategy(true).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent:         testAgentHostPortUDS(apicommon.UnprivilegedSingleAgentContainerName, 1234, false),
		},
		{
			Name: "apm enabled, use uds and host port enabled but no custom host port",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(true, nil).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent:         testAgentHostPortUDS(apicommon.TraceAgentContainerName, 8126, false),
		},
		{
			Name: "apm enabled, use uds and host port enabled but no custom host port with single container strategy",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(true, nil).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				WithSingleContainerStrategy(true).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent:         testAgentHostPortUDS(apicommon.UnprivilegedSingleAgentContainerName, 8126, false),
		},
		{
			Name: "apm enabled, host port enabled host network",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(true, nil).
				WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{
					HostNetwork: apiutils.NewBoolPointer(true),
				}).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent:         testAgentHostPortUDS(apicommon.TraceAgentContainerName, 8126, true),
		},
		{
			Name: "apm enabled, host port enabled host network with single container strategy",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(true, nil).
				WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{
					HostNetwork: apiutils.NewBoolPointer(true),
				}).
				WithSingleContainerStrategy(true).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent:         testAgentHostPortUDS(apicommon.UnprivilegedSingleAgentContainerName, 8126, true),
		},
		{
			Name: "apm enabled, custom host port host network",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(true, apiutils.NewInt32Pointer(1234)).
				WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{
					HostNetwork: apiutils.NewBoolPointer(true),
				}).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent:         testAgentHostPortUDS(apicommon.TraceAgentContainerName, 1234, true),
		},
		{
			Name: "apm enabled, custom host port host network with single container strategy",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(true, apiutils.NewInt32Pointer(1234)).
				WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{
					HostNetwork: apiutils.NewBoolPointer(true),
				}).
				WithSingleContainerStrategy(true).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent:         testAgentHostPortUDS(apicommon.UnprivilegedSingleAgentContainerName, 1234, true),
		},
		{
			Name: "basic apm single step instrumentation",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(true, apiutils.NewInt32Pointer(8126)).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				WithAdmissionControllerEnabled(true).
				WithAPMSingleStepInstrumentationEnabled(true, nil, nil, nil, false, "", nil).
				WithSingleContainerStrategy(false).
				Build(),
			WantConfigure: true,
			ClusterAgent:  testAPMInstrumentation(),
		},
		{
			Name: "error apm single step instrumentation without language detection",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(true, apiutils.NewInt32Pointer(8126)).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				WithAdmissionControllerEnabled(true).
				WithClusterAgentTag(minInstrumentationTargetsVersion).
				WithAPMSingleStepInstrumentationEnabled(true,
					nil,
					[]string{"foo", "bar"},
					map[string]string{
						"java": "1.2.4",
					},
					false,
					"",
					[]v2alpha1.SSITarget{
						{
							Name: "sometarget",
							PodSelector: &v1.LabelSelector{
								MatchLabels: map[string]string{
									"key": "value",
								},
								MatchExpressions: []v1.LabelSelectorRequirement{
									{
										Key:      "somekey",
										Operator: v1.LabelSelectorOpIn,
										Values:   []string{"value1", "value2"},
									},
								},
							},
							NamespaceSelector: &v2alpha1.NamespaceSelector{
								MatchNames: []string{"name1", "name2"},
								MatchLabels: map[string]string{
									"key1": "val1",
									"key2": "val2",
								},
								MatchExpressions: []v1.LabelSelectorRequirement{
									{
										Key:      "somekey1",
										Operator: v1.LabelSelectorOpIn,
										Values:   []string{"value1", "value2"},
									},
								},
							},
							TracerVersions: map[string]string{
								"dotnet": "2",
								"java":   "1",
							},
							TracerConfigs: []corev1.EnvVar{
								{
									Name:  "DD_PROFILING_ENABLED",
									Value: "true",
								},
								{
									Name:  "DD_DATA_JOBS_ENABLED",
									Value: "true",
								},
							},
						},
					}).
				WithSingleContainerStrategy(false).
				Build(),
			WantConfigure: true,
			ClusterAgent:  testAPMInstrumentationFull(),
			WantDependenciesFunc: func(t testing.TB, store store.StoreClient) {
				_, found := store.Get(kubernetes.ClusterRoleBindingKind, "", "-apm-cluster-agent")
				if found {
					t.Error("Should not have created proper RBAC for language detection because language detection is not enabled.")
				}
			},
		},
		{
			Name: "step instrumentation precedence",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAPMEnabled(false).
				WithAPMHostPortEnabled(true, apiutils.NewInt32Pointer(8126)).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				WithAPMSingleStepInstrumentationEnabled(true, nil, nil, nil, false, "", nil).
				WithAdmissionControllerEnabled(true).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "step instrumentation w/o AC",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(true, apiutils.NewInt32Pointer(8126)).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				WithAPMSingleStepInstrumentationEnabled(true, nil, nil, nil, false, "", nil).
				WithAdmissionControllerEnabled(false).
				WithSingleContainerStrategy(false).
				Build(),
			WantConfigure: true,
			Agent:         testTraceAgentEnabled(apicommon.TraceAgentContainerName),
			ClusterAgent:  testAPMInstrumentationDisabledWithAC(),
		},
		{
			Name: "single step instrumentation namespace specific",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(true, apiutils.NewInt32Pointer(8126)).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				WithAPMSingleStepInstrumentationEnabled(false, []string{"foo", "bar"}, nil, map[string]string{"java": "1.2.4"}, false, "", nil).
				WithAdmissionControllerEnabled(true).
				Build(),
			WantConfigure: true,
			ClusterAgent:  testAPMInstrumentationNamespaces(),
		},
		{
			Name: "single step instrumentation with language detection enabled, process check runs in process agent",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(true, apiutils.NewInt32Pointer(8126)).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				WithAPMSingleStepInstrumentationEnabled(true, nil, nil, nil, true, "", nil).
				WithAdmissionControllerEnabled(true).
				Build(),
			WantConfigure: true,
			ClusterAgent:  testAPMInstrumentationWithLanguageDetectionEnabledForClusterAgent(),
			Agent:         testAPMInstrumentationWithLanguageDetectionForNodeAgent(true, false),
			WantDependenciesFunc: func(t testing.TB, store store.StoreClient) {
				_, found := store.Get(kubernetes.ClusterRoleBindingKind, "", "-apm-cluster-agent")
				if !found {
					t.Error("Should have created proper RBAC for language detection")
				}
			},
		},
		{
			Name: "single step instrumentation without language detection enabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(true, apiutils.NewInt32Pointer(8126)).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				WithAPMSingleStepInstrumentationEnabled(true, nil, nil, nil, false, "", nil).
				WithAdmissionControllerEnabled(true).
				Build(),
			WantConfigure: true,
			ClusterAgent:  testAPMInstrumentation(),
			Agent:         testAPMInstrumentationWithLanguageDetectionForNodeAgent(false, false),
			WantDependenciesFunc: func(t testing.TB, store store.StoreClient) {
				_, found := store.Get(kubernetes.ClusterRoleBindingKind, "", "-apm-cluster-agent")
				if found {
					t.Error("Should not have created RBAC for language detection")
				}
			},
		},
		{
			Name: "single step instrumentation with language detection enabled, process check runs in core agent",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(true, apiutils.NewInt32Pointer(8126)).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				WithAPMSingleStepInstrumentationEnabled(true, nil, nil, nil, true, "", nil).
				WithAdmissionControllerEnabled(true).
				WithComponentOverride(
					v2alpha1.NodeAgentComponentName,
					v2alpha1.DatadogAgentComponentOverride{
						Image: &v2alpha1.AgentImageConfig{Tag: "7.60.0"},
					},
				).
				WithProcessChecksInCoreAgent(true).
				Build(),
			WantConfigure: true,
			ClusterAgent:  testAPMInstrumentationWithLanguageDetectionEnabledForClusterAgent(),
			Agent:         testAPMInstrumentationWithLanguageDetectionForNodeAgent(true, true),
			WantDependenciesFunc: func(t testing.TB, store store.StoreClient) {
				_, found := store.Get(kubernetes.ClusterRoleBindingKind, "", "-apm-cluster-agent")
				if !found {
					t.Error("Should have created proper RBAC for language detection")
				}
			},
		},
		{
			Name: "error tracking standalone",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(false, apiutils.NewInt32Pointer(8126)).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				WithErrorTrackingStandalone(true).
				Build(),
			WantConfigure: true,
			Agent:         testAgentErrorTrackingStandalone(),
		},
		{
			Name: "single step instrumentation with custom injector image",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithAPMHostPortEnabled(true, apiutils.NewInt32Pointer(8126)).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				WithAPMSingleStepInstrumentationEnabled(true, nil, nil, nil, false, "0.27.0", nil).
				WithAdmissionControllerEnabled(true).
				Build(),
			WantConfigure: true,
			ClusterAgent:  testAPMInstrumentationWithCustomInjectorImage(),
		},
	}

	tests.Run(t, buildAPMFeature)
}

func testTraceAgentEnabled(containerName apicommon.AgentContainerName) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)
			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[containerName]
			expectedAgentEnvs := []*corev1.EnvVar{
				{
					Name:  common.DDAPMEnabled,
					Value: "true",
				},
				{
					Name:  DDAPMNonLocalTraffic,
					Value: "true",
				},
				{
					Name:  DDAPMReceiverPort,
					Value: "8126",
				},
				{
					Name:  DDAPMReceiverSocket,
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

			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommon.TraceAgentContainerName]
			expectedAgentEnvs := []*corev1.EnvVar{
				{
					Name:  common.DDAPMEnabled,
					Value: "true",
				},
				{
					Name:  DDAPMNonLocalTraffic,
					Value: "true",
				},
				{
					Name:  DDAPMReceiverPort,
					Value: "8126",
				},
			}
			assert.True(
				t,
				apiutils.IsEqualStruct(agentEnvs, expectedAgentEnvs),
				"Trace Agent ENVs \ndiff = %s", cmp.Diff(agentEnvs, expectedAgentEnvs),
			)

			agentPorts := mgr.PortMgr.PortsByC[apicommon.TraceAgentContainerName]
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

func testAgentErrorTrackingStandalone() *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommon.TraceAgentContainerName]
			expectedAgentEnvs := []*corev1.EnvVar{
				{
					Name:  common.DDAPMEnabled,
					Value: "true",
				},
				{
					Name:  DDAPMReceiverSocket,
					Value: apmSocketLocalPath,
				},
				{
					Name:  common.DDAPMErrorTrackingStandaloneEnabled,
					Value: "true",
				},
			}
			assert.True(
				t,
				apiutils.IsEqualStruct(agentEnvs, expectedAgentEnvs),
				"Trace Agent ENVs \ndiff = %s", cmp.Diff(agentEnvs, expectedAgentEnvs),
			)
		},
	)
}

func testAgentUDSOnly(agentContainerName apicommon.AgentContainerName) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[agentContainerName]
			expectedAgentEnvs := []*corev1.EnvVar{
				{
					Name:  common.DDAPMEnabled,
					Value: "true",
				},
				{
					Name:  DDAPMReceiverSocket,
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
					Name:      apmSocketVolumeName,
					MountPath: apmSocketVolumeLocalPath,
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
					Name: apmSocketVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: common.DogstatsdAPMSocketHostPath,
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

			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName]
			expectedAgentEnvs := []*corev1.EnvVar{
				{
					Name:  DDAPMInstrumentationEnabled,
					Value: "true",
				},
				{
					Name:  DDAPMInstrumentationDisabledNamespaces,
					Value: "[\"foo\",\"bar\"]",
				},
				{
					Name:  DDAPMInstrumentationLibVersions,
					Value: "{\"java\":\"1.2.4\"}",
				},
				{
					Name:  DDAPMInstrumentationTargets,
					Value: `[{"name":"sometarget","podSelector":{"matchLabels":{"key":"value"},"matchExpressions":[{"key":"somekey","operator":"In","values":["value1","value2"]}]},"namespaceSelector":{"matchNames":["name1","name2"],"matchLabels":{"key1":"val1","key2":"val2"},"matchExpressions":[{"key":"somekey1","operator":"In","values":["value1","value2"]}]},"ddTraceVersions":{"dotnet":"2","java":"1"},"ddTraceConfigs":[{"name":"DD_PROFILING_ENABLED","value":"true"},{"name":"DD_DATA_JOBS_ENABLED","value":"true"}]}]`,
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

			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName]

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

			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName]
			expectedAgentEnvs := []*corev1.EnvVar{
				{
					Name:  DDAPMInstrumentationEnabled,
					Value: "false",
				},
				{
					Name:  DDAPMInstrumentationEnabledNamespaces,
					Value: "[\"foo\",\"bar\"]",
				},
				{
					Name:  DDAPMInstrumentationLibVersions,
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

			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName]
			expectedAgentEnvs := []*corev1.EnvVar{
				{
					Name:  DDAPMInstrumentationEnabled,
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

func testAPMInstrumentationWithLanguageDetectionEnabledForClusterAgent() *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			// Test Cluster Agent Env Vars
			clusterAgentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName]
			expectedClusterAgentEnvs := []*corev1.EnvVar{
				{
					Name:  DDAPMInstrumentationEnabled,
					Value: "true",
				},
				{
					Name:  DDLanguageDetectionEnabled,
					Value: "true",
				},
			}
			assert.True(
				t,
				apiutils.IsEqualStruct(clusterAgentEnvs, expectedClusterAgentEnvs),
				"Cluster Agent ENVs \ndiff = %s", cmp.Diff(clusterAgentEnvs, expectedClusterAgentEnvs),
			)
		},
	)
}

func testAPMInstrumentationWithCustomInjectorImage() *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName]
			expectedAgentEnvs := []*corev1.EnvVar{
				{
					Name:  DDAPMInstrumentationEnabled,
					Value: "true",
				},
				{
					Name:  DDAPMInstrumentationInjectorImageTag,
					Value: "0.27.0",
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

func testAPMInstrumentationWithLanguageDetectionForNodeAgent(languageDetectionEnabled bool, processChecksInCoreAgent bool) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			coreAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
			processAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.ProcessAgentContainerName]

			var expectedEnvVars []*corev1.EnvVar
			if languageDetectionEnabled {
				expectedEnvVars = []*corev1.EnvVar{
					{
						Name:  DDLanguageDetectionEnabled,
						Value: "true",
					},
					{
						Name:  common.DDProcessConfigRunInCoreAgent,
						Value: apiutils.BoolToString(&processChecksInCoreAgent),
					},
				}
			}

			// Assert Env Vars Added to Core Agent Container
			assert.True(
				t,
				apiutils.IsEqualStruct(coreAgentEnvVars, expectedEnvVars),
				"Core Agent Container ENVs \ndiff = %s", cmp.Diff(coreAgentEnvVars, expectedEnvVars),
			)

			// Assert Env Vars Added to Process Agent Container
			assert.True(
				t,
				apiutils.IsEqualStruct(processAgentEnvVars, expectedEnvVars),
				"Process Agent Container ENVs \ndiff = %s", cmp.Diff(processAgentEnvVars, expectedEnvVars),
			)
		},
	)
}

func testAgentHostPortUDS(agentContainerName apicommon.AgentContainerName, hostPort int32, hostNetwork bool) *test.ComponentTest {
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
					Name:  common.DDAPMEnabled,
					Value: "true",
				},
				{
					Name:  DDAPMNonLocalTraffic,
					Value: "true",
				},
				{
					Name:  DDAPMReceiverPort,
					Value: strconv.Itoa(int(receiverPortValue)),
				},
				{
					Name:  DDAPMReceiverSocket,
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
					Name:      apmSocketVolumeName,
					MountPath: apmSocketVolumeLocalPath,
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
					Name: apmSocketVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: common.DogstatsdAPMSocketHostPath,
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
