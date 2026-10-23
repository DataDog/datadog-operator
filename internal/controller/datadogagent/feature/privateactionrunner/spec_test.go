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
	const shared = "private_action_runner:\n  actions_allowlist:\n    - com.datadoghq.script.runPredefinedScript\n  task_concurrency: 3\n"
	const custom = "private_action_runner:\n  self_enroll: true\n  identity_secret_name: cluster-par-identity\n"
	tests := []struct {
		name        string
		config      v2alpha1.PrivateActionRunnerFeatureConfig
		annotations map[string]string
		agentTag    string
		wantNode    bool
		wantCluster bool
		wantSplit   bool
		wantNodeYML string
		wantClusYML string
	}{
		{
			name:     "shared enable and config apply to both agents",
			config:   v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true), ConfigData: new(shared)},
			wantNode: true, wantCluster: true, wantSplit: true,
			wantNodeYML: shared, wantClusYML: shared,
		},
		{
			name:     "default config enables both agents",
			config:   v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true)},
			wantNode: true, wantCluster: true, wantSplit: true,
			wantNodeYML: defaultConfigData, wantClusYML: defaultConfigData,
		},
		{
			name: "node config overrides shared config without affecting cluster",
			config: v2alpha1.PrivateActionRunnerFeatureConfig{
				Enabled: new(true), ConfigData: new(shared),
				NodeAgent: &v2alpha1.PrivateActionRunnerNodeAgentConfig{ConfigData: new(custom), SplitEnabled: new(false)},
			},
			wantNode: true, wantCluster: true,
			wantNodeYML: custom, wantClusYML: shared,
		},
		{
			name: "cluster config overrides shared config without affecting node",
			config: v2alpha1.PrivateActionRunnerFeatureConfig{
				Enabled: new(true), ConfigData: new(shared),
				ClusterAgent: &v2alpha1.PrivateActionRunnerClusterAgentConfig{ConfigData: new(custom), K8sRemediationEnabled: new(true)},
			},
			wantNode: true, wantCluster: true, wantSplit: true,
			wantNodeYML: shared, wantClusYML: custom,
		},
		{
			name: "disable node only",
			config: v2alpha1.PrivateActionRunnerFeatureConfig{
				Enabled: new(true), ConfigData: new(shared),
				NodeAgent: &v2alpha1.PrivateActionRunnerNodeAgentConfig{Enabled: new(false)},
			},
			wantCluster: true, wantClusYML: shared,
		},
		{
			name: "disable cluster only",
			config: v2alpha1.PrivateActionRunnerFeatureConfig{
				Enabled: new(true), ConfigData: new(shared),
				ClusterAgent: &v2alpha1.PrivateActionRunnerClusterAgentConfig{Enabled: new(false)},
			},
			wantNode: true, wantSplit: true, wantNodeYML: shared,
		},
		{
			name: "enable node despite shared disable",
			config: v2alpha1.PrivateActionRunnerFeatureConfig{
				Enabled: new(false), ConfigData: new(shared),
				NodeAgent: &v2alpha1.PrivateActionRunnerNodeAgentConfig{Enabled: new(true)},
			},
			wantNode: true, wantSplit: true, wantNodeYML: shared,
		},
		{
			name: "enable cluster without shared enable",
			config: v2alpha1.PrivateActionRunnerFeatureConfig{
				ClusterAgent: &v2alpha1.PrivateActionRunnerClusterAgentConfig{Enabled: new(true)},
			},
			wantCluster: true, wantClusYML: defaultConfigData,
		},
		{
			name: "old node agent cannot use split mode",
			config: v2alpha1.PrivateActionRunnerFeatureConfig{
				Enabled: new(true), NodeAgent: &v2alpha1.PrivateActionRunnerNodeAgentConfig{SplitEnabled: new(true)},
			},
			agentTag: "7.83.0", wantNode: true, wantCluster: true,
			wantNodeYML: defaultConfigData, wantClusYML: defaultConfigData,
		},
		{
			name: "empty spec block overrides legacy enable annotations",
			annotations: map[string]string{
				featureutils.EnablePrivateActionRunnerAnnotation:             "true",
				featureutils.EnableClusterAgentPrivateActionRunnerAnnotation: "true",
			},
		},
		{
			name:   "spec overrides legacy disable config and split annotations",
			config: v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true), ConfigData: new(shared)},
			annotations: map[string]string{
				featureutils.EnablePrivateActionRunnerAnnotation:                  "false",
				featureutils.EnableClusterAgentPrivateActionRunnerAnnotation:      "false",
				featureutils.EnablePrivateActionRunnerSplitAnnotation:             "false",
				featureutils.PrivateActionRunnerConfigDataAnnotation:              "invalid: [",
				featureutils.ClusterAgentPrivateActionRunnerConfigDataAnnotation:  "invalid: [",
				featureutils.ClusterAgentPrivateActionRunnerK8sRemediationEnabled: "true",
			},
			wantNode: true, wantCluster: true, wantSplit: true, wantNodeYML: shared, wantClusYML: shared,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dda := &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{Name: "datadog", Namespace: "default", Annotations: tt.annotations},
				Spec:       v2alpha1.DatadogAgentSpec{Features: &v2alpha1.DatadogFeatures{PrivateActionRunner: &tt.config}},
			}
			if tt.agentTag != "" {
				dda.Spec.Override = map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
					v2alpha1.NodeAgentComponentName: {Image: &v2alpha1.AgentImageConfig{Tag: tt.agentTag}},
				}
			}
			f := buildPrivateActionRunnerFeature(nil).(*privateActionRunnerFeature)
			req := f.Configure(dda, &dda.Spec, nil)
			require.NoError(t, f.configErr)
			assert.Equal(t, tt.wantNode, req.Agent.IsEnabled())
			assert.Equal(t, tt.wantCluster, req.ClusterAgent.IsEnabled())
			assert.Equal(t, tt.wantSplit, f.splitEnabled)
			if tt.wantNode {
				assert.Contains(t, req.Agent.Containers, apicommon.PrivateActionRunnerContainerName)
				assertRunnerConfig(t, tt.wantNodeYML, f.nodeConfigData)
			}
			if tt.wantCluster {
				assertRunnerConfig(t, tt.wantClusYML, f.clusterConfigData)
			}
			if tt.config.ConfigData != nil {
				assert.Equal(t, shared, *tt.config.ConfigData, "shared input must remain unchanged")
			}
			wantRemediation := tt.config.ClusterAgent != nil && tt.config.ClusterAgent.K8sRemediationEnabled != nil && *tt.config.ClusterAgent.K8sRemediationEnabled
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

