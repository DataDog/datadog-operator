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
	customPath         = "/custom/host/filepath.sock"
)

func Test_admissionControllerFeature_Configure(t *testing.T) {
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
			Name:          "v1alpha1 admission controller enabled",
			DDAv1:         newV1Agent(true),
			WantConfigure: true,
			ClusterAgent:  testDCAResources("hostip"),
		},
		//////////////////////////
		// v2Alpha1.DatadogAgent
		//////////////////////////
		{
			Name: "v2alpha1 Admission Controller not enabled",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "v2alpha1 Admission Controller enabled with basic setup",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				admissionControllerWantFunc(v2alpha1.AdmissionControllerFeatureConfig{
					Enabled: apiutils.NewBoolPointer(true),
				}),
			),
		},
		{
			Name: "v2alpha1 Admission Controller enabled with communication mode, custom service name, webhook name, registry, and failure policy",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithMutateUnlabelled(true).
				WithAgentCommunicationMode("testCommunicationMode").
				WithServiceName("testServiceName").
				WithWebhookName("testWebhookName").
				WithAdmissionControllerRegistry("testRegistry").
				WithAdmissionControllerFailurePolicy("testFailurePolicy").
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				admissionControllerWantFunc(v2alpha1.AdmissionControllerFeatureConfig{
					Enabled:                apiutils.NewBoolPointer(true),
					MutateUnlabelled:       apiutils.NewBoolPointer(true),
					AgentCommunicationMode: apiutils.NewStringPointer("testCommunicationMode"),
					ServiceName:            apiutils.NewStringPointer("testServiceName"),
					WebhookName:            apiutils.NewStringPointer("testWebhookName"),
					Registry:               apiutils.NewStringPointer("testRegistry"),
					FailurePolicy:          apiutils.NewStringPointer("testFailurePolicy"),
				}),
			),
		},
		{
			Name: "v2alpha1 Admission Controller enabled with APM uds mode",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithAPMEnabled(true).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				admissionControllerWantFunc(v2alpha1.AdmissionControllerFeatureConfig{
					Enabled:                apiutils.NewBoolPointer(true),
					AgentCommunicationMode: apiutils.NewStringPointer("apm"),
				}),
			),
		},
		{
			Name: "v2alpha1 Admission Controller enabled with DSD uds mode",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithDogstatsdUnixDomainSocketConfigEnabled(true).
				WithDogstatsdUnixDomainSocketConfigPath(customPath).
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				admissionControllerWantFunc(v2alpha1.AdmissionControllerFeatureConfig{
					Enabled:                apiutils.NewBoolPointer(true),
					AgentCommunicationMode: apiutils.NewStringPointer("dsd"),
				}),
			),
		},
		{
			Name: "v2alpha1 Admission Controller enabled with sidecar injection enabled",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithSidecarInjectionSetup(v2alpha1.AgentSidecarInjectionConfig{
					Enabled:                          apiutils.NewBoolPointer(true),
					ClusterAgentCommunicationEnabled: apiutils.NewBoolPointer(true),
					Provider:                         apiutils.NewStringPointer("testProvider"),
					Registry:                         apiutils.NewStringPointer("testRegistryName"),
					Image: &apicommonv1.AgentImageConfig{
						Name: "agent",
						Tag:  "7.53.0",
					},
				}).
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				admissionControllerWantFunc(v2alpha1.AdmissionControllerFeatureConfig{
					Enabled: apiutils.NewBoolPointer(true),
					AgentSidecarInjection: &v2alpha1.AgentSidecarInjectionConfig{
						Enabled:                          apiutils.NewBoolPointer(true),
						ClusterAgentCommunicationEnabled: apiutils.NewBoolPointer(true),
						Provider:                         apiutils.NewStringPointer("testProvider"),
						Registry:                         apiutils.NewStringPointer("testRegistryName"),
						Image: &apicommonv1.AgentImageConfig{
							Name: "agent",
							Tag:  "7.53.0",
						},
					},
				}),
			),
		},
	}

	tests.Run(t, buildAdmissionControllerFeature)
}

