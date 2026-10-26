// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	featureutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func TestPrivateActionRunnerSpec(t *testing.T) {
	tests := []struct {
		name              string
		config            v2alpha1.PrivateActionRunnerFeatureConfig
		annotations       map[string]string
		agentTag          string
		kubernetesActions bool
		wantNode          bool
		wantCluster       bool
		wantSplit         bool
		wantNodeYML       string
		wantClusYML       string
	}{
		{
			name: "shared enable and config apply to both agents",
			config: v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true), ConfigData: new(`private_action_runner:
  actions_allowlist:
    - com.datadoghq.script.runPredefinedScript
  task_concurrency: 3
`)},
			wantNode: true, wantCluster: true, wantSplit: true,
			wantNodeYML: `private_action_runner:
  enabled: true
  actions_allowlist:
    - com.datadoghq.script.runPredefinedScript
  task_concurrency: 3
`,
			wantClusYML: `private_action_runner:
  enabled: true
  actions_allowlist:
    - com.datadoghq.script.runPredefinedScript
  task_concurrency: 3
`,
		},
		{
			name:     "default config enables both agents",
			config:   v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true)},
			wantNode: true, wantCluster: true, wantSplit: true,
			wantNodeYML: `private_action_runner:
  enabled: true
`,
			wantClusYML: `private_action_runner:
  enabled: true
`,
		},
		{
			name: "node config overrides shared config without affecting cluster",
			config: v2alpha1.PrivateActionRunnerFeatureConfig{
				Enabled: new(true), ConfigData: new(`private_action_runner:
  actions_allowlist:
    - com.datadoghq.script.runPredefinedScript
  task_concurrency: 3
`),
				NodeAgent: &v2alpha1.PrivateActionRunnerNodeAgentConfig{ConfigData: new(`private_action_runner:
  self_enroll: true
  identity_secret_name: cluster-par-identity
`)},
			},
			wantNode: true, wantCluster: true, wantSplit: true,
			wantNodeYML: `private_action_runner:
  enabled: true
  self_enroll: true
  identity_secret_name: cluster-par-identity
`,
			wantClusYML: `private_action_runner:
  enabled: true
  actions_allowlist:
    - com.datadoghq.script.runPredefinedScript
  task_concurrency: 3
`,
		},
		{
			name: "cluster config overrides shared config without affecting node",
			config: v2alpha1.PrivateActionRunnerFeatureConfig{
				Enabled: new(true), ConfigData: new(`private_action_runner:
  actions_allowlist:
    - com.datadoghq.script.runPredefinedScript
  task_concurrency: 3
`),
				ClusterAgent: &v2alpha1.PrivateActionRunnerClusterAgentConfig{ConfigData: new(`private_action_runner:
  self_enroll: true
  identity_secret_name: cluster-par-identity
`)},
			},
			wantNode: true, wantCluster: true, wantSplit: true,
			wantNodeYML: `private_action_runner:
  enabled: true
  actions_allowlist:
    - com.datadoghq.script.runPredefinedScript
  task_concurrency: 3
`,
			wantClusYML: `private_action_runner:
  enabled: true
  self_enroll: true
  identity_secret_name: cluster-par-identity
`,
		},
		{
			name: "disable node only",
			config: v2alpha1.PrivateActionRunnerFeatureConfig{
				Enabled: new(true), ConfigData: new(`private_action_runner:
  actions_allowlist:
    - com.datadoghq.script.runPredefinedScript
  task_concurrency: 3
`),
				NodeAgent: &v2alpha1.PrivateActionRunnerNodeAgentConfig{Enabled: new(false)},
			},
			wantCluster: true,
			wantClusYML: `private_action_runner:
  enabled: true
  actions_allowlist:
    - com.datadoghq.script.runPredefinedScript
  task_concurrency: 3
`,
		},
		{
			name: "disable cluster only",
			config: v2alpha1.PrivateActionRunnerFeatureConfig{
				Enabled: new(true), ConfigData: new(`private_action_runner:
  actions_allowlist:
    - com.datadoghq.script.runPredefinedScript
  task_concurrency: 3
`),
				ClusterAgent: &v2alpha1.PrivateActionRunnerClusterAgentConfig{Enabled: new(false)},
			},
			wantNode: true, wantSplit: true,
			wantNodeYML: `private_action_runner:
  enabled: true
  actions_allowlist:
    - com.datadoghq.script.runPredefinedScript
  task_concurrency: 3
`,
		},
		{
			name: "enable node despite shared disable",
			config: v2alpha1.PrivateActionRunnerFeatureConfig{
				Enabled: new(false), ConfigData: new(`private_action_runner:
  actions_allowlist:
    - com.datadoghq.script.runPredefinedScript
  task_concurrency: 3
`),
				NodeAgent: &v2alpha1.PrivateActionRunnerNodeAgentConfig{Enabled: new(true)},
			},
			wantNode: true, wantSplit: true,
			wantNodeYML: `private_action_runner:
  enabled: true
  actions_allowlist:
    - com.datadoghq.script.runPredefinedScript
  task_concurrency: 3
`,
		},
		{
			name: "enable cluster without shared enable",
			config: v2alpha1.PrivateActionRunnerFeatureConfig{
				ClusterAgent: &v2alpha1.PrivateActionRunnerClusterAgentConfig{Enabled: new(true)},
			},
			wantCluster: true,
			wantClusYML: `private_action_runner:
  enabled: true
`,
		},
		{
			name: "old node agent cannot use split mode",
			config: v2alpha1.PrivateActionRunnerFeatureConfig{
				Enabled: new(true),
			},
			agentTag: "7.83.0", wantNode: true, wantCluster: true,
			wantNodeYML: `private_action_runner:
  enabled: true
`,
			wantClusYML: `private_action_runner:
  enabled: true
`,
		},
		{
			name: "empty spec block overrides legacy enable annotations",
			annotations: map[string]string{
				featureutils.EnablePrivateActionRunnerAnnotation:             "true",
				featureutils.EnableClusterAgentPrivateActionRunnerAnnotation: "true",
			},
		},
		{
			name: "spec overrides enable and config annotations but preserves split and remediation annotations",
			config: v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true), ConfigData: new(`private_action_runner:
  actions_allowlist:
    - com.datadoghq.script.runPredefinedScript
  task_concurrency: 3
`)},
			annotations: map[string]string{
				featureutils.EnablePrivateActionRunnerAnnotation:                  "false",
				featureutils.EnableClusterAgentPrivateActionRunnerAnnotation:      "false",
				featureutils.EnablePrivateActionRunnerSplitAnnotation:             "false",
				featureutils.PrivateActionRunnerConfigDataAnnotation:              "invalid: [",
				featureutils.ClusterAgentPrivateActionRunnerConfigDataAnnotation:  "invalid: [",
				featureutils.ClusterAgentPrivateActionRunnerK8sRemediationEnabled: "true",
			},
			wantNode: true, wantCluster: true,
			wantNodeYML: `private_action_runner:
  enabled: true
  actions_allowlist:
    - com.datadoghq.script.runPredefinedScript
  task_concurrency: 3
`,
			wantClusYML: `private_action_runner:
  enabled: true
  actions_allowlist:
    - com.datadoghq.script.runPredefinedScript
  task_concurrency: 3
`,
		},
		{
			name: "explicit shared config takes precedence over Kubernetes Actions defaults",
			config: v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true), ConfigData: new(`private_action_runner:
  actions_allowlist:
    - com.datadoghq.script.runPredefinedScript
  task_concurrency: 3
`)},
			kubernetesActions: true,
			wantNode:          true, wantCluster: true, wantSplit: true,
			wantNodeYML: `private_action_runner:
  enabled: true
  actions_allowlist:
    - com.datadoghq.script.runPredefinedScript
  task_concurrency: 3
`,
			wantClusYML: `private_action_runner:
  enabled: true
  actions_allowlist:
    - com.datadoghq.script.runPredefinedScript
  task_concurrency: 3
`,
		},
		{
			name: "explicit cluster disable takes precedence over Kubernetes Actions",
			config: v2alpha1.PrivateActionRunnerFeatureConfig{
				Enabled: new(true), ConfigData: new(`private_action_runner:
  actions_allowlist:
    - com.datadoghq.script.runPredefinedScript
  task_concurrency: 3
`),
				ClusterAgent: &v2alpha1.PrivateActionRunnerClusterAgentConfig{Enabled: new(false)},
			},
			kubernetesActions: true,
			wantNode:          true, wantSplit: true,
			wantNodeYML: `private_action_runner:
  enabled: true
  actions_allowlist:
    - com.datadoghq.script.runPredefinedScript
  task_concurrency: 3
`,
		},
		{
			name:              "empty spec block overrides automatic Kubernetes Actions configuration",
			kubernetesActions: true,
		},
		{
			name:     "empty spec config data still enables both agents",
			config:   v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true), ConfigData: new("")},
			wantNode: true, wantCluster: true, wantSplit: true,
			wantNodeYML: `private_action_runner:
  enabled: true
`,
			wantClusYML: `private_action_runner:
  enabled: true
`,
		},
		{
			name: "spec adds a missing PAR section and preserves unrelated config",
			config: v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true), ConfigData: new(`some_other_config:
  key: value
`)},
			wantNode: true, wantCluster: true, wantSplit: true,
			wantNodeYML: `some_other_config:
  key: value
private_action_runner:
  enabled: true
`,
			wantClusYML: `some_other_config:
  key: value
private_action_runner:
  enabled: true
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dda := &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{Name: "datadog", Namespace: "default", Annotations: tt.annotations},
				Spec:       v2alpha1.DatadogAgentSpec{Features: &v2alpha1.DatadogFeatures{PrivateActionRunner: &tt.config}},
			}
			if tt.kubernetesActions {
				dda.Spec.Features.KubernetesActions = &v2alpha1.KubernetesActionsFeatureConfig{Enabled: new(true)}
			}
			if tt.agentTag != "" {
				dda.Spec.Override = map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
					v2alpha1.NodeAgentComponentName: {Image: &v2alpha1.AgentImageConfig{Tag: tt.agentTag}},
				}
			}
			before := dda.DeepCopy()
			f := buildPrivateActionRunnerFeature(nil).(*privateActionRunnerFeature)
			req := f.Configure(dda, &dda.Spec, nil)
			require.NoError(t, f.configErr)
			assert.Equal(t, tt.wantNode, req.Agent.IsEnabled())
			assert.Equal(t, tt.wantCluster, req.ClusterAgent.IsEnabled())
			assert.Equal(t, tt.wantSplit, f.splitEnabled)
			if tt.wantNode {
				assert.Contains(t, req.Agent.Containers, apicommon.PrivateActionRunnerContainerName)
				assert.YAMLEq(t, tt.wantNodeYML, f.nodeConfigData)
			}
			if tt.wantCluster {
				assert.YAMLEq(t, tt.wantClusYML, f.clusterConfigData)
			}
			assert.Equal(t, before.Spec, dda.Spec, "configuring the runner must not modify the spec")
			wantRemediation := tt.annotations[featureutils.ClusterAgentPrivateActionRunnerK8sRemediationEnabled] == "true"
			assert.Equal(t, tt.wantCluster && wantRemediation, f.k8sRemediationEnabled)

			scheme := runtime.NewScheme()
			require.NoError(t, corev1.AddToScheme(scheme))
			require.NoError(t, v2alpha1.AddToScheme(scheme))
			resources := feature.NewResourceManagers(store.NewStore(dda, &store.StoreOptions{Scheme: scheme}))
			require.NoError(t, f.ManageDependencies(resources))
			for _, component := range []struct {
				enabled bool
				name    string
				config  string
			}{
				{tt.wantNode, f.getConfigMapName(), f.nodeConfigData},
				{tt.wantCluster, f.getClusterAgentConfigMapName(), f.clusterConfigData},
			} {
				obj, found := resources.Store().Get(kubernetes.ConfigMapKind, "default", component.name)
				require.Equal(t, component.enabled, found)
				if found {
					assert.YAMLEq(t, component.config, obj.(*corev1.ConfigMap).Data[privateActionRunnerFileName])
				}
			}
		})
	}
}

func TestPrivateActionRunnerSpecInvalidConfig(t *testing.T) {
	for _, data := range []string{"private_action_runner: [", "private_action_runner: false", "private_action_runner:\n  enabled: true\n  task_concurrency: invalid"} {
		for _, component := range []string{"node", "cluster"} {
			t.Run(component+"/"+data, func(t *testing.T) {
				config := &v2alpha1.PrivateActionRunnerFeatureConfig{}
				if component == "node" {
					config.NodeAgent = &v2alpha1.PrivateActionRunnerNodeAgentConfig{Enabled: new(true), ConfigData: new(data)}
				} else {
					config.ClusterAgent = &v2alpha1.PrivateActionRunnerClusterAgentConfig{Enabled: new(true), ConfigData: new(data)}
				}
				f := buildPrivateActionRunnerFeature(nil)
				dda := &v2alpha1.DatadogAgent{Spec: v2alpha1.DatadogAgentSpec{Features: &v2alpha1.DatadogFeatures{PrivateActionRunner: config}}}
				f.Configure(dda, &dda.Spec, nil)
				require.Error(t, f.ManageDependencies(nil))
			})
		}
	}
}

func TestPrivateActionRunnerSpecYAML(t *testing.T) {
	var dda v2alpha1.DatadogAgent
	require.NoError(t, yaml.Unmarshal([]byte(`spec:
  features:
    privateActionRunner:
      enabled: true
      configData: |
        private_action_runner:
          actions_allowlist:
            - com.datadoghq.script.runPredefinedScript
          enabled: false
          task_concurrency: 3
      clusterAgent:
        enabled: false
