// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func TestPrivateActionRunnerSpec(t *testing.T) {
	tests := []struct {
		name        string
		spec        string
		annotations map[string]string
		wantNode    bool
		wantCluster bool
		wantSplit   bool
	}{
		{name: "minimal feature enables both agents", spec: `features:
  privateActionRunner:
    enabled: true
`, wantNode: true, wantCluster: true, wantSplit: true},
		{name: "explicit disable overrides annotations and Kubernetes Actions", spec: `features:
  privateActionRunner:
    enabled: false
  kubernetesActions:
    enabled: true
`, annotations: map[string]string{
			"agent.datadoghq.com/private-action-runner-enabled":         "true",
			"cluster-agent.datadoghq.com/private-action-runner-enabled": "true",
		}},
		{name: "empty feature block overrides automatic activation", spec: `features:
  privateActionRunner: {}
  kubernetesActions:
    enabled: true
`},
		{name: "component custom config disables node PAR", spec: `features:
  privateActionRunner:
    enabled: true
override:
  nodeAgent:
    customConfigurations:
      datadog.yaml:
        enabled: false
`, wantCluster: true},
		{name: "component custom config disables cluster PAR", spec: `features:
  privateActionRunner:
    enabled: true
override:
  clusterAgent:
    customConfigurations:
      datadog-cluster.yaml:
        enabled: false
`, wantNode: true, wantSplit: true},
		{name: "spec ignores legacy config but retains split opt out", spec: `features:
  privateActionRunner:
    enabled: true
`, annotations: map[string]string{
			"agent.datadoghq.com/private-action-runner-enabled":            "false",
			"cluster-agent.datadoghq.com/private-action-runner-enabled":    "false",
			"agent.datadoghq.com/private-action-runner-configdata":         "invalid: [",
			"cluster-agent.datadoghq.com/private-action-runner-configdata": "invalid: [",
			"agent.datadoghq.com/private-action-runner-split-enabled":      "false",
		}, wantNode: true, wantCluster: true},
		{name: "old images retain legacy mode", spec: `features:
  privateActionRunner:
    enabled: true
override:
  nodeAgent:
    image:
      tag: 7.83.0
`, wantNode: true, wantCluster: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dda := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Name: "datadog", Namespace: "default", Annotations: tt.annotations}}
			require.NoError(t, yaml.UnmarshalStrict([]byte(tt.spec), &dda.Spec))
			before := dda.DeepCopy()
			f := buildPrivateActionRunnerFeature(nil).(*privateActionRunnerFeature)
			req := f.Configure(dda, &dda.Spec, nil)
			require.NoError(t, f.configErr)
			assert.Equal(t, tt.wantNode, req.Agent.IsEnabled())
			assert.Equal(t, tt.wantCluster, req.ClusterAgent.IsEnabled())
			assert.Equal(t, tt.wantSplit, f.splitEnabled)
			assert.Equal(t, before.Spec, dda.Spec)
			if tt.wantCluster {
				assert.Nil(t, f.clusterConfig.SelfEnroll)
				assert.Empty(t, f.clusterConfig.IdentitySecretName)
				assert.NotContains(t, f.clusterConfigData, "self_enroll")
				assert.NotContains(t, f.clusterConfigData, "identity_secret_name")
			}
		})
	}
}

