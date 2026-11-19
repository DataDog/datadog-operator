// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/images"
)

func TestGetComponentVersion(t *testing.T) {
	tests := []struct {
		name     string
		override *v2alpha1.DatadogAgentComponentOverride
		want     string
	}{
		{
			name: "null component override",
		},
		{
			name:     "no image override",
			override: &v2alpha1.DatadogAgentComponentOverride{},
		},
		{
			name: "tagged full name takes precedence over tag",
			override: &v2alpha1.DatadogAgentComponentOverride{Image: &v2alpha1.AgentImageConfig{
				Name: "registry/cluster-agent:7.85.0",
				Tag:  "7.84.0",
			}},
			want: "7.85.0",
		},
		{
			name: "tagged short name takes precedence over tag",
			override: &v2alpha1.DatadogAgentComponentOverride{Image: &v2alpha1.AgentImageConfig{
				Name: "agent:7.85.0",
				Tag:  "7.84.0",
			}},
			want: "7.85.0",
		},
		{
			name: "explicit latest name takes precedence over tag",
			override: &v2alpha1.DatadogAgentComponentOverride{Image: &v2alpha1.AgentImageConfig{
				Name: "registry/agent:latest",
				Tag:  "7.84.0",
			}},
			want: "latest",
		},
		{
			name: "tag with untagged name",
			override: &v2alpha1.DatadogAgentComponentOverride{Image: &v2alpha1.AgentImageConfig{
				Name: "agent",
				Tag:  "7.84.0",
			}},
			want: "7.84.0",
		},
		{
			name: "tag only",
			override: &v2alpha1.DatadogAgentComponentOverride{Image: &v2alpha1.AgentImageConfig{
				Tag: "7.84.0",
			}},
			want: "7.84.0",
		},
		{
			name: "tagged name with registry port takes precedence over tag",
			override: &v2alpha1.DatadogAgentComponentOverride{Image: &v2alpha1.AgentImageConfig{
				Name: "registry:5000/agent:7.85.0",
				Tag:  "7.84.0",
			}},
			want: "7.85.0",
		},
		{
			name: "registry port is not a tag",
			override: &v2alpha1.DatadogAgentComponentOverride{Image: &v2alpha1.AgentImageConfig{
				Name: "registry:5000/agent",
				Tag:  "7.84.0",
			}},
			want: "7.84.0",
		},
		{
			name: "tagged JMX name takes precedence over tag",
			override: &v2alpha1.DatadogAgentComponentOverride{Image: &v2alpha1.AgentImageConfig{
				Name: "registry/agent:7.85.0-jmx",
				Tag:  "7.84.0",
			}},
			want: "7.85.0",
		},
		{
			name: "combined image variant suffixes preserve the prerelease",
			override: &v2alpha1.DatadogAgentComponentOverride{Image: &v2alpha1.AgentImageConfig{
				Tag: "7.86.0-rc.1-fips-full",
			}},
			want: "7.86.0-rc.1",
		},
		{
			name: "Windows JMX image",
			override: &v2alpha1.DatadogAgentComponentOverride{Image: &v2alpha1.AgentImageConfig{
				Name: "registry/agent:7.86.0-servercore-jmx",
			}},
			want: "7.86.0",
		},
		{
			name: "untagged name only",
			override: &v2alpha1.DatadogAgentComponentOverride{Image: &v2alpha1.AgentImageConfig{
				Name: "agent",
			}},
		},
		{
			name:     "empty image override",
			override: &v2alpha1.DatadogAgentComponentOverride{Image: &v2alpha1.AgentImageConfig{}},
		},
		{
			name: "pull policy only",
			override: &v2alpha1.DatadogAgentComponentOverride{Image: &v2alpha1.AgentImageConfig{
				PullPolicy: new(corev1.PullAlways),
			}},
		},
	}

	defaultVersions := map[v2alpha1.ComponentName]string{
		v2alpha1.NodeAgentComponentName:           images.AgentLatestVersion,
		v2alpha1.ClusterAgentComponentName:        images.ClusterAgentLatestVersion,
		v2alpha1.ClusterChecksRunnerComponentName: images.AgentLatestVersion,
	}
	for componentName, defaultVersion := range defaultVersions {
		assert.Equal(t, defaultVersion, GetComponentVersionFromSpec(nil, componentName))
		assert.Equal(t, defaultVersion, GetComponentVersionFromSpec(&v2alpha1.DatadogAgentSpec{}, componentName))
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for componentName, defaultVersion := range defaultVersions {
				t.Run(string(componentName), func(t *testing.T) {
					spec := v2alpha1.DatadogAgentSpec{
						Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
							componentName: tt.override,
						},
					}
					want := tt.want
					if want == "" {
						want = defaultVersion
					}
					assert.Equal(t, want, GetComponentVersionFromSpec(&spec, componentName))
					for _, dda := range []metav1.Object{
						&v2alpha1.DatadogAgent{Spec: spec},
						&v1alpha1.DatadogAgentInternal{Spec: spec},
					} {
						assert.Equal(t, want, GetComponentVersion(dda, componentName), "%T", dda)
					}
				})
			}
		})
	}
}