`), &dda))
	f := buildPrivateActionRunnerFeature(nil).(*privateActionRunnerFeature)
	req := f.Configure(&dda, &dda.Spec, nil)
	require.True(t, req.Agent.IsEnabled())
	require.False(t, req.ClusterAgent.IsEnabled())
	config, err := parsePrivateActionRunnerConfig(f.nodeConfigData)
	require.NoError(t, err)
	assert.True(t, config.Enabled)
	assert.Equal(t, []string{"com.datadoghq.script.runPredefinedScript"}, config.ActionsAllowlist)
	managers := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})
	require.NoError(t, f.ManageNodeAgent(managers))
	assert.Contains(t, managers.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName], &corev1.EnvVar{Name: DDPARTaskConcurrency, Value: "3"})

	dda.Spec.Features.PrivateActionRunner.Enabled = new(false)
	f.Configure(&dda, &dda.Spec, nil)
	assert.False(t, f.nodeEnabled)
	assert.False(t, f.splitEnabled)
	assert.Nil(t, f.clusterConfig)
}

func TestPrivateActionRunnerConfigOverridesYAML(t *testing.T) {
	var dda v2alpha1.DatadogAgent
	require.NoError(t, yaml.UnmarshalStrict([]byte(`spec:
  features:
    privateActionRunner:
      enabled: true
      configData: |
        private_action_runner:
          actions_allowlist:
            - com.datadoghq.script.runPredefinedScript
          task_concurrency: 3
          advanced_option: preserved
      nodeAgent:
        configData: |
          private_action_runner:
            actions_allowlist:
              - com.datadoghq.script.testConnection
      clusterAgent:
        configData: |
          private_action_runner:
            actions_allowlist: []
