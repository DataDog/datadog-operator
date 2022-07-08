// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestrator

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
	mergerfake "github.com/DataDog/datadog-operator/controllers/datadogagent/merger/fake"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func createEmptyFakeManager(t testing.TB) feature.PodTemplateManagers {
	mgr := fake.NewPodTemplateManagers(t)
	return mgr
}

func Test_orchestratorExplorerFeature_Configure(t *testing.T) {
	ddav1OrchestratorDisable := v1alpha1.DatadogAgent{
		Spec: v1alpha1.DatadogAgentSpec{
			Features: v1alpha1.DatadogFeatures{
				OrchestratorExplorer: &v1alpha1.OrchestratorExplorerConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
			},
		},
	}

	ddav1OrchestratorEnable := ddav1OrchestratorDisable.DeepCopy()
	{
		ddav1OrchestratorEnable.Spec.Features.OrchestratorExplorer.Enabled = apiutils.NewBoolPointer(true)
		ddav1OrchestratorEnable.Spec.Features.OrchestratorExplorer.Scrubbing = &v1alpha1.Scrubbing{
			Containers: apiutils.NewBoolPointer(true),
		}
		ddav1OrchestratorEnable.Spec.Features.OrchestratorExplorer.ExtraTags = []string{"a:z", "b:y", "c:x"}
		ddav1OrchestratorEnable.Spec.Features.OrchestratorExplorer.DDUrl = apiutils.NewStringPointer("https://foo.bar")

	}

	ddav2OrchestratorDisable := v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
			},
		},
	}
	ddav2OrchestratorEnable := ddav2OrchestratorDisable.DeepCopy()
	{
		ddav2OrchestratorEnable.Spec.Features.OrchestratorExplorer.Enabled = apiutils.NewBoolPointer(true)
		ddav2OrchestratorEnable.Spec.Features.OrchestratorExplorer.ScrubContainers = apiutils.NewBoolPointer(true)
		ddav2OrchestratorEnable.Spec.Features.OrchestratorExplorer.ExtraTags = []string{"a:z", "b:y", "c:x"}
		ddav2OrchestratorEnable.Spec.Features.OrchestratorExplorer.DDUrl = apiutils.NewStringPointer("https://foo.bar")
	}

	orchestratorClusterAgentWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)
		dcaEnvVars := mgr.EnvVarMgr.EnvVarsByC[mergerfake.AllContainers]

		want := []*corev1.EnvVar{
			{
				Name:  apicommon.DDOrchestratorExplorerEnabled,
				Value: "true",
			},
			{
				Name:  apicommon.DDOrchestratorExplorerContainerScrubbingEnabled,
				Value: "true",
			},
			{
				Name:  apicommon.DDOrchestratorExplorerExtraTags,
				Value: `["a:z","b:y","c:x"]`,
			},
			{
				Name:  apicommon.DDOrchestratorExplorerDDUrl,
				Value: "https://foo.bar",
			},
		}
		assert.True(t, apiutils.IsEqualStruct(dcaEnvVars, want), "DCA envvars \ndiff = %s", cmp.Diff(dcaEnvVars, want))
	}

	orchestratorNodeAgentWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)
		agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.ProcessAgentContainerName]

		want := []*corev1.EnvVar{
			{
				Name:  apicommon.DDOrchestratorExplorerEnabled,
				Value: "true",
			},
			{
				Name:  apicommon.DDOrchestratorExplorerContainerScrubbingEnabled,
				Value: "true",
			},
			{
				Name:  apicommon.DDOrchestratorExplorerExtraTags,
				Value: `["a:z","b:y","c:x"]`,
			},
			{
				Name:  apicommon.DDOrchestratorExplorerDDUrl,
				Value: "https://foo.bar",
			},
		}
		assert.True(t, apiutils.IsEqualStruct(agentEnvVars, want), "Process agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, want))
	}

	tests := test.FeatureTestSuite{
		//////////////////////////
		// v1Alpha1.DatadogAgent
		//////////////////////////
		{
			Name:          "v1alpha1 orchestrator not enabled",
			DDAv1:         ddav1OrchestratorDisable.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "v1alpha1 orchestrator enabled",
			DDAv1:         ddav1OrchestratorEnable,
			WantConfigure: true,
			ClusterAgent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc:   orchestratorClusterAgentWantFunc,
			},
			Agent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc:   orchestratorNodeAgentWantFunc,
			},
		},
		//////////////////////////
		// v2Alpha1.DatadogAgent
		//////////////////////////
		{
			Name:          "v2alpha1 orchestrator not enabled",
			DDAv2:         ddav2OrchestratorDisable.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "v2alpha1 orchestrator enabled",
			DDAv2:         ddav2OrchestratorEnable,
			WantConfigure: true,
			ClusterAgent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc:   orchestratorClusterAgentWantFunc,
			},
			Agent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc:   orchestratorNodeAgentWantFunc,
			},
		},
	}

	tests.Run(t, buildOrchestratorExplorerFeature)
}
