// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/experimental"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/images"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	pkgutils "github.com/DataDog/datadog-operator/pkg/utils"
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
	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dda",
			Namespace: "default",
			Annotations: map[string]string{
				"agent.datadoghq.com/private-action-runner-enabled":       "true",
				"agent.datadoghq.com/private-action-runner-split-enabled": "true",
			},
		},
		Spec: v2alpha1.DatadogAgentSpec{
			Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
				v2alpha1.NodeAgentComponentName: {
					Image: &v2alpha1.AgentImageConfig{Tag: "7.84.0"},
				},
			},
		},
	}
	f := buildPrivateActionRunnerFeature(nil)
	f.Configure(dda, &dda.Spec, nil)

	gracePeriod := int64(30)
	managers := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			TerminationGracePeriodSeconds: &gracePeriod,
			Containers: []corev1.Container{
				{
					Name:    string(apicommon.PrivateActionRunnerContainerName),
					Command: []string{"privateactionrunner", "run"},
					Args:    []string{"-c=/etc/datadog-agent/datadog.yaml"},
				},
			},
		},
	})

	require.NoError(t, f.ManageNodeAgent(managers))

	parContainer := managers.PodTemplateSpec().Spec.Containers[0]
	assert.Equal(t, []string{privateActionRunnerEntrypoint}, parContainer.Command)
	assert.Empty(t, parContainer.Args)
	require.NotNil(t, parContainer.ReadinessProbe)
	require.NotNil(t, parContainer.ReadinessProbe.Exec)
	assert.Equal(t, []string{privateActionRunnerProbe}, parContainer.ReadinessProbe.Exec.Command)
	assert.Equal(t, privateActionRunnerGracePeriod, *managers.PodTemplateSpec().Spec.TerminationGracePeriodSeconds)

	var runVolumeFound bool
	for _, volume := range managers.VolumeMgr.Volumes {
		if volume.Name == privateActionRunnerRunVolumeName {
			runVolumeFound = true
			require.NotNil(t, volume.EmptyDir)
		}
	}
	assert.True(t, runVolumeFound)

	var runMountFound bool
	for _, mount := range managers.VolumeMountMgr.VolumeMountsByC[apicommon.PrivateActionRunnerContainerName] {
		if mount.Name == privateActionRunnerRunVolumeName {
			runMountFound = true
			assert.Equal(t, privateActionRunnerRunPath, mount.MountPath)
		}
	}
	assert.True(t, runMountFound)

	envs := managers.EnvVarMgr.EnvVarsByC[apicommon.PrivateActionRunnerContainerName]
	assert.Contains(t, envs, &corev1.EnvVar{Name: "DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED", Value: "true"})
	assert.Contains(t, envs, &corev1.EnvVar{Name: "DD_PRIVATE_ACTION_RUNNER_EXTRA_CONFIG_PATH", Value: PrivateActionRunnerConfigPath})
	assert.Contains(t, envs, &corev1.EnvVar{Name: "DD_PM_SOCKET_PATH", Value: privateActionRunnerSocketPath})
}

func Test_privateActionRunnerFeature_ManageNodeAgentMonolithicMode(t *testing.T) {
	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dda",
			Namespace: "default",
			Annotations: map[string]string{
				"agent.datadoghq.com/private-action-runner-enabled":       "true",
				"agent.datadoghq.com/private-action-runner-split-enabled": "false",
			},
		},
	}
	f := buildPrivateActionRunnerFeature(nil)
	f.Configure(dda, &dda.Spec, nil)

	command := []string{"privateactionrunner", "run"}
	args := []string{"-c=/etc/datadog-agent/datadog.yaml"}
	gracePeriod := int64(30)
	managers := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			TerminationGracePeriodSeconds: &gracePeriod,
			Containers: []corev1.Container{{
				Name:    string(apicommon.PrivateActionRunnerContainerName),
				Command: command,
				Args:    args,
			}},
		},
	})

	require.NoError(t, f.ManageNodeAgent(managers))

	parContainer := managers.PodTemplateSpec().Spec.Containers[0]
	assert.Equal(t, command, parContainer.Command)
	assert.Equal(t, args, parContainer.Args)
	assert.Nil(t, parContainer.ReadinessProbe)
	assert.Equal(t, gracePeriod, *managers.PodTemplateSpec().Spec.TerminationGracePeriodSeconds)
	assert.Empty(t, managers.EnvVarMgr.EnvVarsByC[apicommon.PrivateActionRunnerContainerName])
	for _, volume := range managers.VolumeMgr.Volumes {
		assert.NotEqual(t, privateActionRunnerRunVolumeName, volume.Name)
	}
}

func Test_privateActionRunnerFeature_RejectsSplitModeOnOldAgent(t *testing.T) {
	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"agent.datadoghq.com/private-action-runner-enabled":       "true",
				"agent.datadoghq.com/private-action-runner-split-enabled": "true",
			},
		},
		Spec: v2alpha1.DatadogAgentSpec{
			Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
				v2alpha1.NodeAgentComponentName: {
					Image: &v2alpha1.AgentImageConfig{Tag: "7.83.0"},
				},
			},
		},
	}
	f := buildPrivateActionRunnerFeature(nil)
	f.Configure(dda, &dda.Spec, nil)

	err := f.ManageNodeAgent(fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{}))
	require.ErrorContains(t, err, "split mode requires Agent >= 7.84.0-0")
}

