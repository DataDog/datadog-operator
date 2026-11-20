// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/go-logr/logr/funcr"
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
	featureutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
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
		{name: "component feature config disables node PAR", spec: `features:
  privateActionRunner:
    enabled: true
    nodeAgent:
      enabled: false
`, wantCluster: true},
		{name: "component feature config disables cluster PAR", spec: `features:
  privateActionRunner:
    enabled: true
    clusterAgent:
      enabled: false
`, wantNode: true, wantSplit: true},
		{name: "component enables node over shared disable", spec: `features:
  privateActionRunner:
    enabled: false
    nodeAgent:
      enabled: true
`, wantNode: true, wantSplit: true},
		{name: "component enables cluster without shared enablement", spec: `features:
  privateActionRunner:
    clusterAgent:
      enabled: true
`, wantCluster: true},
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
			// A disabled agent must not veto components required by other features.
			if !tt.wantNode {
				assert.Nil(t, req.Agent.IsRequired)
			}
			if !tt.wantCluster {
				assert.Nil(t, req.ClusterAgent.IsRequired)
			}
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
					v2alpha1.ClusterAgentComponentName: {CustomConfigurations: map[v2alpha1.AgentConfigFileName]v2alpha1.CustomConfig{
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
            enabled: false
            actions_allowlist: [raw-action]
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
		dda.Spec.Override[component.name] = &v2alpha1.DatadogAgentComponentOverride{CustomConfigurations: map[v2alpha1.AgentConfigFileName]v2alpha1.CustomConfig{
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
	assert.Empty(t, f.clusterConfig.ActionsAllowlist, "raw allowlists are consumed by the Agent")
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
	for _, data := range []string{"private_action_runner: [", "private_action_runner: false", "private_action_runner:\n  self_enroll: invalid"} {
		t.Run(data, func(t *testing.T) {
			dda := &v2alpha1.DatadogAgent{Spec: v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{PrivateActionRunner: &v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true)}},
				Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
					v2alpha1.ClusterAgentComponentName: {CustomConfigurations: map[v2alpha1.AgentConfigFileName]v2alpha1.CustomConfig{
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
			settings := &PrivateActionRunnerConfig{}
			err := readClusterEnrollmentSettings(owner, &v2alpha1.DatadogAgentComponentOverride{
				CustomConfigurations: map[v2alpha1.AgentConfigFileName]v2alpha1.CustomConfig{
					v2alpha1.ClusterAgentConfigFile: {ConfigMap: &tt.ref, ConfigData: new("invalid: [")},
				},
			}, reader, settings)
			if tt.error != "" {
				require.ErrorContains(t, err, tt.error)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, new(false), settings.SelfEnroll)
			assert.Equal(t, "custom-identity", settings.IdentitySecretName)
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
	assert.Empty(t, f.clusterConfig.ActionsAllowlist, "raw allowlists are consumed by the Agent")
	assert.Nil(t, f.clusterConfig.SelfEnroll)
	dda.Spec.Override[v2alpha1.ClusterAgentComponentName].CustomConfigurations[v2alpha1.ClusterAgentConfigFile] = v2alpha1.CustomConfig{ConfigData: new("private_action_runner: [")}
	req = f.Configure(dda, &dda.Spec, nil)
	assert.True(t, req.ClusterAgent.IsEnabled())
	require.Error(t, f.ManageDependencies(nil))
}

func TestKubernetesActionsLegacyConfigurationSkipsCustomConfiguration(t *testing.T) {
	dda := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Name: "datadog", Namespace: "default", Annotations: map[string]string{
		"cluster-agent.datadoghq.com/private-action-runner-configdata": "private_action_runner:\n  identity_secret_name: legacy-identity\n",
	}}}
	require.NoError(t, yaml.UnmarshalStrict([]byte(`features:
  kubernetesActions:
    enabled: true
override:
  clusterAgent:
    customConfigurations:
      datadog-cluster.yaml:
        configMap:
          name: cluster-config
`), &dda.Spec))
	// No Kubernetes reader: legacy config data takes precedence over the custom configuration.
	f := buildPrivateActionRunnerFeature(nil).(*privateActionRunnerFeature)
	req := f.Configure(dda, &dda.Spec, nil)
	require.NoError(t, f.configErr)
	assert.True(t, req.ClusterAgent.IsEnabled())
	assert.Equal(t, "legacy-identity", f.clusterConfig.IdentitySecretName)
}

func TestPrivateActionRunnerTypedSettings(t *testing.T) {
	shared := []string{"com.datadoghq.script.runPredefinedScript"}
	local := []string{"com.datadoghq.http.request"}
	empty := []string{}
	for _, tt := range []struct {
		name          string
		global        *v2alpha1.PrivateActionRunnerFeatureConfig
		component     *v2alpha1.PrivateActionRunnerComponentConfig
		wantEnabled   bool
		wantAllowlist []string
	}{
		{name: "allowlist is optional", global: &v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true)}, wantEnabled: true},
		{name: "inherit shared allowlist", global: &v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true), ActionsAllowlist: &shared}, wantEnabled: true, wantAllowlist: shared},
		{name: "empty component inherits shared settings", global: &v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true), ActionsAllowlist: &shared}, component: &v2alpha1.PrivateActionRunnerComponentConfig{}, wantEnabled: true, wantAllowlist: shared},
		{name: "component replaces shared allowlist", global: &v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true), ActionsAllowlist: &shared}, component: &v2alpha1.PrivateActionRunnerComponentConfig{ActionsAllowlist: &local}, wantEnabled: true, wantAllowlist: local},
		{name: "empty component list clears shared allowlist", global: &v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true), ActionsAllowlist: &shared}, component: &v2alpha1.PrivateActionRunnerComponentConfig{ActionsAllowlist: &empty}, wantEnabled: true, wantAllowlist: empty},
		{name: "component enables PAR over shared disable", global: &v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(false)}, component: &v2alpha1.PrivateActionRunnerComponentConfig{Enabled: new(true)}, wantEnabled: true},
		{name: "component enables PAR without shared enablement", global: &v2alpha1.PrivateActionRunnerFeatureConfig{}, component: &v2alpha1.PrivateActionRunnerComponentConfig{Enabled: new(true)}, wantEnabled: true},
		{name: "component disables PAR", global: &v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true)}, component: &v2alpha1.PrivateActionRunnerComponentConfig{Enabled: new(false)}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			config, err := configurationFromFeature(tt.global, tt.component)
			require.NoError(t, err)
			assert.Equal(t, tt.wantEnabled, config.enabled)
			if tt.wantEnabled {
				extra, parseErr := parsePrivateActionRunnerConfig(config.configData)
				require.NoError(t, parseErr)
				assert.True(t, extra.Enabled)
				assert.Equal(t, tt.wantAllowlist, extra.ActionsAllowlist)
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
    nodeAgent:
      enabled: true
      actionsAllowlist:
        - com.datadoghq.http.request
    clusterAgent:
      enabled: true
      actionsAllowlist: []
override:
  nodeAgent:
    customConfigurations:
      datadog.yaml:
        configData: |
          private_action_runner:
            enabled: false
            actions_allowlist: [raw-action]
            task_concurrency: 3
  clusterAgent:
    customConfigurations:
      datadog-cluster.yaml:
        configData: |
          private_action_runner:
            actions_allowlist: [raw-action]
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
	assert.Nil(t, f.clusterConfig.TaskConcurrency, "advanced Agent settings must stay in the custom file")
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

func TestPrivateActionRunnerDisabledComponents(t *testing.T) {
	for _, tt := range []struct {
		name          string
		node, cluster bool
	}{
		{name: "both disabled"},
		{name: "node disabled", cluster: true},
		{name: "cluster disabled", node: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dda := &v2alpha1.DatadogAgent{Spec: v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{PrivateActionRunner: &v2alpha1.PrivateActionRunnerFeatureConfig{
					Enabled:      new(true),
					NodeAgent:    &v2alpha1.PrivateActionRunnerComponentConfig{Enabled: new(tt.node)},
					ClusterAgent: &v2alpha1.PrivateActionRunnerComponentConfig{Enabled: new(tt.cluster)},
				}},
			}}
			f := buildPrivateActionRunnerFeature(nil).(*privateActionRunnerFeature)
			req := f.Configure(dda, &dda.Spec, nil)
			require.NoError(t, f.configErr)
			assert.Equal(t, tt.node, req.Agent.IsEnabled())
			assert.Equal(t, tt.cluster, req.ClusterAgent.IsEnabled())
			assert.True(t, req.IsConfigured(), "disabled features must still configure running agents")
			if !tt.node {
				assert.NotContains(t, req.Agent.Containers, apicommon.PrivateActionRunnerContainerName)
				for _, container := range []apicommon.AgentContainerName{apicommon.CoreAgentContainerName, apicommon.UnprivilegedSingleAgentContainerName} {
					managers := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: string(container)}}}})
					if container == apicommon.UnprivilegedSingleAgentContainerName {
						require.NoError(t, f.ManageSingleContainerNodeAgent(managers))
					} else {
						require.NoError(t, f.ManageNodeAgent(managers))
					}
					assert.Contains(t, managers.EnvVarMgr.EnvVarsByC[container], &corev1.EnvVar{Name: DDPAREnabled, Value: "false"})
					assert.Empty(t, managers.VolumeMgr.Volumes)
				}
			}
			if !tt.cluster {
				managers := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: string(apicommon.ClusterAgentContainerName)}}}})
				require.NoError(t, f.ManageClusterAgent(managers))
				assert.Contains(t, managers.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName], &corev1.EnvVar{Name: DDPAREnabled, Value: "false"})
				assert.Empty(t, managers.VolumeMgr.Volumes)
			}
		})
	}
}

