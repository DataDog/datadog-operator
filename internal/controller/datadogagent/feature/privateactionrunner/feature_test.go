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

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func Test_privateActionRunnerFeature_Configure(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		wantFunc    func(t *testing.T, reqComp feature.RequiredComponents)
	}{
		{
			name:        "feature not enabled (no annotation)",
			annotations: nil,
			wantFunc: func(t *testing.T, reqComp feature.RequiredComponents) {
				assert.False(t, reqComp.Agent.IsEnabled())
			},
		},
		{
			name: "feature enabled via annotation",
			annotations: map[string]string{
				"agent.datadoghq.com/private-action-runner-enabled": "true",
			},
			wantFunc: func(t *testing.T, reqComp feature.RequiredComponents) {
				assert.True(t, reqComp.Agent.IsEnabled())
				assert.Contains(t, reqComp.Agent.Containers, apicommon.CoreAgentContainerName)
				assert.Contains(t, reqComp.Agent.Containers, apicommon.PrivateActionRunnerContainerName)
			},
		},
		{
			name: "feature explicitly disabled via annotation",
			annotations: map[string]string{
				"agent.datadoghq.com/private-action-runner-enabled": "false",
			},
			wantFunc: func(t *testing.T, reqComp feature.RequiredComponents) {
				assert.False(t, reqComp.Agent.IsEnabled())
				assert.False(t, reqComp.ClusterAgent.IsEnabled())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := buildPrivateActionRunnerFeature(nil)
			dda := &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: tt.annotations,
				},
			}
			reqComp := f.Configure(
				dda,
				&v2alpha1.DatadogAgentSpec{},
				nil,
			)
			tt.wantFunc(t, reqComp)
		})
	}
}

func Test_privateActionRunnerFeature_ManageNodeAgent(t *testing.T) {
	f := buildPrivateActionRunnerFeature(nil)
	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dda",
			Namespace: "default",
			Annotations: map[string]string{
				"agent.datadoghq.com/private-action-runner-enabled": "true",
				"agent.datadoghq.com/private-action-runner-configdata": `private_action_runner:
	enabled: true
    private_key: some-key
    urn: urn:dd:apps:on-prem-runner:us1:1:runner-abc
    actions_allowlist:
        - com.datadoghq.script.testConnection
        - com.datadoghq.kubernetes.core.listPod`,
			},
		},
	}
	f.Configure(dda, &v2alpha1.DatadogAgentSpec{}, nil)

	// Create test managers
	podTmpl := corev1.PodTemplateSpec{}
	managers := fake.NewPodTemplateManagers(t, podTmpl)

	// Call ManageNodeAgent
	err := f.ManageNodeAgent(managers, "")
	assert.NoError(t, err)

	// Verify volume is mounted
	volumes := managers.VolumeMgr.Volumes
	assert.Len(t, volumes, 1, "Should have exactly one volume")
	vol := volumes[0]
	assert.Equal(t, "test-dda-privateactionrunner-config", vol.Name, "Volume name should match")
	assert.NotNil(t, vol.VolumeSource.ConfigMap, "Volume should be a ConfigMap volume")
	assert.Equal(t, "test-dda-privateactionrunner", vol.VolumeSource.ConfigMap.Name, "ConfigMap name should match")

	// Verify volume mount
	volumeMounts := managers.VolumeMountMgr.VolumeMountsByC[apicommon.PrivateActionRunnerContainerName]
	assert.Len(t, volumeMounts, 1, "Should have exactly one volume mount")
	mount := volumeMounts[0]
	assert.Equal(t, "test-dda-privateactionrunner-config", mount.Name, "Mount name should match")
	assert.Equal(t, "/etc/datadog-agent/privateactionrunner.yaml", mount.MountPath, "Mount path should be the hardcoded path")
	assert.Equal(t, "privateactionrunner.yaml", mount.SubPath, "SubPath should mount the file directly")
	assert.True(t, mount.ReadOnly, "Mount should be read-only")

	// Verify hash
	assert.NotEmpty(t, managers.AnnotationMgr.Annotations)
	assert.NotEmpty(t, managers.AnnotationMgr.Annotations["checksum/private_action_runner-custom-config"])
	assert.Equal(t, "7aca0ab8a2cb083533a5552c17a50aa3", managers.AnnotationMgr.Annotations["checksum/private_action_runner-custom-config"])
}

func Test_privateActionRunnerFeature_ID(t *testing.T) {
	f := buildPrivateActionRunnerFeature(nil)
	assert.Equal(t, string(feature.PrivateActionRunnerIDType), string(f.ID()))
}

