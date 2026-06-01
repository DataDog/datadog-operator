// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogcsidriver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/allowlistsynchronizer"
)

func TestIsGKEAutopilotEnabled(t *testing.T) {
	tests := []struct {
		name        string
		instance    *v1alpha1.DatadogCSIDriver
		expected    bool
	}{
		{
			name:     "nil instance",
			instance: nil,
			expected: false,
		},
		{
			name: "no annotations",
			instance: &v1alpha1.DatadogCSIDriver{
				ObjectMeta: metav1.ObjectMeta{},
			},
			expected: false,
		},
		{
			name: "autopilot not set",
			instance: &v1alpha1.DatadogCSIDriver{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"other-annotation": "value",
					},
				},
			},
			expected: false,
		},
		{
			name: "autopilot enabled (lowercase)",
			instance: &v1alpha1.DatadogCSIDriver{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						GKEAutopilotAnnotation: "true",
					},
				},
			},
			expected: true,
		},
		{
			name: "autopilot enabled (uppercase)",
			instance: &v1alpha1.DatadogCSIDriver{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						GKEAutopilotAnnotation: "TRUE",
					},
				},
			},
			expected: true,
		},
		{
			name: "autopilot disabled",
			instance: &v1alpha1.DatadogCSIDriver{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						GKEAutopilotAnnotation: "false",
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsGKEAutopilotEnabled(tt.instance))
		})
	}
}

func TestIsGKEAutopilotLegacy(t *testing.T) {
	tests := []struct {
		name     string
		instance *v1alpha1.DatadogCSIDriver
		expected bool
	}{
		{
			name:     "nil instance",
			instance: nil,
			expected: false,
		},
		{
			name: "legacy not set",
			instance: &v1alpha1.DatadogCSIDriver{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						GKEAutopilotAnnotation: "true",
					},
				},
			},
			expected: false,
		},
		{
			name: "legacy enabled",
			instance: &v1alpha1.DatadogCSIDriver{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						GKEAutopilotAnnotation:       "true",
						GKEAutopilotLegacyAnnotation: "true",
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsGKEAutopilotLegacy(tt.instance))
		})
	}
}

func TestGetGKEAutopilotAllowlistVersion(t *testing.T) {
	tests := []struct {
		name     string
		instance *v1alpha1.DatadogCSIDriver
		expected string
	}{
		{
			name:     "nil instance",
			instance: nil,
			expected: "",
		},
		{
			name: "version not set",
			instance: &v1alpha1.DatadogCSIDriver{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						GKEAutopilotAnnotation: "true",
					},
				},
			},
			expected: "",
		},
		{
			name: "custom version set",
			instance: &v1alpha1.DatadogCSIDriver{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						GKEAutopilotAnnotation:                 "true",
						GKEAutopilotAllowlistVersionAnnotation: "v2.0.0",
					},
				},
			},
			expected: "v2.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, GetGKEAutopilotAllowlistVersion(tt.instance))
		})
	}
}

func TestGetGKEAutopilotLabels(t *testing.T) {
	tests := []struct {
		name     string
		instance *v1alpha1.DatadogCSIDriver
		expected map[string]string
	}{
		{
			name: "autopilot disabled returns nil",
			instance: &v1alpha1.DatadogCSIDriver{
				ObjectMeta: metav1.ObjectMeta{},
			},
			expected: nil,
		},
		{
			name: "autopilot legacy returns nil",
			instance: &v1alpha1.DatadogCSIDriver{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						GKEAutopilotAnnotation:       "true",
						GKEAutopilotLegacyAnnotation: "true",
					},
				},
			},
			expected: nil,
		},
		{
			name: "autopilot enabled returns matching-allowlist label",
			instance: &v1alpha1.DatadogCSIDriver{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						GKEAutopilotAnnotation: "true",
					},
				},
			},
			expected: map[string]string{
				GKEMatchingAllowlistLabelKey: allowlistsynchronizer.CSIMatchingAllowlistLabel,
			},
		},
		{
			name: "autopilot with custom version",
			instance: &v1alpha1.DatadogCSIDriver{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						GKEAutopilotAnnotation:                 "true",
						GKEAutopilotAllowlistVersionAnnotation: "v2.0.0",
					},
				},
			},
			expected: map[string]string{
				GKEMatchingAllowlistLabelKey: "datadog-datadog-csi-driver-daemonset-exemption-v2.0.0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetGKEAutopilotLabels(tt.instance)
			assert.Equal(t, tt.expected, result)
		})
	}
}