func TestPrivateActionRunnerDoesNotReadUnneededCustomConfigurations(t *testing.T) {
	dda := &v2alpha1.DatadogAgent{Spec: v2alpha1.DatadogAgentSpec{
		Features: &v2alpha1.DatadogFeatures{PrivateActionRunner: &v2alpha1.PrivateActionRunnerFeatureConfig{
			Enabled:      new(true),
			ClusterAgent: &v2alpha1.PrivateActionRunnerComponentConfig{Enabled: new(false)},
		}},
		Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
			v2alpha1.NodeAgentComponentName: {CustomConfigurations: map[v2alpha1.AgentConfigFileName]v2alpha1.CustomConfig{
				v2alpha1.AgentGeneralConfigFile: {ConfigMap: &v2alpha1.ConfigMapConfig{Name: "node-config"}},
			}},
			v2alpha1.ClusterAgentComponentName: {CustomConfigurations: map[v2alpha1.AgentConfigFileName]v2alpha1.CustomConfig{
				v2alpha1.ClusterAgentConfigFile: {ConfigMap: &v2alpha1.ConfigMapConfig{Name: "cluster-config"}},
			}},
		},
	}}
	// No Kubernetes reader: node configuration and disabled cluster configuration need no RBAC inspection.
	f := buildPrivateActionRunnerFeature(nil).(*privateActionRunnerFeature)
	req := f.Configure(dda, &dda.Spec, nil)
	require.NoError(t, f.configErr)
	assert.True(t, req.Agent.IsEnabled())
	assert.False(t, req.ClusterAgent.IsEnabled())
}

