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
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
)

func TestDefaultPodTemplateJemalloc(t *testing.T) {
	dda := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{
		Name: "datadog", Annotations: map[string]string{component.DisableJemallocAnnotation: "true"},
	}, Spec: v2alpha1.DatadogAgentSpec{
		Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
			v2alpha1.NodeAgentComponentName: {DatadogAgentComponentOverrideConfig: v2alpha1.DatadogAgentComponentOverrideConfig{Image: &v2alpha1.AgentImageConfig{Tag: "7.87.0"}}},
		},
	}}
	for _, tt := range []struct {
		name       string
		containers []apicommon.AgentContainerName
	}{
		{"multiple containers", []apicommon.AgentContainerName{
			apicommon.CoreAgentContainerName, apicommon.TraceAgentContainerName,
			apicommon.SystemProbeContainerName, apicommon.ProcessAgentContainerName,
			apicommon.SecurityAgentContainerName, apicommon.OtelAgent,
			apicommon.PrivateActionRunnerContainerName, apicommon.HostProfiler,
			apicommon.AgentDataPlaneContainerName, apicommon.FlightRecorderContainerName,
			apicommon.AgentCheckRunnerContainerName,
		}},
		{"single container", []apicommon.AgentContainerName{apicommon.UnprivilegedSingleAgentContainerName}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pod := NewDefaultAgentPodTemplateSpec(dda, feature.RequiredComponent{Containers: tt.containers}, nil)
			assert.Len(t, pod.Spec.Containers, len(tt.containers))
			for _, c := range pod.Spec.Containers {
				var envs []corev1.EnvVar
				for _, env := range c.Env {
					if env.Name == "LD_PRELOAD" || env.Name == "MALLOC_CONF" {
						envs = append(envs, env)
					}
				}
				if c.Name == string(apicommon.OtelAgent) {
					assert.Empty(t, envs, c.Name)
				} else {
					assert.Equal(t, []corev1.EnvVar{{Name: "LD_PRELOAD", Value: "/opt/lib/nosys.so"}}, envs, c.Name)
				}
			}
			for _, c := range pod.Spec.InitContainers {
				for _, env := range c.Env {
					assert.NotContains(t, []string{"LD_PRELOAD", "MALLOC_CONF"}, env.Name, c.Name)
				}
			}
		})
	}
}
