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
		{
			Name: "KubernetesActions enabled with RC and PAR",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("ddaDCA").
				WithKubernetesActionsEnabled(true).
				WithKubernetesActionsCommunication(true, true).
				Build(),
			WantConfigure: true,
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(kubernetesActionsClusterAgentWantFunc),
		},
		{
			// Older Cluster Agents always use Remote Configuration: rc=false is ignored and
			// no channel setting is passed.
			Name: "KubernetesActions enabled with PAR only on a cluster agent without channel settings",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("ddaDCA").
				WithKubernetesActionsEnabled(true).
				WithKubernetesActionsCommunication(false, true).
				WithClusterAgentTag("7.85.0").
				Build(),
			WantConfigure: true,
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(kubernetesActionsClusterAgentWantFunc),
		},
		{
			// PAR needs Cluster Agent 7.83.0: on an older one the runner is not enabled and
			// Remote Configuration keeps delivering actions.
			Name: "KubernetesActions enabled with PAR on a cluster agent without the PAR bundle",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("ddaDCA").
				WithKubernetesActionsEnabled(true).
				WithKubernetesActionsCommunication(true, true).
				WithClusterAgentTag("7.82.0").
				Build(),
			WantConfigure: true,
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(kubernetesActionsClusterAgentWantFunc),
		},
		{
			Name: "KubernetesActions enabled with PAR only",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("ddaDCA").
				WithKubernetesActionsEnabled(true).
				WithKubernetesActionsCommunication(false, true).
				WithClusterAgentTag("7.86.0").
				Build(),
			WantConfigure: true,
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(kubernetesActionsPAROnlyClusterAgentWantFunc),
		},
		{
			// Both channels are on by default: no channel setting is passed.
			Name: "KubernetesActions enabled with default channels on a cluster agent with channel settings",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("ddaDCA").
				WithKubernetesActionsEnabled(true).
				WithClusterAgentTag("7.86.0").
				Build(),
			WantConfigure: true,
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(kubernetesActionsClusterAgentWantFunc),
		},
		{
			Name: "KubernetesActions enabled with RC only",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("ddaDCA").
				WithKubernetesActionsEnabled(true).
				WithKubernetesActionsCommunication(true, false).
				WithClusterAgentTag("7.86.0").
				Build(),
			WantConfigure: true,
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(kubernetesActionsRCOnlyClusterAgentWantFunc),
		},
		{
			Name: "KubernetesActions enabled with RC and PAR on a cluster agent with channel settings",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("ddaDCA").
				WithKubernetesActionsEnabled(true).
				WithKubernetesActionsCommunication(true, true).
				WithClusterAgentTag("7.86.0").
				Build(),
			WantConfigure: true,
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(kubernetesActionsClusterAgentWantFunc),
		},
		{
			// Remote Configuration disabled globally: the feature is still configured (the
			// Cluster Agent refuses to start, which is intended) and the operator logs why.
			Name: "KubernetesActions enabled with Remote Configuration disabled globally",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("ddaDCA").
				WithKubernetesActionsEnabled(true).
				WithRemoteConfigEnabled(false).
				Build(),
			WantConfigure: true,
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(kubernetesActionsClusterAgentWantFunc),
		},
		{
			Name: "KubernetesActions enabled with no communication channel",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("ddaDCA").
				WithKubernetesActionsEnabled(true).
				WithKubernetesActionsCommunication(false, false).
				Build(),
			WantConfigure: false,
		},
	}

	tests.Run(t, buildKubernetesActionsFeature)
}

// kubernetesActionsClusterAgentWantFunc expects only the feature switch: both channels are on
// (the agent default), or the Cluster Agent predates the channel settings.
func kubernetesActionsClusterAgentWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	assertKubernetesActionsEnvVars(t, mgrInterface)
}

func kubernetesActionsPAROnlyClusterAgentWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	assertKubernetesActionsEnvVars(t, mgrInterface, &corev1.EnvVar{Name: DDKubeActionsRemoteConfigEnabled, Value: "false"})
}

func kubernetesActionsRCOnlyClusterAgentWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	assertKubernetesActionsEnvVars(t, mgrInterface, &corev1.EnvVar{Name: DDKubeActionsPAREnabled, Value: "false"})
}

func assertKubernetesActionsEnvVars(t testing.TB, mgrInterface feature.PodTemplateManagers, channelEnvVars ...*corev1.EnvVar) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)
	dcaEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName]

	want := append([]*corev1.EnvVar{
		{
			Name:  DDKubeActionsEnabled,
			Value: "true",
		},
	}, channelEnvVars...)
	assert.True(t, apiutils.IsEqualStruct(dcaEnvVars, want), "DCA envvars \ndiff = %s", cmp.Diff(dcaEnvVars, want))
}

func Test_kubernetesActionsFeature_NoOpManagers(t *testing.T) {
	f := buildKubernetesActionsFeature(nil)

	assert.NoError(t, f.ManageSingleContainerNodeAgent(nil))
	assert.NoError(t, f.ManageNodeAgent(nil))
	assert.NoError(t, f.ManageClusterChecksRunner(nil))
	assert.NoError(t, f.ManageOtelAgentGateway(nil))
}
