// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubeactions

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

func Test_kubeActionsFeature_Configure(t *testing.T) {
	tests := test.FeatureTestSuite{
		{
			Name:          "KubeActions not configured",
			DDA:           testutils.NewDatadogAgentBuilder().Build(),
			WantConfigure: false,
		},
		{
			Name: "KubeActions explicitly disabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithKubeActionsEnabled(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "KubeActions enabled but cluster agent version too low",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("ddaDCA").
				WithKubeActionsEnabled(true).
				WithClusterAgentTag("7.78.0").
				Build(),
			WantConfigure: false,
		},
		{
			Name: "KubeActions enabled at minimum cluster agent version",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("ddaDCA").
				WithKubeActionsEnabled(true).
				WithClusterAgentTag("7.79.0").
				Build(),
			WantConfigure: true,
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(kubeActionsClusterAgentWantFunc),
		},
		{
			Name: "KubeActions enabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("ddaDCA").
				WithKubeActionsEnabled(true).
				Build(),
			WantConfigure: true,
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(kubeActionsClusterAgentWantFunc),
		},
	}

	tests.Run(t, buildKubeActionsFeature)
}

func kubeActionsClusterAgentWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
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

func Test_kubeActionsFeature_NoOpManagers(t *testing.T) {
	f := buildKubeActionsFeature(nil)

	assert.NoError(t, f.ManageSingleContainerNodeAgent(nil))
	assert.NoError(t, f.ManageNodeAgent(nil))
	assert.NoError(t, f.ManageClusterChecksRunner(nil))
	assert.NoError(t, f.ManageOtelAgentGateway(nil))
}
