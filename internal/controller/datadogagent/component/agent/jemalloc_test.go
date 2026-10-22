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
		Name: "datadog", Annotations: map[string]string{component.EnableJemallocAnnotation: "true"},
	}}
	for _, tt := range []struct {
		name       string
		containers []apicommon.AgentContainerName
	}{
		{"multiple containers", []apicommon.AgentContainerName{
			apicommon.CoreAgentContainerName, apicommon.TraceAgentContainerName,
			apicommon.SystemProbeContainerName, apicommon.ProcessAgentContainerName,
			apicommon.SecurityAgentContainerName, apicommon.OtelAgent,
		}},
		{"single container", []apicommon.AgentContainerName{apicommon.UnprivilegedSingleAgentContainerName}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pod := NewDefaultAgentPodTemplateSpec(dda, feature.RequiredComponent{Containers: tt.containers}, nil)
			for _, c := range append(pod.Spec.Containers, pod.Spec.InitContainers...) {
				var envs []corev1.EnvVar
				for _, env := range c.Env {
					if env.Name == "LD_PRELOAD" || env.Name == "MALLOC_CONF" {
						envs = append(envs, env)
					}
				}
				switch apicommon.AgentContainerName(c.Name) {
				case apicommon.CoreAgentContainerName, apicommon.TraceAgentContainerName, apicommon.SystemProbeContainerName:
					assert.Equal(t, component.JemallocEnvVars(dda, nil), envs, c.Name)
				default:
					assert.Empty(t, envs, c.Name)
				}
			}
		})
	}
}
