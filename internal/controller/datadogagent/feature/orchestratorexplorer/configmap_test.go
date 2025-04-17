// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestratorexplorer

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
)

func Test_buildOrchestratorExplorerConfigMap(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	defaultConfig := `---
cluster_check: false
ad_identifiers:
  - _kube_orchestrator
init_config:

instances:
  - skip_leader_election: false
`

	customConfigData := `cluster_check: true
init_config:
instances:
  - collectors:
      - nodes
      - services`

	tests := []struct {
		name              string
		feature           *orchestratorExplorerFeature
		wantErr           bool
		wantConfigMapName string
		wantConfigData    string
		preloadCM         bool
	}{
		{
			name: "default case - no custom config",
			feature: &orchestratorExplorerFeature{
				owner: &metav1.ObjectMeta{
					Name:      "test-agent",
					Namespace: "test-ns",
				},
				configConfigMapName: "test-agent-orchestrator-explorer-config",
				customConfig:        nil,
				customResources:     nil,
			},
			wantErr:           false,
			wantConfigMapName: "test-agent-orchestrator-explorer-config",
			wantConfigData:    defaultConfig,
		},
		{
			name: "custom config data provided",
			feature: &orchestratorExplorerFeature{
				owner: &metav1.ObjectMeta{
					Name:      "test-agent",
					Namespace: "test-ns",
				},
				configConfigMapName: "test-agent-orchestrator-explorer-config",
				customConfig: &v2alpha1.CustomConfig{
					ConfigData: &customConfigData,
				},
				customResources:     []string{"datadoghq.com/v1alpha1/datadogmetrics"},
				remoteConfigEnabled: true,
			},
			wantErr:           false,
			wantConfigMapName: "test-agent-orchestrator-explorer-config",
			wantConfigData: `cluster_check: true
init_config: null
instances:
- collectors:
  - nodes
  - services
  crd_collectors:
  - datadoghq.com/v1alpha1/datadogmetrics
`,
		},
		{
			name: "custom ConfigMap provided with remote config disabled",
			feature: &orchestratorExplorerFeature{
				owner: &metav1.ObjectMeta{
					Name:      "test-agent",
					Namespace: "test-ns",
				},
				configConfigMapName: "test-agent-orchestrator-explorer-config",
				customConfig: &v2alpha1.CustomConfig{
					ConfigMap: &v2alpha1.ConfigMapConfig{
						Name: "custom-cm",
					},
				},
				remoteConfigEnabled: false,
			},
			wantErr:           false,
			wantConfigMapName: "",
			wantConfigData:    "",
		},
		{
			name: "custom ConfigMap provided with remote config enabled",
			feature: &orchestratorExplorerFeature{
				owner: &metav1.ObjectMeta{
					Name:      "test-agent",
					Namespace: "test-ns",
				},
				k8sClient: fake.NewClientBuilder().WithScheme(scheme).WithObjects(
					&corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "custom-cm",
							Namespace: "test-ns",
						},
						Data: map[string]string{
							"orchestrator.yaml": customConfigData,
						},
					},
				).Build(),
				configConfigMapName: "test-agent-orchestrator-explorer-config",
				customConfig: &v2alpha1.CustomConfig{
					ConfigMap: &v2alpha1.ConfigMapConfig{
						Name: "custom-cm",
					},
				},
				customResources:     []string{"datadoghq.com/v1alpha1/datadogmetrics"},
				remoteConfigEnabled: true,
			},
			wantErr:           false,
			wantConfigMapName: "test-agent-orchestrator-explorer-config",
			wantConfigData: `cluster_check: true
init_config: null
instances:
- collectors:
  - nodes
  - services
  crd_collectors:
  - datadoghq.com/v1alpha1/datadogmetrics
`,
		},
		{
			name: "invalid custom config data",
			feature: &orchestratorExplorerFeature{
				owner: &metav1.ObjectMeta{
					Name:      "test-agent",
					Namespace: "test-ns",
				},
				configConfigMapName: "test-agent-orchestrator-explorer-config",
				customConfig: &v2alpha1.CustomConfig{
					ConfigData: apiutils.NewStringPointer("invalid: :\nyaml: content"),
				},
			},
			wantErr: true,
		},
		{
			name: "custom ConfigMap not found",
			feature: &orchestratorExplorerFeature{
				owner: &metav1.ObjectMeta{
					Name:      "test-agent",
					Namespace: "test-ns",
				},
				k8sClient:           fake.NewClientBuilder().WithScheme(scheme).Build(),
				configConfigMapName: "test-agent-orchestrator-explorer-config",
				customConfig: &v2alpha1.CustomConfig{
					ConfigMap: &v2alpha1.ConfigMapConfig{
						Name: "nonexistent-cm",
					},
				},
				remoteConfigEnabled: true,
			},
			wantErr:           false,
			wantConfigMapName: "test-agent-orchestrator-explorer-config",
			wantConfigData:    defaultConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up logger
			tt.feature.logger = logr.Discard()

			// Call the function
			cm, err := tt.feature.buildOrchestratorExplorerConfigMap()

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			if tt.wantConfigMapName == "" {
				require.Nil(t, cm)
				return
			}

			require.NotNil(t, cm)
			require.Equal(t, tt.wantConfigMapName, cm.Name)
			require.Equal(t, tt.feature.owner.GetNamespace(), cm.Namespace)

			if tt.wantConfigData != "" {
				require.Contains(t, cm.Data, orchestratorExplorerConfFileName)
				require.Equal(t, tt.wantConfigData, cm.Data[orchestratorExplorerConfFileName])
			}
		})
	}
}

