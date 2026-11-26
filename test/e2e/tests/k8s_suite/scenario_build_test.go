// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package k8ssuite

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/DataDog/datadog-operator/test/e2e/common"
)

// TestBuildDatadogAgent checks that the DatadogAgent applied by applyScenario
// gets what the e2e-framework's DatadogAgent component used to add. It needs no
// infrastructure.
func TestBuildDatadogAgent(t *testing.T) {
	const fakeIntakeURL = "http://fakeintake.example:80"

	t.Run("manifest without name or override", func(t *testing.T) {
		dda, err := buildDatadogAgent(scenario{ddaName: "dda-minimum", ddaPath: common.DdaMinimalPath}, "my-cluster", fakeIntakeURL, true)
		require.NoError(t, err)

		assert.Equal(t, "dda-minimum", dda.GetName())
		assert.Equal(t, common.NamespaceName, dda.GetNamespace())
		assert.Equal(t, "datadog-agent-minimum", dda.GetLabels()["agent.datadoghq.com/e2e-test"])

		assertNested(t, dda, "public.ecr.aws/datadog", "spec", "global", "registry")
		assertNested(t, dda, false, "spec", "global", "kubelet", "tlsVerify")
		assertNested(t, dda, "my-cluster", "spec", "global", "clusterName")
		assertNested(t, dda, ddaCredentialsSecretName, "spec", "global", "credentials", "apiSecret", "secretName")
		assertNested(t, dda, "app-key", "spec", "global", "credentials", "appSecret", "keyName")

		for _, component := range []string{"nodeAgent", "clusterAgent", "clusterChecksRunner"} {
			assertEnv(t, dda, component, "DD_DD_URL", fakeIntakeURL)
			assertEnv(t, dda, component, "DD_LOGS_CONFIG_USE_HTTP", "true")
			secrets, _, err := unstructured.NestedSlice(dda.Object, "spec", "override", component, "image", "pullSecrets")
			require.NoError(t, err)
			assert.Equal(t, []any{map[string]any{"name": imagePullSecretName}}, secrets, component)
		}
	})

	t.Run("manifest with a nodeAgent override", func(t *testing.T) {
		path := filepath.Join(common.ManifestsPath, "datadog-agent-process-passwd.yaml")
		dda, err := buildDatadogAgent(scenario{ddaName: "datadog-agent-process-passwd", ddaPath: path}, "my-cluster", fakeIntakeURL, false)
		require.NoError(t, err)

		assert.Equal(t, "datadog-agent-process-passwd", dda.GetName())
		// The manifest's override is kept, and the fakeintake settings are merged into it.
		manifest, err := decodeManifest(path)
		require.NoError(t, err)
		tag, found, err := unstructured.NestedString(manifest[0].Object, "spec", "override", "nodeAgent", "image", "tag")
		require.NoError(t, err)
		require.True(t, found, "the manifest sets a nodeAgent image tag")
		assertNested(t, dda, tag, "spec", "override", "nodeAgent", "image", "tag")
		assertEnv(t, dda, "nodeAgent", "DD_DD_URL", fakeIntakeURL)
		assertEnv(t, dda, "clusterAgent", "DD_DD_URL", fakeIntakeURL)
		_, found, err = unstructured.NestedSlice(dda.Object, "spec", "override", "nodeAgent", "image", "pullSecrets")
		require.NoError(t, err)
		assert.False(t, found, "no pull secret without an image pull registry")
	})

	t.Run("every TestGenericK8s manifest builds", func(t *testing.T) {
		for _, path := range []string{
			common.DdaMinimalPath,
			filepath.Join(common.ManifestsPath, "datadog-agent-process-passwd.yaml"),
			filepath.Join(common.ManifestsPath, "datadog-agent-ccr-enabled.yaml"),
			filepath.Join(common.ManifestsPath, "datadog-agent-logs.yaml"),
			filepath.Join(common.ManifestsPath, "apm", "datadog-agent-apm.yaml"),
			filepath.Join(common.ManifestsPath, "dogstatsd", "datadog-agent-dsd-udp.yaml"),
			filepath.Join(common.ManifestsPath, "dogstatsd", "datadog-agent-dsd-udp-adp.yaml"),
			filepath.Join(common.ManifestsPath, "dogstatsd", "datadog-agent-dsd-uds.yaml"),
			filepath.Join(common.ManifestsPath, "dogstatsd", "datadog-agent-dsd-uds-adp.yaml"),
		} {
			dda, err := buildDatadogAgent(scenario{ddaName: "name", ddaPath: path}, "my-cluster", fakeIntakeURL, true)
			require.NoError(t, err, path)
			assert.Equal(t, "DatadogAgent", dda.GetKind(), path)
			for _, component := range []string{"nodeAgent", "clusterAgent", "clusterChecksRunner"} {
				assertEnv(t, dda, component, "DD_DD_URL", fakeIntakeURL)
			}
		}
	})
}

func assertNested(t *testing.T, obj *unstructured.Unstructured, expected any, fields ...string) {
	t.Helper()
	value, found, err := unstructured.NestedFieldNoCopy(obj.Object, fields...)
	require.NoError(t, err)
	require.True(t, found, "%v not found", fields)
	assert.Equal(t, expected, value, "%v", fields)
}

func assertEnv(t *testing.T, obj *unstructured.Unstructured, component, name, value string) {
	t.Helper()
	env, _, err := unstructured.NestedSlice(obj.Object, "spec", "override", component, "env")
	require.NoError(t, err)
	for _, e := range env {
		if e.(map[string]any)["name"] == name {
			assert.Equal(t, value, e.(map[string]any)["value"], "%s %s", component, name)
			return
		}
	}
	t.Errorf("%s: env var %s not found in %v", component, name, env)
}
