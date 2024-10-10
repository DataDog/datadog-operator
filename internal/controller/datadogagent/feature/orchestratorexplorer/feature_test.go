// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestratorexplorer

import (
	"fmt"
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	v2alpha1test "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1/test"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	mergerfake "github.com/DataDog/datadog-operator/internal/controller/datadogagent/merger/fake"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

var customConfDataV2 = `cluster_check: false
init_config:
instances:
  - skip_leader_election: false
    collectors:
      - clusterrolebindings`

var expectedOrchestratorEnvsV2 = []*corev1.EnvVar{
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

func Test_orchestratorExplorerFeature_Configure(t *testing.T) {
	tests := test.FeatureTestSuite{
		{
			Name: "orchestrator explorer not enabled",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithOrchestratorExplorerEnabled(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "orchestrator explorer enabled",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithOrchestratorExplorerEnabled(true).
				WithOrchestratorExplorerScrubContainers(true).
				WithOrchestratorExplorerExtraTags([]string{"a:z", "b:y", "c:x"}).
				WithOrchestratorExplorerDDUrl("https://foo.bar").
				WithOrchestratorExplorerCustomConfigData(customConfDataV2).
				WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{Image: &v2alpha1.AgentImageConfig{Tag: "7.51.0"}}).
				Build(),
			WantConfigure: true,
			ClusterAgent:  orchestratorExplorerClusterAgentWantFuncV2(),
			Agent:         test.NewDefaultComponentTest().WithWantFunc(orchestratorExplorerNodeAgentNoProcessAgentWantFunc),
		},
		{
			Name: "orchestrator explorer enabled and runs on cluster checks runner",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithOrchestratorExplorerEnabled(true).
				WithOrchestratorExplorerScrubContainers(true).
				WithOrchestratorExplorerExtraTags([]string{"a:z", "b:y", "c:x"}).
				WithOrchestratorExplorerDDUrl("https://foo.bar").
				WithOrchestratorExplorerCustomConfigData(customConfDataV2).
				WithClusterChecksEnabled(true).
				WithClusterChecksUseCLCEnabled(true).
				WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{Image: &v2alpha1.AgentImageConfig{Tag: "7.51.0"}}).
				Build(),
			WantConfigure:       true,
			ClusterAgent:        orchestratorExplorerClusterAgentWantFuncV2(),
			Agent:               test.NewDefaultComponentTest().WithWantFunc(orchestratorExplorerNodeAgentNoProcessAgentWantFunc),
			ClusterChecksRunner: test.NewDefaultComponentTest().WithWantFunc(orchestratorExplorerClusterChecksRunnerWantFunc),
		},
		{
			Name: "orchestrator explorer enabled on version requiring process agent",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithOrchestratorExplorerEnabled(true).
				WithOrchestratorExplorerScrubContainers(true).
				WithOrchestratorExplorerExtraTags([]string{"a:z", "b:y", "c:x"}).
				WithOrchestratorExplorerDDUrl("https://foo.bar").
				WithOrchestratorExplorerCustomConfigData(customConfDataV2).
				WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{Image: &v2alpha1.AgentImageConfig{Tag: "7.50.0"}}).
				Build(),
			WantConfigure: true,
			ClusterAgent:  orchestratorExplorerClusterAgentWantFuncV2(),
			Agent:         test.NewDefaultComponentTest().WithWantFunc(orchestratorExplorerNodeAgentWantFunc),
		},
	}

	tests.Run(t, buildOrchestratorExplorerFeature)
}

func orchestratorExplorerNodeAgentWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)
	agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.ProcessAgentContainerName]
	assert.True(t, apiutils.IsEqualStruct(agentEnvVars, expectedOrchestratorEnvsV2), "Process agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, expectedOrchestratorEnvsV2))
	agentEnvVars = mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
	assert.True(t, apiutils.IsEqualStruct(agentEnvVars, expectedOrchestratorEnvsV2), "Core agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, expectedOrchestratorEnvsV2))
}

func orchestratorExplorerNodeAgentNoProcessAgentWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)
	agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.ProcessAgentContainerName]
	assert.True(t, apiutils.IsEqualStruct(agentEnvVars, nil), "Process agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, expectedOrchestratorEnvsV2))
	agentEnvVars = mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
	assert.True(t, apiutils.IsEqualStruct(agentEnvVars, expectedOrchestratorEnvsV2), "Core agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, expectedOrchestratorEnvsV2))
}

func orchestratorExplorerClusterChecksRunnerWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)
	runnerEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterChecksRunnersContainerName]
	assert.True(t, apiutils.IsEqualStruct(runnerEnvs, expectedOrchestratorEnvsV2), "Cluster Checks Runner envvars \ndiff = %s", cmp.Diff(runnerEnvs, expectedOrchestratorEnvsV2))
}

func orchestratorExplorerClusterAgentWantFuncV2() *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)
			dcaEnvVars := mgr.EnvVarMgr.EnvVarsByC[mergerfake.AllContainers]
			assert.True(t, apiutils.IsEqualStruct(dcaEnvVars, expectedOrchestratorEnvsV2), "DCA envvars \ndiff = %s", cmp.Diff(dcaEnvVars, expectedOrchestratorEnvsV2))

			// check annotation
			customConfig := v2alpha1.CustomConfig{
				ConfigData: apiutils.NewStringPointer(customConfDataV2),
			}
			hash, err := comparison.GenerateMD5ForSpec(&customConfig)
			assert.NoError(t, err)
			wantAnnotations := map[string]string{
				fmt.Sprintf(apicommon.MD5ChecksumAnnotationKey, feature.OrchestratorExplorerIDType): hash,
			}
			annotations := mgr.AnnotationMgr.Annotations
			assert.True(t, apiutils.IsEqualStruct(annotations, wantAnnotations), "Annotations \ndiff = %s", cmp.Diff(annotations, wantAnnotations))
		},
	)
}
