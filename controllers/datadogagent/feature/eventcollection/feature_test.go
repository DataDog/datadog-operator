// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eventcollection

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	v2alpha1test "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1/test"

	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/test"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

func Test_eventCollectionFeature_Configure(t *testing.T) {
	tests := test.FeatureTestSuite{
		{
			Name: "v2alpha1 Event Collection not enabled",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithEventCollectionEnabled(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "v2alpha1 Event Collection enabled",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithName("ddaDCA").
				WithEventCollectionEnabled(true).
				Build(),
			WantConfigure: true,
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(eventCollectionClusterAgentWantFunc),
		},
	}

	tests.Run(t, buildEventCollectionFeature)
}

func eventCollectionClusterAgentWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)
	dcaEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.ClusterAgentContainerName]

	want := []*corev1.EnvVar{
		{
			Name:  apicommon.DDCollectKubernetesEvents,
			Value: "true",
		},
		{
			Name:  apicommon.DDLeaderElection,
			Value: "true",
		},
		{
			Name:  apicommon.DDLeaderLeaseName,
			Value: "ddaDCA-leader-election",
		},
		{
			Name:  apicommon.DDClusterAgentTokenName,
			Value: "ddaDCA-token",
		},
	}
	assert.True(t, apiutils.IsEqualStruct(dcaEnvVars, want), "DCA envvars \ndiff = %s", cmp.Diff(dcaEnvVars, want))
}
