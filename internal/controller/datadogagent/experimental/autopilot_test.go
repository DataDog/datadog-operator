// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package experimental

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestHasOtelAgentContainer verifies that the otel-agent container detection used to
// gate the v1.0.5 WorkloadAllowlist exemption inclusion works for both the present
// and absent cases. See OTAGENT-980.
func TestHasOtelAgentContainer(t *testing.T) {
	tests := []struct {
		name       string
		containers []corev1.Container
		expected   bool
	}{
		{
			name: "no otel-agent container",
			containers: []corev1.Container{
				{Name: string(apicommon.CoreAgentContainerName)},
				{Name: string(apicommon.TraceAgentContainerName)},
			},
			expected: false,
		},
		{
			name: "otel-agent container present",
			containers: []corev1.Container{
				{Name: string(apicommon.CoreAgentContainerName)},
				{Name: string(apicommon.OtelAgent)},
			},
			expected: true,
		},
		{
			name:       "empty containers",
			containers: nil,
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: tt.containers},
			})
			assert.Equal(t, tt.expected, hasOtelAgentContainer(manager))
		})
	}
}

// TestApplyExperimentalAutopilotOverrides_SynchronizerInvocation verifies that the
// autopilot override branch invokes the synchronizer creator with the correct flag
// derived from the rendered pod template (OTAGENT-980).
func TestApplyExperimentalAutopilotOverrides_SynchronizerInvocation(t *testing.T) {
	// Stub out the synchronizer creator so tests don't hit kubeconfig loading.
	origCreator := createAllowlistSynchronizer
	defer func() { createAllowlistSynchronizer = origCreator }()

	autopilotAnnotation := map[string]string{
		ExperimentalAnnotationPrefix + "/" + ExperimentalAutopilotSubkey: "true",
	}

	tests := []struct {
		name                 string
		annotations          map[string]string
		containers           []corev1.Container
		expectInvoked        bool
		expectOtelFlagPassed bool
	}{
		{
			name:                 "autopilot disabled does not invoke synchronizer",
			annotations:          nil,
			containers:           []corev1.Container{{Name: string(apicommon.CoreAgentContainerName)}},
			expectInvoked:        false,
			expectOtelFlagPassed: false,
		},
		{
			name:                 "autopilot enabled without otel-agent invokes synchronizer with false",
			annotations:          autopilotAnnotation,
			containers:           []corev1.Container{{Name: string(apicommon.CoreAgentContainerName)}},
			expectInvoked:        true,
			expectOtelFlagPassed: false,
		},
		{
			name:                 "autopilot enabled with otel-agent invokes synchronizer with true",
			annotations:          autopilotAnnotation,
			containers:           []corev1.Container{{Name: string(apicommon.CoreAgentContainerName)}, {Name: string(apicommon.OtelAgent)}},
			expectInvoked:        true,
			expectOtelFlagPassed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var invoked bool
			var passedFlag bool
			createAllowlistSynchronizer = func(otelCollectorEnabled bool) {
				invoked = true
				passedFlag = otelCollectorEnabled
			}

			dda := &metav1.ObjectMeta{Annotations: tt.annotations}
			manager := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: tt.containers},
			})

			applyExperimentalAutopilotOverrides(dda, manager)

			assert.Equal(t, tt.expectInvoked, invoked, "synchronizer creator invocation")
			if tt.expectInvoked {
				assert.Equal(t, tt.expectOtelFlagPassed, passedFlag, "otelCollectorEnabled flag")
			}
		})
	}
}
