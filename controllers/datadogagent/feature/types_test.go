// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package feature

import "testing"

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
