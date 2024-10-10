// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package prometheusscrape

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	v2alpha1test "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1/test"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"

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
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithPrometheusScrapeEnabled(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "Prometheus scrape enabled",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithPrometheusScrapeEnabled(true).
				Build(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					wantEnvVars := []*corev1.EnvVar{
						{
							Name:  apicommon.DDPrometheusScrapeEnabled,
							Value: "true",
						},
						{
							Name:  apicommon.DDPrometheusScrapeServiceEndpoints,
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
							Name:  apicommon.DDPrometheusScrapeEnabled,
							Value: "true",
						},
						{
							Name:  apicommon.DDPrometheusScrapeServiceEndpoints,
							Value: "false",
						},
					}
					assertContainerEnvVars(t, mgrInterface, apicommon.ClusterAgentContainerName, wantEnvVars)
				},
			),
		},
		{
			Name: "Prometheus scrape service endpoints enabled",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithPrometheusScrapeEnabled(true).
				WithPrometheusScrapeServiceEndpoints(true).
				Build(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					wantEnvVars := []*corev1.EnvVar{
						{
							Name:  apicommon.DDPrometheusScrapeEnabled,
							Value: "true",
						},
						{
							Name:  apicommon.DDPrometheusScrapeServiceEndpoints,
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
							Name:  apicommon.DDPrometheusScrapeEnabled,
							Value: "true",
						},
						{
							Name:  apicommon.DDPrometheusScrapeServiceEndpoints,
							Value: "true",
						},
					}
					assertContainerEnvVars(t, mgrInterface, apicommon.ClusterAgentContainerName, wantEnvVars)
				},
			),
		},
		{
			Name: "Prometheus scrape additional configs",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithPrometheusScrapeEnabled(true).
				WithPrometheusScrapeAdditionalConfigs(yamlConfigs).
				Build(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					wantEnvVars := []*corev1.EnvVar{
						{
							Name:  apicommon.DDPrometheusScrapeEnabled,
							Value: "true",
						},
						{
							Name:  apicommon.DDPrometheusScrapeServiceEndpoints,
							Value: "false",
						},
						{
							Name:  apicommon.DDPrometheusScrapeChecks,
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
							Name:  apicommon.DDPrometheusScrapeEnabled,
							Value: "true",
						},
						{
							Name:  apicommon.DDPrometheusScrapeServiceEndpoints,
							Value: "false",
						},
						{
							Name:  apicommon.DDPrometheusScrapeChecks,
							Value: jsonConfigs,
						},
					}
					assertContainerEnvVars(t, mgrInterface, apicommon.ClusterAgentContainerName, wantEnvVars)
				},
			),
		},
		{
			Name: "version specified",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithPrometheusScrapeEnabled(true).
				WithPrometheusScrapeVersion(1).
				Build(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					wantEnvVars := []*corev1.EnvVar{
						{
							Name:  apicommon.DDPrometheusScrapeEnabled,
							Value: "true",
						},
						{
							Name:  apicommon.DDPrometheusScrapeServiceEndpoints,
							Value: "false",
						},
						{
							Name:  apicommon.DDPrometheusScrapeVersion,
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
							Name:  apicommon.DDPrometheusScrapeEnabled,
							Value: "true",
						},
						{
							Name:  apicommon.DDPrometheusScrapeServiceEndpoints,
							Value: "false",
						},
						{
							Name:  apicommon.DDPrometheusScrapeVersion,
							Value: "1",
						},
					}
					assertContainerEnvVars(t, mgrInterface, apicommon.ClusterAgentContainerName, wantEnvVars)
				},
			),
		},
	}

	tests.Run(t, buildPrometheusScrapeFeature)
}

func assertContainerEnvVars(t testing.TB, mgrInterface feature.PodTemplateManagers, containerName apicommon.AgentContainerName, wantEnvVars []*corev1.EnvVar) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)
	envVars := mgr.EnvVarMgr.EnvVarsByC[containerName]
	assert.True(t, apiutils.IsEqualStruct(envVars, wantEnvVars), "%s envvars \ndiff = %s", containerName, cmp.Diff(envVars, wantEnvVars))
}
