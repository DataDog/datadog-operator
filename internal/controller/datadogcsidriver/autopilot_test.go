// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogcsidriver

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-operator/pkg/allowlistsynchronizer"
)

func TestDetectGKEAutopilotMode(t *testing.T) {
	tests := []struct {
		name         string
		platformInfo PlatformInfo
		expected     GKEAutopilotMode
	}{
		{
			name:         "nil platformInfo",
			platformInfo: nil,
			expected:     GKEAutopilotModeNone,
		},
		{
			name: "no Autopilot CRDs",
			platformInfo: &mockPlatformInfo{
				supportedResources: map[string]bool{},
			},
			expected: GKEAutopilotModeNone,
		},
		{
			name: "modern Autopilot (WorkloadAllowlist)",
			platformInfo: &mockPlatformInfo{
				supportedResources: map[string]bool{
					"WorkloadAllowlist": true,
				},
			},
			expected: GKEAutopilotModeModern,
		},
		{
			name: "modern Autopilot with both CRDs",
			platformInfo: &mockPlatformInfo{
				supportedResources: map[string]bool{
					"WorkloadAllowlist":     true,
					"AllowlistedV2Workload": true,
				},
			},
			expected: GKEAutopilotModeModern,
		},
		{
			name: "legacy Autopilot (AllowlistedV2Workload only)",
			platformInfo: &mockPlatformInfo{
				supportedResources: map[string]bool{
					"AllowlistedV2Workload": true,
				},
			},
			expected: GKEAutopilotModeLegacy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, DetectGKEAutopilotMode(tt.platformInfo))
		})
	}
}

func TestIsGKEAutopilot(t *testing.T) {
	tests := []struct {
		name         string
		platformInfo PlatformInfo
		expected     bool
	}{
		{
			name:         "nil platformInfo",
			platformInfo: nil,
			expected:     false,
		},
		{
			name: "no Autopilot CRDs",
			platformInfo: &mockPlatformInfo{
				supportedResources: map[string]bool{},
			},
			expected: false,
		},
		{
			name: "modern Autopilot",
			platformInfo: &mockPlatformInfo{
				supportedResources: map[string]bool{
					"WorkloadAllowlist": true,
				},
			},
			expected: true,
		},
		{
			name: "legacy Autopilot",
			platformInfo: &mockPlatformInfo{
				supportedResources: map[string]bool{
					"AllowlistedV2Workload": true,
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsGKEAutopilot(tt.platformInfo))
		})
	}
}

func TestIsGKEAutopilotModern(t *testing.T) {
	tests := []struct {
		name         string
		platformInfo PlatformInfo
		expected     bool
	}{
		{
			name: "modern Autopilot",
			platformInfo: &mockPlatformInfo{
				supportedResources: map[string]bool{
					"WorkloadAllowlist": true,
				},
			},
			expected: true,
		},
		{
			name: "legacy Autopilot",
			platformInfo: &mockPlatformInfo{
				supportedResources: map[string]bool{
					"AllowlistedV2Workload": true,
				},
			},
			expected: false,
		},
		{
			name: "not Autopilot",
			platformInfo: &mockPlatformInfo{
				supportedResources: map[string]bool{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsGKEAutopilotModern(tt.platformInfo))
		})
	}
}

func TestIsGKEAutopilotLegacy(t *testing.T) {
	tests := []struct {
		name         string
		platformInfo PlatformInfo
		expected     bool
	}{
		{
			name: "modern Autopilot",
			platformInfo: &mockPlatformInfo{
				supportedResources: map[string]bool{
					"WorkloadAllowlist": true,
				},
			},
			expected: false,
		},
		{
			name: "legacy Autopilot",
			platformInfo: &mockPlatformInfo{
				supportedResources: map[string]bool{
					"AllowlistedV2Workload": true,
				},
			},
			expected: true,
		},
		{
			name: "not Autopilot",
			platformInfo: &mockPlatformInfo{
				supportedResources: map[string]bool{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsGKEAutopilotLegacy(tt.platformInfo))
		})
	}
}

func TestGetGKEAutopilotLabels(t *testing.T) {
	tests := []struct {
		name         string
		platformInfo PlatformInfo
		expected     map[string]string
	}{
		{
			name: "not Autopilot returns nil",
			platformInfo: &mockPlatformInfo{
				supportedResources: map[string]bool{},
			},
			expected: nil,
		},
		{
			name: "legacy Autopilot returns nil",
			platformInfo: &mockPlatformInfo{
				supportedResources: map[string]bool{
					"AllowlistedV2Workload": true,
				},
			},
			expected: nil,
		},
		{
			name: "modern Autopilot returns matching-allowlist label",
			platformInfo: &mockPlatformInfo{
				supportedResources: map[string]bool{
					"WorkloadAllowlist": true,
				},
			},
			expected: map[string]string{
				GKEMatchingAllowlistLabelKey: allowlistsynchronizer.CSIMatchingAllowlistLabel,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetGKEAutopilotLabels(tt.platformInfo)
			assert.Equal(t, tt.expected, result)
		})
	}
}
