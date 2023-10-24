// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package externalmetrics

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/dependencies"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/test"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

const (
	secretName = "apisecretname"
	apiKeyName = "apikeyname"
)

func TestExternalMetricsFeature(t *testing.T) {
	// secretV1 := v1alpha1.DatadogCredentials{
	// 	APISecret: &apicommonv1.SecretConfig{
	// 		SecretName: secretName,
	// 		KeyName: apiKeyName,
	// 	},
	// 	APPSecret: &apicommonv1.SecretConfig{
	// 		SecretName: secretName,
	// 		KeyName: appKeyName,
	// 	},
	// }
	secretV2 := v2alpha1.DatadogCredentials{
		APIKey: apiutils.NewStringPointer("12345"),
		APISecret: &apicommonv1.SecretConfig{
			SecretName: secretName,
			KeyName:    apiKeyName,
		},
		AppKey: apiutils.NewStringPointer("09876"),
	}

	tests := test.FeatureTestSuite{
		//////////////////////////
		// v1Alpha1.DatadogAgent
		//////////////////////////
		// {
		// 	Name:          "v1alpha1 external metrics not enabled",
		// 	DDAv1:         newV1Agent(false, false, false, v1alpha1.DatadogCredentials{}),
		// 	WantConfigure: false,
		// },
		// {
		// 	Name:          "v1alpha1 external metrics enabled",
		// 	DDAv1:         newV1Agent(true, true, false, v1alpha1.DatadogCredentials{}),
		// 	WantConfigure: true,
		// 	ClusterAgent:  testDCAResources(true, false, false),
		// },
		// {
		// 	Name:          "v1alpha1 external metrics enabled, wpa controller enabled",
		// 	DDAv1:         newV1Agent(true, true, true, v1alpha1.DatadogCredentials{}),
		// 	WantConfigure: true,
		// 	ClusterAgent:  testDCAResources(true, true, false),
		// },
		// {
		// 	Name:          "v1alpha1 external metrics enabled, ddm disabled",
		// 	DDAv1:         newV1Agent(true, false, false, v1alpha1.DatadogCredentials{}),
		// 	WantConfigure: true,
		// 	ClusterAgent:  testDCAResources(false, false, false),
		// },
		// {
		// 	Name:          "v1alpha1 external metrics enabled, keys set",
		// 	DDAv1:         newV1Agent(true, true, false, secretV1),
		// 	WantConfigure: true,
		// 	ClusterAgent:  testDCAResources(true, false, true),
		// },

		//////////////////////////
		// v2Alpha1.DatadogAgent
		//////////////////////////
		{
			Name:          "v2alpha1 external metrics not enabled",
			DDAv2:         newV2Agent(false, true, false, false, v2alpha1.DatadogCredentials{}),
			WantConfigure: false,
		},
		{
			Name:          "v2alpha1 external metrics enabled",
			DDAv2:         newV2Agent(true, true, true, false, v2alpha1.DatadogCredentials{}),
			WantConfigure: true,
			ClusterAgent:  testDCAResources(true, false, false),
		},
		{
			Name:          "v2alpha1 external metrics enabled, wpa controller enabled",
			DDAv2:         newV2Agent(true, true, true, true, v2alpha1.DatadogCredentials{}),
			WantConfigure: true,
			ClusterAgent:  testDCAResources(true, true, false),
		},
		{
			Name:          "v2alpha1 external metrics enabled, ddm disabled",
			DDAv2:         newV2Agent(true, true, false, false, v2alpha1.DatadogCredentials{}),
			WantConfigure: true,
			ClusterAgent:  testDCAResources(false, false, false),
		},
		{
			Name:          "v2alpha1 external metrics enabled, secrets set",
			DDAv2:         newV2Agent(true, true, true, false, secretV2),
			WantConfigure: true,
			ClusterAgent:  testDCAResources(true, false, true),
		},
		{
			Name:          "v2alpha1 external metrics enabled, secrets set, registerAPIService enabled",
			DDAv2:         newV2Agent(true, true, true, false, secretV2),
			WantConfigure: true,
			WantDependenciesFunc: func(t testing.TB, store dependencies.StoreClient) {
				apiServiceName := "v1beta1.external.metrics.k8s.io"
				ns := ""

				_, found := store.Get(kubernetes.APIServiceKind, ns, apiServiceName)
				if !found {
					t.Error("Should have created an APIService")
				}
			},
			ClusterAgent: testDCAResources(true, false, true),
		},
		{
			Name:          "v2alpha1 external metrics enabled, secrets set, registerAPIService disabled",
			DDAv2:         newV2Agent(true, false, true, false, secretV2),
			WantConfigure: true,
			WantDependenciesFunc: func(t testing.TB, store dependencies.StoreClient) {
				apiServiceName := "v1beta1.external.metrics.k8s.io"
				ns := ""

				_, found := store.Get(kubernetes.APIServiceKind, ns, apiServiceName)
				if found {
					t.Error("Shouldn't have created an APIService")
				}
			},
			ClusterAgent: testDCAResources(true, false, true),
		},
	}

	tests.Run(t, buildExternalMetricsFeature)
}

