// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package render

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

func newMinimalDDA() *v2alpha1.DatadogAgent {
	return &v2alpha1.DatadogAgent{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "datadoghq.com/v2alpha1",
			Kind:       "DatadogAgent",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "datadog",
		},
		Spec: v2alpha1.DatadogAgentSpec{
			Global: &v2alpha1.GlobalConfig{
				Credentials: &v2alpha1.DatadogCredentials{
					APIKey: ptrString("fake-api-key"),
				},
			},
		},
	}
}

func ptrString(s string) *string { return &s }
func ptrBool(b bool) *bool       { return &b }

// v2Opts returns RenderOptions with DDAI disabled (v2 path).
func v2Opts() RenderOptions {
	return RenderOptions{DatadogAgentInternalEnabled: false}
}

// v3Opts returns RenderOptions with DDAI enabled (v3 path).
func v3Opts() RenderOptions {
	return RenderOptions{DatadogAgentInternalEnabled: true}
}

func TestRenderManifests_MinimalDDA(t *testing.T) {
	dda := newMinimalDDA()

	objects, err := RenderManifests(dda, nil, v2Opts())
	require.NoError(t, err)
	require.NotEmpty(t, objects, "expected at least one rendered resource")

	var hasDaemonSet, hasDeployment, hasServiceAccount, hasClusterRole bool
	for _, obj := range objects {
		switch obj.(type) {
		case *appsv1.DaemonSet:
			hasDaemonSet = true
		case *appsv1.Deployment:
			hasDeployment = true
		case *corev1.ServiceAccount:
			hasServiceAccount = true
		case *rbacv1.ClusterRole:
			hasClusterRole = true
		}
	}
	assert.True(t, hasDaemonSet, "expected a DaemonSet (node agent)")
	assert.True(t, hasDeployment, "expected a Deployment (cluster agent)")
	assert.True(t, hasServiceAccount, "expected a ServiceAccount")
	assert.True(t, hasClusterRole, "expected a ClusterRole")
}

func TestRenderManifests_Namespace(t *testing.T) {
	dda := newMinimalDDA()
	dda.Namespace = "custom-ns"

	objects, err := RenderManifests(dda, nil, v2Opts())
	require.NoError(t, err)

	for _, obj := range objects {
		if obj.GetNamespace() != "" {
			assert.Equal(t, "custom-ns", obj.GetNamespace(),
				"resource %s/%s has wrong namespace", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetName())
		}
	}
}

func TestRenderManifests_CleanedOutput(t *testing.T) {
	dda := newMinimalDDA()

	objects, err := RenderManifests(dda, nil, v2Opts())
	require.NoError(t, err)

	for _, obj := range objects {
		assert.Empty(t, obj.GetResourceVersion(), "resourceVersion should be empty")
		assert.Empty(t, string(obj.GetUID()), "UID should be empty")
		assert.Nil(t, obj.GetOwnerReferences(), "ownerReferences should be nil")
		assert.Nil(t, obj.GetManagedFields(), "managedFields should be nil")
	}
}

func TestRenderManifests_APMEnabled(t *testing.T) {
	dda := newMinimalDDA()
	dda.Spec.Features = &v2alpha1.DatadogFeatures{
		APM: &v2alpha1.APMFeatureConfig{
			Enabled: ptrBool(true),
		},
	}

	objects, err := RenderManifests(dda, nil, v2Opts())
	require.NoError(t, err)

	var ds *appsv1.DaemonSet
	for _, obj := range objects {
		if d, ok := obj.(*appsv1.DaemonSet); ok {
			ds = d
			break
		}
	}
	require.NotNil(t, ds, "expected a DaemonSet")

	found := false
	for _, container := range ds.Spec.Template.Spec.Containers {
		for _, env := range container.Env {
			if env.Name == "DD_APM_ENABLED" && env.Value == "true" {
				found = true
			}
		}
	}
	assert.True(t, found, "expected DD_APM_ENABLED=true env var in DaemonSet when APM is enabled")
}

