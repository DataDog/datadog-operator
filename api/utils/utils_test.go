// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"

	"k8s.io/apimachinery/pkg/util/intstr"
)

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
