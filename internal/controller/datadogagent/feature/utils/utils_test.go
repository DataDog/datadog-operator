// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/stretchr/testify/assert"
)

func specWithImageTag(tag string) *v2alpha1.DatadogAgentSpec {
	return &v2alpha1.DatadogAgentSpec{
		Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
			v2alpha1.NodeAgentComponentName: {
				Image: &v2alpha1.AgentImageConfig{Tag: tag},
			},
		},
	}
}

func specWithNoImage() *v2alpha1.DatadogAgentSpec {
	return &v2alpha1.DatadogAgentSpec{
		Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{},
	}
}

func TestShouldRunProcessChecksInCoreAgent(t *testing.T) {
	tests := []struct {
		name string
		spec *v2alpha1.DatadogAgentSpec
		want bool
	}{
		{
			name: "agent 7.52 - below min version",
			spec: specWithImageTag("7.52.0"),
			want: false,
		},
		{
			name: "agent 7.60 - at min version",
			spec: specWithImageTag("7.60.0"),
			want: true,
		},
		{
			name: "agent 7.77 - above min, below removed",
			spec: specWithImageTag("7.77.0"),
			want: true,
		},
		{
			name: "agent 7.78 - at removed version",
			spec: specWithImageTag("7.78.0"),
			want: true,
		},
		{
			name: "no image override - uses default",
			spec: specWithNoImage(),
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ShouldRunProcessChecksInCoreAgent(tt.spec))
		})
	}
}

func TestNeedsRunInCoreAgentEnvVar(t *testing.T) {
	tests := []struct {
		name string
		spec *v2alpha1.DatadogAgentSpec
		want bool
	}{
		{
			name: "agent 7.52 - below min version, no envvar needed",
			spec: specWithImageTag("7.52.0"),
			want: false,
		},
		{
			name: "agent 7.60 - needs envvar",
			spec: specWithImageTag("7.60.0"),
			want: true,
		},
		{
			name: "agent 7.64 - needs envvar",
			spec: specWithImageTag("7.64.0"),
			want: true,
		},
		{
			name: "agent 7.77 - needs envvar",
			spec: specWithImageTag("7.77.0"),
			want: true,
		},
		{
			name: "agent 7.78 - config removed, no envvar needed",
			spec: specWithImageTag("7.78.0"),
			want: false,
		},
		{
			name: "agent 7.80 - config removed, no envvar needed",
			spec: specWithImageTag("7.80.0"),
			want: false,
		},
		{
			name: "no image override - uses default latest version",
			spec: specWithNoImage(),
			want: true, // AgentLatestVersion is 7.77.0, which is < 7.78
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, NeedsRunInCoreAgentEnvVar(tt.spec))
		})
	}
}