func generateEnvVars(config v2alpha1.AdmissionControllerFeatureConfig) []*corev1.EnvVar {
	envVars := []*corev1.EnvVar{
		{
			Name:  apicommon.DDAdmissionControllerEnabled,
			Value: "true",
		},
	}

	if config.MutateUnlabelled != nil && *config.MutateUnlabelled {
		envVars = append(envVars, &corev1.EnvVar{
			Name:  apicommon.DDAdmissionControllerMutateUnlabelled,
			Value: "true",
		})
	} else {
		envVars = append(envVars, &corev1.EnvVar{
			Name:  apicommon.DDAdmissionControllerMutateUnlabelled,
			Value: "false",
		})
	}

	if config.Registry != nil && *config.Registry != "" {
		serviceEnvVars := &corev1.EnvVar{
			Name:  apicommon.DDAdmissionControllerRegistryName,
			Value: *config.Registry,
		}
		envVars = append(envVars, serviceEnvVars)
	}

	if config.ServiceName != nil && *config.ServiceName != "" {
		serviceEnvVars := &corev1.EnvVar{
			Name:  apicommon.DDAdmissionControllerServiceName,
			Value: *config.ServiceName,
		}
		envVars = append(envVars, serviceEnvVars)
	}

	if config.AgentCommunicationMode != nil && *config.AgentCommunicationMode != "" {
		configMode := *config.AgentCommunicationMode
		if configMode == "apm" || configMode == "dsd" {
			configModeEnvVars := &corev1.EnvVar{
				Name:  apicommon.DDAdmissionControllerInjectConfigMode,
				Value: "socket",
			}
			envVars = append(envVars, configModeEnvVars)
		} else {
			configModeEnvVars := &corev1.EnvVar{
				Name:  apicommon.DDAdmissionControllerInjectConfigMode,
				Value: configMode,
			}
			envVars = append(envVars, configModeEnvVars)
		}
	}

	envVars = append(envVars, &corev1.EnvVar{Name: apicommon.DDAdmissionControllerLocalServiceName, Value: "-agent"})

	if config.FailurePolicy != nil && *config.FailurePolicy != "" {
		serviceEnvVars := &corev1.EnvVar{
			Name:  apicommon.DDAdmissionControllerFailurePolicy,
			Value: *config.FailurePolicy,
		}
		envVars = append(envVars, serviceEnvVars)
	}
	if config.WebhookName != nil && *config.WebhookName != "" {
		webhookEnvVars := &corev1.EnvVar{
			Name:  apicommon.DDAdmissionControllerWebhookName,
			Value: *config.WebhookName,
		}
		envVars = append(envVars, webhookEnvVars)
	} else {
		webhookEnvVars := &corev1.EnvVar{
			Name:  apicommon.DDAdmissionControllerWebhookName,
			Value: "datadog-webhook",
		}
		envVars = append(envVars, webhookEnvVars)
	}

	if config.AgentSidecarInjection != nil && config.AgentSidecarInjection.Enabled != nil && *config.AgentSidecarInjection.Enabled {
		envVars = append(envVars, []*corev1.EnvVar{
			{
				Name:  apicommon.DDAdmissionControllerAgentSidecarEnabled,
				Value: "true",
			},
			{
				Name:  apicommon.DDAdmissionControllerAgentSidecarClusterAgentEnabled,
				Value: "true",
			},
			{
				Name:  apicommon.DDAdmissionControllerAgentSidecarProvider,
				Value: "testProvider",
			},
			{
				Name:  apicommon.DDAdmissionControllerAgentSidecarRegistry,
				Value: "testRegistryName",
			},
			{
				Name:  apicommon.DDAdmissionControllerAgentSidecarImageName,
				Value: "agent",
			},
			{
				Name:  apicommon.DDAdmissionControllerAgentSidecarImageTag,
				Value: "7.53.0",
			},
		}...)
	}

	return envVars
}

func admissionControllerWantFunc(config v2alpha1.AdmissionControllerFeatureConfig) func(testing.TB, feature.PodTemplateManagers) {
	return func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)
		dcaEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.ClusterAgentContainerName]
		want := generateEnvVars(config)
		assert.True(
			t,
			apiutils.IsEqualStruct(dcaEnvVars, want),
			"DCA envvars \ndiff = %s", cmp.Diff(dcaEnvVars, want),
		)
	}
}

func newV1Agent(enabled bool) *v1alpha1.DatadogAgent {
	return &v1alpha1.DatadogAgent{
		Spec: v1alpha1.DatadogAgentSpec{
			ClusterAgent: v1alpha1.DatadogAgentSpecClusterAgentSpec{
				Config: &v1alpha1.ClusterAgentConfig{
					AdmissionController: &v1alpha1.AdmissionControllerConfig{
						Enabled:                apiutils.NewBoolPointer(enabled),
						MutateUnlabelled:       apiutils.NewBoolPointer(false),
						ServiceName:            apiutils.NewStringPointer("testServiceName"),
						AgentCommunicationMode: apiutils.NewStringPointer("hostip"),
					},
				},
			},
		},
	}
}

func testDCAResources(acm string) *test.ComponentTest {
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
					Value: "false",
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
			if acm != "" {
				acmEnv := corev1.EnvVar{
					Name:  apicommon.DDAdmissionControllerInjectConfigMode,
					Value: acm,
				}
				expectedAgentEnvs = append(expectedAgentEnvs, &acmEnv)
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
