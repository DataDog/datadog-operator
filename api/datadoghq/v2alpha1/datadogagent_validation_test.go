// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v2alpha1

import (
	"testing"

	"k8s.io/utils/ptr"
)

func Test_validateVSock(t *testing.T) {
	tests := []struct {
		name    string
		spec    *DatadogAgentSpec
		wantErr bool
	}{
		{
			name:    "no global - no error",
			spec:    &DatadogAgentSpec{},
			wantErr: false,
		},
		{
			name: "vsock disabled - no error",
			spec: &DatadogAgentSpec{
				Global: &GlobalConfig{},
			},
			wantErr: false,
		},
		{
			name: "vsock Full mode + CWS without directSend - no error",
			spec: &DatadogAgentSpec{
				Global: &GlobalConfig{
					VSock: &VSockConfig{Enabled: ptr.To(true), Mode: ptr.To(VSockModeFull)},
				},
				Features: &DatadogFeatures{
					CWS: &CWSFeatureConfig{Enabled: ptr.To(true)},
				},
			},
			wantErr: false,
		},
		{
			name: "deprecated useVSock (maps to Full) + CWS without directSend - no error",
			spec: &DatadogAgentSpec{
				Global: &GlobalConfig{
					UseVSock: ptr.To(true),
				},
				Features: &DatadogFeatures{
					CWS: &CWSFeatureConfig{Enabled: ptr.To(true)},
				},
			},
			wantErr: false,
		},
		{
			name: "vsock SystemProbe mode + CWS disabled - no error",
			spec: &DatadogAgentSpec{
				Global: &GlobalConfig{
					VSock: &VSockConfig{Enabled: ptr.To(true), Mode: ptr.To(VSockModeSystemProbe)},
				},
				Features: &DatadogFeatures{
					CWS: &CWSFeatureConfig{Enabled: ptr.To(false)},
				},
			},
			wantErr: false,
		},
		{
			name: "vsock SystemProbe mode + CWS enabled + directSend - no error",
			spec: &DatadogAgentSpec{
				Global: &GlobalConfig{
					VSock: &VSockConfig{Enabled: ptr.To(true), Mode: ptr.To(VSockModeSystemProbe)},
				},
				Features: &DatadogFeatures{
					CWS: &CWSFeatureConfig{Enabled: ptr.To(true), DirectSendFromSystemProbe: ptr.To(true)},
				},
			},
			wantErr: false,
		},
		{
			name: "vsock SystemProbe mode + CWS enabled without directSend - error",
			spec: &DatadogAgentSpec{
				Global: &GlobalConfig{
					VSock: &VSockConfig{Enabled: ptr.To(true), Mode: ptr.To(VSockModeSystemProbe)},
				},
				Features: &DatadogFeatures{
					CWS: &CWSFeatureConfig{Enabled: ptr.To(true)},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateVSock(tt.spec)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateVSock() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
