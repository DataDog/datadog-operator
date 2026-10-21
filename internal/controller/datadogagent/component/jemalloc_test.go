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
	for _, value := range []string{"", "false", "TRUE", "true"} {
		t.Run(value, func(t *testing.T) {
			dda := &metav1.ObjectMeta{Annotations: map[string]string{EnableJemallocAnnotation: value}}
			assert.Equal(t, value == "true", len(JemallocEnvVars(dda, nil)) == 2)
		})
	}
	assert.Empty(t, JemallocEnvVars(&metav1.ObjectMeta{}, nil))
}

func TestJemallocEnvVarsVersionGate(t *testing.T) {
	tests := []struct {
		name        string
		nodeImage   *v2alpha1.AgentImageConfig
		runnerImage *v2alpha1.AgentImageConfig
		wantNode    bool
		wantRunner  bool
	}{
		{name: "default images", wantNode: true, wantRunner: true},
		{name: "empty image configuration", nodeImage: &v2alpha1.AgentImageConfig{}, runnerImage: &v2alpha1.AgentImageConfig{}, wantNode: true, wantRunner: true},
		{name: "both too old", nodeImage: &v2alpha1.AgentImageConfig{Tag: "7.83.9"}, runnerImage: &v2alpha1.AgentImageConfig{Tag: "7.83.9"}},
		{name: "minimum version", nodeImage: &v2alpha1.AgentImageConfig{Tag: "7.84.0"}, runnerImage: &v2alpha1.AgentImageConfig{Tag: "7.84.0"}, wantNode: true, wantRunner: true},
		{name: "minimum prerelease", nodeImage: &v2alpha1.AgentImageConfig{Tag: "7.84.0-rc.1"}, runnerImage: &v2alpha1.AgentImageConfig{Tag: "7.84.0-rc.1"}, wantNode: true, wantRunner: true},
		{name: "newer version", nodeImage: &v2alpha1.AgentImageConfig{Tag: "7.85.0"}, runnerImage: &v2alpha1.AgentImageConfig{Tag: "7.85.0"}, wantNode: true, wantRunner: true},
		{name: "old node new runner", nodeImage: &v2alpha1.AgentImageConfig{Tag: "7.83.0"}, runnerImage: &v2alpha1.AgentImageConfig{Tag: "7.84.0"}, wantRunner: true},
		{name: "new node old runner", nodeImage: &v2alpha1.AgentImageConfig{Tag: "7.84.0"}, runnerImage: &v2alpha1.AgentImageConfig{Tag: "7.83.0"}, wantNode: true},
		{name: "version from image name", nodeImage: &v2alpha1.AgentImageConfig{Name: "registry.example/agent:7.84.0-jmx"}, runnerImage: &v2alpha1.AgentImageConfig{Name: "registry.example/agent:7.83.0"}, wantNode: true},
		{name: "tag overrides image name", nodeImage: &v2alpha1.AgentImageConfig{Name: "registry.example/agent:7.83.0", Tag: "7.84.0"}, runnerImage: &v2alpha1.AgentImageConfig{Name: "registry.example/agent:7.84.0", Tag: "7.83.0"}, wantNode: true},
		{name: "unknown versions", nodeImage: &v2alpha1.AgentImageConfig{Tag: "latest"}, runnerImage: &v2alpha1.AgentImageConfig{Name: "registry.example/agent:custom"}, wantNode: true, wantRunner: true},
	}
	dda := &metav1.ObjectMeta{Annotations: map[string]string{EnableJemallocAnnotation: "true"}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, c := range []struct {
				image *v2alpha1.AgentImageConfig
				want  bool
			}{{tt.nodeImage, tt.wantNode}, {tt.runnerImage, tt.wantRunner}} {
				envs := JemallocEnvVars(dda, c.image)
				if c.want {
					assert.Equal(t, []corev1.EnvVar{
						{Name: "LD_PRELOAD", Value: "/opt/lib/nosys.so /opt/datadog-agent/embedded/lib/libjemalloc.so"},
						{Name: "MALLOC_CONF", Value: "background_thread:true,dirty_decay_ms:5000"},
					}, envs)
				} else {
					assert.Empty(t, envs)
				}
			}
		})
	}
}
