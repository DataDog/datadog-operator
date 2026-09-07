// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
)

func TestEnableConfigSyncForDirectSend(t *testing.T) {
	containers := []apicommon.AgentContainerName{
		apicommon.CoreAgentContainerName,
		apicommon.SystemProbeContainerName,
	}

	t.Run("sets both env vars on every container", func(t *testing.T) {
		managers := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})

		EnableConfigSyncForDirectSend(managers, containers)

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
		userPort := &corev1.EnvVar{Name: common.DDAgentIpcPort, Value: "1234"}
		managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, userPort)

		EnableConfigSyncForDirectSend(managers, containers)

		want := []*corev1.EnvVar{
			userPort,
			{Name: common.DDAgentIpcConfigRefreshInterval, Value: DefaultAgentIpcConfigRefreshInterval},
		}
		assert.Equal(t, want, managers.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName])
	})
}
