// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package feature

import (
	"reflect"
	"testing"

	common "github.com/DataDog/datadog-operator/api/crds/datadoghq/common"
)

func Test_merge(t *testing.T) {
	trueValue := true
	falseValue := false

	tests := []struct {
		name string
		a    *bool
		b    *bool
		want *bool
	}{
		{
			name: "both nil",
			a:    nil,
			b:    nil,
			want: nil,
		},
		{
			name: "a false",
			a:    &falseValue,
			b:    nil,
			want: &falseValue,
		},
		{
			name: "a false, b true",
			a:    &falseValue,
			b:    &trueValue,
			want: &falseValue,
		},
		{
			name: "a true, b false",
			a:    &trueValue,
			b:    &falseValue,
			want: &falseValue,
		},
		{
			name: "a nil, b true",
			a:    nil,
			b:    &trueValue,
			want: &trueValue,
		},
		{
			name: "a true, b true",
			a:    &trueValue,
			b:    &trueValue,
			want: &trueValue,
		},
		{
			name: "a true, b nil",
			a:    &trueValue,
			b:    nil,
			want: &trueValue,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := merge(tt.a, tt.b)
			gotSet := got != nil
			wantSet := tt.want != nil
			if gotSet != wantSet {
				t.Fatalf("merge() = %v, want nil", *got)
			}
			if wantSet && *got != *tt.want {
				t.Fatalf("merge() = %v, want %v", *got, *tt.want)
			}
		})
	}
}

func Test_mergeSlices(t *testing.T) {
	tests := []struct {
		name string
		a    []common.AgentContainerName
		b    []common.AgentContainerName
		want []common.AgentContainerName
	}{
		{
			name: "empty slices",
			a:    []common.AgentContainerName{},
			b:    []common.AgentContainerName{},
			want: []common.AgentContainerName{},
		},
		{
			name: "nil slices",
			a:    nil,
			b:    nil,
			want: nil,
		},
		{
			name: "a not empty, b empty",
			a:    []common.AgentContainerName{common.ClusterAgentContainerName},
			b:    []common.AgentContainerName{},
			want: []common.AgentContainerName{common.ClusterAgentContainerName},
		},
		{
			name: "a,b same data",
			a:    []common.AgentContainerName{common.ClusterAgentContainerName},
			b:    []common.AgentContainerName{common.ClusterAgentContainerName},
			want: []common.AgentContainerName{common.ClusterAgentContainerName},
		},
		{
			name: "a,b merge data",
			a:    []common.AgentContainerName{common.ClusterAgentContainerName, common.ClusterAgentContainerName},
			b:    []common.AgentContainerName{common.ClusterAgentContainerName, common.ProcessAgentContainerName},
			want: []common.AgentContainerName{
				common.ClusterAgentContainerName,
				common.ClusterAgentContainerName,
				common.ProcessAgentContainerName,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mergeSlices(tt.a, tt.b); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("mergeSlices() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRequiredComponent_IsEnabled(t *testing.T) {
	trueValue := true
	falseValue := false

	type fields struct {
		IsRequired *bool
		Containers []common.AgentContainerName
	}

	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{
			name: "isEnabled == false, empty",
			fields: fields{
				IsRequired: nil,
				Containers: nil,
			},
			want: false,
		},
		{
			name: "isEnabled == true",
			fields: fields{
				IsRequired: &trueValue,
				Containers: nil,
			},
			want: true,
		},
		{
			name: "isEnabled == false",
			fields: fields{
				IsRequired: &falseValue,
				Containers: nil,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc := &RequiredComponent{
				IsRequired: tt.fields.IsRequired,
				Containers: tt.fields.Containers,
			}
			if got := rc.IsEnabled(); got != tt.want {
				t.Errorf("RequiredComponent.IsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRequiredComponent_IsConfigured(t *testing.T) {
	trueValue := true
	falseValue := false

	type fields struct {
		IsRequired *bool
		Containers []common.AgentContainerName
	}

	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{
			name: "IsConfigured == false, empty",
			fields: fields{
				IsRequired: nil,
				Containers: nil,
			},
			want: false,
		},
		{
			name: "IsConfigured == true, isRequired == true",
			fields: fields{
				IsRequired: &trueValue,
				Containers: nil,
			},
			want: true,
		},
		{
			name: "IsConfigured == true, isRequired == false",
			fields: fields{
				IsRequired: &falseValue,
				Containers: nil,
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc := &RequiredComponent{
				IsRequired: tt.fields.IsRequired,
				Containers: tt.fields.Containers,
			}
			if got := rc.IsConfigured(); got != tt.want {
				t.Errorf("RequiredComponent.IsConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}
