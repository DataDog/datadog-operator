// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package admissioncontroller

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
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
	tests := test.FeatureTestSuite{
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
			ClusterAgent:  testBasicAdmissionController(),
		},
		{
			Name: "v2alpha1 Admission Controller enabled with full configuration",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithMutateUnlabelled(true).
				WithServiceName("testServiceName").
				WithAgentCommunicationMode("testConfigMode").
				WithWebhookName("testWebhookName").
				Build(),
			WantConfigure: true,
			ClusterAgent:  testAdmissionControllerFull(),
		},
		{
			Name: "v2alpha1 Admission Controller enabled with enabled with APM uds mode",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithAgentCommunicationMode("socket").
				WithAPMEnabled(true).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				Build(),
			WantConfigure: true,
			ClusterAgent:  testAdmissionControllerWithAPMUsingUDS(),
		},
		{
			Name: "v2alpha1 Admission Controller enabled with DSD uds mode",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithAgentCommunicationMode("socket").
				WithDogstatsdUnixDomainSocketConfigEnabled(true).
				WithDogstatsdUnixDomainSocketConfigPath(customPath).
				Build(),
			WantConfigure: true,
			ClusterAgent:  testAdmissionControllerWithDSDUsingUDS(),
		},
		{
			Name: "v2alpha1 Admission Controller enabled with sidecar injection enabled",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithSidecarInjectionEnabled(true).
				Build(),
			WantConfigure: true,
			ClusterAgent:  testSidecarInjection(),
		},
	}
	tests.Run(t, buildAdmissionControllerFeature)
}

func testBasicAdmissionController() *test.ComponentTest {
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
					Name:  apicommon.DDAdmissionControllerLocalServiceName,
					Value: "-agent",
				},
				{
					Name:  apicommon.DDAdmissionControllerWebhookName,
					Value: "datadog-webhook",
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

func testAdmissionControllerFull() *test.ComponentTest {
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
					Value: "testConfigMode",
				},
				{
					Name:  apicommon.DDAdmissionControllerLocalServiceName,
					Value: "-agent",
				},
				{
					Name:  apicommon.DDAdmissionControllerWebhookName,
					Value: "testWebhookName",
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

func testAdmissionControllerWithAPMUsingUDS() *test.ComponentTest {
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
					Name:  apicommon.DDAdmissionControllerInjectConfigMode,
					Value: "socket",
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
			assert.True(
				t,
				apiutils.IsEqualStruct(agentEnvs, expectedAgentEnvs),
				"Cluster Agent ENVs \ndiff = %s", cmp.Diff(agentEnvs, expectedAgentEnvs),
			)
		},
	)
}

func testAdmissionControllerWithDSDUsingUDS() *test.ComponentTest {
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
					Name:  apicommon.DDAdmissionControllerInjectConfigMode,
					Value: "socket",
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
			assert.True(
				t,
				apiutils.IsEqualStruct(agentEnvs, expectedAgentEnvs),
				"Cluster Agent ENVs \ndiff = %s", cmp.Diff(agentEnvs, expectedAgentEnvs),
			)
		},
	)
}

func testSidecarInjection() *test.ComponentTest {
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
					Name:  apicommon.DDAdmissionControllerLocalServiceName,
					Value: "-agent",
				},
				{
					Name:  apicommon.DDAdmissionControllerWebhookName,
					Value: "datadog-webhook",
				},
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
				{
					Name:  apicommon.DDAdmissionControllerAgentSidecarImageTag,
					Value: "7.53.0",
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
