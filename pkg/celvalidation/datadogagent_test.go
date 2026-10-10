// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package celvalidation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

func TestValidateDatadogAgent_CommonLabels_ReservedKeys(t *testing.T) {
	tests := []struct {
		name           string
		labels         map[string]string
		wantErr        bool
		errMsgContains string
	}{
		{
			name:    "no extra labels",
			labels:  nil,
			wantErr: false,
		},
		{
			name: "valid non-reserved labels",
			labels: map[string]string{
				"team":               "platform",
				"cost-center":        "ops",
				"my.company.com/env": "prod",
			},
			wantErr: false,
		},
		{
			name: "reserved agent.datadoghq.com prefix",
			labels: map[string]string{
				"agent.datadoghq.com/datadogagentprofile": "my-profile",
			},
			wantErr:        true,
			errMsgContains: "reserved key",
		},
		{
			name: "reserved operator.datadoghq.com prefix",
			labels: map[string]string{
				"operator.datadoghq.com/managed-by-store": "true",
			},
			wantErr:        true,
			errMsgContains: "reserved key",
		},
		{
			name: "reserved datadoghq.com prefix",
			labels: map[string]string{
				"datadoghq.com/custom": "value",
			},
			wantErr:        true,
			errMsgContains: "reserved key",
		},
		{
			name: "mix of valid and reserved keys",
			labels: map[string]string{
				"team":                     "platform",
				"agent.datadoghq.com/name": "foo",
			},
			wantErr:        true,
			errMsgContains: "reserved key",
		},
		{
			name: "invalid label key with spaces",
			labels: map[string]string{
				"invalid key": "value",
			},
			wantErr:        true,
			errMsgContains: "invalid label key",
		},
		{
			name: "invalid label value with slash",
			labels: map[string]string{
				"valid-key": "invalid/value",
			},
			wantErr:        true,
			errMsgContains: "invalid value",
		},
		{
			name: "invalid label key too long",
			labels: map[string]string{
				// 64 chars — exceeds the 63-char limit for a simple name
				"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa": "value",
			},
			wantErr:        true,
			errMsgContains: "invalid label key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dda := &v2alpha1.DatadogAgent{
				Spec: v2alpha1.DatadogAgentSpec{
					Global: &v2alpha1.GlobalConfig{
						Credentials: &v2alpha1.DatadogCredentials{
							APIKey: ptr.To("key"),
						},
						CommonLabels: tt.labels,
					},
				},
			}
			err := validateDatadogAgent(t, dda)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsgContains != "" {
					assert.Contains(t, err.Error(), tt.errMsgContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// validateDatadogAgent runs the two halves of DatadogAgent validation the way
// the reconciler does: the CEL rules, then the label-syntax check that stayed
// in Go because IsQualifiedName and IsValidLabelValue are more than a regex.
func validateDatadogAgent(t *testing.T, dda *v2alpha1.DatadogAgent) error {
	t.Helper()
	if err := ddaRules(t).ValidateObject(context.Background(), dda, dda.Namespace, dda.Name); err != nil {
		return err
	}
	if dda.Spec.Global != nil {
		return v2alpha1.ValidateCommonLabelSyntax(dda.Spec.Global.CommonLabels)
	}
	return nil
}