func TestPrivateActionRunnerSpecEnrollmentRBAC(t *testing.T) {
	for _, tt := range []struct {
		name       string
		configData string
		selfEnroll *bool
		secretName string
	}{
		{name: "omitted settings", secretName: "private-action-runner-identity"},
		{name: "null self enrollment", configData: "private_action_runner:\n  self_enroll: null\n", secretName: "private-action-runner-identity"},
		{name: "explicit self enrollment", configData: "private_action_runner:\n  self_enroll: true\n", selfEnroll: new(true), secretName: "private-action-runner-identity"},
		{name: "manual enrollment", configData: "private_action_runner:\n  self_enroll: false\n", selfEnroll: new(false)},
		{name: "custom identity", configData: "private_action_runner:\n  identity_secret_name: custom-identity\n", secretName: "custom-identity"},
		{name: "explicit legacy identity", configData: "private_action_runner:\n  identity_secret_name: datadog-private-action-runner-identity\n", secretName: "datadog-private-action-runner-identity"},
		{name: "file identity with omitted enrollment", configData: "private_action_runner:\n  identity_use_k8s_secret: false\n"},
		{name: "file identity with explicit enrollment", configData: "private_action_runner:\n  identity_use_k8s_secret: false\n  self_enroll: true\n", selfEnroll: new(true)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, corev1.AddToScheme(scheme))
			require.NoError(t, v2alpha1.AddToScheme(scheme))
			dda := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Name: "datadog", Namespace: "default", Annotations: map[string]string{
				"cluster-agent.datadoghq.com/private-action-runner-k8s-remediation-enabled": "true",
			}}, Spec: v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{PrivateActionRunner: &v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true)}},
				Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
					v2alpha1.ClusterAgentComponentName: {CustomConfigurations: map[v2alpha1.AgentConfigFileName]v2alpha1.AgentCustomConfig{
						v2alpha1.ClusterAgentConfigFile: {ConfigData: new(tt.configData)},
					}},
				},
			}}
			f := buildPrivateActionRunnerFeature(nil).(*privateActionRunnerFeature)
			req := f.Configure(dda, &dda.Spec, nil)
			require.True(t, req.ClusterAgent.IsEnabled())
			require.NoError(t, f.configErr)
			assert.Equal(t, tt.selfEnroll, f.clusterConfig.SelfEnroll)
			// The extra files must not override the Agent's defaults or the user's main config.
			assert.YAMLEq(t, defaultConfigData, f.nodeConfigData)
			assert.YAMLEq(t, defaultConfigData, f.clusterConfigData)
			resources := feature.NewResourceManagers(store.NewStore(dda, &store.StoreOptions{Scheme: scheme}))
			require.NoError(t, f.ManageDependencies(resources))
			role, roleFound := resources.Store().Get(kubernetes.RolesKind, dda.Namespace, f.getRbacResourcesName())
			_, bindingFound := resources.Store().Get(kubernetes.RoleBindingKind, dda.Namespace, f.getRbacResourcesName())
			wantRBAC := tt.secretName != ""
			require.Equal(t, wantRBAC, roleFound)
			assert.Equal(t, wantRBAC, bindingFound)
			if wantRBAC {
				assert.Equal(t, getClusterAgentRBACPolicyRules(tt.secretName), role.(*rbacv1.Role).Rules)
			}
			remediation, found := resources.Store().Get(kubernetes.ClusterRolesKind, "", f.getRbacResourcesName())
			require.True(t, found)
			assert.Equal(t, getK8sRemediationPolicyRules(), remediation.(*rbacv1.ClusterRole).Rules)
		})
	}
}

func TestPrivateActionRunnerCustomConfigurations(t *testing.T) {
	dda := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Name: "datadog", Namespace: "default"}}
	require.NoError(t, yaml.UnmarshalStrict([]byte(`features:
  privateActionRunner:
    enabled: true
override:
  nodeAgent:
    customConfigurations:
      datadog.yaml:
        configData: |
          private_action_runner:
            actions_allowlist:
              - com.datadoghq.script.runPredefinedScript
            task_concurrency: 3
  clusterAgent:
    customConfigurations:
      datadog-cluster.yaml:
        configData: |
          private_action_runner:
            self_enroll: false
            identity_secret_name: custom-identity
            actions_allowlist: []
`), &dda.Spec))
	f := buildPrivateActionRunnerFeature(nil).(*privateActionRunnerFeature)
	f.Configure(dda, &dda.Spec, nil)
	require.NoError(t, f.configErr)
	// User data stays in the main custom configuration file; the extra file supplies activation only.
	assert.YAMLEq(t, `private_action_runner:
  enabled: true
`, f.nodeConfigData)
	assert.YAMLEq(t, "private_action_runner:\n  enabled: true\n", f.clusterConfigData)
	assert.Equal(t, new(false), f.clusterConfig.SelfEnroll)
	assert.Equal(t, "custom-identity", f.clusterConfig.IdentitySecretName)
	assert.Empty(t, f.clusterConfig.ActionsAllowlist)
	assert.NotContains(t, f.nodeConfigData, "actions_allowlist")
	assert.NotContains(t, f.nodeConfigData, "task_concurrency")
}

