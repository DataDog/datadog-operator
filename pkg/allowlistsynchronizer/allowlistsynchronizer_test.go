// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package allowlistsynchronizer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComputeAllowlistPaths(t *testing.T) {
	tests := []struct {
		name                 string
		otelCollectorEnabled bool
		expected             []string
	}{
		{
			name:                 "OTel collector disabled",
			otelCollectorEnabled: false,
			expected: []string{
				"Datadog/datadog/datadog-datadog-daemonset-exemption-v1.0.1.yaml",
			},
		},
		{
			name:                 "OTel collector enabled adds v1.0.5",
			otelCollectorEnabled: true,
			expected: []string{
				"Datadog/datadog/datadog-datadog-daemonset-exemption-v1.0.1.yaml",
				"Datadog/datadog/datadog-datadog-daemonset-exemption-v1.0.5.yaml",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ComputeAllowlistPaths(tt.otelCollectorEnabled))
		})
	}
}

func TestNewAllowlistSynchronizer(t *testing.T) {
	paths := ComputeAllowlistPaths(true)
	s := newAllowlistSynchronizer(paths)

	assert.Equal(t, "datadog-synchronizer", s.Name)
	assert.Equal(t, "AllowlistSynchronizer", s.Kind)
	assert.Equal(t, "pre-install,pre-upgrade", s.Annotations["helm.sh/hook"])
	assert.Equal(t, paths, s.Spec.AllowlistPaths)
}