func TestRenderManifests_DDAIEnabled(t *testing.T) {
	dda := newMinimalDDA()

	objects, err := RenderManifests(dda, nil, v3Opts())
	require.NoError(t, err)
	require.NotEmpty(t, objects, "expected at least one rendered resource via DDAI path")

	var hasDaemonSet, hasDeployment, hasServiceAccount, hasClusterRole, hasDDAI bool
	for _, obj := range objects {
		switch obj.(type) {
		case *appsv1.DaemonSet:
			hasDaemonSet = true
		case *appsv1.Deployment:
			hasDeployment = true
		case *corev1.ServiceAccount:
			hasServiceAccount = true
		case *rbacv1.ClusterRole:
			hasClusterRole = true
		case *v1alpha1.DatadogAgentInternal:
			hasDDAI = true
		}
	}
	assert.True(t, hasDDAI, "expected a DatadogAgentInternal resource via DDAI path")
	assert.True(t, hasDaemonSet, "expected a DaemonSet (node agent) via DDAI path")
	assert.True(t, hasDeployment, "expected a Deployment (cluster agent) via DDAI path")
	assert.True(t, hasServiceAccount, "expected a ServiceAccount via DDAI path")
	assert.True(t, hasClusterRole, "expected a ClusterRole via DDAI path")
}

func TestRenderManifests_DDAIDisabled(t *testing.T) {
	dda := newMinimalDDA()

	objects, err := RenderManifests(dda, nil, v2Opts())
	require.NoError(t, err)
	require.NotEmpty(t, objects, "expected at least one rendered resource via v2 path")

	var hasDaemonSet, hasDeployment, hasServiceAccount, hasClusterRole bool
	for _, obj := range objects {
		switch obj.(type) {
		case *appsv1.DaemonSet:
			hasDaemonSet = true
		case *appsv1.Deployment:
			hasDeployment = true
		case *corev1.ServiceAccount:
			hasServiceAccount = true
		case *rbacv1.ClusterRole:
			hasClusterRole = true
		}
	}
	assert.True(t, hasDaemonSet, "expected a DaemonSet (node agent) via v2 path")
	assert.True(t, hasDeployment, "expected a Deployment (cluster agent) via v2 path")
	assert.True(t, hasServiceAccount, "expected a ServiceAccount via v2 path")
	assert.True(t, hasClusterRole, "expected a ClusterRole via v2 path")
}

func TestSerializeObjects_YAML(t *testing.T) {
	objects := []client.Object{
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cm",
				Namespace: "default",
			},
			Data: map[string]string{"key": "value"},
		},
	}

	s := newScheme()
	var buf bytes.Buffer
	err := serializeObjects(objects, "yaml", s, &buf)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "kind: ConfigMap")
	assert.Contains(t, output, "apiVersion: v1")
	assert.Contains(t, output, "name: test-cm")
}

func TestSerializeObjects_JSON(t *testing.T) {
	objects := []client.Object{
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cm",
				Namespace: "default",
			},
		},
	}

	s := newScheme()
	var buf bytes.Buffer
	err := serializeObjects(objects, "json", s, &buf)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, `"kind": "ConfigMap"`)
	assert.Contains(t, output, `"apiVersion": "v1"`)
}

func TestSerializeObjects_MultiDocument(t *testing.T) {
	objects := []client.Object{
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm1", Namespace: "default"}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm2", Namespace: "default"}},
	}

	s := newScheme()
	var buf bytes.Buffer
	err := serializeObjects(objects, "yaml", s, &buf)
	require.NoError(t, err)

	docs := strings.Split(buf.String(), "---\n")
	assert.Len(t, docs, 2, "expected two YAML documents separated by ---")
}

func TestLoadDatadogAgent(t *testing.T) {
	t.Run("missing name", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "dda.yaml")
		err := os.WriteFile(tmpFile, []byte(`
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
spec:
  global: {}
`), 0644)
		require.NoError(t, err)

		_, err = loadDatadogAgent(tmpFile, "")
		assert.ErrorContains(t, err, "metadata.name")
	})

	t.Run("defaults", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "dda.yaml")
		err := os.WriteFile(tmpFile, []byte(`
metadata:
  name: test
spec:
  global: {}
`), 0644)
		require.NoError(t, err)

		dda, err := loadDatadogAgent(tmpFile, "")
		require.NoError(t, err)
		assert.Equal(t, "datadoghq.com/v2alpha1", dda.APIVersion)
		assert.Equal(t, "DatadogAgent", dda.Kind)
		assert.Equal(t, "datadog", dda.Namespace)
	})

	t.Run("namespace override", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "dda.yaml")
		err := os.WriteFile(tmpFile, []byte(`
metadata:
  name: test
  namespace: original
spec:
  global: {}
`), 0644)
		require.NoError(t, err)

		dda, err := loadDatadogAgent(tmpFile, "override-ns")
		require.NoError(t, err)
		assert.Equal(t, "override-ns", dda.Namespace)
	})
}