func Test_orchestratorExplorerCheckConfig(t *testing.T) {
	crs := []string{"datadoghq.com/v1alpha1/datadogmetrics", "datadoghq.com/v1alpha1/watermarkpodautoscalers"}

	got := orchestratorExplorerCheckConfig(false, crs)
	want := `---
cluster_check: false
ad_identifiers:
  - _kube_orchestrator
init_config:

instances:
  - skip_leader_election: false
    crd_collectors:
      - datadoghq.com/v1alpha1/datadogmetrics
      - datadoghq.com/v1alpha1/watermarkpodautoscalers
`
	require.Equal(t, want, got)
}

func Test_buildConfigMapWithConfigData(t *testing.T) {
	ns := "test-ns"
	defaultConfig := `cluster_check: true
init_config:
instances:
  - collectors:
      - nodes
`
	invalidConfig := `init_config:`

	crs := []string{"datadoghq.com/v1alpha1/mycr"}

	tests := []struct {
		name           string
		customConfig   *string
		remoteEnabled  bool
		expectErr      bool
		expectCRMerged bool
	}{
		{
			name:           "Remote config enabled - CRs should be merged",
			customConfig:   &defaultConfig,
			remoteEnabled:  true,
			expectErr:      false,
			expectCRMerged: true,
		},
		{
			name:           "Remote config disabled - returns original config",
			customConfig:   &defaultConfig,
			remoteEnabled:  false,
			expectErr:      false,
			expectCRMerged: false,
		},
		{
			name:          "Invalid config - should return error",
			customConfig:  &invalidConfig,
			remoteEnabled: true,
			expectErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &orchestratorExplorerFeature{
				owner: &metav1.ObjectMeta{
					Namespace: ns,
				},
				customConfig: &v2alpha1.CustomConfig{
					ConfigData: tt.customConfig,
				},
				configConfigMapName: "orchestrator-config",
				customResources:     crs,
				remoteConfigEnabled: tt.remoteEnabled,
			}

			cm, err := f.buildConfigMapWithConfigData()

			if tt.expectErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, cm)
			require.Contains(t, cm.Data, orchestratorExplorerConfFileName)

			content := cm.Data[orchestratorExplorerConfFileName]

			if tt.expectCRMerged {
				for _, cr := range crs {
					require.Contains(t, content, cr, "Expected CR %q to be merged into config", cr)
				}
			} else {
				for _, cr := range crs {
					require.NotContains(t, content, cr, "Did not expect CR %q to be merged", cr)
				}
			}
		})
	}
}

