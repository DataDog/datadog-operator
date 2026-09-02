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
	"k8s.io/utils/ptr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
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
	err := f.ManageNodeAgent(managers)
	assert.NoError(t, err)

	// Verify volumes (1 configmap + 3 host volumes)
	volumes := managers.VolumeMgr.Volumes
	assert.Len(t, volumes, 4)
	assert.Equal(t, "test-dda-privateactionrunner-config", volumes[0].Name, "Volume name should match")
	assert.NotNil(t, volumes[0].VolumeSource.ConfigMap, "Volume should be a ConfigMap volume")
	assert.Equal(t, "test-dda-privateactionrunner", volumes[0].VolumeSource.ConfigMap.Name, "ConfigMap name should match")

	volumeNames := make(map[string]bool)
	for _, v := range volumes {
		volumeNames[v.Name] = true
	}
	assert.True(t, volumeNames[common.ProcdirVolumeName])
	assert.True(t, volumeNames[common.SystemProbeOSReleaseDirVolumeName])
	assert.True(t, volumeNames[hostVarLogVolumeName])

	// Verify volume mounts (1 configmap + 3 host mounts)
	volumeMounts := managers.VolumeMountMgr.VolumeMountsByC[apicommon.PrivateActionRunnerContainerName]
	assert.Len(t, volumeMounts, 4)
	mount := volumeMounts[0]
	assert.Equal(t, "test-dda-privateactionrunner-config", mount.Name, "Mount name should match")
	assert.Equal(t, "/etc/datadog-agent/privateactionrunner.yaml", mount.MountPath, "Mount path should be the hardcoded path")
	assert.Equal(t, "privateactionrunner.yaml", mount.SubPath, "SubPath should mount the file directly")
	assert.True(t, mount.ReadOnly, "Mount should be read-only")

	mountNames := make(map[string]bool)
	for _, m := range volumeMounts {
		mountNames[m.Name] = true
	}
	assert.True(t, mountNames[common.ProcdirVolumeName])
	assert.True(t, mountNames[common.SystemProbeOSReleaseDirVolumeName])
	assert.True(t, mountNames[hostVarLogVolumeName])

	// Verify host mounts are read-only
	for _, m := range volumeMounts {
		if m.Name == common.ProcdirVolumeName || m.Name == common.SystemProbeOSReleaseDirVolumeName || m.Name == hostVarLogVolumeName {
			assert.True(t, m.ReadOnly, "mount %s should be read-only", m.Name)
		}
	}

	// Verify NET_RAW capability
	capabilities := managers.SecurityContextMgr.CapabilitiesByC[apicommon.PrivateActionRunnerContainerName]
	assert.Contains(t, capabilities, corev1.Capability("NET_RAW"))

	assert.Empty(t, managers.AnnotationMgr.Annotations)
}

func Test_privateActionRunnerFeature_ManageNodeAgentSplitMode(t *testing.T) {
	testScheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(testScheme))
	require.NoError(t, v2alpha1.AddToScheme(testScheme))

	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dda",
			Namespace: "default",
			Annotations: map[string]string{
				"agent.datadoghq.com/private-action-runner-enabled": "true",
				"agent.datadoghq.com/private-action-runner-configdata": `private_action_runner:
  enabled: true
  split_enabled: true`,
			},
		},
	}
	f := buildPrivateActionRunnerFeature(nil)
	f.Configure(dda, &v2alpha1.DatadogAgentSpec{}, nil)

	resourceManagers := feature.NewResourceManagers(store.NewStore(dda, &store.StoreOptions{Scheme: testScheme}))
	require.NoError(t, f.ManageDependencies(resourceManagers))
	obj, found := resourceManagers.Store().Get(kubernetes.ConfigMapKind, "default", "test-dda-privateactionrunner")
	require.True(t, found)
	configMap := obj.(*corev1.ConfigMap)
	assert.Equal(t, privateActionRunnerControlProcessConfig, configMap.Data[privateActionRunnerControlProcessFileName])
	assert.Equal(t, privateActionRunnerExecutorProcessConfig, configMap.Data[privateActionRunnerExecutorProcessFileName])

	podTmpl := corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:    string(apicommon.PrivateActionRunnerContainerName),
				Command: []string{"monolithic-command"},
			}},
		},
	}
	managers := fake.NewPodTemplateManagers(t, podTmpl)
	require.NoError(t, f.ManageNodeAgent(managers))

	require.Len(t, managers.Tpl.Spec.Containers, 1)
	assert.Equal(t, []string{
		privateActionRunnerPythonPath,
		"-c",
		privateActionRunnerProcessManagerBootstrap,
	}, managers.Tpl.Spec.Containers[0].Command)
	assert.Nil(t, managers.Tpl.Spec.Containers[0].Args)

	processVolumeName := "test-dda-privateactionrunner-processes"
	var processVolume *corev1.Volume
	for i := range managers.VolumeMgr.Volumes {
		if managers.VolumeMgr.Volumes[i].Name == processVolumeName {
			processVolume = managers.VolumeMgr.Volumes[i]
			break
		}
	}
	require.NotNil(t, processVolume)
	assert.Equal(t, "test-dda-privateactionrunner", processVolume.ConfigMap.Name)
	assert.ElementsMatch(t, []corev1.KeyToPath{
		{Key: privateActionRunnerControlProcessFileName, Path: privateActionRunnerControlProcessFileName},
		{Key: privateActionRunnerExecutorProcessFileName, Path: privateActionRunnerExecutorProcessFileName},
	}, processVolume.ConfigMap.Items)

	mounts := managers.VolumeMountMgr.VolumeMountsByC[apicommon.PrivateActionRunnerContainerName]
	assert.Contains(t, mounts, &corev1.VolumeMount{
		Name:      processVolumeName,
		MountPath: privateActionRunnerProcessesPath,
		ReadOnly:  true,
	})
	assert.Contains(t, mounts, ptr.To(common.GetVolumeMountForRunPath()))
	assert.Contains(t, managers.EnvVarMgr.EnvVarsByC[apicommon.PrivateActionRunnerContainerName], &corev1.EnvVar{
		Name:  "DD_PM_SOCKET_PATH",
		Value: privateActionRunnerProcessManagerSocketPath,
	})
}