func Test_privateActionRunnerFeature_ConfigMapContent(t *testing.T) {
	testScheme := runtime.NewScheme()
	_ = corev1.AddToScheme(testScheme)
	_ = v2alpha1.AddToScheme(testScheme)

	tests := []struct {
		name            string
		annotations     map[string]string
		expectConfigMap bool
		expectedYAML    string
		expectedHash    string
	}{
		{
			name: "feature disabled",
			annotations: map[string]string{
				"agent.datadoghq.com/private-action-runner-enabled": "false",
			},
			expectConfigMap: false,
		},
		{
			name: "enabled without configdata - uses default",
			annotations: map[string]string{
				"agent.datadoghq.com/private-action-runner-enabled": "true",
			},
			expectConfigMap: true,
			expectedYAML:    defaultConfigData,
			expectedHash:    "57aedff9cb18bcec9b12a3974ef6fc55",
		},
		{
			name: "enabled with configdata - passes through directly",
			annotations: map[string]string{
				"agent.datadoghq.com/private-action-runner-enabled": "true",
				"agent.datadoghq.com/private-action-runner-configdata": `private_action_runner:
    private_key: some-key
    urn: urn:dd:apps:on-prem-runner:us1:1:runner-abc
    self_enroll: false
    actions_allowlist:
        - com.datadoghq.script.testConnection
        - com.datadoghq.script.enrichScript`,
			},
			expectConfigMap: true,
			expectedYAML: `private_action_runner:
    private_key: some-key
    urn: urn:dd:apps:on-prem-runner:us1:1:runner-abc
    self_enroll: false
    actions_allowlist:
        - com.datadoghq.script.testConnection
        - com.datadoghq.script.enrichScript`,
			expectedHash: "76f45ac891d62eb42272bbe26f32fb7c",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := buildPrivateActionRunnerFeature(nil)
			dda := &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-dda",
					Namespace:   "default",
					Annotations: tt.annotations,
				},
			}
			f.Configure(dda, &v2alpha1.DatadogAgentSpec{}, nil)

			storeOptions := &store.StoreOptions{
				Scheme: testScheme,
			}
			resourceManagers := feature.NewResourceManagers(store.NewStore(dda, storeOptions))

			err := f.ManageDependencies(resourceManagers, "")
			require.NoError(t, err)

			if !tt.expectConfigMap {
				// Verify no ConfigMap was created
				_, found := resourceManagers.Store().Get(kubernetes.ConfigMapKind, "default", "test-dda-privateactionrunner")
				assert.False(t, found, "ConfigMap should not be created when feature is disabled")
				return
			}

			// Verify ConfigMap was created
			configMapName := "test-dda-privateactionrunner"
			cm, found := resourceManagers.Store().Get(kubernetes.ConfigMapKind, "default", configMapName)
			require.True(t, found, "ConfigMap should be created")
			require.NotNil(t, cm)

			configMap, ok := cm.(*corev1.ConfigMap)
			require.True(t, ok, "Object should be a ConfigMap")
			assert.Equal(t, configMapName, configMap.Name, "ConfigMap name should match")
			assert.Equal(t, "default", configMap.Namespace, "Namespace should match")
			require.Contains(t, configMap.Data, "privateactionrunner.yaml", "ConfigMap must contain privateactionrunner.yaml")

			yamlContent := configMap.Data["privateactionrunner.yaml"]

			// Verify content matches expected
			assert.Equal(t, tt.expectedYAML, yamlContent, "ConfigMap content should match expected output")

			// Verify hash
			assert.NotEmpty(t, configMap.Annotations)
			assert.NotEmpty(t, configMap.Annotations["checksum/private_action_runner-custom-config"])
			assert.Equal(t, tt.expectedHash, configMap.Annotations["checksum/private_action_runner-custom-config"])
		})
	}
}

