// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package admissioncontroller

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

func TestAdmissionControllerFeature(t *testing.T) {
	apmUDS := &v2alpha1.APMFeatureConfig{
		Enabled: apiutils.NewBoolPointer(true),
		UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
			Enabled: apiutils.NewBoolPointer(true),
		},
	}
	dsdUDS := &v2alpha1.DogstatsdFeatureConfig{
		UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
			Enabled: apiutils.NewBoolPointer(true),
		},
	}
	globalConfig := &v2alpha1.GlobalConfig{
		Registry: apiutils.NewStringPointer("globalRegistryName"),
	}
	tests := test.FeatureTestSuite{
		//////////////////////////
		// v1Alpha1.DatadogAgent
		//////////////////////////
		{
			Name:          "v1alpha1 admission controller not enabled",
			DDAv1:         newV1Agent(false),
			WantConfigure: false,
		},
		{
			Name:          "v1alpha1 admission controller enabled, cwsInstrumentation not enabled",
			DDAv1:         newV1Agent(true),
			WantConfigure: true,
			ClusterAgent:  testDCAResources("hostip", "", false),
		},
		//////////////////////////
		// v2Alpha1.DatadogAgent
		//////////////////////////
		{
			Name:          "v2alpha1 admission controller not enabled",
			DDAv2:         newV2Agent(false, "", "", false, &v2alpha1.APMFeatureConfig{}, &v2alpha1.DogstatsdFeatureConfig{}, nil),
			WantConfigure: false,
		},
		{
			Name:          "v2alpha1 admission controller enabled",
			DDAv2:         newV2Agent(true, "", "", false, &v2alpha1.APMFeatureConfig{}, &v2alpha1.DogstatsdFeatureConfig{}, nil),
			WantConfigure: true,
			ClusterAgent:  testDCAResources("", "", false),
		},
		{
			Name:          "v2alpha1 admission controller enabled, apm uses uds",
			DDAv2:         newV2Agent(true, "", "", false, apmUDS, &v2alpha1.DogstatsdFeatureConfig{}, nil),
			WantConfigure: true,
			ClusterAgent:  testDCAResources("socket", "", false),
		},
		{
			Name:          "v2alpha1 admission controller enabled, dsd uses uds",
			DDAv2:         newV2Agent(true, "", "", false, &v2alpha1.APMFeatureConfig{}, dsdUDS, nil),
			WantConfigure: true,
			ClusterAgent:  testDCAResources("socket", "", false),
		},
		{
			Name:          "v2alpha1 admission controller enabled, add custom registry in global config",
			DDAv2:         newV2Agent(true, "", "globalRegistryName", false, &v2alpha1.APMFeatureConfig{}, &v2alpha1.DogstatsdFeatureConfig{}, globalConfig),
			WantConfigure: true,
			ClusterAgent:  testDCAResources("", "globalRegistryName", false),
		},
		{
			Name:          "v2alpha1 admission controller enabled, add custom registry in global config, override with feature config",
			DDAv2:         newV2Agent(true, "", "testRegistryName", false, &v2alpha1.APMFeatureConfig{}, &v2alpha1.DogstatsdFeatureConfig{}, globalConfig),
			WantConfigure: true,
			ClusterAgent:  testDCAResources("", "testRegistryName", false),
		},
		{
			Name:          "v2alpha1 admission controller enabled, cwsInstrumentation enabled",
			DDAv2:         newV2Agent(true, "", "", true, &v2alpha1.APMFeatureConfig{}, &v2alpha1.DogstatsdFeatureConfig{}, nil),
			WantConfigure: true,
			ClusterAgent:  testDCAResources("", "", true),
		},
	}

	tests.Run(t, buildAdmissionControllerFeature)
}

