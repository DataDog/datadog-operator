// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eventcollection

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/test"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

func Test_eventCollectionFeature_ConfigureV1(t *testing.T) {
	// v1alpha1
	ddav1EventCollectionDisabled := v1alpha1.DatadogAgent{
		Spec: v1alpha1.DatadogAgentSpec{
			Agent: v1alpha1.DatadogAgentSpecAgentSpec{
				Config: &v1alpha1.NodeAgentConfig{
					CollectEvents:  apiutils.NewBoolPointer(false),
					LeaderElection: apiutils.NewBoolPointer(false),
				},
			},
		},
	}

	ddav1EventCollectionAgentEnabled := v1alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ddaNode",
		},
		Spec: v1alpha1.DatadogAgentSpec{
			Agent: v1alpha1.DatadogAgentSpecAgentSpec{
				Config: &v1alpha1.NodeAgentConfig{
					LeaderElection: apiutils.NewBoolPointer(true),
					CollectEvents:  apiutils.NewBoolPointer(true),
				},
			},
			ClusterAgent: v1alpha1.DatadogAgentSpecClusterAgentSpec{
				Config: &v1alpha1.ClusterAgentConfig{
					CollectEvents: apiutils.NewBoolPointer(false),
				},
			},
		},
	}

	ddav1EventCollectionDCAEnabled := v1alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ddaDCA",
		},
		Spec: v1alpha1.DatadogAgentSpec{
			Agent: v1alpha1.DatadogAgentSpecAgentSpec{
				Config: &v1alpha1.NodeAgentConfig{
					LeaderElection: apiutils.NewBoolPointer(true),
				},
			},
			ClusterAgent: v1alpha1.DatadogAgentSpecClusterAgentSpec{
				Config: &v1alpha1.ClusterAgentConfig{
					CollectEvents: apiutils.NewBoolPointer(true),
				},
			},
		},
	}

	eventCollectionClusterAgentWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
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

	eventCollectionAgentNodeWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)

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
				Value: "ddaNode-leader-election",
			},
			{
				Name:  apicommon.DDClusterAgentTokenName,
				Value: "ddaNode-token",
			},
		}
		coreAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(coreAgentEnvVars, want), "Agent envvars \ndiff = %s", cmp.Diff(coreAgentEnvVars, want))

	}

	tests := test.FeatureTestSuite{
		//////////////////////////
		// v1Alpha1.DatadogAgent
		//////////////////////////
		{
			Name:          "v1alpha1 Event Collection not enabled",
			DDAv1:         ddav1EventCollectionDisabled.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "v1alpha1 Event Collection on node agent enabled",
			DDAv1:         ddav1EventCollectionAgentEnabled.DeepCopy(),
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(eventCollectionAgentNodeWantFunc),
		},
		{
			Name:          "v1alpha1 Event Collection on DCA enabled",
			DDAv1:         ddav1EventCollectionDCAEnabled.DeepCopy(),
			WantConfigure: true,
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(eventCollectionClusterAgentWantFunc),
		},
	}

	tests.Run(t, buildEventCollectionFeature)
}