func Test_privateActionRunnerFeature_ConfigureClusterAgent(t *testing.T) {
	tests := []struct {
		name                      string
		annotations               map[string]string
		wantClusterAgentEnabled   bool
		wantNodeAgentEnabled      bool
		expectedClusterConfigData string
	}{
		{
			name:                    "cluster agent not enabled (no annotation)",
			annotations:             nil,
			wantClusterAgentEnabled: false,
			wantNodeAgentEnabled:    false,
		},
		{
			name: "cluster agent enabled via annotation",
			annotations: map[string]string{
				"cluster-agent.datadoghq.com/private-action-runner-enabled": "true",
			},
			wantClusterAgentEnabled:   true,
			wantNodeAgentEnabled:      false,
			expectedClusterConfigData: defaultConfigData,
		},
		{
			name: "cluster agent enabled with custom config",
			annotations: map[string]string{
				"cluster-agent.datadoghq.com/private-action-runner-enabled": "true",
				"cluster-agent.datadoghq.com/private-action-runner-configdata": `private_action_runner:
  enabled: true
  self_enroll: true
  identity_secret_name: my-custom-secret`,
			},
			wantClusterAgentEnabled: true,
			wantNodeAgentEnabled:    false,
			expectedClusterConfigData: `private_action_runner:
  enabled: true
  self_enroll: true
  identity_secret_name: my-custom-secret`,
		},
		{
			name: "cluster agent explicitly disabled",
			annotations: map[string]string{
				"cluster-agent.datadoghq.com/private-action-runner-enabled": "false",
			},
			wantClusterAgentEnabled: false,
			wantNodeAgentEnabled:    false,
		},
		{
			name: "annotation true but config says enabled false - should force enable",
			annotations: map[string]string{
				"cluster-agent.datadoghq.com/private-action-runner-enabled": "true",
				"cluster-agent.datadoghq.com/private-action-runner-configdata": `private_action_runner:
  enabled: false
  self_enroll: true
  identity_secret_name: my-secret`,
			},
			wantClusterAgentEnabled:   true,
			wantNodeAgentEnabled:      false,
			expectedClusterConfigData: "", // Don't validate config fields since Enabled is forced to true
		},
		{
			name: "both node and cluster agent enabled",
			annotations: map[string]string{
				"agent.datadoghq.com/private-action-runner-enabled":         "true",
				"cluster-agent.datadoghq.com/private-action-runner-enabled": "true",
				"cluster-agent.datadoghq.com/private-action-runner-configdata": `private_action_runner:
  enabled: true
  self_enroll: false
  urn: urn:dd:apps:on-prem-runner:us1:1:runner-xyz`,
			},
			wantClusterAgentEnabled: true,
			wantNodeAgentEnabled:    true,
			expectedClusterConfigData: `private_action_runner:
  enabled: true
  self_enroll: false
  urn: urn:dd:apps:on-prem-runner:us1:1:runner-xyz`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := buildPrivateActionRunnerFeature(nil)
			dda := &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: tt.annotations,
				},
			}
			reqComp := f.Configure(dda, &v2alpha1.DatadogAgentSpec{}, nil)

			assert.Equal(t, tt.wantClusterAgentEnabled, reqComp.ClusterAgent.IsEnabled())
			assert.Equal(t, tt.wantNodeAgentEnabled, reqComp.Agent.IsEnabled())

			parFeat, ok := f.(*privateActionRunnerFeature)
			require.True(t, ok)

			// Check if cluster config is set correctly
			if tt.wantClusterAgentEnabled {
				assert.NotNil(t, parFeat.clusterConfig, "clusterConfig should not be nil when enabled")
				assert.True(t, parFeat.clusterConfig.Enabled, "clusterConfig.Enabled should be true")
				assert.NotEmpty(t, parFeat.clusterConfigData, "clusterConfigData should not be empty when enabled")

				// Validate the raw config data matches expected
				if tt.expectedClusterConfigData != "" {
					assert.Equal(t, tt.expectedClusterConfigData, parFeat.clusterConfigData)
				}
			} else {
				assert.Nil(t, parFeat.clusterConfig, "clusterConfig should be nil when not enabled")
			}
		})
	}
}

