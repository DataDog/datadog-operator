// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package prometheusscrape

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

	// v1alpha1
	ddav1PrometheusScrapeDisabled := v1alpha1.DatadogAgent{
		Spec: v1alpha1.DatadogAgentSpec{
			Features: v1alpha1.DatadogFeatures{
				PrometheusScrape: &v1alpha1.PrometheusScrapeConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
			},
		},
	}

	ddav1PrometheusScrapeEnabled := ddav1PrometheusScrapeDisabled.DeepCopy()
	{
		ddav1PrometheusScrapeEnabled.Spec.Features.PrometheusScrape.Enabled = apiutils.NewBoolPointer(true)
	}

	ddav1PrometheusScrapeServiceEndpoints := ddav1PrometheusScrapeEnabled.DeepCopy()
	{
		ddav1PrometheusScrapeServiceEndpoints.Spec.Features.PrometheusScrape.ServiceEndpoints = apiutils.NewBoolPointer(true)
	}

	ddav1PrometheusScrapeAdditionalConfigs := ddav1PrometheusScrapeEnabled.DeepCopy()
	{
		ddav1PrometheusScrapeAdditionalConfigs.Spec.Features.PrometheusScrape.AdditionalConfigs = apiutils.NewStringPointer(yamlConfigs)
	}

	// v2alpha1
	ddav2PrometheusScrapeDisabled := v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
			},
		},
	}

	ddav2PrometheusScrapeEnabled := ddav2PrometheusScrapeDisabled.DeepCopy()
	{
		ddav2PrometheusScrapeEnabled.Spec.Features.PrometheusScrape.Enabled = apiutils.NewBoolPointer(true)
	}

	ddav2PrometheusScrapeServiceEndpoints := ddav2PrometheusScrapeEnabled.DeepCopy()
	{
		ddav2PrometheusScrapeServiceEndpoints.Spec.Features.PrometheusScrape.EnableServiceEndpoints = apiutils.NewBoolPointer(true)
	}

	ddav2PrometheusScrapeAdditionalConfigs := ddav2PrometheusScrapeEnabled.DeepCopy()
	{
		ddav2PrometheusScrapeAdditionalConfigs.Spec.Features.PrometheusScrape.AdditionalConfigs = apiutils.NewStringPointer(yamlConfigs)
	}

	tests := test.FeatureTestSuite{
		///////////////////////////
		// v1alpha1.DatadogAgent //
		///////////////////////////
		{
			Name:          "v1alpha1 Prometheus scrape not enabled",
			DDAv1:         ddav1PrometheusScrapeDisabled.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "v1alpha1 Prometheus scrape enabled",
			DDAv1:         ddav1PrometheusScrapeEnabled,
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
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
					coreAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentEnvVars, wantEnvVars), "Core Agent envvars \ndiff = %s", cmp.Diff(coreAgentEnvVars, wantEnvVars))
				},
			),
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					dcaEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.ClusterAgentContainerName]
					want := []*corev1.EnvVar{
						{
							Name:  apicommon.DDPrometheusScrapeEnabled,
							Value: "true",
						},
						{
							Name:  apicommon.DDPrometheusScrapeServiceEndpoints,
							Value: "false",
						},
					}
					assert.True(t, apiutils.IsEqualStruct(dcaEnvVars, want), "DCA envvars \ndiff = %s", cmp.Diff(dcaEnvVars, want))
				},
			),
		},
		{
			Name:          "v1alpha1 Prometheus scrape service endpoints enabled",
			DDAv1:         ddav1PrometheusScrapeServiceEndpoints,
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
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
					coreAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentEnvVars, wantEnvVars), "Core Agent envvars \ndiff = %s", cmp.Diff(coreAgentEnvVars, wantEnvVars))
				},
			),
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					dcaEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.ClusterAgentContainerName]
					want := []*corev1.EnvVar{
						{
							Name:  apicommon.DDPrometheusScrapeEnabled,
							Value: "true",
						},
						{
							Name:  apicommon.DDPrometheusScrapeServiceEndpoints,
							Value: "true",
						},
					}
					assert.True(t, apiutils.IsEqualStruct(dcaEnvVars, want), "DCA envvars \ndiff = %s", cmp.Diff(dcaEnvVars, want))
				},
			),
		},
		{
			Name:          "v1alpha1 Prometheus scrape additional configs",
			DDAv1:         ddav1PrometheusScrapeAdditionalConfigs,
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
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
					coreAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentEnvVars, wantEnvVars), "Core Agent envvars \ndiff = %s", cmp.Diff(coreAgentEnvVars, wantEnvVars))
				},
			),
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					dcaEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.ClusterAgentContainerName]
					want := []*corev1.EnvVar{
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
					assert.True(t, apiutils.IsEqualStruct(dcaEnvVars, want), "DCA envvars \ndiff = %s", cmp.Diff(dcaEnvVars, want))
				},
			),
		},
		// ///////////////////////////
		// // v2alpha1.DatadogAgent //
		// ///////////////////////////
		{
			Name:          "v2alpha1 Prometheus scrape not enabled",
			DDAv2:         ddav2PrometheusScrapeDisabled.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "v2alpha1 Prometheus scrape enabled",
			DDAv2:         ddav2PrometheusScrapeEnabled,
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
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
					coreAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentEnvVars, wantEnvVars), "Core Agent envvars \ndiff = %s", cmp.Diff(coreAgentEnvVars, wantEnvVars))
				},
			),
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					dcaEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.ClusterAgentContainerName]
					want := []*corev1.EnvVar{
						{
							Name:  apicommon.DDPrometheusScrapeEnabled,
							Value: "true",
						},
						{
							Name:  apicommon.DDPrometheusScrapeServiceEndpoints,
							Value: "false",
						},
					}
					assert.True(t, apiutils.IsEqualStruct(dcaEnvVars, want), "DCA envvars \ndiff = %s", cmp.Diff(dcaEnvVars, want))
				},
			),
		},
		{
			Name:          "v2alpha1 Prometheus scrape service endpoints enabled",
			DDAv2:         ddav2PrometheusScrapeServiceEndpoints,
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
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
					coreAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentEnvVars, wantEnvVars), "Core Agent envvars \ndiff = %s", cmp.Diff(coreAgentEnvVars, wantEnvVars))
				},
			),
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					dcaEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.ClusterAgentContainerName]
					want := []*corev1.EnvVar{
						{
							Name:  apicommon.DDPrometheusScrapeEnabled,
							Value: "true",
						},
						{
							Name:  apicommon.DDPrometheusScrapeServiceEndpoints,
							Value: "true",
						},
					}
					assert.True(t, apiutils.IsEqualStruct(dcaEnvVars, want), "DCA envvars \ndiff = %s", cmp.Diff(dcaEnvVars, want))
				},
			),
		},
		{
			Name:          "v2alpha1 Prometheus scrape additional configs",
			DDAv2:         ddav2PrometheusScrapeAdditionalConfigs,
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
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
					coreAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
					assert.True(t, apiutils.IsEqualStruct(coreAgentEnvVars, wantEnvVars), "Core Agent envvars \ndiff = %s", cmp.Diff(coreAgentEnvVars, wantEnvVars))
				},
			),
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					dcaEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.ClusterAgentContainerName]
					want := []*corev1.EnvVar{
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
					assert.True(t, apiutils.IsEqualStruct(dcaEnvVars, want), "DCA envvars \ndiff = %s", cmp.Diff(dcaEnvVars, want))
				},
			),
		},
	}

	tests.Run(t, buildPrometheusScrapeFeature)
}