func TestPrivateActionRunnerDisabledClusterAgentWarning(t *testing.T) {
	const warning = "Warning: Private Action Runner is enabled on the Cluster Agent, but override.clusterAgent.disabled is true; the Cluster Agent will not be deployed"
	enabled := &v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true)}
	disabledOverride := &v2alpha1.DatadogAgentComponentOverride{Disabled: new(true)}
	for _, tt := range []struct {
		name              string
		par               *v2alpha1.PrivateActionRunnerFeatureConfig
		annotations       map[string]string
		kubernetesActions bool
		override          *v2alpha1.DatadogAgentComponentOverride
		wantWarning       bool
	}{
		{name: "feature enabled and Cluster Agent disabled", par: enabled, override: disabledOverride, wantWarning: true},
		{name: "component enabled over shared disable", par: &v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(false), ClusterAgent: &v2alpha1.PrivateActionRunnerComponentConfig{Enabled: new(true)}}, override: disabledOverride, wantWarning: true},
		{name: "legacy annotation enabled and Cluster Agent disabled", annotations: map[string]string{featureutils.EnableClusterAgentPrivateActionRunnerAnnotation: "true"}, override: disabledOverride, wantWarning: true},
		{name: "Kubernetes Actions enabled and Cluster Agent disabled", kubernetesActions: true, override: disabledOverride, wantWarning: true},
		{name: "feature enabled without component override", par: enabled},
		{name: "component override leaves disablement unset", par: enabled, override: &v2alpha1.DatadogAgentComponentOverride{}},
		{name: "Cluster Agent explicitly enabled", par: enabled, override: &v2alpha1.DatadogAgentComponentOverride{Disabled: new(false)}},
		{name: "PAR disabled", par: &v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(false)}, override: disabledOverride},
		{name: "PAR disabled on Cluster Agent only", par: &v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true), ClusterAgent: &v2alpha1.PrivateActionRunnerComponentConfig{Enabled: new(false)}}, override: disabledOverride},
		{name: "only node PAR enabled", annotations: map[string]string{featureutils.EnablePrivateActionRunnerAnnotation: "true"}, override: disabledOverride},
		{name: "PAR absent", override: disabledOverride},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var warnings []string
			logger := funcr.New(func(_, message string) {
				if strings.Contains(message, warning) {
					warnings = append(warnings, message)
				}
			}, funcr.Options{})
			dda := &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{Annotations: tt.annotations},
				Spec: v2alpha1.DatadogAgentSpec{
					Features: &v2alpha1.DatadogFeatures{PrivateActionRunner: tt.par},
				},
			}
			if tt.kubernetesActions {
				dda.Spec.Features.KubernetesActions = &v2alpha1.KubernetesActionsFeatureConfig{Enabled: new(true)}
			}
			if tt.override != nil {
				dda.Spec.Override = map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{v2alpha1.ClusterAgentComponentName: tt.override}
			}
			f := buildPrivateActionRunnerFeature(&feature.Options{Logger: logger})
			req := f.Configure(dda, &dda.Spec, nil)
			if tt.wantWarning {
				assert.Len(t, warnings, 1)
				assert.True(t, req.ClusterAgent.IsEnabled(), "the warning must preserve feature requirements; component overrides are applied later")
			} else {
				assert.Empty(t, warnings)
			}
		})
	}
}
