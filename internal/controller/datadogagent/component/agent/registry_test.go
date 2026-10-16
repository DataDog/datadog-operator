// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
)

func volumeNames(volumes []corev1.Volume) []string {
	names := make([]string, 0, len(volumes))
	for _, v := range volumes {
		names = append(names, v.Name)
	}
	return names
}

func envNames(envs []corev1.EnvVar) []string {
	names := make([]string, 0, len(envs))
	for _, e := range envs {
		names = append(names, e.Name)
	}
	return names
}

func newTestPodTemplate(containers ...apicommon.AgentContainerName) *corev1.PodTemplateSpec {
	dda := &metav1.ObjectMeta{Name: "foo", Namespace: "default"}
	return NewDefaultAgentPodTemplateSpec(dda, feature.RequiredComponent{IsRequired: new(true), Containers: containers}, nil)
}

var scaffoldVolumeNames = []string{
	common.LogDatadogVolumeName,
	common.AuthVolumeName,
	common.InstallInfoVolumeName,
	common.ChecksdVolumeName,
	common.ConfdVolumeName,
	common.ConfigVolumeName,
	common.ProcdirVolumeName,
	common.CgroupsVolumeName,
	common.DogstatsdSocketVolumeName,
	common.CriSocketVolumeName,
	common.RunPathVolumeName,
	common.TmpVolumeName,
}

func TestPodTemplateWithCoreAgent(t *testing.T) {
	// The core Agent is always first, whatever the request order.
	pod := newTestPodTemplate(apicommon.TraceAgentContainerName, apicommon.OtelAgent, apicommon.CoreAgentContainerName, apicommon.SystemProbeContainerName)

	assert.Equal(t, []string{"agent", "trace-agent", "otel-agent", "system-probe"}, containerNames(pod.Spec.Containers))
	assert.Equal(t, []string{"init-volume", "init-config", "seccomp-setup"}, containerNames(pod.Spec.InitContainers))
	assert.Equal(t, agentImage(), pod.Spec.InitContainers[0].Image)
	assert.Equal(t, append(append([]string{}, scaffoldVolumeNames...), common.SeccompSecurityVolumeName, common.SeccompRootVolumeName), volumeNames(pod.Spec.Volumes))

	otel := pod.Spec.Containers[2]
	assert.Contains(t, otel.Command, "--core-config="+agentCustomConfigVolumePath)
	assert.Contains(t, envNames(otel.Env), common.DDClusterAgentEnabled)
}

func TestPodTemplateSingleContainer(t *testing.T) {
	pod := newTestPodTemplate(apicommon.UnprivilegedSingleAgentContainerName)

	assert.Equal(t, []string{string(apicommon.UnprivilegedSingleAgentContainerName)}, containerNames(pod.Spec.Containers))
	assert.Equal(t, []string{"init-volume", "init-config"}, containerNames(pod.Spec.InitContainers))
	assert.Equal(t, scaffoldVolumeNames, volumeNames(pod.Spec.Volumes))
}

func TestPodTemplateWithoutCoreAgent(t *testing.T) {
	standaloneVolumeNames := []string{common.LogDatadogVolumeName, common.ConfigVolumeName, common.AuthVolumeName, common.TmpVolumeName, common.RunPathVolumeName}

	tests := []struct {
		name           string
		containers     []apicommon.AgentContainerName
		wantContainers []string
	}{
		{name: "DDOT", containers: []apicommon.AgentContainerName{apicommon.OtelAgent}, wantContainers: []string{"otel-agent"}},
		{name: "host profiler", containers: []apicommon.AgentContainerName{apicommon.HostProfiler}, wantContainers: []string{"host-profiler"}},
		{name: "DDOT and host profiler", containers: []apicommon.AgentContainerName{apicommon.HostProfiler, apicommon.OtelAgent}, wantContainers: []string{"host-profiler", "otel-agent"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := newTestPodTemplate(tt.containers...)

			assert.Equal(t, tt.wantContainers, containerNames(pod.Spec.Containers))
			assert.Equal(t, []string{"init-volume"}, containerNames(pod.Spec.InitContainers))
			assert.Equal(t, ddotCollectorImage(), pod.Spec.InitContainers[0].Image)
			assert.Equal(t, standaloneVolumeNames, volumeNames(pod.Spec.Volumes))

			for _, c := range pod.Spec.Containers {
				for _, arg := range c.Command {
					assert.NotContains(t, arg, "--core-config", "container %s", c.Name)
					assert.NotContains(t, arg, "--sync-delay", "container %s", c.Name)
				}
				env := envNames(c.Env)
				assert.NotContains(t, env, common.DDClusterAgentEnabled, "container %s", c.Name)
				assert.NotContains(t, env, common.DDClusterAgentKubeServiceName, "container %s", c.Name)
				assert.NotContains(t, env, common.DDClusterAgentTokenName, "container %s", c.Name)
				assert.Contains(t, env, common.DDAuthTokenFilePath, "container %s", c.Name)
			}
		})
	}

	t.Run("otel-agent owns the auth volume", func(t *testing.T) {
		pod := newTestPodTemplate(apicommon.OtelAgent)
		for _, m := range pod.Spec.Containers[0].VolumeMounts {
			if m.Name == common.AuthVolumeName {
				assert.False(t, m.ReadOnly)
			}
		}
	})
}

func TestPodTemplateWithoutContainers(t *testing.T) {
	pod := newTestPodTemplate()

	assert.Empty(t, pod.Spec.Containers)
}
