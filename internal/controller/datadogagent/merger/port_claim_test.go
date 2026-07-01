// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func tracePort(port int32) corev1.ServicePort {
	return corev1.ServicePort{
		Name:       "traceport",
		Protocol:   corev1.ProtocolTCP,
		Port:       port,
		TargetPort: intstr.FromInt(int(port)),
	}
}

func TestPortClaimAnnotationKey(t *testing.T) {
	key := PortClaimAnnotationKey("sample-dap", "apm")
	assert.Equal(t, "operator.datadoghq.com/port-claim.sample-dap.apm", key)
	assert.Equal(t, "sample-dap", portClaimKeyDDAI(key))
	assert.Equal(t, "", portClaimKeyDDAI("operator.datadoghq.com/managed-by-store"))
}

func TestEncodeDecodeServicePorts(t *testing.T) {
	ports := []corev1.ServicePort{tracePort(8126)}
	encoded, err := EncodeServicePorts(ports)
	require.NoError(t, err)

	decoded, err := DecodeServicePorts(encoded)
	require.NoError(t, err)
	assert.Equal(t, ports, decoded)
}

func TestMergePortClaims(t *testing.T) {
	apm8126, err := EncodeServicePorts([]corev1.ServicePort{tracePort(8126)})
	require.NoError(t, err)
	apm8127, err := EncodeServicePorts([]corev1.ServicePort{tracePort(8127)})
	require.NoError(t, err)

	tests := []struct {
		name        string
		annotations map[string]string
		wantPorts   []corev1.ServicePort
		wantOwner   string // non-empty => expect a conflict attributed to this DDAI
	}{
		{
			name:        "no claims",
			annotations: map[string]string{"unrelated": "x"},
			wantPorts:   nil,
		},
		{
			name: "single claim",
			annotations: map[string]string{
				PortClaimAnnotationKey("sample-dap", "apm"): apm8126,
			},
			wantPorts: []corev1.ServicePort{tracePort(8126)},
		},
		{
			name: "two claims agree (shared)",
			annotations: map[string]string{
				PortClaimAnnotationKey("datadog", "apm"):    apm8126,
				PortClaimAnnotationKey("sample-dap", "apm"): apm8126,
			},
			wantPorts: []corev1.ServicePort{tracePort(8126)},
		},
		{
			name: "two claims conflict",
			annotations: map[string]string{
				PortClaimAnnotationKey("datadog", "apm"):    apm8126,
				PortClaimAnnotationKey("sample-dap", "apm"): apm8127,
			},
			// "datadog" sorts before "sample-dap", so the latter is the one that
			// fails to merge into the accumulated set.
			wantOwner: "sample-dap",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ports, conflict := MergePortClaims(tt.annotations)
			if tt.wantOwner != "" {
				require.NotNil(t, conflict)
				assert.Equal(t, tt.wantOwner, conflict.DDAIName)
				assert.True(t, errors.Is(conflict, ErrServicePortConflict))
				return
			}
			require.Nil(t, conflict)
			assert.Equal(t, tt.wantPorts, ports)
		})
	}
}
