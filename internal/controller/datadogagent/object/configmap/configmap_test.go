// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package configmap

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildConfigMapConfigData(t *testing.T) {
	t.Run("returns nil when config data is missing", func(t *testing.T) {
		got, err := BuildConfigMapConfigData("agents", nil, "datadog-config", "datadog.yaml")

		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("returns nil when config data is empty", func(t *testing.T) {
		empty := ""

		got, err := BuildConfigMapConfigData("agents", &empty, "datadog-config", "datadog.yaml")

		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("builds a configmap for valid yaml", func(t *testing.T) {
		configData := "logs_enabled: true\n"

		got, err := BuildConfigMapConfigData("agents", &configData, "datadog-config", "datadog.yaml")

		require.NoError(t, err)
		require.Equal(t, "datadog-config", got.Name)
		require.Equal(t, "agents", got.Namespace)
		require.Equal(t, map[string]string{"datadog.yaml": configData}, got.Data)
	})

	t.Run("returns an error for invalid yaml", func(t *testing.T) {
		configData := ":\n"

		got, err := BuildConfigMapConfigData("agents", &configData, "datadog-config", "datadog.yaml")

		require.Error(t, err)
		require.Nil(t, got)
	})
}

func TestBuildConfigMapMulti(t *testing.T) {
	t.Run("accepts arbitrary data when validation is disabled", func(t *testing.T) {
		got, err := BuildConfigMapMulti("agents", map[string]string{
			"check.py": "def check(): pass",
		}, "checks", false)

		require.NoError(t, err)
		require.Equal(t, "checks", got.Name)
		require.Equal(t, map[string]string{"check.py": "def check(): pass"}, got.Data)
	})

	t.Run("keeps valid yaml and reports invalid yaml when validation is enabled", func(t *testing.T) {
		got, err := BuildConfigMapMulti("agents", map[string]string{
			"valid.yaml": "instances: []\n",
			"bad.yaml":   ":\n",
		}, "checks", true)

		require.Error(t, err)
		require.Equal(t, map[string]string{"valid.yaml": "instances: []\n"}, got.Data)
	})
}
