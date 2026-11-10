// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/clusteragent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/clusterchecksrunner"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
)

func healthPlatformEnvValue(t *testing.T, pod *corev1.PodTemplateSpec, container apicommon.AgentContainerName) (string, int) {
	t.Helper()
	for _, c := range pod.Spec.Containers {
		if c.Name != string(container) {
			continue
		}
		value, count := "", 0
		for _, env := range c.Env {
			if env.Name == common.DDHealthPlatformEnabled {
				value, count = env.Value, count+1
			}
		}
		return value, count
	}
	require.Failf(t, "container not found", "%s", container)
	return "", 0
}

// TestHealthPlatformDisabledByDefault checks that the Agent Health Platform is turned off on every
// component that runs Autodiscovery, and that a user's env override turns it back on.
func TestHealthPlatformDisabledByDefault(t *testing.T) {
	dda := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Name: "datadog", Namespace: "default"}}
	targets := []struct {
		component v2alpha1.ComponentName
		container apicommon.AgentContainerName
		newPod    func() *corev1.PodTemplateSpec
	}{
		{v2alpha1.NodeAgentComponentName, apicommon.CoreAgentContainerName, func() *corev1.PodTemplateSpec {
			return agent.NewDefaultAgentPodTemplateSpec(dda, feature.RequiredComponent{
				Containers: []apicommon.AgentContainerName{apicommon.CoreAgentContainerName},
			}, nil)
		}},
		{v2alpha1.NodeAgentComponentName, apicommon.UnprivilegedSingleAgentContainerName, func() *corev1.PodTemplateSpec {
			return agent.NewDefaultAgentPodTemplateSpec(dda, feature.RequiredComponent{
				Containers: []apicommon.AgentContainerName{apicommon.UnprivilegedSingleAgentContainerName},
			}, nil)
		}},
		{v2alpha1.ClusterAgentComponentName, apicommon.ClusterAgentContainerName, func() *corev1.PodTemplateSpec {
			return clusteragent.NewDefaultClusterAgentPodTemplateSpec(dda, &dda.Spec)
		}},
		{v2alpha1.ClusterChecksRunnerComponentName, apicommon.ClusterChecksRunnersContainerName, func() *corev1.PodTemplateSpec {
			return clusterchecksrunner.NewDefaultClusterChecksRunnerPodTemplateSpec(dda, &dda.Spec)
		}},
	}

	for _, tt := range []struct {
		name     string
		override func(container apicommon.AgentContainerName) *v2alpha1.DatadogAgentComponentOverride
		want     string
	}{
		{
			name: "default",
			want: "false",
		},
		{
			name: "component env override wins",
			override: func(apicommon.AgentContainerName) *v2alpha1.DatadogAgentComponentOverride {
				return &v2alpha1.DatadogAgentComponentOverride{
					Env: []corev1.EnvVar{{Name: common.DDHealthPlatformEnabled, Value: "true"}},
				}
			},
			want: "true",
		},
		{
			name: "container env override wins",
			override: func(container apicommon.AgentContainerName) *v2alpha1.DatadogAgentComponentOverride {
				return &v2alpha1.DatadogAgentComponentOverride{
					Containers: map[apicommon.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
						container: {Env: []corev1.EnvVar{{Name: common.DDHealthPlatformEnabled, Value: "true"}}},
					},
				}
			},
			want: "true",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, target := range targets {
				pod := target.newPod()
				if tt.override != nil {
					manager := feature.NewPodTemplateManagers(pod)
					PodTemplateSpec(zap.New(), manager, tt.override(target.container), target.component, dda.Name)
				}
				value, count := healthPlatformEnvValue(t, pod, target.container)
				assert.Equal(t, 1, count, "%s/%s", target.component, target.container)
				assert.Equal(t, tt.want, value, "%s/%s", target.component, target.container)
			}
		})
	}
}
