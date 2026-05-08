// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package global

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/providercaps"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// TestNodeAgentProviderSpec_EKS asserts the EKS-EC2 provider spec injects:
//   - DD_HOSTNAME_FILE env var on every main container and on init-config
//   - cloudinit-instance-id-file HostPath volume + read-only mount on every
//     enumerated agent container
func TestNodeAgentProviderSpec_EKS(t *testing.T) {
	tmpl := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: string(apicommon.CoreAgentContainerName)},
				{Name: string(apicommon.TraceAgentContainerName)},
			},
			InitContainers: []corev1.Container{
				{Name: string(apicommon.InitVolumeContainerName)},
				{Name: string(apicommon.InitConfigContainerName)},
			},
		},
	}
	mgr := feature.NewPodTemplateManagers(tmpl)

	providercaps.ApplyNodeAgentProviderCapabilities(mgr, kubernetes.EKSEC2UseHostnameFromFileProvider, NodeAgentProviderSpec)

	wantEnv := corev1.EnvVar{Name: "DD_HOSTNAME_FILE", Value: "/var/lib/cloud/data/instance-id"}
	for _, c := range tmpl.Spec.Containers {
		assert.Contains(t, c.Env, wantEnv, "main container %q should get DD_HOSTNAME_FILE", c.Name)
	}
	// init-config receives the env var; init-volume does not (matches helm).
	var initConfig, initVolume corev1.Container
	for _, c := range tmpl.Spec.InitContainers {
		switch c.Name {
		case string(apicommon.InitConfigContainerName):
			initConfig = c
		case string(apicommon.InitVolumeContainerName):
			initVolume = c
		}
	}
	assert.Contains(t, initConfig.Env, wantEnv, "init-config should get DD_HOSTNAME_FILE")
	assert.NotContains(t, initVolume.Env, wantEnv, "init-volume should NOT get DD_HOSTNAME_FILE")

	// HostPath volume is added at pod level.
	var vol corev1.Volume
	for _, v := range tmpl.Spec.Volumes {
		if v.Name == "cloudinit-instance-id-file" {
			vol = v
		}
	}
	assert.NotNil(t, vol.HostPath, "cloudinit-instance-id-file volume should be a HostPath")
	assert.Equal(t, "/var/lib/cloud/data/instance-id", vol.HostPath.Path)
	assert.Equal(t, ptr.To(corev1.HostPathFile), vol.HostPath.Type)

	// Read-only mount is added on each main container.
	wantMount := corev1.VolumeMount{
		Name:      "cloudinit-instance-id-file",
		MountPath: "/var/lib/cloud/data/instance-id",
		ReadOnly:  true,
	}
	for _, c := range tmpl.Spec.Containers {
		assert.Contains(t, c.VolumeMounts, wantMount, "main container %q should mount cloudinit-instance-id-file", c.Name)
	}
}

// TestNodeAgentProviderSpec_NoProvider asserts no provider == no mutations.
func TestNodeAgentProviderSpec_NoProvider(t *testing.T) {
	tmpl := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: string(apicommon.CoreAgentContainerName)}},
		},
	}
	mgr := feature.NewPodTemplateManagers(tmpl)

	providercaps.ApplyNodeAgentProviderCapabilities(mgr, "", NodeAgentProviderSpec)

	assert.Empty(t, tmpl.Spec.Containers[0].Env)
	assert.Empty(t, tmpl.Spec.Containers[0].VolumeMounts)
	assert.Empty(t, tmpl.Spec.Volumes)
}
