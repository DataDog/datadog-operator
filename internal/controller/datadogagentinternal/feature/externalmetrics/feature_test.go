// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package externalmetrics

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/test"
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

	secret := v2alpha1.DatadogCredentials{
		APIKey: apiutils.NewStringPointer("12345"),
		APISecret: &v2alpha1.SecretConfig{
			SecretName: secretName,
			KeyName:    apiKeyName,
		},
		AppKey: apiutils.NewStringPointer("09876"),
	}

	tests := test.FeatureTestSuite{
		{
			Name:          "external metrics not enabled",
			DDAI:          newAgent(false, true, false, false, v2alpha1.DatadogCredentials{}),
			WantConfigure: false,
		},
		{
			Name:          "external metrics enabled",
			DDAI:          newAgent(true, true, true, false, v2alpha1.DatadogCredentials{}),
			WantConfigure: true,
			ClusterAgent:  testDCAResources(true, false, false),
		},
		{
			Name:          "external metrics enabled, wpa controller enabled",
			DDAI:          newAgent(true, true, true, true, v2alpha1.DatadogCredentials{}),
			WantConfigure: true,
			ClusterAgent:  testDCAResources(true, true, false),
		},
		{
			Name:          "external metrics enabled, ddm disabled",
			DDAI:          newAgent(true, true, false, false, v2alpha1.DatadogCredentials{}),
			WantConfigure: true,
			ClusterAgent:  testDCAResources(false, false, false),
		},
		{
			Name:          "external metrics enabled, secrets set",
			DDAI:          newAgent(true, true, true, false, secret),
			WantConfigure: true,
			ClusterAgent:  testDCAResources(true, false, true),
		},
		{
			Name:          "external metrics enabled, secrets set, registerAPIService enabled",
			DDAI:          newAgent(true, true, true, false, secret),
			WantConfigure: true,
			WantDependenciesFunc: func(t testing.TB, store store.StoreClient) {
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
			Name:          "external metrics enabled, secrets set, registerAPIService disabled",
			DDAI:          newAgent(true, false, true, false, secret),
			WantConfigure: true,
			WantDependenciesFunc: func(t testing.TB, store store.StoreClient) {
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

func newAgent(enabled, registerAPIService, useDDM, wpaController bool, secret v2alpha1.DatadogCredentials) *v1alpha1.DatadogAgentInternal {
	return &v1alpha1.DatadogAgentInternal{
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

			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName]
			expectedAgentEnvs := []*corev1.EnvVar{
				{
					Name:  DDExternalMetricsProviderEnabled,
					Value: "true",
				},
				{
					Name:  DDExternalMetricsProviderPort,
					Value: "8443",
				},
				{
					Name:  DDExternalMetricsProviderUseDatadogMetric,
					Value: apiutils.BoolToString(&useDDM),
				},
				{
					Name:  DDExternalMetricsProviderWPAController,
					Value: apiutils.BoolToString(&wpaController),
				},
			}
			if keySecrets {
				secretEnvs := []*corev1.EnvVar{
					{
						Name: DDExternalMetricsProviderAPIKey,
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
						Name: DDExternalMetricsProviderAppKey,
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "-metrics-server", // from default secret name
								},
								Key: v2alpha1.DefaultAPPKeyKey,
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

			agentPorts := mgr.PortMgr.PortsByC[apicommon.ClusterAgentContainerName]
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