func Test_buildConfigMapWithCustomConfig(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	type fields struct {
		remoteEnabled bool
		configData    map[string]string
		customCRs     []string
	}
	tests := []struct {
		name        string
		fields      fields
		expectNil   bool
		expectErr   bool
		preloadCM   bool
		cmName      string
		expectedCRs []string
	}{
		{
			name: "Remote config disabled",
			fields: fields{
				remoteEnabled: false,
			},
			expectNil: true,
			expectErr: false,
		},
		{
			name: "Missing custom ConfigMap",
			fields: fields{
				remoteEnabled: true,
			},
			cmName:    "nonexistent",
			expectErr: true,
		},
		{
			name: "Invalid YAML in custom ConfigMap",
			fields: fields{
				remoteEnabled: true,
				configData: map[string]string{
					"orchestrator.yaml": "bad_yaml: :",
				},
			},
			preloadCM: true,
			expectErr: true,
		},
		{
			name: "Valid config, new CRs added",
			fields: fields{
				remoteEnabled: true,
				configData: map[string]string{
					"orchestrator.yaml": `cluster_check: true
init_config:
instances:
  - collectors:
      - nodes
`,
				},
				customCRs: []string{"datadoghq.com/v1alpha1/datadogmetrics"},
			},
			preloadCM:   true,
			expectedCRs: []string{"datadoghq.com/v1alpha1/datadogmetrics"},
		},
		{
			name: "CRs already present, no change",
			fields: fields{
				remoteEnabled: true,
				configData: map[string]string{
					"orchestrator.yaml": `cluster_check: true
init_config:
instances:
  - collectors:
      - nodes
    crd_collectors:
      - datadoghq.com/v1alpha1/datadogmetrics
`,
				},
				customCRs: []string{"datadoghq.com/v1alpha1/datadogmetrics"},
			},
			preloadCM:   true,
			expectedCRs: []string{"datadoghq.com/v1alpha1/datadogmetrics"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns := "test-ns"
			cmName := tt.cmName
			if cmName == "" {
				cmName = "user-cm"
			}

			var objs []client.Object
			if tt.preloadCM {
				objs = append(objs, &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      cmName,
						Namespace: ns,
					},
					Data: tt.fields.configData,
				})
			}

			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()

			f := &orchestratorExplorerFeature{
				k8sClient:                k8sClient,
				configConfigMapName:      "generated",
				remoteConfigEnabled:      tt.fields.remoteEnabled,
				customResources:          tt.fields.customCRs,
				runInClusterChecksRunner: true,
				customConfig: &v2alpha1.CustomConfig{
					ConfigMap: &v2alpha1.ConfigMapConfig{
						Name: cmName,
					},
				},
				owner: &metav1.ObjectMeta{
					Namespace: ns,
				},
			}

			cm, err := f.buildConfigMapWithCustomConfig()
			if tt.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			if tt.expectNil {
				require.Nil(t, cm)
				return
			}

			require.NotNil(t, cm)
			merged := cm.Data["orchestrator.yaml"]
			for _, cr := range tt.expectedCRs {
				require.Contains(t, merged, cr)
			}
		})
	}
}