// Test_privateActionRunnerFeature_ProfileDDAI_ConfigMapNames verifies that when PAR is
// enabled on a profile DDAI (whose name differs from the parent DDA), the ConfigMaps are
// named after the DDA (not the DDAI) so all profile DDAIs share the same ConfigMap.
func Test_privateActionRunnerFeature_ProfileDDAI_ConfigMapNames(t *testing.T) {
	testScheme := runtime.NewScheme()
	_ = corev1.AddToScheme(testScheme)
	_ = v2alpha1.AddToScheme(testScheme)

	// Simulate a profile DDAI: name differs from parent DDA, but DDA name is in the label.
	profileDDAI := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "compute-nodeless-200m-v2",
			Namespace: "default",
			Labels: map[string]string{
				apicommon.DatadogAgentNameLabelKey: "datadog-agent",
			},
			Annotations: map[string]string{
				"agent.datadoghq.com/private-action-runner-enabled":         "true",
				"cluster-agent.datadoghq.com/private-action-runner-enabled": "true",
			},
		},
	}

	f := buildPrivateActionRunnerFeature(nil)
	f.Configure(profileDDAI, &v2alpha1.DatadogAgentSpec{}, nil)

	storeOptions := &store.StoreOptions{Scheme: testScheme}
	resourceManagers := feature.NewResourceManagers(store.NewStore(profileDDAI, storeOptions))
	err := f.ManageDependencies(resourceManagers)
	require.NoError(t, err)

	// Node agent ConfigMap must use the DDA name so all DDAIs share the same ConfigMap.
	_, found := resourceManagers.Store().Get(kubernetes.ConfigMapKind, "default", "datadog-agent-privateactionrunner")
	assert.True(t, found, "node agent ConfigMap should use DDA name, not profile DDAI name")
	_, wrongFound := resourceManagers.Store().Get(kubernetes.ConfigMapKind, "default", "compute-nodeless-200m-v2-privateactionrunner")
	assert.False(t, wrongFound, "node agent ConfigMap must NOT use profile DDAI name")

	// Cluster agent ConfigMap must use the DDA name for the same reason.
	_, caFound := resourceManagers.Store().Get(kubernetes.ConfigMapKind, "default", "datadog-agent-clusteragent-privateactionrunner")
	assert.True(t, caFound, "cluster agent ConfigMap should use DDA name, not profile DDAI name")
	_, caWrongFound := resourceManagers.Store().Get(kubernetes.ConfigMapKind, "default", "compute-nodeless-200m-v2-clusteragent-privateactionrunner")
	assert.False(t, caWrongFound, "cluster agent ConfigMap must NOT use profile DDAI name")
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

			err := f.ManageDependencies(resourceManagers)
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

			assert.Empty(t, configMap.Annotations)
		})
	}
}

func Test_privateActionRunnerFeature_ConfigureClusterAgent(t *testing.T) {
	tests := []struct {
		name                      string
		annotations               map[string]string
		wantClusterAgentEnabled   bool
		wantNodeAgentEnabled      bool
		wantK8sRemediationEnabled bool
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
		{
			name: "k8s remediation annotation enabled",
			annotations: map[string]string{
				"cluster-agent.datadoghq.com/private-action-runner-enabled":                 "true",
				"cluster-agent.datadoghq.com/private-action-runner-k8s-remediation-enabled": "true",
			},
			wantClusterAgentEnabled:   true,
			wantNodeAgentEnabled:      false,
			wantK8sRemediationEnabled: true,
			expectedClusterConfigData: defaultConfigData,
		},
		{
			name: "k8s remediation annotation disabled",
			annotations: map[string]string{
				"cluster-agent.datadoghq.com/private-action-runner-enabled":                 "true",
				"cluster-agent.datadoghq.com/private-action-runner-k8s-remediation-enabled": "false",
			},
			wantClusterAgentEnabled:   true,
			wantNodeAgentEnabled:      false,
			wantK8sRemediationEnabled: false,
			expectedClusterConfigData: defaultConfigData,
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

			assert.Equal(t, tt.wantK8sRemediationEnabled, parFeat.k8sRemediationEnabled, "k8sRemediationEnabled should match")
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
			err := f.ManageDependencies(resourceManagers)
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
			err = f.ManageClusterAgent(managers)
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

			// Verify container args were modified (ENTRYPOINT=/entrypoint.sh stays in Command, CMD goes in Args)
			podTemplate := managers.PodTemplateSpec()
			containerFound := false
			for _, container := range podTemplate.Spec.Containers {
				if container.Name == string(apicommon.ClusterAgentContainerName) {
					containerFound = true
					assert.NotEmpty(t, container.Args, "Container args should be set")
					assert.Contains(t, container.Args, "datadog-cluster-agent")
					assert.Contains(t, container.Args, "start")
					assert.Contains(t, container.Args, "-E=/etc/datadog-agent/privateactionrunner.yaml", "Container args should contain -E flag")
					break
				}
			}
			assert.True(t, containerFound, "Cluster agent container should be found")

			assert.Empty(t, managers.AnnotationMgr.Annotations)
		})
	}
}
