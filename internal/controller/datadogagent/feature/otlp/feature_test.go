// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package otlp

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/utils/ptr"

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
)

func TestOTLPFeature(t *testing.T) {
	tests := test.FeatureTestSuite{
		{
			Name: "gRPC and HTTP enabled, APM",
			DDA: withForcedLocalService(newAgent(Settings{
				EnabledGRPC:         true,
				EnabledGRPCHostPort: true,
				CustomGRPCHostPort:  4317,
				EndpointGRPC:        "0.0.0.0:4317",
				EnabledHTTP:         true,
				EnabledHTTPHostPort: true,
				CustomHTTPHostPort:  4318,
				EndpointHTTP:        "0.0.0.0:4318",
				APM:                 true,
			})),
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
			DDA: newAgentSingleContainer(Settings{
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
			DDA: newAgent(Settings{
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
			DDA: newAgentSingleContainer(Settings{
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
			DDA: newAgent(Settings{
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
			DDA: newAgentSingleContainer(Settings{
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
			DDA: newAgent(Settings{
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
			DDA: newAgentSingleContainer(Settings{
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
			DDA: newAgent(Settings{
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
			DDA: newAgentSingleContainer(Settings{
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

func TestApplyOTLPDDASharedDependencies(t *testing.T) {
	dda := withForcedLocalService(
		testutils.NewInitializedDatadogAgentBuilder("default", "datadog").
			WithOTLPGRPCSettings(true, true, 4317, "0.0.0.0:4317").
			WithOTLPHTTPSettings(true, true, 4318, "0.0.0.0:4318").
			Build(),
	)
	depsStore := store.NewStore(nil, &store.StoreOptions{
		PlatformInfo: kubernetes.NewPlatformInfo(
			&version.Info{
				Major:      "1",
				Minor:      "32",
				GitVersion: "1.32.0",
			},
			nil,
			nil,
		),
	})
	managers := feature.NewResourceManagers(depsStore)

	err := applyOTLPDDASharedDependencies(dda, &dda.Spec, dda, &dda.Spec, managers)
	assert.NoError(t, err)

	serviceObject, found := depsStore.Get(kubernetes.ServicesKind, "default", "datadog-agent")
	if assert.True(t, found) {
		service := serviceObject.(*corev1.Service)
		assert.Equal(t, common.GetAgentLocalServiceSelector(dda), service.Spec.Selector)
		if assert.NotNil(t, service.Spec.InternalTrafficPolicy) {
			assert.Equal(t, corev1.ServiceInternalTrafficPolicyLocal, *service.Spec.InternalTrafficPolicy)
		}
		assert.Equal(t, []corev1.ServicePort{
			{
				Protocol:    corev1.ProtocolTCP,
				TargetPort:  intstr.FromInt(4317),
				Port:        4317,
				Name:        otlpGRPCPortName,
				AppProtocol: ptr.To(common.KubernetesAppProtocolH2C),
			},
			{
				Protocol:   corev1.ProtocolTCP,
				TargetPort: intstr.FromInt(4318),
				Port:       4318,
				Name:       otlpHTTPPortName,
			},
		}, service.Spec.Ports)
	}
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

func newAgent(set Settings) *v2alpha1.DatadogAgent {
	return testutils.NewDatadogAgentBuilder().
		WithOTLPGRPCSettings(set.EnabledGRPC, set.EnabledGRPCHostPort, set.CustomGRPCHostPort, set.EndpointGRPC).
		WithOTLPHTTPSettings(set.EnabledHTTP, set.EnabledHTTPHostPort, set.CustomHTTPHostPort, set.EndpointHTTP).
		WithAPMEnabled(set.APM).
		Build()
}

func newAgentSingleContainer(set Settings) *v2alpha1.DatadogAgent {
	return testutils.NewDatadogAgentBuilder().
		WithOTLPGRPCSettings(set.EnabledGRPC, set.EnabledGRPCHostPort, set.CustomGRPCHostPort, set.EndpointGRPC).
		WithOTLPHTTPSettings(set.EnabledHTTP, set.EnabledHTTPHostPort, set.CustomHTTPHostPort, set.EndpointHTTP).
		WithAPMEnabled(set.APM).
		WithSingleContainerStrategy(true).
		Build()
}

func withForcedLocalService(dda *v2alpha1.DatadogAgent) *v2alpha1.DatadogAgent {
	dda.Spec.Global.LocalService = &v2alpha1.LocalService{
		ForceEnableLocalService: ptr.To(true),
	}
	return dda
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