func Test_getAndValidateCustomConfig(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	namespace := "default"
	cmName := "test-configmap"

	validConfig := `cluster_check: true
init_config:
instances:
  - collectors:
      - nodes
`

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: namespace,
		},
		Data: map[string]string{
			"orchestrator.yaml": validConfig,
		},
	}

	tests := []struct {
		name                string
		clientObjs          []client.Object
		clusterCheckEnabled bool
		expectErr           bool
		expectedInstCount   int
	}{
		{
			name:                "valid single-entry configmap with cluster checks off",
			clientObjs:          []client.Object{cm},
			clusterCheckEnabled: false,
			expectErr:           false,
			expectedInstCount:   1,
		},
		{
			name:                "valid single-entry configmap with cluster checks on",
			clientObjs:          []client.Object{cm},
			clusterCheckEnabled: true,
			expectErr:           false,
			expectedInstCount:   1,
		},
		{
			name:                "configmap missing",
			clientObjs:          []client.Object{}, // no CM provided
			clusterCheckEnabled: false,
			expectErr:           true,
		},
		{
			name: "empty configmap",
			clientObjs: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: namespace},
				},
			},
			clusterCheckEnabled: false,
			expectErr:           true,
		},
		{
			name: "multi-entry configmap with cluster checks off (invalid)",
			clientObjs: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: namespace},
					Data: map[string]string{
						"file1.yaml": validConfig,
						"file2.yaml": validConfig,
					},
				},
			},
			clusterCheckEnabled: false,
			expectErr:           true,
		},
		{
			name: "multi-entry configmap with cluster checks on (valid)",
			clientObjs: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: namespace},
					Data: map[string]string{
						"file1.yaml": validConfig,
						"file2.yaml": validConfig,
					},
				},
			},
			clusterCheckEnabled: true,
			expectErr:           false,
			expectedInstCount:   2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.clientObjs...).
				Build()

			data, instances, err := getAndValidateCustomConfig(fakeClient, namespace, cmName, tt.clusterCheckEnabled)
			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedInstCount, len(instances))
				require.Equal(t, tt.expectedInstCount, len(data))
			}
		})
	}
}

func Test_getUniqueCustomResources(t *testing.T) {
	tests := []struct {
		name            string
		customResources []string
		instances       []orchestratorInstance
		expected        []string
	}{
		{
			name:            "no instances, return all customResources",
			customResources: []string{"a", "b", "c"},
			instances:       nil,
			expected:        []string{"a", "b", "c"},
		},
		{
			name:            "all CRDs already present in instances",
			customResources: []string{"a", "b"},
			instances: []orchestratorInstance{
				{CRDCollectors: []string{"a", "b"}},
			},
			expected: []string{},
		},
		{
			name:            "some CRDs are new",
			customResources: []string{"a", "b", "c"},
			instances: []orchestratorInstance{
				{CRDCollectors: []string{"a"}},
			},
			expected: []string{"b", "c"},
		},
		{
			name:            "no CRDs provided",
			customResources: []string{},
			instances: []orchestratorInstance{
				{CRDCollectors: []string{"a"}},
			},
			expected: []string{},
		},
		{
			name:            "multiple instances with overlapping CRDs",
			customResources: []string{"x", "y", "z"},
			instances: []orchestratorInstance{
				{CRDCollectors: []string{"x"}},
				{CRDCollectors: []string{"y"}},
			},
			expected: []string{"z"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getUniqueCustomResources(tt.customResources, tt.instances)
			require.ElementsMatch(t, tt.expected, result)
		})
	}
}

