// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package utils

import (
	"testing"
)

func TestMaxInt(t *testing.T) {
	type args struct {
		val0 int
		vals []int
	}
	tests := []struct {
		name string
		args args
		want int
	}{
		{
			name: "one value",
			args: args{val0: 5, vals: nil},
			want: 5,
		},
		{
			name: "several value 5, 7, 4",
			args: args{val0: 5, vals: []int{7, 4}},
			want: 7,
		},
		{
			name: "two value",
			args: args{val0: 5, vals: []int{4}},
			want: 5,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MaxInt(tt.args.val0, tt.args.vals...); got != tt.want {
				t.Errorf("MaxInt() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMinInt(t *testing.T) {
	type args struct {
		val0 int
		vals []int
	}
	tests := []struct {
		name string
		args args
		want int
	}{
		{
			name: "one value",
			args: args{val0: 5, vals: nil},
			want: 5,
		},
		{
			name: "several value 5, 7, 4",
			args: args{val0: 5, vals: []int{7, 4}},
			want: 4,
		},
		{
			name: "two value",
			args: args{val0: 5, vals: []int{4}},
			want: 4,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MinInt(tt.args.val0, tt.args.vals...); got != tt.want {
				t.Errorf("MinInt() = %v, want %v", got, tt.want)
			}
		})
	}
}
