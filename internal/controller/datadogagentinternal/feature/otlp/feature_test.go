// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package otlp

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/test"
	"github.com/DataDog/datadog-operator/pkg/testutils"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestOTLPFeature(t *testing.T) {
	tests := test.FeatureTestSuite{
		{
			Name: "gRPC and HTTP enabled, APM",
			DDAI: newAgent(Settings{
				EnabledGRPC:         true,
				EnabledGRPCHostPort: true,
				CustomGRPCHostPort:  4317,
				EndpointGRPC:        "0.0.0.0:4317",
				EnabledHTTP:         true,
				EnabledHTTPHostPort: true,
				CustomHTTPHostPort:  4318,
				EndpointHTTP:        "0.0.0.0:4318",
				APM:                 true,
			}),
			WantConfigure: true,
			Agent: testExpected(Expected{
				EnvVars: []*corev1.EnvVar{
					{
						Name:  DDOTLPgRPCEndpoint,
						Value: "0.0.0.0:4317",
					},
					{
						Name:  DDOTLPHTTPEndpoint,
						Value: "0.0.0.0:4318",
					},
				},
				CheckTraceAgent: true,
				Ports: []*corev1.ContainerPort{
					{
						Name:          otlpGRPCPortName,
						ContainerPort: 4317,
						HostPort:      4317,
						Protocol:      corev1.ProtocolTCP,
					},
					{
						Name:          otlpHTTPPortName,
						ContainerPort: 4318,
						HostPort:      4318,
						Protocol:      corev1.ProtocolTCP,
					},
				},
			}),
		},
		{
			Name: "[single container] gRPC and HTTP enabled, APM",
			DDAI: newAgentSingleContainer(Settings{
				EnabledGRPC:         true,
				EnabledGRPCHostPort: true,
				EndpointGRPC:        "0.0.0.0:4317",
				EnabledHTTP:         true,
				EnabledHTTPHostPort: true,
				EndpointHTTP:        "0.0.0.0:4318",
				APM:                 true,
			}),
			WantConfigure: true,
			Agent: testExpectedSingleContainer(Expected{
				EnvVars: []*corev1.EnvVar{
					{
						Name:  DDOTLPgRPCEndpoint,
						Value: "0.0.0.0:4317",
					},
					{
						Name:  DDOTLPHTTPEndpoint,
						Value: "0.0.0.0:4318",
					},
				},
				CheckTraceAgent: true,
				Ports: []*corev1.ContainerPort{
					{
						Name:          otlpGRPCPortName,
						ContainerPort: 4317,
						HostPort:      4317,
						Protocol:      corev1.ProtocolTCP,
					},
					{
						Name:          otlpHTTPPortName,
						ContainerPort: 4318,
						HostPort:      4318,
						Protocol:      corev1.ProtocolTCP,
					},
				},
			}),
		},
		{
			Name: "gRPC and HTTP enabled, hostPorts disabled",
			DDAI: newAgent(Settings{
				EnabledGRPC:         true,
				EnabledGRPCHostPort: false,
				EndpointGRPC:        "0.0.0.0:4317",
				EnabledHTTP:         true,
				EnabledHTTPHostPort: false,
				EndpointHTTP:        "0.0.0.0:4318",
				APM:                 true,
			}),
			WantConfigure: true,
			Agent: testExpected(Expected{
				EnvVars: []*corev1.EnvVar{
					{
						Name:  DDOTLPgRPCEndpoint,
						Value: "0.0.0.0:4317",
					},
					{
						Name:  DDOTLPHTTPEndpoint,
						Value: "0.0.0.0:4318",
					},
				},
				CheckTraceAgent: true,
				Ports: []*corev1.ContainerPort{
					{
						Name:          otlpGRPCPortName,
						ContainerPort: 4317,
						Protocol:      corev1.ProtocolTCP,
					},
					{
						Name:          otlpHTTPPortName,
						ContainerPort: 4318,
						Protocol:      corev1.ProtocolTCP,
					},
				},
			}),
		},
		{
			Name: "[single container] gRPC and HTTP enabled, hostPorts disabled",
			DDAI: newAgentSingleContainer(Settings{
				EnabledGRPC:         true,
				EnabledGRPCHostPort: false,
				EndpointGRPC:        "0.0.0.0:4317",
				EnabledHTTP:         true,
				EnabledHTTPHostPort: false,
				EndpointHTTP:        "0.0.0.0:4318",
				APM:                 true,
			}),
			WantConfigure: true,
			Agent: testExpectedSingleContainer(Expected{
				EnvVars: []*corev1.EnvVar{
					{
						Name:  DDOTLPgRPCEndpoint,
						Value: "0.0.0.0:4317",
					},
					{
						Name:  DDOTLPHTTPEndpoint,
						Value: "0.0.0.0:4318",
					},
				},
				CheckTraceAgent: true,
				Ports: []*corev1.ContainerPort{
					{
						Name:          otlpGRPCPortName,
						ContainerPort: 4317,
						Protocol:      corev1.ProtocolTCP,
					},
					{
						Name:          otlpHTTPPortName,
						ContainerPort: 4318,
						Protocol:      corev1.ProtocolTCP,
					},
				},
			}),
		},
		{
			Name: "gRPC and HTTP enabled, custom hostports",
			DDAI: newAgent(Settings{
				EnabledGRPC:         true,
				EnabledGRPCHostPort: true,
				CustomGRPCHostPort:  4315,
				EndpointGRPC:        "0.0.0.0:4317",
				EnabledHTTP:         true,
				EnabledHTTPHostPort: true,
				CustomHTTPHostPort:  4316,
				EndpointHTTP:        "0.0.0.0:4318",
				APM:                 true,
			}),
			WantConfigure: true,
			Agent: testExpected(Expected{
				EnvVars: []*corev1.EnvVar{
					{
						Name:  DDOTLPgRPCEndpoint,
						Value: "0.0.0.0:4317",
					},
					{
						Name:  DDOTLPHTTPEndpoint,
						Value: "0.0.0.0:4318",
					},
				},
				CheckTraceAgent: true,
				Ports: []*corev1.ContainerPort{
					{
						Name:          otlpGRPCPortName,
						ContainerPort: 4317,
						HostPort:      4315,
						Protocol:      corev1.ProtocolTCP,
					},
					{
						Name:          otlpHTTPPortName,
						ContainerPort: 4318,
						HostPort:      4316,
						Protocol:      corev1.ProtocolTCP,
					},
				},
			}),
		},
		{
			Name: "[single container] gRPC and HTTP enabled, custom hostports",
			DDAI: newAgentSingleContainer(Settings{
				EnabledGRPC:         true,
				EnabledGRPCHostPort: true,
				CustomGRPCHostPort:  4315,
				EndpointGRPC:        "0.0.0.0:4317",
				EnabledHTTP:         true,
				EnabledHTTPHostPort: true,
				CustomHTTPHostPort:  4316,
				EndpointHTTP:        "0.0.0.0:4318",
				APM:                 true,
			}),
			WantConfigure: true,
			Agent: testExpectedSingleContainer(Expected{
				EnvVars: []*corev1.EnvVar{
					{
						Name:  DDOTLPgRPCEndpoint,
						Value: "0.0.0.0:4317",
					},
					{
						Name:  DDOTLPHTTPEndpoint,
						Value: "0.0.0.0:4318",
					},
				},
				CheckTraceAgent: true,
				Ports: []*corev1.ContainerPort{
					{
						Name:          otlpGRPCPortName,
						ContainerPort: 4317,
						HostPort:      4315,
						Protocol:      corev1.ProtocolTCP,
					},
					{
						Name:          otlpHTTPPortName,
						ContainerPort: 4318,
						HostPort:      4316,
						Protocol:      corev1.ProtocolTCP,
					},
				},
			}),
		},
		{
			Name: "gRPC enabled, no APM",
			DDAI: newAgent(Settings{
				EnabledGRPC:         true,
				EnabledGRPCHostPort: true,
				CustomGRPCHostPort:  0,
				EndpointGRPC:        "0.0.0.0:4317",
			}),
			WantConfigure: true,
			Agent: testExpected(Expected{
				EnvVars: []*corev1.EnvVar{
					{
						Name:  DDOTLPgRPCEndpoint,
						Value: "0.0.0.0:4317",
					},
				},
				Ports: []*corev1.ContainerPort{
					{
						Name:          otlpGRPCPortName,
						ContainerPort: 4317,
						HostPort:      4317,
						Protocol:      corev1.ProtocolTCP,
					},
				},
			}),
		},
		{
			Name: "[single container] gRPC enabled, no APM",
			DDAI: newAgentSingleContainer(Settings{
				EnabledGRPC:         true,
				EnabledGRPCHostPort: true,
				CustomGRPCHostPort:  0,
				EndpointGRPC:        "0.0.0.0:4317",
			}),
			WantConfigure: true,
			Agent: testExpectedSingleContainer(Expected{
				EnvVars: []*corev1.EnvVar{
					{
						Name:  DDOTLPgRPCEndpoint,
						Value: "0.0.0.0:4317",
					},
				},
				Ports: []*corev1.ContainerPort{
					{
						Name:          otlpGRPCPortName,
						ContainerPort: 4317,
						HostPort:      4317,
						Protocol:      corev1.ProtocolTCP,
					},
				},
			}),
		},
		{
			Name: "HTTP enabled, APM",
			DDAI: newAgent(Settings{
				EnabledHTTP:         true,
				EnabledHTTPHostPort: true,
				CustomHTTPHostPort:  0,
				EndpointHTTP:        "somehostname:4318",
				APM:                 true,
			}),
			WantConfigure: true,
			Agent: testExpected(Expected{
				EnvVars: []*corev1.EnvVar{
					{
						Name:  DDOTLPHTTPEndpoint,
						Value: "somehostname:4318",
					},
				},
				CheckTraceAgent: true,
				Ports: []*corev1.ContainerPort{
					{
						Name:          otlpHTTPPortName,
						ContainerPort: 4318,
						HostPort:      4318,
						Protocol:      corev1.ProtocolTCP,
					},
				},
			}),
		},
		{
			Name: "[single container] HTTP enabled, APM",
			DDAI: newAgentSingleContainer(Settings{
				EnabledHTTP:         true,
				EnabledHTTPHostPort: true,
				CustomHTTPHostPort:  0,
				EndpointHTTP:        "somehostname:4318",
				APM:                 true,
			}),
			WantConfigure: true,
			Agent: testExpectedSingleContainer(Expected{
				EnvVars: []*corev1.EnvVar{
					{
						Name:  DDOTLPHTTPEndpoint,
						Value: "somehostname:4318",
					},
				},
				CheckTraceAgent: true,
				Ports: []*corev1.ContainerPort{
					{
						Name:          otlpHTTPPortName,
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
	EnabledGRPC         bool
	CustomGRPCHostPort  int32
	EnabledGRPCHostPort bool
	EndpointGRPC        string
	EnabledHTTP         bool
	CustomHTTPHostPort  int32
	EnabledHTTPHostPort bool
	EndpointHTTP        string

	APM bool
}

func newAgent(set Settings) *v1alpha1.DatadogAgentInternal {
	return testutils.NewDatadogAgentInternalBuilder().
		WithOTLPGRPCSettings(set.EnabledGRPC, set.EnabledGRPCHostPort, set.CustomGRPCHostPort, set.EndpointGRPC).
		WithOTLPHTTPSettings(set.EnabledHTTP, set.EnabledHTTPHostPort, set.CustomHTTPHostPort, set.EndpointHTTP).
		WithAPMEnabled(set.APM).
		Build()
}

func newAgentSingleContainer(set Settings) *v1alpha1.DatadogAgentInternal {
	return testutils.NewDatadogAgentInternalBuilder().
		WithOTLPGRPCSettings(set.EnabledGRPC, set.EnabledGRPCHostPort, set.CustomGRPCHostPort, set.EndpointGRPC).
		WithOTLPHTTPSettings(set.EnabledHTTP, set.EnabledHTTPHostPort, set.CustomHTTPHostPort, set.EndpointHTTP).
		WithAPMEnabled(set.APM).
		WithSingleContainerStrategy(true).
		Build()
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

			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
			assert.True(
				t,
				apiutils.IsEqualStruct(agentEnvs, exp.EnvVars),
				"Core Agent ENVs \ndiff = %s", cmp.Diff(agentEnvs, exp.EnvVars),
			)

			if exp.CheckTraceAgent {
				agentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommon.TraceAgentContainerName]
				assert.True(
					t,
					apiutils.IsEqualStruct(agentEnvs, exp.EnvVars),
					"Trace Agent ENVs \ndiff = %s", cmp.Diff(agentEnvs, exp.EnvVars),
				)
			}

			agentPorts := mgr.PortMgr.PortsByC[apicommon.CoreAgentContainerName]
			assert.True(
				t,
				apiutils.IsEqualStruct(agentPorts, exp.Ports),
				"Core Agent Ports \ndiff = %s", cmp.Diff(agentPorts, exp.Ports),
			)
		},
	)
}

func testExpectedSingleContainer(exp Expected) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommon.UnprivilegedSingleAgentContainerName]
			assert.True(
				t,
				apiutils.IsEqualStruct(agentEnvs, exp.EnvVars),
				"Core Agent ENVs \ndiff = %s", cmp.Diff(agentEnvs, exp.EnvVars),
			)

			if exp.CheckTraceAgent {
				agentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommon.UnprivilegedSingleAgentContainerName]
				assert.True(
					t,
					apiutils.IsEqualStruct(agentEnvs, exp.EnvVars),
					"Trace Agent ENVs \ndiff = %s", cmp.Diff(agentEnvs, exp.EnvVars),
				)
			}

			agentPorts := mgr.PortMgr.PortsByC[apicommon.UnprivilegedSingleAgentContainerName]
			assert.True(
				t,
				apiutils.IsEqualStruct(agentPorts, exp.Ports),
				"Core Agent Ports \ndiff = %s", cmp.Diff(agentPorts, exp.Ports),
			)
		},
	)
}
