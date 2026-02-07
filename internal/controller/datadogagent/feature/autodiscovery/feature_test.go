// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscovery

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	"github.com/DataDog/datadog-operator/pkg/testutils"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func Test_autodiscoveryFeature_Configure_NoExtras(t *testing.T) {
	dda := testutils.NewDatadogAgentBuilder().Build()
	// No extras set

	tests := test.FeatureTestSuite{
		{
			Name:          "autodiscovery not configured when no extras",
			DDA:           dda,
			WantConfigure: false,
		},
	}

	tests.Run(t, buildAutodiscoveryFeature)
}

func Test_autodiscoveryFeature_EnvVars_Set(t *testing.T) {
	b := testutils.NewDatadogAgentBuilder()
	dda := b.Build()
	dda.Spec.Features.Autodiscovery = &v2alpha1.AutodiscoveryConfig{ExtraIgnoreAutoConfig: []string{"redis", "postgres"}}

	wantEnv := []*corev1.EnvVar{
		{Name: DDIgnoreAutoConf, Value: "redis postgres"},
	}

	tests := test.FeatureTestSuite{
		{
			Name:          "append extras to DD_IGNORE_AUTOCONF for DCA and Agent",
			DDA:           dda,
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
				mgr := mgrInterface.(*fake.PodTemplateManagers)
				got := mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName]
				assert.True(t, apiutils.IsEqualStruct(got, wantEnv), "DCA envvars diff = %s", cmp.Diff(got, wantEnv))
			}),
			Agent: test.NewDefaultComponentTest().WithWantFunc(func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
				mgr := mgrInterface.(*fake.PodTemplateManagers)
				got := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
				assert.True(t, apiutils.IsEqualStruct(got, wantEnv), "Agent envvars diff = %s", cmp.Diff(got, wantEnv))
			}),
		},
		{
			Name:          "append extras in single-container strategy",
			DDA:           testutils.NewDatadogAgentBuilder().WithSingleContainerStrategy(true).Build(),
			WantConfigure: true,
			// override Autodiscovery on the built DDA
			Options: &test.Options{},
			Agent: test.NewDefaultComponentTest().WithCreateFunc(func(t testing.TB) (feature.PodTemplateManagers, string) {
				tpl, provider := test.NewDefaultComponentTest().CreateFunc(t)
				// Set extras on the DDA via closure captured below (set directly on spec)
				return tpl, provider
			}).WithWantFunc(func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
				// In single container, the env var is added to the unprivileged single agent container
				mgr := mgrInterface.(*fake.PodTemplateManagers)
				got := mgr.EnvVarMgr.EnvVarsByC[apicommon.UnprivilegedSingleAgentContainerName]
				assert.True(t, apiutils.IsEqualStruct(got, wantEnv), "Single agent envvars diff = %s", cmp.Diff(got, wantEnv))
			}),
		},
	}

	// Inject extras into the second test's DDA (the suite creates its own DDA per test)
	tests[1].DDA.Spec.Features.Autodiscovery = &v2alpha1.AutodiscoveryConfig{ExtraIgnoreAutoConfig: []string{"redis", "postgres"}}

	tests.Run(t, buildAutodiscoveryFeature)
}

func Test_autodiscoveryFeature_EnvVars_MergeExisting(t *testing.T) {
	b := testutils.NewDatadogAgentBuilder()
	dda := b.Build()
	dda.Spec.Features.Autodiscovery = &v2alpha1.AutodiscoveryConfig{ExtraIgnoreAutoConfig: []string{"redis", "postgres"}}

	tests := test.FeatureTestSuite{
		{
			Name:          "merge with existing DD_IGNORE_AUTOCONF in DCA",
			DDA:           dda,
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().
				WithCreateFunc(func(t testing.TB) (feature.PodTemplateManagers, string) {
					tpl, provider := test.NewDefaultComponentTest().CreateFunc(t)
					// Pre-populate existing env var
					tpl.(*fake.PodTemplateManagers).EnvVarMgr.AddEnvVarToContainer(apicommon.ClusterAgentContainerName, &corev1.EnvVar{Name: DDIgnoreAutoConf, Value: "kubernetes_state"})
					return tpl, provider
				}).
				WithWantFunc(func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					got := mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName]
					want := []*corev1.EnvVar{{Name: DDIgnoreAutoConf, Value: "kubernetes_state redis postgres"}}
					assert.True(t, apiutils.IsEqualStruct(got, want), "DCA merged envvars diff = %s", cmp.Diff(got, want))
				}),
		},
		{
			Name:          "merge with existing DD_IGNORE_AUTOCONF in Agent",
			DDA:           dda,
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().
				WithCreateFunc(func(t testing.TB) (feature.PodTemplateManagers, string) {
					tpl, provider := test.NewDefaultComponentTest().CreateFunc(t)
					// Pre-populate existing env var
					tpl.(*fake.PodTemplateManagers).EnvVarMgr.AddEnvVarToContainer(apicommon.CoreAgentContainerName, &corev1.EnvVar{Name: DDIgnoreAutoConf, Value: "kubernetes_state"})
					return tpl, provider
				}).
				WithWantFunc(func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					got := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
					want := []*corev1.EnvVar{{Name: DDIgnoreAutoConf, Value: "kubernetes_state redis postgres"}}
					assert.True(t, apiutils.IsEqualStruct(got, want), "Agent merged envvars diff = %s", cmp.Diff(got, want))
				}),
		},
		{
			Name:          "merge with existing DD_IGNORE_AUTOCONF in single-container Agent",
			DDA:           testutils.NewDatadogAgentBuilder().WithSingleContainerStrategy(true).Build(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().
				WithCreateFunc(func(t testing.TB) (feature.PodTemplateManagers, string) {
					tpl, provider := test.NewDefaultComponentTest().CreateFunc(t)
					tpl.(*fake.PodTemplateManagers).EnvVarMgr.AddEnvVarToContainer(apicommon.UnprivilegedSingleAgentContainerName, &corev1.EnvVar{Name: DDIgnoreAutoConf, Value: "kubernetes_state"})
					return tpl, provider
				}).
				WithWantFunc(func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					got := mgr.EnvVarMgr.EnvVarsByC[apicommon.UnprivilegedSingleAgentContainerName]
					want := []*corev1.EnvVar{{Name: DDIgnoreAutoConf, Value: "kubernetes_state redis postgres"}}
					assert.True(t, apiutils.IsEqualStruct(got, want), "Single agent merged envvars diff = %s", cmp.Diff(got, want))
				}),
		},
	}

	// Inject extras into tests that build their own DDA
	tests[2].DDA.Spec.Features.Autodiscovery = &v2alpha1.AutodiscoveryConfig{ExtraIgnoreAutoConfig: []string{"redis", "postgres"}}

	tests.Run(t, buildAutodiscoveryFeature)
}