func Test_privateActionRunnerFeature_ManageClusterAgent_ConfigMap(t *testing.T) {
	testScheme := runtime.NewScheme()
	_ = corev1.AddToScheme(testScheme)
	_ = v2alpha1.AddToScheme(testScheme)

	tests := []struct {
		name                      string
		configData                string
		expectedClusterConfigData string
	}{
		{
			name: "self-enroll with identity secret",
			configData: `private_action_runner:
  enabled: true
  self_enroll: true
  identity_secret_name: my-par-identity`,
			expectedClusterConfigData: `private_action_runner:
  enabled: true
  self_enroll: true
  identity_secret_name: my-par-identity`,
		},
		{
			name: "manual enrollment with URN and private key",
			configData: `private_action_runner:
  enabled: true
  self_enroll: false
  urn: urn:dd:apps:on-prem-runner:us1:1:runner-abc
  private_key: my-secret-key
  identity_secret_name: par-secret`,
			expectedClusterConfigData: `private_action_runner:
  enabled: true
  self_enroll: false
  urn: urn:dd:apps:on-prem-runner:us1:1:runner-abc
  private_key: my-secret-key
  identity_secret_name: par-secret`,
		},
		{
			name: "with actions allowlist",
			configData: `private_action_runner:
  enabled: true
  self_enroll: true
  actions_allowlist:
    - com.datadoghq.http.request
    - com.datadoghq.kubernetes.core.listPod
    - com.datadoghq.traceroute`,
			expectedClusterConfigData: `private_action_runner:
  enabled: true
  self_enroll: true
  actions_allowlist:
    - com.datadoghq.http.request
    - com.datadoghq.kubernetes.core.listPod
    - com.datadoghq.traceroute`,
		},
		{
			name:                      "default config (minimal)",
			configData:                defaultConfigData,
			expectedClusterConfigData: defaultConfigData,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := buildPrivateActionRunnerFeature(nil)
			dda := &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dda",
					Namespace: "default",
					Annotations: map[string]string{
						"cluster-agent.datadoghq.com/private-action-runner-enabled":    "true",
						"cluster-agent.datadoghq.com/private-action-runner-configdata": tt.configData,
					},
				},
			}
			f.Configure(dda, &v2alpha1.DatadogAgentSpec{}, nil)

			// Create store and resource managers for ConfigMap creation
			storeOptions := &store.StoreOptions{
				Scheme: testScheme,
			}
			resourceManagers := feature.NewResourceManagers(store.NewStore(dda, storeOptions))

			// Call ManageDependencies to create the ConfigMap
			err := f.ManageDependencies(resourceManagers, "")
			require.NoError(t, err)

			// Verify ConfigMap was created with correct data
			cm, found := resourceManagers.Store().Get(kubernetes.ConfigMapKind, "default", "test-dda-clusteragent-privateactionrunner")
			require.True(t, found, "ConfigMap should be created")
			require.NotNil(t, cm)

			configMap, ok := cm.(*corev1.ConfigMap)
			require.True(t, ok, "Object should be a ConfigMap")
			assert.Equal(t, "test-dda-clusteragent-privateactionrunner", configMap.Name)
			assert.Equal(t, "default", configMap.Namespace)
			require.Contains(t, configMap.Data, "privateactionrunner.yaml", "ConfigMap must contain privateactionrunner.yaml")

			// Verify the ConfigMap contains the expected config data
			yamlContent := configMap.Data["privateactionrunner.yaml"]
			assert.Equal(t, tt.expectedClusterConfigData, yamlContent, "ConfigMap content should match expected")

			// Create test managers with a container
			podTmpl := corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: string(apicommon.ClusterAgentContainerName),
						},
					},
				},
			}
			managers := fake.NewPodTemplateManagers(t, podTmpl)

			// Call ManageClusterAgent
			err = f.ManageClusterAgent(managers, "")
			assert.NoError(t, err)

			// Verify volume was added
			volumes := managers.VolumeMgr.Volumes
			assert.Len(t, volumes, 1, "Expected 1 volume to be added")
			assert.Equal(t, "test-dda-privateactionrunner-config", volumes[0].Name)
			assert.NotNil(t, volumes[0].ConfigMap)
			assert.Equal(t, "test-dda-clusteragent-privateactionrunner", volumes[0].ConfigMap.Name)

			// Verify volume mount was added
			volumeMounts := managers.VolumeMountMgr.VolumeMountsByC[apicommon.ClusterAgentContainerName]
			assert.Len(t, volumeMounts, 1, "Expected 1 volume mount to be added")
			assert.Equal(t, "test-dda-privateactionrunner-config", volumeMounts[0].Name)
			assert.Equal(t, "/etc/datadog-agent/privateactionrunner.yaml", volumeMounts[0].MountPath)
			assert.Equal(t, "privateactionrunner.yaml", volumeMounts[0].SubPath)
			assert.True(t, volumeMounts[0].ReadOnly)

			// Verify container command was modified
			podTemplate := managers.PodTemplateSpec()
			containerFound := false
			for _, container := range podTemplate.Spec.Containers {
				if container.Name == string(apicommon.ClusterAgentContainerName) {
					containerFound = true
					assert.NotEmpty(t, container.Command, "Container command should be set")
					assert.Contains(t, container.Command, "datadog-cluster-agent")
					assert.Contains(t, container.Command, "start")
					assert.Contains(t, container.Command, "-E=/etc/datadog-agent/privateactionrunner.yaml", "Container command should contain -E flag")
					break
				}
			}
			assert.True(t, containerFound, "Cluster agent container should be found")

			// Verify checksum annotation was added
			annotations := managers.AnnotationMgr.Annotations
			checksumKey := object.GetChecksumAnnotationKey(feature.PrivateActionRunnerIDType)
			_, foundAnnotation := annotations[checksumKey]
			assert.True(t, foundAnnotation, "Checksum annotation should be present")
		})
	}
}
