// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package highavailability

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	v2alpha1test "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1/test"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/test"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestHAFeature(t *testing.T) {
	tests := test.FeatureTestSuite{
		//////////////////////////
		// v2Alpha1.DatadogAgent
		//////////////////////////
		{
			Name: "v2alpha1 high availability not enabled",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithHighAvailabilityEnabled(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "v2alpha1 high availability not enabled, with multi-process container",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithHighAvailabilityEnabled(false).
				WithMultiProcessContainer(true).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "v2alpha1 high availability enabled",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithHighAvailabilityEnabled(true).
				Build(),
			WantConfigure: true,
			Agent:         testAgentEnabled(apicommonv1.CoreAgentContainerName),
		},
		{
			Name: "v2alpha1 high availability enabled, with multi-process container",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithHighAvailabilityEnabled(true).
				WithMultiProcessContainer(true).
				Build(),
			WantConfigure: true,
			Agent:         testAgentEnabled(apicommonv1.UnprivilegedMultiProcessAgentContainerName),
		},
	}

	tests.Run(t, buildHighAvailabilityFeature)
}

func testAgentEnabled(agentContainerName apicommonv1.AgentContainerName) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			actualAgentEnvs := mgr.EnvVarMgr.EnvVarsByC[agentContainerName]

			// We always care about the OPW configuration being set when HA is enabled.
			expectedAgentEnvs := []*corev1.EnvVar{
				{
					Name:  apicommon.DDObservabilityPipelinesWorkerMetricsEnabled,
					Value: "true",
				},
				{
					Name:  apicommon.DDObservabilityPipelinesWorkerMetricsUrl,
					Value: apicommon.DefaultHAUrl,
				},
				{
					Name:  apicommon.DDObservabilityPipelinesWorkerLogsEnabled,
					Value: "true",
				},
				{
					Name:  apicommon.DDObservabilityPipelinesWorkerLogsUrl,
					Value: apicommon.DefaultHAUrl,
				},
			}

			// We only care about the environment variable for enabling HA (aka ADP) in
			// multi-process mode, when supervisord is running each process.
			if agentContainerName == apicommonv1.UnprivilegedMultiProcessAgentContainerName {
				expectedAgentEnvs = append(expectedAgentEnvs, &corev1.EnvVar{
					Name:  apicommon.DDHAEnabled,
					Value: "true",
				})
			}

			assert.True(
				t,
				containsEnvVarSubset(actualAgentEnvs, expectedAgentEnvs),
				"Core Agent ENVs \ndiff = %s", cmp.Diff(actualAgentEnvs, expectedAgentEnvs),
			)
		},
	)
}

func containsEnvVarSubset(source []*corev1.EnvVar, subset []*corev1.EnvVar) bool {
	// We only support matching against static values rather than computed values.
	for _, otherVar := range subset {
		evContained := false

		for _, sourceVar := range source {
			if sourceVar.Name == otherVar.Name && sourceVar.Value == otherVar.Value {
				evContained = true
				break
			}
		}

		if !evContained {
			return false
		}
	}

	return true
}
