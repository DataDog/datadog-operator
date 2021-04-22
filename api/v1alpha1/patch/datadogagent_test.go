// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package patch

import (
	"reflect"
	"testing"

	"github.com/DataDog/datadog-operator/api/v1alpha1"
	"github.com/google/go-cmp/cmp"
)

func TestDatadogAgent(t *testing.T) {
	tests := []struct {
		name        string
		da          *v1alpha1.DatadogAgent
		want        *v1alpha1.DatadogAgent
		wantPatched bool
	}{
		{
			name:        "nothing to patch",
			da:          &v1alpha1.DatadogAgent{},
			want:        &v1alpha1.DatadogAgent{},
			wantPatched: false,
		},
		{
			name: "patch logCollection",
			da: &v1alpha1.DatadogAgent{
				Spec: v1alpha1.DatadogAgentSpec{
					Agent: &v1alpha1.DatadogAgentSpecAgentSpec{
						Log: &v1alpha1.LogCollectionConfig{
							Enabled: v1alpha1.NewBoolPointer(true),
						},
					},
				},
			},
			want: &v1alpha1.DatadogAgent{
				Spec: v1alpha1.DatadogAgentSpec{
					Features: v1alpha1.DatadogFeatures{
						LogCollection: &v1alpha1.LogCollectionConfig{
							Enabled: v1alpha1.NewBoolPointer(true),
						},
					},
					Agent: &v1alpha1.DatadogAgentSpecAgentSpec{
						Log: &v1alpha1.LogCollectionConfig{
							Enabled: v1alpha1.NewBoolPointer(true),
						},
					},
				},
			},
			wantPatched: true,
		},
		{
			name: "don't patch existing LogCollection",
			da: &v1alpha1.DatadogAgent{
				Spec: v1alpha1.DatadogAgentSpec{
					Features: v1alpha1.DatadogFeatures{
						LogCollection: &v1alpha1.LogCollectionConfig{
							Enabled: v1alpha1.NewBoolPointer(false),
						},
					},
					Agent: &v1alpha1.DatadogAgentSpecAgentSpec{
						Log: &v1alpha1.LogCollectionConfig{
							Enabled: v1alpha1.NewBoolPointer(true),
						},
					},
				},
			},
			want: &v1alpha1.DatadogAgent{
				Spec: v1alpha1.DatadogAgentSpec{
					Features: v1alpha1.DatadogFeatures{
						LogCollection: &v1alpha1.LogCollectionConfig{
							Enabled: v1alpha1.NewBoolPointer(false),
						},
					},
					Agent: &v1alpha1.DatadogAgentSpecAgentSpec{
						Log: &v1alpha1.LogCollectionConfig{
							Enabled: v1alpha1.NewBoolPointer(true),
						},
					},
				},
			},
			wantPatched: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := CopyAndPatchDatadogAgent(tt.da)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DatadogAgent() %s", cmp.Diff(got, tt.want))
			}
			if got1 != tt.wantPatched {
				t.Errorf("DatadogAgent() got1 = %v, want %v", got1, tt.wantPatched)
			}
		})
	}
}