// func newV1Agent(enabled, useDDM, wpaController bool, secret v1alpha1.DatadogCredentials) *v1alpha1.DatadogAgent {
// 	return &v1alpha1.DatadogAgent{
// 		Spec: v1alpha1.DatadogAgentSpec{
// 			ClusterAgent: v1alpha1.DatadogAgentSpecClusterAgentSpec{
// 				Config: &v1alpha1.ClusterAgentConfig{
// 					ExternalMetrics: &v1alpha1.ExternalMetricsConfig{
// 						Enabled:  apiutils.NewBoolPointer(enabled),
// 						WpaController: wpaController,
// 						UseDatadogMetrics: useDDM,
// 						Port: apiutils.NewInt32Pointer(8443),
// 						Credentials: &secret,
// 					},
// 				},
// 			},
// 		},
// 	}
// }

func newV2Agent(enabled, registerAPIService, useDDM, wpaController bool, secret v2alpha1.DatadogCredentials) *v2alpha1.DatadogAgent {
	return &v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
					Enabled:            apiutils.NewBoolPointer(enabled),
					RegisterAPIService: apiutils.NewBoolPointer(registerAPIService),
					WPAController:      apiutils.NewBoolPointer(wpaController),
					UseDatadogMetrics:  apiutils.NewBoolPointer(useDDM),
					Port:               apiutils.NewInt32Pointer(8443),
					Endpoint: &v2alpha1.Endpoint{
						Credentials: &secret,
					},
				},
			},
		},
	}
}

func testDCAResources(useDDM, wpaController, keySecrets bool) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.ClusterAgentContainerName]
			expectedAgentEnvs := []*corev1.EnvVar{
				{
					Name:  apicommon.DDExternalMetricsProviderEnabled,
					Value: "true",
				},
				{
					Name:  apicommon.DDExternalMetricsProviderPort,
					Value: "8443",
				},
				{
					Name:  apicommon.DDExternalMetricsProviderUseDatadogMetric,
					Value: apiutils.BoolToString(&useDDM),
				},
				{
					Name:  apicommon.DDExternalMetricsProviderWPAController,
					Value: apiutils.BoolToString(&wpaController),
				},
			}
			if keySecrets {
				secretEnvs := []*corev1.EnvVar{
					{
						Name: apicommon.DDExternalMetricsProviderAPIKey,
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: secretName,
								},
								Key: apiKeyName,
							},
						},
					},
					{
						Name: apicommon.DDExternalMetricsProviderAppKey,
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "-metrics-server", // from default secret name
								},
								Key: apicommon.DefaultAPPKeyKey,
							},
						},
					},
				}
				expectedAgentEnvs = append(expectedAgentEnvs, secretEnvs...)
			}
			assert.True(
				t,
				apiutils.IsEqualStruct(agentEnvs, expectedAgentEnvs),
				"Cluster Agent ENVs \ndiff = %s", cmp.Diff(agentEnvs, expectedAgentEnvs),
			)

			agentPorts := mgr.PortMgr.PortsByC[apicommonv1.ClusterAgentContainerName]
			expectedPorts := []*corev1.ContainerPort{
				{
					Name:          "metricsapi",
					ContainerPort: 8443,
					Protocol:      corev1.ProtocolTCP,
				},
			}
			assert.True(
				t,
				apiutils.IsEqualStruct(agentPorts, expectedPorts),
				"Cluster Agent Ports \ndiff = %s", cmp.Diff(agentPorts, expectedPorts),
			)
		},
	)
}
