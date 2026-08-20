// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestratorexplorer

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	mergerfake "github.com/DataDog/datadog-operator/internal/controller/datadogagent/merger/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/testutils"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

var customConfData = `cluster_check: false
init_config:
instances:
  - skip_leader_election: false
    collectors:
      - clusterrolebindings`

var expectedOrchestratorEnvs = []*corev1.EnvVar{
	{
		Name:  DDOrchestratorExplorerEnabled,
		Value: "true",
	},
	{
		Name:  DDOrchestratorExplorerContainerScrubbingEnabled,
		Value: "true",
	},
	{
		Name:  DDOrchestratorExplorerExtraTags,
		Value: `["a:z","b:y","c:x"]`,
	},
	{
		Name:  DDOrchestratorExplorerDDUrl,
		Value: "https://foo.bar",
	},
}

func Test_orchestratorExplorerFeature_Configure(t *testing.T) {
	tests := test.FeatureTestSuite{
		{
			Name: "orchestrator explorer not enabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOrchestratorExplorerEnabled(false).
				Build(),
			WantConfigure: true,
		},
		{
			Name: "orchestrator explorer enabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOrchestratorExplorerEnabled(true).
				WithOrchestratorExplorerScrubContainers(true).
				WithOrchestratorExplorerExtraTags([]string{"a:z", "b:y", "c:x"}).
				WithOrchestratorExplorerDDUrl("https://foo.bar").
				WithOrchestratorExplorerCustomConfigData(customConfData).
				WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{Image: &v2alpha1.AgentImageConfig{Tag: "7.51.0"}}).
				Build(),
			WantConfigure: true,
			ClusterAgent:  orchestratorExplorerClusterAgentWantFunc(),
			Agent:         test.NewDefaultComponentTest().WithWantFunc(orchestratorExplorerNodeAgentNoProcessAgentWantFunc),
		},
		{
			Name: "orchestrator explorer enabled with autoscaling and custom CRs",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOrchestratorExplorerEnabled(true).
				WithWorkloadAutoscalerEnabled(true).
				WithOrchestratorExplorerCustomResources([]string{"datadoghq.com/v1alpha1/datadogpodautoscalers", "datadoghq.com/v1alpha1/watermarkpodautoscalers"}).
				Build(),
			WantConfigure: true,
			WantDependenciesFunc: func(t testing.TB, sc store.StoreClient) {
				cm, found := sc.Get(kubernetes.ConfigMapKind, "", "-orchestrator-explorer-config")
				assert.True(t, found)
				assert.NotNil(t, cm)

				cmData := cm.(*corev1.ConfigMap).Data["orchestrator.yaml"]
				want := `---
cluster_check: false
ad_identifiers:
  - _kube_orchestrator
init_config:

instances:
  - skip_leader_election: false
    crd_collectors:
      - datadoghq.com/v1alpha1/watermarkpodautoscalers
      - datadoghq.com/v1alpha2/datadogpodautoscalers
`

				assert.Equal(t, want, cmData)
			},
		},
		{
			Name: "orchestrator explorer enabled with Karpenter and EKS NodeClass CRs",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOrchestratorExplorerEnabled(true).
				WithOrchestratorExplorerCustomResources([]string{"karpenter.k8s.aws/v1/nodeclasses", "eks.amazonaws.com/v1/nodeclasses"}).
				Build(),
			WantConfigure: true,
			WantDependenciesFunc: func(t testing.TB, sc store.StoreClient) {
				cm, found := sc.Get(kubernetes.ConfigMapKind, "", "-orchestrator-explorer-config")
				assert.True(t, found)
				assert.NotNil(t, cm)

				cmData := cm.(*corev1.ConfigMap).Data["orchestrator.yaml"]
				want := `---
cluster_check: false
ad_identifiers:
  - _kube_orchestrator
init_config:

instances:
  - skip_leader_election: false
    crd_collectors:
      - eks.amazonaws.com/v1/nodeclasses
      - karpenter.k8s.aws/v1/nodeclasses
`

				assert.Equal(t, want, cmData)
			},
		},
		{
			Name: "orchestrator explorer enabled and runs on cluster checks runner",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOrchestratorExplorerEnabled(true).
				WithOrchestratorExplorerScrubContainers(true).
				WithOrchestratorExplorerExtraTags([]string{"a:z", "b:y", "c:x"}).
				WithOrchestratorExplorerDDUrl("https://foo.bar").
				WithOrchestratorExplorerCustomConfigData(customConfData).
				WithClusterChecksEnabled(true).
				WithClusterChecksUseCLCEnabled(true).
				WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{Image: &v2alpha1.AgentImageConfig{Tag: "7.51.0"}}).
				Build(),
			WantConfigure:       true,
			ClusterAgent:        orchestratorExplorerClusterAgentWantFunc(),
			Agent:               test.NewDefaultComponentTest().WithWantFunc(orchestratorExplorerNodeAgentNoProcessAgentWantFunc),
			ClusterChecksRunner: test.NewDefaultComponentTest().WithWantFunc(orchestratorExplorerClusterChecksRunnerWantFunc),
		},
		{
			Name: "orchestrator explorer enabled on version requiring process agent",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOrchestratorExplorerEnabled(true).
				WithOrchestratorExplorerScrubContainers(true).
				WithOrchestratorExplorerExtraTags([]string{"a:z", "b:y", "c:x"}).
				WithOrchestratorExplorerDDUrl("https://foo.bar").
				WithOrchestratorExplorerCustomConfigData(customConfData).
				WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{Image: &v2alpha1.AgentImageConfig{Tag: "7.50.0"}}).
				Build(),
			WantConfigure: true,
			ClusterAgent:  orchestratorExplorerClusterAgentWantFunc(),
			Agent:         test.NewDefaultComponentTest().WithWantFunc(orchestratorExplorerNodeAgentWantFunc),
		},
		{
			Name: "orchestrator explorer enabled with network CRDs",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOrchestratorExplorerEnabled(true).
				WithAnnotations(map[string]string{"agent.datadoghq.com/network-crds-enabled": "true"}).
				WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{Image: &v2alpha1.AgentImageConfig{Tag: "7.51.0"}}).
				Build(),
			WantConfigure: true,
			ClusterAgent:  orchestratorExplorerClusterAgentWithNetworkCRDsWantFunc(),
			Agent:         test.NewDefaultComponentTest().WithWantFunc(orchestratorExplorerNodeAgentWithNetworkCRDsWantFunc),
		},
	}

	tests.Run(t, buildOrchestratorExplorerFeature)
}

func orchestratorExplorerNodeAgentWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)
	agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.ProcessAgentContainerName]
	assert.True(t, apiutils.IsEqualStruct(agentEnvVars, expectedOrchestratorEnvs), "Process agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, expectedOrchestratorEnvs))
	agentEnvVars = mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
	assert.True(t, apiutils.IsEqualStruct(agentEnvVars, expectedOrchestratorEnvs), "Core agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, expectedOrchestratorEnvs))
}

func orchestratorExplorerNodeAgentNoProcessAgentWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)
	agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.ProcessAgentContainerName]
	assert.True(t, apiutils.IsEqualStruct(agentEnvVars, nil), "Process agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, expectedOrchestratorEnvs))
	agentEnvVars = mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
	assert.True(t, apiutils.IsEqualStruct(agentEnvVars, expectedOrchestratorEnvs), "Core agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, expectedOrchestratorEnvs))
}

func orchestratorExplorerClusterChecksRunnerWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)
	runnerEnvs := append(mgr.EnvVarMgr.EnvVarsByC[apicommon.AllContainers], mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterChecksRunnersContainerName]...)
	assert.True(t, apiutils.IsEqualStruct(runnerEnvs, expectedOrchestratorEnvs), "Cluster Checks Runner envvars \ndiff = %s", cmp.Diff(runnerEnvs, expectedOrchestratorEnvs))
}

var expectedOrchestratorNetworkCRDEnvs = []*corev1.EnvVar{
	{
		Name:  DDOrchestratorExplorerEnabled,
		Value: "true",
	},
	{
		Name:  DDOrchestratorExplorerContainerScrubbingEnabled,
		Value: "false",
	},
	{
		Name:  DDOrchestratorExplorerOOTBGatewayAPI,
		Value: "true",
	},
	{
		Name:  DDOrchestratorExplorerOOTBServiceMesh,
		Value: "true",
	},
	{
		Name:  DDOrchestratorExplorerOOTBIngressControllers,
		Value: "true",
	},
}

func orchestratorExplorerNodeAgentWithNetworkCRDsWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)
	agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
	assert.True(t, apiutils.IsEqualStruct(agentEnvVars, expectedOrchestratorNetworkCRDEnvs), "Core agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, expectedOrchestratorNetworkCRDEnvs))
}

func orchestratorExplorerClusterAgentWithNetworkCRDsWantFunc() *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)
			dcaEnvVars := mgr.EnvVarMgr.EnvVarsByC[mergerfake.AllContainers]
			assert.True(t, apiutils.IsEqualStruct(dcaEnvVars, expectedOrchestratorNetworkCRDEnvs), "DCA envvars \ndiff = %s", cmp.Diff(dcaEnvVars, expectedOrchestratorNetworkCRDEnvs))
		},
	)
}

func orchestratorExplorerClusterAgentWantFunc() *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)
			dcaEnvVars := mgr.EnvVarMgr.EnvVarsByC[mergerfake.AllContainers]
			assert.True(t, apiutils.IsEqualStruct(dcaEnvVars, expectedOrchestratorEnvs), "DCA envvars \ndiff = %s", cmp.Diff(dcaEnvVars, expectedOrchestratorEnvs))

			annotations := mgr.AnnotationMgr.Annotations
			assert.Empty(t, annotations)
		},
	)
}
