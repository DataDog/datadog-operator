// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package otlp

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

func TestOTLPFeature(t *testing.T) {
	tests := test.FeatureTestSuite{
		//////////////////////////
		// v1Alpha1.DatadogAgent
		//////////////////////////
		{
			Name: "v1alpha1 gRPC and HTTP enabled, APM",
			DDAv1: newV1Agent(Settings{
				EnabledGRPC:  true,
				EndpointGRPC: "0.0.0.0:4317",
				EnabledHTTP:  true,
				EndpointHTTP: "0.0.0.0:4318",
				APM:          true,
			}),
			WantConfigure: true,
			Agent: testExpected(Expected{
				EnvVars: []*corev1.EnvVar{
					{
						Name:  apicommon.DDOTLPgRPCEndpoint,
						Value: "0.0.0.0:4317",
					},
					{
						Name:  apicommon.DDOTLPHTTPEndpoint,
						Value: "0.0.0.0:4318",
					},
				},
				CheckTraceAgent: true,
				Ports: []*corev1.ContainerPort{
					{
						Name:          apicommon.OTLPGRPCPortName,
						ContainerPort: 4317,
						HostPort:      4317,
						Protocol:      corev1.ProtocolTCP,
					},
					{
						Name:          apicommon.OTLPHTTPPortName,
						ContainerPort: 4318,
						HostPort:      4318,
						Protocol:      corev1.ProtocolTCP,
					},
				},
			}),
		},
		{
			Name: "v1alpha1 gRPC enabled, APM",
			DDAv1: newV1Agent(Settings{
				EnabledGRPC:  true,
				EndpointGRPC: "0.0.0.0:4317",
				APM:          true,
			}),
			WantConfigure: true,
			Agent: testExpected(Expected{
				EnvVars: []*corev1.EnvVar{
					{
						Name:  apicommon.DDOTLPgRPCEndpoint,
						Value: "0.0.0.0:4317",
					},
				},
				CheckTraceAgent: true,
				Ports: []*corev1.ContainerPort{
					{
						Name:          apicommon.OTLPGRPCPortName,
						ContainerPort: 4317,
						HostPort:      4317,
						Protocol:      corev1.ProtocolTCP,
					},
				},
			}),
		},
		{
			Name: "v1alpha1 HTTP enabled, no APM",
			DDAv1: newV1Agent(Settings{
				EnabledHTTP:  true,
				EndpointHTTP: "localhost:4318",
			}),
			WantConfigure: true,
			Agent: testExpected(Expected{
				EnvVars: []*corev1.EnvVar{
					{
						Name:  apicommon.DDOTLPHTTPEndpoint,
						Value: "localhost:4318",
					},
				},
				Ports: []*corev1.ContainerPort{
					{
						Name:          apicommon.OTLPHTTPPortName,
						ContainerPort: 4318,
						HostPort:      4318,
						Protocol:      corev1.ProtocolTCP,
					},
				},
			}),
		},

		//////////////////////////
		// v2Alpha1.DatadogAgent
		//////////////////////////
		{
			Name: "v2alpha1 gRPC and HTTP enabled, APM",
			DDAv2: newV2Agent(Settings{
				EnabledGRPC:  true,
				EndpointGRPC: "0.0.0.0:4317",
				EnabledHTTP:  true,
				EndpointHTTP: "0.0.0.0:4318",
				APM:          true,
			}),
			WantConfigure: true,
			Agent: testExpected(Expected{
				EnvVars: []*corev1.EnvVar{
					{
						Name:  apicommon.DDOTLPgRPCEndpoint,
						Value: "0.0.0.0:4317",
					},
					{
						Name:  apicommon.DDOTLPHTTPEndpoint,
						Value: "0.0.0.0:4318",
					},
				},
				CheckTraceAgent: true,
				Ports: []*corev1.ContainerPort{
					{
						Name:          apicommon.OTLPGRPCPortName,
						ContainerPort: 4317,
						HostPort:      4317,
						Protocol:      corev1.ProtocolTCP,
					},
					{
						Name:          apicommon.OTLPHTTPPortName,
						ContainerPort: 4318,
						HostPort:      4318,
						Protocol:      corev1.ProtocolTCP,
					},
				},
			}),
		},
		{
			Name: "[mono-container] v2alpha1 gRPC and HTTP enabled, APM",
			DDAv2: newV2MonoAgent(Settings{
				EnabledGRPC:  true,
				EndpointGRPC: "0.0.0.0:4317",
				EnabledHTTP:  true,
				EndpointHTTP: "0.0.0.0:4318",
				APM:          true,
			}),
			WantConfigure: true,
			Agent: testExpectedMono(Expected{
				EnvVars: []*corev1.EnvVar{
					{
						Name:  apicommon.DDOTLPgRPCEndpoint,
						Value: "0.0.0.0:4317",
					},
					{
						Name:  apicommon.DDOTLPHTTPEndpoint,
						Value: "0.0.0.0:4318",
					},
				},
				CheckTraceAgent: true,
				Ports: []*corev1.ContainerPort{
					{
						Name:          apicommon.OTLPGRPCPortName,
						ContainerPort: 4317,
						HostPort:      4317,
						Protocol:      corev1.ProtocolTCP,
					},
					{
						Name:          apicommon.OTLPHTTPPortName,
						ContainerPort: 4318,
						HostPort:      4318,
						Protocol:      corev1.ProtocolTCP,
					},
				},
			}),
		},
		{
			Name: "v2alpha1 gRPC enabled, no APM",
			DDAv2: newV2Agent(Settings{
				EnabledGRPC:  true,
				EndpointGRPC: "0.0.0.0:4317",
			}),
			WantConfigure: true,
			Agent: testExpected(Expected{
				EnvVars: []*corev1.EnvVar{
					{
						Name:  apicommon.DDOTLPgRPCEndpoint,
						Value: "0.0.0.0:4317",
					},
				},
				Ports: []*corev1.ContainerPort{
					{
						Name:          apicommon.OTLPGRPCPortName,
						ContainerPort: 4317,
						HostPort:      4317,
						Protocol:      corev1.ProtocolTCP,
					},
				},
			}),
		},
		{
			Name: "[mono-container] v2alpha1 gRPC enabled, no APM",
			DDAv2: newV2MonoAgent(Settings{
				EnabledGRPC:  true,
				EndpointGRPC: "0.0.0.0:4317",
			}),
			WantConfigure: true,
			Agent: testExpectedMono(Expected{
				EnvVars: []*corev1.EnvVar{
					{
						Name:  apicommon.DDOTLPgRPCEndpoint,
						Value: "0.0.0.0:4317",
					},
				},
				Ports: []*corev1.ContainerPort{
					{
						Name:          apicommon.OTLPGRPCPortName,
						ContainerPort: 4317,
						HostPort:      4317,
						Protocol:      corev1.ProtocolTCP,
					},
				},
			}),
		},
		{
			Name: "v2alpha1 HTTP enabled, APM",
			DDAv2: newV2Agent(Settings{
				EnabledHTTP:  true,
				EndpointHTTP: "somehostname:4318",
				APM:          true,
			}),
			WantConfigure: true,
			Agent: testExpected(Expected{
				EnvVars: []*corev1.EnvVar{
					{
						Name:  apicommon.DDOTLPHTTPEndpoint,
						Value: "somehostname:4318",
					},
				},
				CheckTraceAgent: true,
				Ports: []*corev1.ContainerPort{
					{
						Name:          apicommon.OTLPHTTPPortName,
						ContainerPort: 4318,
						HostPort:      4318,
						Protocol:      corev1.ProtocolTCP,
					},
				},
			}),
		},
		{
			Name: "[mono-container] v2alpha1 HTTP enabled, APM",
			DDAv2: newV2MonoAgent(Settings{
				EnabledHTTP:  true,
				EndpointHTTP: "somehostname:4318",
				APM:          true,
			}),
			WantConfigure: true,
			Agent: testExpectedMono(Expected{
				EnvVars: []*corev1.EnvVar{
					{
						Name:  apicommon.DDOTLPHTTPEndpoint,
						Value: "somehostname:4318",
					},
				},
				CheckTraceAgent: true,
				Ports: []*corev1.ContainerPort{
					{
						Name:          apicommon.OTLPHTTPPortName,
						ContainerPort: 4318,
						HostPort:      4318,
						Protocol:      corev1.ProtocolTCP,
					},
				},
			}),
		},
	}

	tests.Run(t, buildOTLPFeature)
}

