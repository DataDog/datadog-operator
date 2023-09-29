// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestratorexplorer

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/test"
	mergerfake "github.com/DataDog/datadog-operator/controllers/datadogagent/merger/fake"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

var customConfDataV1 = `cluster_check: false
init_config:
instances:
  - skip_leader_election: false
    collectors:
      - clusterrolebindings`

var expectedOrchestratorEnvsV1 = []*corev1.EnvVar{
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

func Test_orchestratorExplorerFeature_ConfigureV1(t *testing.T) {
	ddav1OrchestratorExplorerDisable := v1alpha1.DatadogAgent{
		Spec: v1alpha1.DatadogAgentSpec{
			Features: v1alpha1.DatadogFeatures{
				OrchestratorExplorer: &v1alpha1.OrchestratorExplorerConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
			},
		},
	}

	ddav1OrchestratorExplorerEnable := ddav1OrchestratorExplorerDisable.DeepCopy()
	{
		ddav1OrchestratorExplorerEnable.Spec.Features.OrchestratorExplorer.Enabled = apiutils.NewBoolPointer(true)
		ddav1OrchestratorExplorerEnable.Spec.Features.OrchestratorExplorer.Scrubbing = &v1alpha1.Scrubbing{
			Containers: apiutils.NewBoolPointer(true),
		}
		ddav1OrchestratorExplorerEnable.Spec.Features.OrchestratorExplorer.ExtraTags = []string{"a:z", "b:y", "c:x"}
		ddav1OrchestratorExplorerEnable.Spec.Features.OrchestratorExplorer.DDUrl = apiutils.NewStringPointer("https://foo.bar")
		ddav1OrchestratorExplorerEnable.Spec.Features.OrchestratorExplorer.Conf = &v1alpha1.CustomConfigSpec{
			ConfigData: &customConfDataV1,
		}

	}

	ddaV1EnabledAndRunInRunner := ddav1OrchestratorExplorerEnable.DeepCopy()
	ddaV1EnabledAndRunInRunner.Spec.ClusterAgent = v1alpha1.DatadogAgentSpecClusterAgentSpec{
		Config: &v1alpha1.ClusterAgentConfig{
			ClusterChecksEnabled: apiutils.NewBoolPointer(true),
		},
	}
	ddaV1EnabledAndRunInRunner.Spec.ClusterChecksRunner = v1alpha1.DatadogAgentSpecClusterChecksRunnerSpec{
		Enabled: apiutils.NewBoolPointer(true),
		Rbac:    &v1alpha1.RbacConfig{},
	}
	ddaV1EnabledAndRunInRunner.Spec.Features.OrchestratorExplorer.ClusterCheck = apiutils.NewBoolPointer(true)

	orchestratorExplorerNodeAgentWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)
		agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.ProcessAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(agentEnvVars, expectedOrchestratorEnvsV1), "Process agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, expectedOrchestratorEnvsV1))
	}

	orchestratorExplorerClusterChecksRunnerWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)
		runnerEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.ClusterChecksRunnersContainerName]
		assert.True(t, apiutils.IsEqualStruct(runnerEnvs, expectedOrchestratorEnvsV1), "Cluster Checks Runner envvars \ndiff = %s", cmp.Diff(runnerEnvs, expectedOrchestratorEnvsV1))
	}

	tests := test.FeatureTestSuite{
		//////////////////////////
		// v1Alpha1.DatadogAgent
		//////////////////////////
		{
			Name:          "v1alpha1 orchestrator explorer not enabled",
			DDAv1:         ddav1OrchestratorExplorerDisable.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "v1alpha1 orchestrator explorer enabled",
			DDAv1:         ddav1OrchestratorExplorerEnable,
			WantConfigure: true,
			ClusterAgent:  orchestratorExplorerClusterAgentWantFuncV1(),
			Agent:         test.NewDefaultComponentTest().WithWantFunc(orchestratorExplorerNodeAgentWantFunc),
		},
		{
			Name:                "v1alpha1 orchestrator explorer enabled and runs on cluster checks runner",
			DDAv1:               ddaV1EnabledAndRunInRunner,
			WantConfigure:       true,
			ClusterAgent:        orchestratorExplorerClusterAgentWantFuncV1(),
			Agent:               test.NewDefaultComponentTest().WithWantFunc(orchestratorExplorerNodeAgentWantFunc),
			ClusterChecksRunner: test.NewDefaultComponentTest().WithWantFunc(orchestratorExplorerClusterChecksRunnerWantFunc),
		},
	}

	tests.Run(t, buildOrchestratorExplorerFeature)
}

func orchestratorExplorerClusterAgentWantFuncV1() *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)
			dcaEnvVars := mgr.EnvVarMgr.EnvVarsByC[mergerfake.AllContainers]
			assert.True(t, apiutils.IsEqualStruct(dcaEnvVars, expectedOrchestratorEnvsV1), "DCA envvars \ndiff = %s", cmp.Diff(dcaEnvVars, expectedOrchestratorEnvsV1))
		},
	)
}