func Test_privateActionRunnerFeature_RejectsSplitModeOnUnknownAgentVersion(t *testing.T) {
	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"agent.datadoghq.com/private-action-runner-enabled":       "true",
				"agent.datadoghq.com/private-action-runner-split-enabled": "true",
			},
		},
		Spec: v2alpha1.DatadogAgentSpec{
			Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
				v2alpha1.NodeAgentComponentName: {
					Image: &v2alpha1.AgentImageConfig{Tag: "dev"},
				},
			},
		},
	}
	f := buildPrivateActionRunnerFeature(nil)
	f.Configure(dda, &dda.Spec, nil)

	err := f.ManageNodeAgent(fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{}))
	require.ErrorContains(t, err, "split mode requires Agent >= 7.84.0-0")
}

func Test_privateActionRunnerFeature_SplitModeImageOverrides(t *testing.T) {
	tests := []struct {
		name         string
		image        v2alpha1.AgentImageConfig
		experimental string
		wantVersion  string
		wantError    bool
	}{
		{
			name:         "older PAR override",
			image:        v2alpha1.AgentImageConfig{Tag: "7.84.0"},
			experimental: `{"private-action-runner":{"tag":"7.83.0"}}`,
			wantVersion:  "7.83.0", wantError: true,
		},
		{
			name:         "newer PAR override",
			image:        v2alpha1.AgentImageConfig{Tag: "7.83.0"},
			experimental: `{"private-action-runner":{"tag":"7.84.0"}}`,
			wantVersion:  "7.84.0",
		},
		{
			name:         "full image name takes precedence over tag",
			image:        v2alpha1.AgentImageConfig{Tag: "7.84.0"},
			experimental: `{"private-action-runner":{"name":"example.com/agent:7.83.0","tag":"7.84.0"}}`,
			wantVersion:  "7.83.0", wantError: true,
		},
		{
			name:         "name-only override inherits component tag",
			image:        v2alpha1.AgentImageConfig{Tag: "7.84.0"},
			experimental: `{"private-action-runner":{"name":"custom-agent"}}`,
			wantVersion:  "7.84.0",
		},
		{
			name:         "unrelated container override",
			image:        v2alpha1.AgentImageConfig{Tag: "7.83.0"},
			experimental: `{"agent":{"tag":"7.84.0"}}`,
			wantVersion:  "7.83.0", wantError: true,
		},
		{
			name:         "malformed override is ignored",
			image:        v2alpha1.AgentImageConfig{Tag: "7.84.0"},
			experimental: `not-json`,
			wantVersion:  "7.84.0",
		},
		{
			name:         "unknown PAR version is rejected",
			image:        v2alpha1.AgentImageConfig{Tag: "7.84.0"},
			experimental: `{"private-action-runner":{"tag":"dev"}}`,
			wantVersion:  "dev", wantError: true,
		},
		{
			name:        "partial component override inherits default version",
			image:       v2alpha1.AgentImageConfig{PullPolicy: new(corev1.PullAlways)},
			wantVersion: images.AgentLatestVersion,
			wantError:   !pkgutils.IsAboveMinVersion(images.AgentLatestVersion, privateActionRunnerSplitMinVersion, new(false)),
		},
		{
			name:         "partial component override with newer PAR",
			image:        v2alpha1.AgentImageConfig{PullPolicy: new(corev1.PullAlways)},
			experimental: `{"private-action-runner":{"tag":"7.84.0"}}`,
			wantVersion:  "7.84.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dda := &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-dda",
					Annotations: map[string]string{
						"agent.datadoghq.com/private-action-runner-enabled":       "true",
						"agent.datadoghq.com/private-action-runner-split-enabled": "true",
						"experimental.agent.datadoghq.com/image-override-config":  tt.experimental,
					},
				},
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {Image: &tt.image},
					},
				},
			}
			f := buildPrivateActionRunnerFeature(nil)
			f.Configure(dda, &dda.Spec, nil)
			managers := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Name:  string(apicommon.PrivateActionRunnerContainerName),
					Image: images.GetLatestAgentImage(),
				}}},
			})

			err := f.ManageNodeAgent(managers)
			if tt.wantError {
				require.ErrorContains(t, err, "split mode requires Agent >= 7.84.0-0")
				require.ErrorContains(t, err, "got "+tt.wantVersion)
			} else {
				require.NoError(t, err)
				assert.Equal(t, []string{privateActionRunnerEntrypoint}, managers.PodTemplateSpec().Spec.Containers[0].Command)
			}

			container := &managers.PodTemplateSpec().Spec.Containers[0]
			container.Image = images.OverrideAgentImage(container.Image, &tt.image)
			experimental.ApplyExperimentalOverrides(logr.Discard(), dda, managers)
			assert.Equal(t, tt.wantVersion, common.GetAgentVersionFromImage(v2alpha1.AgentImageConfig{Name: container.Image}))
		})
	}
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