type Settings struct {
	EnabledGRPC  bool
	EndpointGRPC string
	EnabledHTTP  bool
	EndpointHTTP string

	APM bool
}

func newV1Agent(set Settings) *v1alpha1.DatadogAgent {
	return &v1alpha1.DatadogAgent{
		Spec: v1alpha1.DatadogAgentSpec{
			Agent: v1alpha1.DatadogAgentSpecAgentSpec{
				OTLP: &v1alpha1.OTLPSpec{Receiver: v1alpha1.OTLPReceiverSpec{Protocols: v1alpha1.OTLPProtocolsSpec{
					GRPC: &v1alpha1.OTLPGRPCSpec{
						Enabled:  &set.EnabledGRPC,
						Endpoint: &set.EndpointGRPC,
					},
					HTTP: &v1alpha1.OTLPHTTPSpec{
						Enabled:  &set.EnabledHTTP,
						Endpoint: &set.EndpointHTTP,
					},
				}}},
				Apm: &v1alpha1.APMSpec{
					Enabled: apiutils.NewBoolPointer(set.APM),
				},
			},
		},
	}
}

func newV2Agent(set Settings) *v2alpha1.DatadogAgent {
	return &v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
					GRPC: &v2alpha1.OTLPGRPCConfig{
						Enabled:  &set.EnabledGRPC,
						Endpoint: &set.EndpointGRPC,
					},
					HTTP: &v2alpha1.OTLPHTTPConfig{
						Enabled:  &set.EnabledHTTP,
						Endpoint: &set.EndpointHTTP,
					},
				}}},
				APM: &v2alpha1.APMFeatureConfig{
					Enabled: apiutils.NewBoolPointer(set.APM),
				},
			},
			Global: &v2alpha1.GlobalConfig{},
		},
	}
}

