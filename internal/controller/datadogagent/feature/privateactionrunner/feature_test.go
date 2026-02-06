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
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	featureutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
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
				featureutils.EnablePrivateActionRunnerAnnotation: "true",
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
				featureutils.EnablePrivateActionRunnerAnnotation: "false",
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
				featureutils.EnablePrivateActionRunnerAnnotation: "true",
				featureutils.PrivateActionRunnerConfigDataAnnotation: `privateactionrunner:
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
	assert.Equal(t, "privateactionrunner-config", vol.Name, "Volume name should match")
	assert.NotNil(t, vol.VolumeSource.ConfigMap, "Volume should be a ConfigMap volume")
	assert.Equal(t, "test-dda-privateactionrunner", vol.VolumeSource.ConfigMap.Name, "ConfigMap name should match")

	// Verify volume mount
	volumeMounts := managers.VolumeMountMgr.VolumeMountsByC[apicommon.PrivateActionRunnerContainerName]
	assert.Len(t, volumeMounts, 1, "Should have exactly one volume mount")
	mount := volumeMounts[0]
	assert.Equal(t, "privateactionrunner-config", mount.Name, "Mount name should match")
	assert.Equal(t, "/etc/datadog-agent/privateactionrunner.yaml", mount.MountPath, "Mount path should be the hardcoded path")
	assert.Equal(t, "privateactionrunner.yaml", mount.SubPath, "SubPath should mount the file directly")
	assert.True(t, mount.ReadOnly, "Mount should be read-only")

	// Verify hash
	assert.NotEmpty(t, managers.AnnotationMgr.Annotations)
	assert.NotEmpty(t, managers.AnnotationMgr.Annotations["checksum/private_action_runner-custom-config"])
	assert.Equal(t, managers.AnnotationMgr.Annotations["checksum/private_action_runner-custom-config"], "749c842cefd79ebc309b2b329b28e3fe")
}

func Test_privateActionRunnerFeature_ID(t *testing.T) {
	f := buildPrivateActionRunnerFeature(nil)
	assert.Equal(t, string(feature.PrivateActionRunnerIDType), string(f.ID()))
}

func Test_privateActionRunnerFeature_ManageSingleContainerNodeAgent(t *testing.T) {
	f := buildPrivateActionRunnerFeature(nil)
	managers := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})

	err := f.ManageSingleContainerNodeAgent(managers, "")
	assert.NoError(t, err)
}

func Test_privateActionRunnerFeature_ManageClusterChecksRunner(t *testing.T) {
	f := buildPrivateActionRunnerFeature(nil)
	managers := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})

	err := f.ManageClusterChecksRunner(managers, "")
	assert.NoError(t, err)
}

func Test_privateActionRunnerFeature_ManageOtelAgentGateway(t *testing.T) {
	f := buildPrivateActionRunnerFeature(nil)
	managers := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})

	err := f.ManageOtelAgentGateway(managers, "")
	assert.NoError(t, err)
}

func Test_privateActionRunnerFeature_ManageDependencies(t *testing.T) {
	f := buildPrivateActionRunnerFeature(nil)
	err := f.ManageDependencies(nil, "")
	assert.NoError(t, err)
}

func Test_privateActionRunnerFeature_ManageClusterAgent(t *testing.T) {
	f := buildPrivateActionRunnerFeature(nil)
	managers := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})

	err := f.ManageClusterAgent(managers, "")
	assert.NoError(t, err)
}

func Test_buildPrivateActionRunnerFeature_WithLogger(t *testing.T) {
	f := buildPrivateActionRunnerFeature(&feature.Options{
		Logger: logr.Discard(),
	})

	parFeat, ok := f.(*privateActionRunnerFeature)
	assert.True(t, ok)
	assert.NotNil(t, parFeat)
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
				featureutils.EnablePrivateActionRunnerAnnotation: "false",
			},
			expectConfigMap: false,
		},
		{
			name: "enabled without configdata - uses default",
			annotations: map[string]string{
				featureutils.EnablePrivateActionRunnerAnnotation: "true",
			},
			expectConfigMap: true,
			expectedYAML:    defaultConfigData,
			expectedHash:    "b7fc921bd4d0b4a60ef4fd8ea98e65a1",
		},
		{
			name: "enabled with configdata - passes through directly",
			annotations: map[string]string{
				featureutils.EnablePrivateActionRunnerAnnotation: "true",
				featureutils.PrivateActionRunnerConfigDataAnnotation: `privateactionrunner:
    private_key: some-key
    urn: urn:dd:apps:on-prem-runner:us1:1:runner-abc
    self_enroll: false
    actions_allowlist:
        - com.datadoghq.script.testConnection
        - com.datadoghq.script.enrichScript`,
			},
			expectConfigMap: true,
			expectedYAML: `privateactionrunner:
    private_key: some-key
    urn: urn:dd:apps:on-prem-runner:us1:1:runner-abc
    self_enroll: false
    actions_allowlist:
        - com.datadoghq.script.testConnection
        - com.datadoghq.script.enrichScript`,
			expectedHash: "5d4b4b221b5bcc3b92792558d6f6bc58",
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
			assert.Equal(t, configMap.Annotations["checksum/private_action_runner-custom-config"], tt.expectedHash)
		})
	}
}