func Test_addCRToConfig(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		crs       []string
		want      string
		expectErr bool
	}{
		{
			name: "valid config with collectors",
			input: `cluster_check: true
init_config:
instances:
  - collectors:
      - nodes
      - services
`,
			crs: []string{
				"datadoghq.com/v1alpha1/datadogmetrics",
				"datadoghq.com/v1alpha1/watermarkpodautoscalers",
			},
			want: `cluster_check: true
init_config: null
instances:
- collectors:
  - nodes
  - services
  crd_collectors:
  - datadoghq.com/v1alpha1/datadogmetrics
  - datadoghq.com/v1alpha1/watermarkpodautoscalers
`,
			expectErr: false,
		},
		{
			name: "invalid config with missing instances",
			input: `cluster_check: true
init_config:
`,
			crs:       []string{"crd"},
			want:      "",
			expectErr: true,
		},
		{
			name: "invalid config with non-list instances",
			input: `cluster_check: true
init_config:
instances:
  collectors:
  - nodes
`,
			crs:       []string{"crd"},
			want:      "",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := addCRToConfig(tt.input, tt.crs)

			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_getConfigConfigMapName(t *testing.T) {
	tests := []struct {
		name                string
		owner               metav1.ObjectMeta
		customConfig        *v2alpha1.CustomConfig
		remoteConfigEnabled bool
		want                string
	}{
		{
			name: "default name - no custom config",
			owner: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "foo",
			},
			customConfig:        nil,
			remoteConfigEnabled: false,
			want:                "test-orchestrator-explorer-config",
		},
		{
			name: "custom config map name",
			owner: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "foo",
			},
			customConfig: &v2alpha1.CustomConfig{
				ConfigMap: &v2alpha1.ConfigMapConfig{
					Name: "orchestrator-config",
				},
			},
			remoteConfigEnabled: false,
			want:                "orchestrator-config",
		},
		{
			name: "custom config with remote config enabled - should use default name",
			owner: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "foo",
			},
			customConfig: &v2alpha1.CustomConfig{
				ConfigMap: &v2alpha1.ConfigMapConfig{
					Name: "orchestrator-config",
				},
			},
			remoteConfigEnabled: true,
			want:                "test-orchestrator-explorer-config",
		},
		{
			name: "custom config name matches default with remote config enabled",
			owner: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "foo",
			},
			customConfig: &v2alpha1.CustomConfig{
				ConfigMap: &v2alpha1.ConfigMapConfig{
					Name: "test-orchestrator-explorer-config",
				},
			},
			remoteConfigEnabled: true,
			want:                "test-orchestrator-explorer-config-rc",
		},
		{
			name: "custom config with ConfigData only",
			owner: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "foo",
			},
			customConfig: &v2alpha1.CustomConfig{
				ConfigData: apiutils.NewStringPointer("some-config"),
			},
			remoteConfigEnabled: false,
			want:                "test-orchestrator-explorer-config",
		},
		{
			name: "empty owner name",
			owner: metav1.ObjectMeta{
				Name:      "",
				Namespace: "foo",
			},
			customConfig:        nil,
			remoteConfigEnabled: false,
			want:                "-orchestrator-explorer-config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &orchestratorExplorerFeature{
				owner:               &tt.owner,
				customConfig:        tt.customConfig,
				remoteConfigEnabled: tt.remoteConfigEnabled,
			}
			got := f.getConfigConfigMapName()
			require.Equal(t, tt.want, got, "getConfigConfigMapName() = %v, want %v", got, tt.want)
		})
	}
}

func Test_getLastKey(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]string
		want string
	}{
		{
			name: "alphabetically ordered keys",
			m: map[string]string{
				"a": "1",
				"b": "2",
				"c": "3",
				"d": "4",
			},
			want: "d",
		},
		{
			name: "reverse ordered keys",
			m: map[string]string{
				"d": "4",
				"c": "3",
				"b": "2",
				"a": "1",
			},
			want: "d",
		},
		{
			name: "single key",
			m: map[string]string{
				"only": "value",
			},
			want: "only",
		},
		{
			name: "empty map",
			m:    map[string]string{},
			want: "",
		},
		{
			name: "numeric keys",
			m: map[string]string{
				"1":  "one",
				"2":  "two",
				"10": "ten",
			},
			want: "2",
		},
		{
			name: "mixed case keys",
			m: map[string]string{
				"A": "upper",
				"a": "lower",
				"B": "UPPER",
				"b": "LOWER",
			},
			want: "b",
		},
		{
			name: "orchestrator config files",
			m: map[string]string{
				"orchestrator.yaml":   "content1",
				"orchestrator.yaml.1": "content2",
				"orchestrator.yaml.2": "content3",
			},
			want: "orchestrator.yaml.2",
		},
		{
			name: "special characters",
			m: map[string]string{
				"key-1": "value1",
				"key.2": "value2",
				"key_3": "value3",
				"key@4": "value4",
				"key#5": "value5",
			},
			want: "key_3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getLastKey(tt.m)
			require.Equal(t, tt.want, got, "getLastKey() = %v, want %v", got, tt.want)
		})
	}
}
