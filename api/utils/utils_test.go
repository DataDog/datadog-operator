// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"

	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestNewPointer(t *testing.T) {
	tests := []struct {
		name string
		val  int
		want int
	}{
		{
			name: "non-nil pointer",
			val:  10,
			want: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ptr := NewPointer(tt.val)
			if ptr == nil || *ptr != tt.want {
				t.Errorf("NewPointer() = %v, want %v", ptr, tt.want)
			}
		})
	}
}

func TestNewDeref(t *testing.T) {
	intVal := 10

	tests := []struct {
		name string
		ptr  *int
		def  int
		want int
	}{
		{
			name: "non-nil pointer",
			ptr:  &intVal,
			def:  0,
			want: 10,
		},
		{
			name: "nil pointer",
			ptr:  nil,
			def:  0,
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewDeref(tt.ptr, tt.def); got != tt.want {
				t.Errorf("NewDeref() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBoolToString(t *testing.T) {
	trueVal := true
	falseVal := false

	tests := []struct {
		name string
		b    *bool
		want string
	}{
		{
			name: "true value",
			b:    &trueVal,
			want: "true",
		},
		{
			name: "false value",
			b:    &falseVal,
			want: "false",
		},
		{
			name: "nil value",
			b:    nil,
			want: "false",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BoolToString(tt.b); got != tt.want {
				t.Errorf("BoolToString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_newIntOrStringPointer(t *testing.T) {
	tests := []struct {
		input string
		want  intstr.IntOrString
	}{
		{
			input: "15",
			want:  intstr.FromInt(15),
		},
		{
			input: "15%",
			want:  intstr.FromString("15%"),
		},
		{
			input: "0",
			want:  intstr.FromInt(0),
		},
		{
			input: "help",
			want:  intstr.FromString("help"),
		},
	}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			output := NewIntOrStringPointer(tt.input)
			if tt.want != *output {
				t.Errorf("newIntOrStringPointer() result is %v but want %v", *output, tt.want)
			}
		})
	}
}
