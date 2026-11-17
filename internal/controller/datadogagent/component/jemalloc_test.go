// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package component

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

func TestJemallocEnvVarsAnnotation(t *testing.T) {
	image := &v2alpha1.AgentImageConfig{Tag: "7.87.0"}
	for _, value := range []string{"", "false", "TRUE", "true"} {
		t.Run(value, func(t *testing.T) {
			dda := &metav1.ObjectMeta{Annotations: map[string]string{DisableJemallocAnnotation: value}}
			if value == "true" {
				assert.Equal(t, []corev1.EnvVar{{Name: "LD_PRELOAD", Value: "/opt/lib/nosys.so"}}, JemallocEnvVars(dda, image))
			} else {
				assert.Empty(t, JemallocEnvVars(dda, image))
			}
		})
	}
	assert.Empty(t, JemallocEnvVars(&metav1.ObjectMeta{}, image))
	assert.Empty(t, JemallocEnvVars(&metav1.ObjectMeta{Annotations: map[string]string{"agent.datadoghq.com/jemalloc-enabled": "true"}}, image))
}

func TestJemallocEnvVarsVersionGate(t *testing.T) {
	tests := []struct {
		name        string
		nodeImage   *v2alpha1.AgentImageConfig
		runnerImage *v2alpha1.AgentImageConfig
		wantNode    bool
		wantRunner  bool
	}{
		{name: "default images"},
		{name: "empty image configuration", nodeImage: &v2alpha1.AgentImageConfig{}, runnerImage: &v2alpha1.AgentImageConfig{}, wantNode: true, wantRunner: true},
		{name: "both too old", nodeImage: &v2alpha1.AgentImageConfig{Tag: "7.86.9"}, runnerImage: &v2alpha1.AgentImageConfig{Tag: "7.86.9"}},
		{name: "minimum version", nodeImage: &v2alpha1.AgentImageConfig{Tag: "7.87.0"}, runnerImage: &v2alpha1.AgentImageConfig{Tag: "7.87.0"}, wantNode: true, wantRunner: true},
		{name: "minimum prerelease", nodeImage: &v2alpha1.AgentImageConfig{Tag: "7.87.0-rc.1"}, runnerImage: &v2alpha1.AgentImageConfig{Tag: "7.87.0-rc.1"}, wantNode: true, wantRunner: true},
		{name: "newer version", nodeImage: &v2alpha1.AgentImageConfig{Tag: "7.88.0"}, runnerImage: &v2alpha1.AgentImageConfig{Tag: "7.88.0"}, wantNode: true, wantRunner: true},
		{name: "old node new runner", nodeImage: &v2alpha1.AgentImageConfig{Tag: "7.86.0"}, runnerImage: &v2alpha1.AgentImageConfig{Tag: "7.87.0"}, wantRunner: true},
		{name: "new node old runner", nodeImage: &v2alpha1.AgentImageConfig{Tag: "7.87.0"}, runnerImage: &v2alpha1.AgentImageConfig{Tag: "7.86.0"}, wantNode: true},
		{name: "version from image name", nodeImage: &v2alpha1.AgentImageConfig{Name: "registry.example/agent:7.87.0-jmx"}, runnerImage: &v2alpha1.AgentImageConfig{Name: "registry.example/agent:7.86.0"}, wantNode: true},
		{name: "tagged image name overrides tag", nodeImage: &v2alpha1.AgentImageConfig{Name: "registry.example/agent:7.86.0", Tag: "7.87.0"}, runnerImage: &v2alpha1.AgentImageConfig{Name: "registry.example/agent:7.87.0", Tag: "7.86.0"}, wantRunner: true},
		{name: "unknown versions", nodeImage: &v2alpha1.AgentImageConfig{Tag: "latest"}, runnerImage: &v2alpha1.AgentImageConfig{Name: "registry.example/agent:custom"}, wantNode: true, wantRunner: true},
	}
	dda := &metav1.ObjectMeta{Annotations: map[string]string{DisableJemallocAnnotation: "true"}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, c := range []struct {
				image *v2alpha1.AgentImageConfig
				want  bool
			}{{tt.nodeImage, tt.wantNode}, {tt.runnerImage, tt.wantRunner}} {
				envs := JemallocEnvVars(dda, c.image)
				if c.want {
					assert.Equal(t, []corev1.EnvVar{
						{Name: "LD_PRELOAD", Value: "/opt/lib/nosys.so"},
					}, envs)
				} else {
					assert.Empty(t, envs)
				}
			}
		})
	}
}