func TestPrivateActionRunnerSharedConfigMap(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, v2alpha1.AddToScheme(scheme))
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "shared-par", Namespace: "default"}, Data: map[string]string{
		"par.yaml": `private_action_runner:
  self_enroll: true
  identity_secret_name: shared-identity
  actions_allowlist:
    - com.datadoghq.script.runPredefinedScript
`,
	}}
	reader := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	dda := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Name: "datadog", Namespace: "default"}, Spec: v2alpha1.DatadogAgentSpec{
		Features: &v2alpha1.DatadogFeatures{PrivateActionRunner: &v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true)}},
		Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{},
	}}
	for _, component := range []struct {
		name v2alpha1.ComponentName
		file v2alpha1.AgentConfigFileName
	}{
		{v2alpha1.NodeAgentComponentName, v2alpha1.AgentGeneralConfigFile},
		{v2alpha1.ClusterAgentComponentName, v2alpha1.ClusterAgentConfigFile},
	} {
		dda.Spec.Override[component.name] = &v2alpha1.DatadogAgentComponentOverride{CustomConfigurations: map[v2alpha1.AgentConfigFileName]v2alpha1.AgentCustomConfig{
			component.file: {ConfigMap: &v2alpha1.ConfigMapConfig{Name: "shared-par", Items: []corev1.KeyToPath{{Key: "par.yaml", Path: string(component.file)}}}},
		}}
	}
	f := buildPrivateActionRunnerFeature(&feature.Options{Client: reader}).(*privateActionRunnerFeature)
	before := dda.DeepCopy()
	req := f.Configure(dda, &dda.Spec, nil)
	require.NoError(t, f.configErr)
	assert.True(t, req.Agent.IsEnabled())
	assert.True(t, req.ClusterAgent.IsEnabled())
	assert.Equal(t, before.Spec, dda.Spec)
	assert.YAMLEq(t, "private_action_runner:\n  enabled: true\n", f.nodeConfigData)
	assert.YAMLEq(t, "private_action_runner:\n  enabled: true\n", f.clusterConfigData)
	assert.Equal(t, "shared-identity", f.clusterConfig.IdentitySecretName)
	assert.Equal(t, []string{"com.datadoghq.script.runPredefinedScript"}, f.clusterConfig.ActionsAllowlist)
	resources := feature.NewResourceManagers(store.NewStore(dda, &store.StoreOptions{Scheme: scheme}))
	require.NoError(t, f.ManageDependencies(resources))
	for _, name := range []string{f.getConfigMapName(), f.getClusterAgentConfigMapName()} {
		obj, found := resources.Store().Get(kubernetes.ConfigMapKind, "default", name)
		require.True(t, found)
		assert.YAMLEq(t, "private_action_runner:\n  enabled: true\n", obj.(*corev1.ConfigMap).Data[privateActionRunnerFileName])
	}
	// Configure may run more than once; its read-only client must survive the state reset.
	f.Configure(dda, &dda.Spec, nil)
	require.NoError(t, f.configErr)
}

func TestPrivateActionRunnerCustomConfigurationErrors(t *testing.T) {
	for _, data := range []string{"private_action_runner: [", "private_action_runner: false", "private_action_runner:\n  task_concurrency: invalid"} {
		t.Run(data, func(t *testing.T) {
			dda := &v2alpha1.DatadogAgent{Spec: v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{PrivateActionRunner: &v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true)}},
				Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
					v2alpha1.ClusterAgentComponentName: {CustomConfigurations: map[v2alpha1.AgentConfigFileName]v2alpha1.AgentCustomConfig{
						v2alpha1.ClusterAgentConfigFile: {ConfigData: new(data)},
					}},
				},
			}}
			f := buildPrivateActionRunnerFeature(nil)
			req := f.Configure(dda, &dda.Spec, nil)
			assert.True(t, req.ClusterAgent.IsEnabled(), "configuration errors must remain visible to dependency management")
			require.Error(t, f.ManageDependencies(nil))
		})
	}
}

