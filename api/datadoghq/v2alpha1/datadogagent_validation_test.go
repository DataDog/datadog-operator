// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v2alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"
)

func TestValidateDatadogAgent_ExtraLabels_ReservedKeys(t *testing.T) {
	tests := []struct {
		name    string
		labels  map[string]string
		wantErr bool
	}{
		{
			name:    "no extra labels",
			labels:  nil,
			wantErr: false,
		},
		{
			name: "valid non-reserved labels",
			labels: map[string]string{
				"team":              "platform",
				"cost-center":       "ops",
				"my.company.com/env": "prod",
			},
			wantErr: false,
		},
		{
			name: "reserved agent.datadoghq.com prefix",
			labels: map[string]string{
				"agent.datadoghq.com/datadogagentprofile": "my-profile",
			},
			wantErr: true,
		},
		{
			name: "reserved operator.datadoghq.com prefix",
			labels: map[string]string{
				"operator.datadoghq.com/managed-by-store": "true",
			},
			wantErr: true,
		},
		{
			name: "reserved datadoghq.com prefix",
			labels: map[string]string{
				"datadoghq.com/custom": "value",
			},
			wantErr: true,
		},
		{
			name: "mix of valid and reserved keys",
			labels: map[string]string{
				"team":                        "platform",
				"agent.datadoghq.com/name":    "foo",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dda := &DatadogAgent{
				Spec: DatadogAgentSpec{
					Global: &GlobalConfig{
						Credentials: &DatadogCredentials{
							APIKey: ptr.To("key"),
						},
						ExtraLabels: tt.labels,
					},
				},
			}
			err := ValidateDatadogAgent(dda)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "reserved key")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
