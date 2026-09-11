// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package experimental

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func TestIsAutopilotEnabled(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		want        bool
	}{
		{name: "no annotations", annotations: nil, want: false},
		{
			name:        "detected GKE Autopilot provider",
			annotations: map[string]string{kubernetes.ProviderAnnotationKey: kubernetes.GKEAutopilotProvider},
			want:        true,
		},
		{
			name:        "other detected provider",
			annotations: map[string]string{kubernetes.ProviderAnnotationKey: kubernetes.EKSCloudProvider},
			want:        false,
		},
		{
			name:        "experimental opt-in annotation",
			annotations: map[string]string{getExperimentalAnnotationKey(ExperimentalAutopilotSubkey): "true"},
			want:        true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsAutopilotEnabled(&metav1.ObjectMeta{Annotations: tt.annotations}))
		})
	}
}

func TestGetAutopilotAllowlistVersionAnnotation(t *testing.T) {
	tests := []struct {
		name       string
		annotation string
		want       string
	}{
		{name: "no annotation", annotation: "", want: ""},
		{name: "explicit override", annotation: "v1.2.3", want: "v1.2.3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dda := &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}},
			}
			if tt.annotation != "" {
				dda.Annotations[getExperimentalAnnotationKey(ExperimentalAutopilotAllowlistVersionSubkey)] = tt.annotation
			}
			got := getExperimentalAnnotation(dda, ExperimentalAutopilotAllowlistVersionSubkey)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestApplyAutopilotWorkloadAllowlistLabel(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
	}{
		{name: "default version", want: "datadog-datadog-daemonset-exemption-v1.0.6"},
		{name: "allowlist version override", version: "v1.2.3", want: "datadog-datadog-daemonset-exemption-v1.2.3"},
		{name: "malformed version falls back to default", version: "invalid", want: "datadog-datadog-daemonset-exemption-v1.0.6"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"existing": "label"}}})

			applyAutopilotWorkloadAllowlistLabel(manager, tt.version)

			assert.Equal(t, tt.want, manager.PodTemplateSpec().Labels["cloud.google.com/matching-allowlist"])
			assert.Equal(t, "label", manager.PodTemplateSpec().Labels["existing"])
		})
	}
}

func TestGetAutopilotCSIAllowlistVersionAnnotation(t *testing.T) {
	tests := []struct {
		name       string
		annotation string
		want       string
	}{
		{name: "no annotation", annotation: "", want: ""},
		{name: "explicit override", annotation: "v1.2.3", want: "v1.2.3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dda := &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}},
			}
			if tt.annotation != "" {
				dda.Annotations[getExperimentalAnnotationKey(ExperimentalAutopilotCSIAllowlistVersionSubkey)] = tt.annotation
			}
			got := getExperimentalAnnotation(dda, ExperimentalAutopilotCSIAllowlistVersionSubkey)
			assert.Equal(t, tt.want, got)
		})
	}
}