func TestPrivateActionRunnerConfigMapConfiguration(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	reader := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "custom", Namespace: "default"},
		Data:       map[string]string{"datadog-cluster.yaml": "private_action_runner:\n  self_enroll: false\n  identity_secret_name: custom-identity\n"},
	}).Build()
	owner := &metav1.ObjectMeta{Namespace: "default"}
	for _, tt := range []struct {
		name  string
		ref   v2alpha1.ConfigMapConfig
		error string
	}{
		{name: "default key", ref: v2alpha1.ConfigMapConfig{Name: "custom"}},
		{name: "missing ConfigMap", ref: v2alpha1.ConfigMapConfig{Name: "missing"}, error: "read custom configuration ConfigMap missing"},
		{name: "missing key", ref: v2alpha1.ConfigMapConfig{Name: "custom", Items: []corev1.KeyToPath{{Key: "missing", Path: "datadog-cluster.yaml"}}}, error: "does not provide datadog-cluster.yaml"},
		{name: "unmounted path", ref: v2alpha1.ConfigMapConfig{Name: "custom", Items: []corev1.KeyToPath{{Key: "datadog-cluster.yaml", Path: "other.yaml"}}}, error: "does not provide datadog-cluster.yaml"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			config, err := configurationFromOverride(owner, &v2alpha1.DatadogAgentComponentOverride{
				CustomConfigurations: map[v2alpha1.AgentConfigFileName]v2alpha1.AgentCustomConfig{
					v2alpha1.ClusterAgentConfigFile: {ConfigMap: &tt.ref, ConfigData: new("invalid: [")},
				},
			}, v2alpha1.ClusterAgentConfigFile, &v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true)}, reader)
			if tt.error != "" {
				require.ErrorContains(t, err, tt.error)
				return
			}
			require.NoError(t, err)
			assert.True(t, config.enabled)
			assert.Equal(t, new(false), config.settings.SelfEnroll)
			assert.Equal(t, "custom-identity", config.settings.IdentitySecretName)
			assert.YAMLEq(t, "private_action_runner:\n  enabled: true\n", config.configData)
		})
	}
}

func TestKubernetesActionsCustomPrivateActionRunnerConfiguration(t *testing.T) {
	dda := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Name: "datadog", Namespace: "default"}}
	require.NoError(t, yaml.UnmarshalStrict([]byte(`features:
  kubernetesActions:
    enabled: true
override:
  clusterAgent:
    customConfigurations:
      datadog-cluster.yaml:
        configData: |
          private_action_runner:
            identity_secret_name: custom-identity
            actions_allowlist:
              - com.datadoghq.http.request
`), &dda.Spec))
	f := buildPrivateActionRunnerFeature(nil).(*privateActionRunnerFeature)
	req := f.Configure(dda, &dda.Spec, nil)
	require.NoError(t, f.configErr)
	assert.False(t, req.Agent.IsEnabled())
	assert.True(t, req.ClusterAgent.IsEnabled())
	assert.Equal(t, "custom-identity", f.clusterConfig.IdentitySecretName)
	assert.Equal(t, []string{"com.datadoghq.http.request"}, f.clusterConfig.ActionsAllowlist)
	assert.Nil(t, f.clusterConfig.SelfEnroll)
	dda.Spec.Override[v2alpha1.ClusterAgentComponentName].CustomConfigurations[v2alpha1.ClusterAgentConfigFile] = v2alpha1.AgentCustomConfig{ConfigData: new("private_action_runner: [")}
	req = f.Configure(dda, &dda.Spec, nil)
	assert.True(t, req.ClusterAgent.IsEnabled())
	require.Error(t, f.ManageDependencies(nil))
}

