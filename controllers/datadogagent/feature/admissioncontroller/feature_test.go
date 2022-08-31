// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package admissioncontroller

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/test"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestExternalMetricsFeature(t *testing.T) {
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
		// 	ClusterAgent:  testDCAResources(),
		// },

		//////////////////////////
		// v2Alpha1.DatadogAgent
		//////////////////////////
		{
			Name:          "v2alpha1 admission controller not enabled",
			DDAv2:         newV2Agent(false),
			WantConfigure: false,
		},
		{
			Name:          "v2alpha1 admission controller enabled",
			DDAv2:         newV2Agent(true),
			WantConfigure: true,
			ClusterAgent:  testDCAResources(),
		},
	}

	tests.Run(t, buildAdmissionControllerFeature)
}

// func newV1Agent(enabled bool) *v1alpha1.DatadogAgent {
// 	return &v1alpha1.DatadogAgent{
// 		Spec: v1alpha1.DatadogAgentSpec{
// 			ClusterAgent: v1alpha1.DatadogAgentSpecClusterAgentSpec{
// 				Config: &v1alpha1.ClusterAgentConfig{
// 					AdmissionController: &v1alpha1.AdmissionControllerConfig{
// 						Enabled:                apiutils.NewBoolPointer(enabled),
// 						MutateUnlabelled:       apiutils.NewBoolPointer(true),
// 						ServiceName:            apiutils.NewStringPointer("testServiceName"),
// 						AgentCommunicationMode: apiutils.NewStringPointer("hostip"),
// 					},
// 				},
// 			},
// 		},
// 	}
// }

func newV2Agent(enabled bool) *v2alpha1.DatadogAgent {
	return &v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
					Enabled:                apiutils.NewBoolPointer(enabled),
					MutateUnlabelled:       apiutils.NewBoolPointer(true),
					ServiceName:            apiutils.NewStringPointer("testServiceName"),
					AgentCommunicationMode: apiutils.NewStringPointer("hostip"),
				},
			},
			Global: &v2alpha1.GlobalConfig{},
		},
	}
}

func testDCAResources() *test.ComponentTest {
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
					Name:  apicommon.DDAdmissionControllerInjectConfigMode,
					Value: "hostip",
				},
				{
					Name:  apicommon.DDAdmissionControllerLocalServiceName,
					Value: "-agent",
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
