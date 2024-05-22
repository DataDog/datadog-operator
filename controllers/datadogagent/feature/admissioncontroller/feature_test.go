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
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/test"
)

const (
	apmSocketHostPath  = apicommon.DogstatsdAPMSocketHostPath + "/" + apicommon.APMSocketName
	apmSocketLocalPath = apicommon.APMSocketVolumeLocalPath + "/" + apicommon.APMSocketName
	customPath         = "/custom/host/filepath.sock"
)

func Test_admissionControllerFeature_Configure(t *testing.T) {
	globalConfig := &v2alpha1.GlobalConfig{
		Registry: apiutils.NewStringPointer("globalRegistryName"),
	}
	tests := test.FeatureTestSuite{
		//////////////////////////
		// v1Alpha1.DatadogAgent
		//////////////////////////
		// {
		// 	Name:          "v1alpha1 admission controller not enabled",
		// 	DDAv1:         newV1Agent(false),
		// 	WantConfigure: false,
		// },
		// {
		// 	Name:          "v1alpha1 admission controller enabled",
		// 	DDAv1:         newV1Agent(true),
		// 	WantConfigure: true,
		// 	ClusterAgent:  testDCAResources("hostip"),
		// },
		// //////////////////////////
		// // v2Alpha1.DatadogAgent
		// //////////////////////////
		{
			Name: "v2alpha1 Admission Controller not enabled",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "v2alpha1 Admission Controller enabled",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				admissionControllerWantFunc(false, false, "", "", ""),
			),
		},

		//////////////////////////
		// v2Alpha1.DatadogAgent
		//////////////////////////
		{
			Name:  "v2alpha1 admission controller not enabled",
			DDAv2: newV2Agent(false, "", "", &v2alpha1.APMFeatureConfig{}, &v2alpha1.DogstatsdFeatureConfig{}, nil),
			Name:  "v2alpha1 Admission Controller not enabled",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "v2alpha1 Admission Controller enabled with config mode",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithAgentCommunicationMode("service").
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				admissionControllerWantFunc(false, false, "service", "", ""),
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
				admissionControllerWantFunc(false, false, "apm", "", ""),
			),
		},
		{
			Name:          "v2alpha1 admission controller enabled, dsd uses uds",
			DDAv2:         newV2Agent(true, "", &v2alpha1.APMFeatureConfig{}, dsdUDS),
			WantConfigure: true,
			ClusterAgent:  testSidecarInjection(),
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
						MutateUnlabelled:       apiutils.NewBoolPointer(false),
						ServiceName:            apiutils.NewStringPointer("testServiceName"),
						AgentCommunicationMode: apiutils.NewStringPointer("hostip"),
					},
				},
			},
		},
	}
}

func testDCAResources(acm string, registry string) *test.ComponentTest {
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

func generateEnvVars(mutate, sidecar bool, configMode, serviceName, webhookName string) []*corev1.EnvVar {
	envVars := []*corev1.EnvVar{
		{
			Name:  apicommon.DDAdmissionControllerEnabled,
			Value: "true",
		},
	}
	if mutate {
		envVars = append(envVars, []*corev1.EnvVar{
			{
				Name:  apicommon.DDAdmissionControllerMutateUnlabelled,
				Value: "true",
			},
		}...)
	} else {
		envVars = append(envVars, []*corev1.EnvVar{
			{
				Name:  apicommon.DDAdmissionControllerMutateUnlabelled,
				Value: "false",
			},
		}...)
	}
	if configMode != "" {
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
		envVars = append(envVars, &corev1.EnvVar{Name: apicommon.DDAdmissionControllerLocalServiceName, Value: "-agent"})
		if webhookName != "" {
			webhookEnvVars := &corev1.EnvVar{
				Name:  apicommon.DDAdmissionControllerWebhookName,
				Value: webhookName,
			}
			envVars = append(envVars, webhookEnvVars)
		} else {
			webhookEnvVars := &corev1.EnvVar{
				Name:  apicommon.DDAdmissionControllerWebhookName,
				Value: "datadog-webhook",
			}
			envVars = append(envVars, webhookEnvVars)
		}
		if serviceName != "" {
			serviceEnvVars := &corev1.EnvVar{
				Name:  apicommon.DDAdmissionControllerServiceName,
				Value: serviceName,
			}
			envVars = append(envVars, serviceEnvVars)
		}

		if sidecar {
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
					Name:  apicommon.DDAdmissionControllerAgentSidecarImageName,
					Value: "agent",
				},
			}...)
		}
	}
	return envVars
}

func admissionControllerWantFunc(mutate, sidecar bool, configMode, serviceName, webhookName string) func(testing.TB, feature.PodTemplateManagers) {
	return func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)
		dcaEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.ClusterAgentContainerName]
		want := generateEnvVars(mutate, sidecar, configMode, serviceName, webhookName)
		assert.True(
			t,
			apiutils.IsEqualStruct(dcaEnvVars, want),
			"DCA envvars \ndiff = %s", cmp.Diff(dcaEnvVars, want),
		)
	}
}