`), &dda))
	shared := *dda.Spec.Features.PrivateActionRunner.ConfigData
	f := buildPrivateActionRunnerFeature(nil).(*privateActionRunnerFeature)
	f.Configure(&dda, &dda.Spec, nil)
	require.NoError(t, f.configErr)
	node, err := parsePrivateActionRunnerConfig(f.nodeConfigData)
	require.NoError(t, err)
	assert.Equal(t, []string{"com.datadoghq.script.testConnection"}, node.ActionsAllowlist)
	assert.Equal(t, []string{}, f.clusterConfig.ActionsAllowlist)
	assert.Nil(t, node.TaskConcurrency, "component config replaces the entire shared configuration")
	assert.Nil(t, f.clusterConfig.TaskConcurrency)
	assert.NotContains(t, f.nodeConfigData, "advanced_option")
	assert.NotContains(t, f.clusterConfigData, "advanced_option")
	assert.Equal(t, shared, *dda.Spec.Features.PrivateActionRunner.ConfigData)

	// Removing overrides restores the common allowlist and unknown Agent options.
	dda.Spec.Features.PrivateActionRunner.NodeAgent = nil
	dda.Spec.Features.PrivateActionRunner.ClusterAgent = nil
	f.Configure(&dda, &dda.Spec, nil)
	require.NoError(t, f.configErr)
	for _, data := range []string{f.nodeConfigData, f.clusterConfigData} {
		config, err := parsePrivateActionRunnerConfig(data)
		require.NoError(t, err)
		assert.True(t, config.Enabled)
		assert.Equal(t, []string{"com.datadoghq.script.runPredefinedScript"}, config.ActionsAllowlist)
		require.NotNil(t, config.TaskConcurrency)
		assert.EqualValues(t, 3, *config.TaskConcurrency)
		assert.Contains(t, data, "advanced_option: preserved")
	}
}