func newV2MonoAgent(set Settings) *v2alpha1.DatadogAgent {
	ddaV2 := newV2Agent(set)
	ddaV2.Spec.Global = &v2alpha1.GlobalConfig{
		ContainerProcessModel: &v2alpha1.ContainerProcessModel{
			UseMultiProcessContainer: apiutils.NewBoolPointer(true),
		},
	}
	return ddaV2
}

type Expected struct {
	EnvVars         []*corev1.EnvVar
	CheckTraceAgent bool
	Ports           []*corev1.ContainerPort
}

func testExpected(exp Expected) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
			assert.True(
				t,
				apiutils.IsEqualStruct(agentEnvs, exp.EnvVars),
				"Core Agent ENVs \ndiff = %s", cmp.Diff(agentEnvs, exp.EnvVars),
			)

			if exp.CheckTraceAgent {
				agentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.TraceAgentContainerName]
				assert.True(
					t,
					apiutils.IsEqualStruct(agentEnvs, exp.EnvVars),
					"Trace Agent ENVs \ndiff = %s", cmp.Diff(agentEnvs, exp.EnvVars),
				)
			}

			agentPorts := mgr.PortMgr.PortsByC[apicommonv1.CoreAgentContainerName]
			assert.True(
				t,
				apiutils.IsEqualStruct(agentPorts, exp.Ports),
				"Core Agent Ports \ndiff = %s", cmp.Diff(agentPorts, exp.Ports),
			)
		},
	)
}

func testExpectedMono(exp Expected) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.NonPrivilegedMonoContainerName]
			assert.True(
				t,
				apiutils.IsEqualStruct(agentEnvs, exp.EnvVars),
				"Core Agent ENVs \ndiff = %s", cmp.Diff(agentEnvs, exp.EnvVars),
			)

			if exp.CheckTraceAgent {
				agentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.NonPrivilegedMonoContainerName]
				assert.True(
					t,
					apiutils.IsEqualStruct(agentEnvs, exp.EnvVars),
					"Trace Agent ENVs \ndiff = %s", cmp.Diff(agentEnvs, exp.EnvVars),
				)
			}

			agentPorts := mgr.PortMgr.PortsByC[apicommonv1.NonPrivilegedMonoContainerName]
			assert.True(
				t,
				apiutils.IsEqualStruct(agentPorts, exp.Ports),
				"Core Agent Ports \ndiff = %s", cmp.Diff(agentPorts, exp.Ports),
			)
		},
	)
}
