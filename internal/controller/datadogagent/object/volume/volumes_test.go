// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package volume

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
)

func TestGetVolumes(t *testing.T) {
	vol, mount := GetVolumes("logs", "/var/log/pods", "/host/var/log/pods", true)

	require.Equal(t, "logs", vol.Name)
	require.NotNil(t, vol.HostPath)
	require.Equal(t, "/var/log/pods", vol.HostPath.Path)
	require.Equal(t, corev1.VolumeMount{
		Name:      "logs",
		MountPath: "/host/var/log/pods",
		ReadOnly:  true,
	}, mount)
}

func TestGetVolumesEmptyDir(t *testing.T) {
	vol, mount := GetVolumesEmptyDir("tmp", "/tmp", false)

	require.Equal(t, "tmp", vol.Name)
	require.NotNil(t, vol.EmptyDir)
	require.Equal(t, "tmp", mount.Name)
	require.Equal(t, "/tmp", mount.MountPath)
	require.False(t, mount.ReadOnly)
}

func TestGetVolumesFromConfigMap(t *testing.T) {
	configMap := &v2alpha1.ConfigMapConfig{
		Name: "custom-checks",
		Items: []corev1.KeyToPath{
			{Key: "redis.yaml", Path: "redis.yaml"},
		},
	}

	vol, mount := GetVolumesFromConfigMap(configMap, "checks", "default-checks", "redisdb")

	require.Equal(t, "checks", vol.Name)
	require.Equal(t, "custom-checks", vol.ConfigMap.Name)
	require.Equal(t, []corev1.KeyToPath{{Key: "redis.yaml", Path: "redis.yaml"}}, vol.ConfigMap.Items)
	require.Equal(t, "checks", mount.Name)
	require.Equal(t, common.ConfigVolumePath+common.ConfdVolumePath+"/redisdb", mount.MountPath)
	require.True(t, mount.ReadOnly)
}

func TestGetVolumeFromCustomConfig(t *testing.T) {
	configData := "logs_enabled: true"

	t.Run("uses referenced configmap when present", func(t *testing.T) {
		vol := GetVolumeFromCustomConfig(v2alpha1.CustomConfig{
			ConfigMap: &v2alpha1.ConfigMapConfig{Name: "custom"},
		}, "default", "config")

		require.Equal(t, "custom", vol.ConfigMap.Name)
	})

	t.Run("uses generated configmap when config data is present", func(t *testing.T) {
		vol := GetVolumeFromCustomConfig(v2alpha1.CustomConfig{
			ConfigData: &configData,
		}, "generated", "config")

		require.Equal(t, "generated", vol.ConfigMap.Name)
	})

	t.Run("returns an empty volume when no custom config is set", func(t *testing.T) {
		require.Empty(t, GetVolumeFromCustomConfig(v2alpha1.CustomConfig{}, "generated", "config"))
	})
}

func TestGetVolumeFromMultiCustomConfig(t *testing.T) {
	t.Run("uses referenced configmap items", func(t *testing.T) {
		vol := GetVolumeFromMultiCustomConfig(&v2alpha1.MultiCustomConfig{
			ConfigMap: &v2alpha1.ConfigMapConfig{
				Name:  "custom-confd",
				Items: []corev1.KeyToPath{{Key: "check.yaml", Path: "check.yaml"}},
			},
		}, "confd", "generated-confd")

		require.Equal(t, "custom-confd", vol.ConfigMap.Name)
		require.Equal(t, []corev1.KeyToPath{{Key: "check.yaml", Path: "check.yaml"}}, vol.ConfigMap.Items)
	})

	t.Run("sorts valid config data keys and skips invalid yaml", func(t *testing.T) {
		vol := GetVolumeFromMultiCustomConfig(&v2alpha1.MultiCustomConfig{
			ConfigDataMap: map[string]string{
				"z.yaml": "init_config: {}\n",
				"bad":    ":\n",
				"a.yaml": "instances: []\n",
			},
		}, "confd", "generated-confd")

		require.Equal(t, "generated-confd", vol.ConfigMap.Name)
		require.Equal(t, []corev1.KeyToPath{
			{Key: "a.yaml", Path: "a.yaml"},
			{Key: "z.yaml", Path: "z.yaml"},
		}, vol.ConfigMap.Items)
	})
}

func TestGetVolumeMountWithSubPath(t *testing.T) {
	require.Equal(t, corev1.VolumeMount{
		Name:      "config",
		MountPath: "/etc/datadog-agent/datadog.yaml",
		SubPath:   "datadog.yaml",
		ReadOnly:  true,
	}, GetVolumeMountWithSubPath("config", "/etc/datadog-agent/datadog.yaml", "datadog.yaml"))
}
