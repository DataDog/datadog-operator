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
