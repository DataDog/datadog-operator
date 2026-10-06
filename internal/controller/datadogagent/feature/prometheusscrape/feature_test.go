// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package prometheusscrape

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	"github.com/DataDog/datadog-operator/pkg/testutils"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func Test_prometheusScrapeFeature_Configure(t *testing.T) {
	yamlConfigs := `
- 
  autodiscovery:
    kubernetes_annotations:
      exclude:
        custom_exclude_label: 'true'
      include:
        custom_include_label: 'true'
    kubernetes_container_names:
    - my-app
  configurations:
  - send_distribution_buckets: true
  timeout: 5`
	jsonConfigs := `[{"autodiscovery":{"kubernetes_annotations":{"exclude":{"custom_exclude_label":"true"},"include":{"custom_include_label":"true"}},"kubernetes_container_names":["my-app"]},"configurations":[{"send_distribution_buckets":true}],"timeout":5}]`

	tests := test.FeatureTestSuite{
		{
			Name: "Prometheus scrape not enabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithPrometheusScrapeEnabled(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "Prometheus scrape enabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithPrometheusScrapeEnabled(true).
				Build(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					wantEnvVars := []*corev1.EnvVar{
						{
							Name:  DDPrometheusScrapeEnabled,
							Value: "true",
						},
						{
							Name:  DDPrometheusScrapeServiceEndpoints,
							Value: "false",
						},
					}
					assertContainerEnvVars(t, mgrInterface, apicommon.CoreAgentContainerName, wantEnvVars)
				},
			),
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					wantEnvVars := []*corev1.EnvVar{
						{
							Name:  DDPrometheusScrapeEnabled,
							Value: "true",
						},
						{
							Name:  DDPrometheusScrapeServiceEndpoints,
							Value: "false",
						},
					}
					assertContainerEnvVars(t, mgrInterface, apicommon.ClusterAgentContainerName, wantEnvVars)
				},
			),
		},
		{
			Name: "Prometheus scrape service endpoints enabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithPrometheusScrapeEnabled(true).
				WithPrometheusScrapeServiceEndpoints(true).
				Build(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					wantEnvVars := []*corev1.EnvVar{
						{
							Name:  DDPrometheusScrapeEnabled,
							Value: "true",
						},
						{
							Name:  DDPrometheusScrapeServiceEndpoints,
							Value: "true",
						},
					}
					assertContainerEnvVars(t, mgrInterface, apicommon.CoreAgentContainerName, wantEnvVars)
				},
			),
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					wantEnvVars := []*corev1.EnvVar{
						{
							Name:  DDPrometheusScrapeEnabled,
							Value: "true",
						},
						{
							Name:  DDPrometheusScrapeServiceEndpoints,
							Value: "true",
						},
					}
					assertContainerEnvVars(t, mgrInterface, apicommon.ClusterAgentContainerName, wantEnvVars)
				},
			),
		},
		{
			Name: "Prometheus scrape additional configs",
			DDA: testutils.NewDatadogAgentBuilder().
				WithPrometheusScrapeEnabled(true).
				WithPrometheusScrapeAdditionalConfigs(yamlConfigs).
				Build(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					wantEnvVars := []*corev1.EnvVar{
						{
							Name:  DDPrometheusScrapeEnabled,
							Value: "true",
						},
						{
							Name:  DDPrometheusScrapeServiceEndpoints,
							Value: "false",
						},
						{
							Name:  DDPrometheusScrapeChecks,
							Value: jsonConfigs,
						},
					}
					assertContainerEnvVars(t, mgrInterface, apicommon.CoreAgentContainerName, wantEnvVars)
				},
			),
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					wantEnvVars := []*corev1.EnvVar{
						{
							Name:  DDPrometheusScrapeEnabled,
							Value: "true",
						},
						{
							Name:  DDPrometheusScrapeServiceEndpoints,
							Value: "false",
						},
						{
							Name:  DDPrometheusScrapeChecks,
							Value: jsonConfigs,
						},
					}
					assertContainerEnvVars(t, mgrInterface, apicommon.ClusterAgentContainerName, wantEnvVars)
				},
			),
		},
		{
			Name: "version specified",
			DDA: testutils.NewDatadogAgentBuilder().
				WithPrometheusScrapeEnabled(true).
				WithPrometheusScrapeVersion(1).
				Build(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					wantEnvVars := []*corev1.EnvVar{
						{
							Name:  DDPrometheusScrapeEnabled,
							Value: "true",
						},
						{
							Name:  DDPrometheusScrapeServiceEndpoints,
							Value: "false",
						},
						{
							Name:  DDPrometheusScrapeVersion,
							Value: "1",
						},
					}
					assertContainerEnvVars(t, mgrInterface, apicommon.CoreAgentContainerName, wantEnvVars)
				},
			),
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					wantEnvVars := []*corev1.EnvVar{
						{
							Name:  DDPrometheusScrapeEnabled,
							Value: "true",
						},
						{
							Name:  DDPrometheusScrapeServiceEndpoints,
							Value: "false",
						},
						{
							Name:  DDPrometheusScrapeVersion,
							Value: "1",
						},
					}
					assertContainerEnvVars(t, mgrInterface, apicommon.ClusterAgentContainerName, wantEnvVars)
				},
			),
		},
		{
			Name: "Prometheus scrape enabled but cluster agent disabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithPrometheusScrapeEnabled(true).
				WithClusterAgentDisabled(true).
				Build(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					wantEnvVars := []*corev1.EnvVar{
						{
							Name:  DDPrometheusScrapeEnabled,
							Value: "true",
						},
						{
							Name:  DDPrometheusScrapeServiceEndpoints,
							Value: "false",
						},
					}
					assertContainerEnvVars(t, mgrInterface, apicommon.CoreAgentContainerName, wantEnvVars)
				},
			),
		},
	}

	tests.Run(t, buildPrometheusScrapeFeature)
}

// Test_prometheusScrapeFeature_Configure_ClusterAgentDisabled verifies that, when the cluster
// agent is disabled via override, Configure does not require it or declare it configured, even
// though Prometheus scrape is enabled and still runs node-side.
func Test_prometheusScrapeFeature_Configure_ClusterAgentDisabled(t *testing.T) {
	dda := testutils.NewDatadogAgentBuilder().
		WithPrometheusScrapeEnabled(true).
		WithClusterAgentDisabled(true).
		Build()

	f := buildPrometheusScrapeFeature(nil)
	reqComp := f.Configure(dda, &dda.Spec, nil)

	assert.False(t, reqComp.ClusterAgent.IsEnabled())
	assert.Empty(t, reqComp.ClusterAgent.Containers)
	assert.True(t, reqComp.Agent.IsEnabled())
}

func assertContainerEnvVars(t testing.TB, mgrInterface feature.PodTemplateManagers, containerName apicommon.AgentContainerName, wantEnvVars []*corev1.EnvVar) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)
	envVars := mgr.EnvVarMgr.EnvVarsByC[containerName]
	assert.True(t, apiutils.IsEqualStruct(envVars, wantEnvVars), "%s envvars \ndiff = %s", containerName, cmp.Diff(envVars, wantEnvVars))
}