func newV1Agent(enabled bool) *v1alpha1.DatadogAgent {
	return &v1alpha1.DatadogAgent{
		Spec: v1alpha1.DatadogAgentSpec{
			ClusterAgent: v1alpha1.DatadogAgentSpecClusterAgentSpec{
				Config: &v1alpha1.ClusterAgentConfig{
					AdmissionController: &v1alpha1.AdmissionControllerConfig{
						Enabled:                apiutils.NewBoolPointer(enabled),
						MutateUnlabelled:       apiutils.NewBoolPointer(true),
						ServiceName:            apiutils.NewStringPointer("testServiceName"),
						AgentCommunicationMode: apiutils.NewStringPointer("hostip"),
					},
				},
			},
		},
	}
}

func newV2Agent(enabled bool, acm, registry string, cwsInstrumentationEnabled bool, apm *v2alpha1.APMFeatureConfig, dsd *v2alpha1.DogstatsdFeatureConfig, global *v2alpha1.GlobalConfig) *v2alpha1.DatadogAgent {
	dda := &v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Global: &v2alpha1.GlobalConfig{},
			Features: &v2alpha1.DatadogFeatures{
				AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
					Enabled:          apiutils.NewBoolPointer(enabled),
					MutateUnlabelled: apiutils.NewBoolPointer(true),
					ServiceName:      apiutils.NewStringPointer("testServiceName"),
					CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
						Enabled: apiutils.NewBoolPointer(cwsInstrumentationEnabled),
						Mode:    apiutils.NewStringPointer("test-mode"),
					},
				},
			},
		},
	}
	if acm != "" {
		dda.Spec.Features.AdmissionController.AgentCommunicationMode = apiutils.NewStringPointer(acm)
	}
	if apm != nil {
		dda.Spec.Features.APM = apm
	}
	if dsd != nil {
		dda.Spec.Features.Dogstatsd = dsd
	}
	if registry != "" {
		dda.Spec.Features.AdmissionController.Registry = apiutils.NewStringPointer(registry)

	}
	if global != nil {
		dda.Spec.Global = global
	}
	return dda
}

func testDCAResources(acm string, registry string, cwsInstrumentationEnabled bool) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.ClusterAgentContainerName]
			expectedAgentEnvs := []*corev1.EnvVar{
				{
					Name:  apicommon.DDAdmissionControllerEnabled,
					Value: "true",
				},
				{
					Name:  apicommon.DDAdmissionControllerMutateUnlabelled,
					Value: "true",
				},
				{
					Name:  apicommon.DDAdmissionControllerServiceName,
					Value: "testServiceName",
				},
				{
					Name:  apicommon.DDAdmissionControllerLocalServiceName,
					Value: "-agent",
				},
				{
					Name:  apicommon.DDAdmissionControllerWebhookName,
					Value: "datadog-webhook",
				},
			}
			if cwsInstrumentationEnabled {
				expectedAgentEnvs = append(expectedAgentEnvs, []*corev1.EnvVar{
					{
						Name:  apicommon.DDAdmissionControllerCWSInstrumentationEnabled,
						Value: apiutils.BoolToString(&cwsInstrumentationEnabled),
					},
					{
						Name:  apicommon.DDAdmissionControllerCWSInstrumentationMode,
						Value: "test-mode",
					},
				}...)
			}
			if acm != "" {
				acmEnv := corev1.EnvVar{
					Name:  apicommon.DDAdmissionControllerInjectConfigMode,
					Value: acm,
				}
				expectedAgentEnvs = append(expectedAgentEnvs, &acmEnv)
			}
			if registry != "" {
				registryEnv := corev1.EnvVar{
					Name:  apicommon.DDAdmissionControllerRegistryName,
					Value: registry,
				}
				expectedAgentEnvs = append(expectedAgentEnvs, &registryEnv)
			}

			assert.ElementsMatch(t,
				agentEnvs,
				expectedAgentEnvs,
				"Cluster Agent ENVs (-want +got):\n%s",
				cmp.Diff(agentEnvs, expectedAgentEnvs),
			)
		},
	)
}