func TestPrivateActionRunnerTypedSettings(t *testing.T) {
	shared := []string{"com.datadoghq.script.runPredefinedScript"}
	local := []string{"com.datadoghq.http.request"}
	empty := []string{}
	for _, tt := range []struct {
		name          string
		global        *v2alpha1.PrivateActionRunnerFeatureConfig
		custom        v2alpha1.AgentCustomConfig
		wantEnabled   bool
		wantAllowlist []string
	}{
		{name: "allowlist is optional", global: &v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true)}, wantEnabled: true},
		{name: "inherit shared allowlist", global: &v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true), ActionsAllowlist: &shared}, wantEnabled: true, wantAllowlist: shared},
		{name: "component replaces shared allowlist", global: &v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true), ActionsAllowlist: &shared}, custom: v2alpha1.AgentCustomConfig{ActionsAllowlist: &local}, wantEnabled: true, wantAllowlist: local},
		{name: "empty component list clears shared and raw allowlists", global: &v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true), ActionsAllowlist: &shared}, custom: v2alpha1.AgentCustomConfig{ActionsAllowlist: &empty, ConfigData: new("private_action_runner:\n  actions_allowlist: [raw-action]\n")}, wantEnabled: true, wantAllowlist: empty},
		{name: "shared typed list overrides raw allowlist", global: &v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true), ActionsAllowlist: &shared}, custom: v2alpha1.AgentCustomConfig{ConfigData: new("private_action_runner:\n  actions_allowlist: [raw-action]\n")}, wantEnabled: true, wantAllowlist: shared},
		{name: "raw allowlist remains available without typed list", global: &v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true)}, custom: v2alpha1.AgentCustomConfig{ConfigData: new("private_action_runner:\n  actions_allowlist: [raw-action]\n")}, wantEnabled: true, wantAllowlist: []string{"raw-action"}},
		{name: "component enables PAR over global disable", global: &v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(false)}, custom: v2alpha1.AgentCustomConfig{Enabled: new(true)}, wantEnabled: true},
		{name: "component disables PAR without parsing unused YAML", global: &v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true)}, custom: v2alpha1.AgentCustomConfig{Enabled: new(false), ConfigData: new("invalid: [")}},
		{name: "typed enable overrides raw disable", global: &v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true)}, custom: v2alpha1.AgentCustomConfig{ConfigData: new("private_action_runner:\n  enabled: false\n")}, wantEnabled: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			override := &v2alpha1.DatadogAgentComponentOverride{CustomConfigurations: map[v2alpha1.AgentConfigFileName]v2alpha1.AgentCustomConfig{v2alpha1.AgentGeneralConfigFile: tt.custom}}
			config, err := configurationFromOverride(&metav1.ObjectMeta{}, override, v2alpha1.AgentGeneralConfigFile, tt.global, nil)
			require.NoError(t, err)
			assert.Equal(t, tt.wantEnabled, config.enabled)
			if tt.wantEnabled {
				assert.Equal(t, tt.wantAllowlist, config.settings.ActionsAllowlist)
				if tt.global.ActionsAllowlist != nil || tt.custom.ActionsAllowlist != nil {
					extra, parseErr := parsePrivateActionRunnerConfig(config.configData)
					require.NoError(t, parseErr)
					assert.Equal(t, tt.wantAllowlist, extra.ActionsAllowlist)
				}
			}
		})
	}
}

func TestPrivateActionRunnerTypedOverridesAndAdvancedConfiguration(t *testing.T) {
	dda := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Name: "datadog", Namespace: "default"}}
	require.NoError(t, yaml.UnmarshalStrict([]byte(`features:
  privateActionRunner:
    enabled: true
    actionsAllowlist:
      - com.datadoghq.script.runPredefinedScript
override:
  nodeAgent:
    customConfigurations:
      datadog.yaml:
        enabled: true
        actionsAllowlist:
          - com.datadoghq.http.request
        configData: |
          private_action_runner:
            task_concurrency: 3
  clusterAgent:
    customConfigurations:
      datadog-cluster.yaml:
        enabled: true
        actionsAllowlist: []
        configData: |
          private_action_runner:
            task_concurrency: 5
            identity_secret_name: custom-identity
`), &dda.Spec))
	// A [] override must survive the DDA-to-DDAI serialization and deepcopy.
	data, err := json.Marshal(dda)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, dda))
	before := dda.DeepCopy()
	f := buildPrivateActionRunnerFeature(nil).(*privateActionRunnerFeature)
	req := f.Configure(dda, &dda.Spec, nil)
	require.NoError(t, f.configErr)
	assert.True(t, req.Agent.IsEnabled())
	assert.True(t, req.ClusterAgent.IsEnabled())
	assert.Equal(t, []string{}, f.clusterConfig.ActionsAllowlist)
	assert.EqualValues(t, 5, *f.clusterConfig.TaskConcurrency)
	assert.Equal(t, "custom-identity", f.clusterConfig.IdentitySecretName)
	assert.Equal(t, before.Spec, dda.Spec)
	node, err := parsePrivateActionRunnerConfig(f.nodeConfigData)
	require.NoError(t, err)
	assert.Equal(t, []string{"com.datadoghq.http.request"}, node.ActionsAllowlist)
	assert.NotContains(t, f.nodeConfigData, "task_concurrency")
	managers := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{
		{Name: string(apicommon.CoreAgentContainerName), Command: []string{"agent", "run"}},
		{Name: string(apicommon.PrivateActionRunnerContainerName)},
	}}})
	require.NoError(t, f.ManageNodeAgent(managers))
	assert.Contains(t, managers.PodTemplateSpec().Spec.Containers[0].Args, "-E="+PrivateActionRunnerConfigPath)
	assert.Contains(t, managers.VolumeMountMgr.VolumeMountsByC[apicommon.CoreAgentContainerName], &corev1.VolumeMount{
		Name: "datadog-" + privateActionRunnerVolumeNameSuffix, MountPath: PrivateActionRunnerConfigPath, SubPath: privateActionRunnerFileName, ReadOnly: true,
	})
}