func assertRunnerConfig(t *testing.T, expected, actual string) {
	t.Helper()
	data, err := overrideEnabledValueInConfigData(expected, true)
	require.NoError(t, err)
	assert.YAMLEq(t, data, actual)
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
      actionsAllowlist:
        - com.datadoghq.script.runPredefinedScript
      configData: |
        private_action_runner:
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

func TestPrivateActionRunnerActionsAllowlistOverridesYAML(t *testing.T) {
	var dda v2alpha1.DatadogAgent
	require.NoError(t, yaml.UnmarshalStrict([]byte(`spec:
  features:
    privateActionRunner:
      enabled: true
      actionsAllowlist:
        - com.datadoghq.script.runPredefinedScript
      nodeAgent:
        actionsAllowlist:
          - com.datadoghq.script.testConnection
      clusterAgent:
        actionsAllowlist: []
`), &dda))
	f := buildPrivateActionRunnerFeature(nil).(*privateActionRunnerFeature)
	f.Configure(&dda, &dda.Spec, nil)
	require.NoError(t, f.configErr)
	node, err := parsePrivateActionRunnerConfig(f.nodeConfigData)
	require.NoError(t, err)
	assert.Equal(t, []string{"com.datadoghq.script.testConnection"}, node.ActionsAllowlist)
	assert.Equal(t, []string{}, f.clusterConfig.ActionsAllowlist)
}

func TestPrivateActionRunnerActionsAllowlist(t *testing.T) {
	shared := []string{"com.datadoghq.script.runPredefinedScript"}
	node := []string{"com.datadoghq.script.testConnection"}
	cluster := []string{"com.datadoghq.http.request"}
	empty := []string{}
	const advanced = "private_action_runner:\n  enabled: true\n  task_concurrency: 3\n  actions_allowlist:\n    - from-config-data\n  advanced_option: preserved\n"
	tests := []struct {
		name        string
		config      v2alpha1.PrivateActionRunnerFeatureConfig
		wantNode    []string
		wantCluster []string
	}{
		{
			name:     "shared list applies to both agents",
			config:   v2alpha1.PrivateActionRunnerFeatureConfig{ActionsAllowlist: &shared},
			wantNode: shared, wantCluster: shared,
		},
		{
			name: "node override replaces shared list",
			config: v2alpha1.PrivateActionRunnerFeatureConfig{
				ActionsAllowlist: &shared,
				NodeAgent:        &v2alpha1.PrivateActionRunnerNodeAgentConfig{ActionsAllowlist: &node},
			},
			wantNode: node, wantCluster: shared,
		},
		{
			name: "cluster override replaces shared list",
			config: v2alpha1.PrivateActionRunnerFeatureConfig{
				ActionsAllowlist: &shared,
				ClusterAgent:     &v2alpha1.PrivateActionRunnerClusterAgentConfig{ActionsAllowlist: &cluster},
			},
			wantNode: shared, wantCluster: cluster,
		},
		{
			name: "both components can override independently",
			config: v2alpha1.PrivateActionRunnerFeatureConfig{
				ActionsAllowlist: &shared,
				NodeAgent:        &v2alpha1.PrivateActionRunnerNodeAgentConfig{ActionsAllowlist: &node},
				ClusterAgent:     &v2alpha1.PrivateActionRunnerClusterAgentConfig{ActionsAllowlist: &cluster},
			},
			wantNode: node, wantCluster: cluster,
		},
		{
			name: "empty node override clears inherited list",
			config: v2alpha1.PrivateActionRunnerFeatureConfig{
				ActionsAllowlist: &shared,
				NodeAgent:        &v2alpha1.PrivateActionRunnerNodeAgentConfig{ActionsAllowlist: &empty},
			},
			wantNode: empty, wantCluster: shared,
		},
		{
			name: "empty cluster override clears inherited list",
			config: v2alpha1.PrivateActionRunnerFeatureConfig{
				ActionsAllowlist: &shared,
				ClusterAgent:     &v2alpha1.PrivateActionRunnerClusterAgentConfig{ActionsAllowlist: &empty},
			},
			wantNode: shared, wantCluster: empty,
		},
		{
			name:     "empty common list overrides raw config",
			config:   v2alpha1.PrivateActionRunnerFeatureConfig{ActionsAllowlist: &empty, ConfigData: new(advanced)},
			wantNode: empty, wantCluster: empty,
		},
		{
			name: "shared typed list takes precedence over component config data",
			config: v2alpha1.PrivateActionRunnerFeatureConfig{
				ActionsAllowlist: &shared,
				NodeAgent:        &v2alpha1.PrivateActionRunnerNodeAgentConfig{ConfigData: new(advanced)},
				ClusterAgent:     &v2alpha1.PrivateActionRunnerClusterAgentConfig{ConfigData: new(advanced)},
			},
			wantNode: shared, wantCluster: shared,
		},
		{
			name: "component typed lists take precedence over raw config",
			config: v2alpha1.PrivateActionRunnerFeatureConfig{
				ConfigData: new(advanced), ActionsAllowlist: &shared,
				NodeAgent:    &v2alpha1.PrivateActionRunnerNodeAgentConfig{ActionsAllowlist: &node},
				ClusterAgent: &v2alpha1.PrivateActionRunnerClusterAgentConfig{ActionsAllowlist: &cluster},
			},
			wantNode: node, wantCluster: cluster,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.config.Enabled = new(true)
			dda := &v2alpha1.DatadogAgent{Spec: v2alpha1.DatadogAgentSpec{Features: &v2alpha1.DatadogFeatures{PrivateActionRunner: &tt.config}}}
			// Exercise the API round trip, including explicit empty lists.
			data, err := yaml.Marshal(dda)
			require.NoError(t, err)
			require.NoError(t, yaml.Unmarshal(data, dda))
			before := dda.DeepCopy()
			f := buildPrivateActionRunnerFeature(nil).(*privateActionRunnerFeature)
			req := f.Configure(dda, &dda.Spec, nil)
			require.NoError(t, f.configErr)
			require.True(t, req.Agent.IsEnabled())
			require.True(t, req.ClusterAgent.IsEnabled())
			for _, component := range []struct {
				data string
				want []string
			}{
				{f.nodeConfigData, tt.wantNode},
				{f.clusterConfigData, tt.wantCluster},
			} {
				config, err := parsePrivateActionRunnerConfig(component.data)
				require.NoError(t, err)
				assert.Equal(t, component.want, config.ActionsAllowlist)
				assert.True(t, config.Enabled)
			}
			assert.Equal(t, before.Spec, dda.Spec, "resolving component lists must not modify the spec")
			if tt.config.ConfigData != nil {
				assert.Contains(t, f.nodeConfigData, "advanced_option: preserved")
				assert.Contains(t, f.clusterConfigData, "advanced_option: preserved")
			}
		})
	}
}
