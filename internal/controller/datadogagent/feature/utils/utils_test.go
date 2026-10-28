// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
)

func newDatadogAgentWithNodeAgentVersion(tag string) *v2alpha1.DatadogAgent {
	return &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{Name: "dda"},
		Spec: v2alpha1.DatadogAgentSpec{
			Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
				v2alpha1.NodeAgentComponentName: {
					Image: &v2alpha1.AgentImageConfig{Tag: tag},
				},
			},
		},
	}
}

func TestEnableConfigSyncForDirectSend(t *testing.T) {
	containers := []apicommon.AgentContainerName{
		apicommon.CoreAgentContainerName,
		apicommon.SystemProbeContainerName,
	}

	t.Run("sets both env vars on every container", func(t *testing.T) {
		managers := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})

		EnableConfigSyncForDirectSend(managers, &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Name: "dda"}}, containers)

		// agents that support the unix socket (the default version does) use it
		want := []*corev1.EnvVar{
			{Name: common.DDAgentIpcUseSocket, Value: "true"},
			{Name: common.DDAgentIpcConfigRefreshInterval, Value: DefaultAgentIpcConfigRefreshInterval},
		}
		for _, container := range containers {
			assert.Equal(t, want, managers.EnvVarMgr.EnvVarsByC[container], "container %s", container)
		}
	})

	t.Run("falls back to the port on agents without unix socket support", func(t *testing.T) {
		managers := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})

		EnableConfigSyncForDirectSend(managers, newDatadogAgentWithNodeAgentVersion("7.74.0"), containers)

		want := []*corev1.EnvVar{
			{Name: common.DDAgentIpcPort, Value: DefaultAgentIpcPort},
			{Name: common.DDAgentIpcConfigRefreshInterval, Value: DefaultAgentIpcConfigRefreshInterval},
		}
		for _, container := range containers {
			assert.Equal(t, want, managers.EnvVarMgr.EnvVarsByC[container], "container %s", container)
		}
	})

	t.Run("keeps a user-provided value", func(t *testing.T) {
		managers := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})
		userSocket := &corev1.EnvVar{Name: common.DDAgentIpcUseSocket, Value: "false"}
		managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, userSocket)

		EnableConfigSyncForDirectSend(managers, &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Name: "dda"}}, containers)

		want := []*corev1.EnvVar{
			userSocket,
			{Name: common.DDAgentIpcConfigRefreshInterval, Value: DefaultAgentIpcConfigRefreshInterval},
		}
		assert.Equal(t, want, managers.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName])
	})
}
