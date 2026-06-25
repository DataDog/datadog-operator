// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetesactions

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	"github.com/DataDog/datadog-operator/pkg/testutils"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

func Test_kubernetesActionsFeature_Configure(t *testing.T) {
	tests := test.FeatureTestSuite{
		{
			Name:          "KubernetesActions not configured",
			DDA:           testutils.NewDatadogAgentBuilder().Build(),
			WantConfigure: false,
		},
		{
			Name: "KubernetesActions explicitly disabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithKubernetesActionsEnabled(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "KubernetesActions enabled but cluster agent version too low",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("ddaDCA").
				WithKubernetesActionsEnabled(true).
				WithClusterAgentTag("7.78.0").
				Build(),
			WantConfigure: false,
		},
		{
			Name: "KubernetesActions enabled at minimum cluster agent version",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("ddaDCA").
				WithKubernetesActionsEnabled(true).
				WithClusterAgentTag("7.79.0").
				Build(),
			WantConfigure: true,
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(kubernetesActionsClusterAgentWantFunc),
		},
		{
			Name: "KubernetesActions enabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("ddaDCA").
				WithKubernetesActionsEnabled(true).
				Build(),
			WantConfigure: true,
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(kubernetesActionsClusterAgentWantFunc),
		},
	}

	tests.Run(t, buildKubernetesActionsFeature)
}

func kubernetesActionsClusterAgentWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)
	dcaEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName]

	want := []*corev1.EnvVar{
		{
			Name:  DDKubeActionsEnabled,
			Value: "true",
		},
	}
	assert.True(t, apiutils.IsEqualStruct(dcaEnvVars, want), "DCA envvars \ndiff = %s", cmp.Diff(dcaEnvVars, want))
}

func Test_kubernetesActionsFeature_NoOpManagers(t *testing.T) {
	f := buildKubernetesActionsFeature(nil)

	assert.NoError(t, f.ManageSingleContainerNodeAgent(nil))
	assert.NoError(t, f.ManageNodeAgent(nil))
	assert.NoError(t, f.ManageClusterChecksRunner(nil))
	assert.NoError(t, f.ManageOtelAgentGateway(nil))
}
