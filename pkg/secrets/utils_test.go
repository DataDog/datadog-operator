// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secrets

import (
	"reflect"
	"testing"
)

func Test_extractHandles(t *testing.T) {
	tests := []struct {
		name    string
		handles []string
		want    []string
		wantErr bool
	}{
		{
			name: "nominal case",
			handles: []string{
				"ENC[api_key]",
				"ENC[app_key]",
			},
			want: []string{
				"api_key",
				"app_key",
			},
			wantErr: false,
		},
		{
			name:    "empty input",
			handles: []string{},
			want:    []string{},
			wantErr: false,
		},
		{
			name:    "nil input",
			handles: nil,
			want:    []string{},
			wantErr: false,
		},
		{
			name: "bracket missing",
			handles: []string{
				"ENC[api_key",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "empty handle",
			handles: []string{
				"ENC[]",
			},
			want: []string{
				"",
			},
			wantErr: false,
		},
		{
			name: "wrong format",
			handles: []string{
				"enc[]",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "padded with spaces and tabs",
			handles: []string{
				" ENC[api_key]\t",
				"\t ENC[app_key] ",
			},
			want: []string{
				"api_key",
				"app_key",
			},
			wantErr: false,
		},
		{
			name: "inner whitespace preserved",
			handles: []string{
				"ENC[ api_key ]",
			},
			want: []string{
				" api_key ",
			},
			wantErr: false,
		},
		{
			name: "padded with newline",
			handles: []string{
				"ENC[api_key]\n",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "multiple brackets",
			handles: []string{
				"ENC[[]]",
			},
			want: []string{
				"[]",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractHandles(tt.handles)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractHandles() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("extractHandles() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsEnc(t *testing.T) {
	tests := []struct {
		name string
		str  string
		want bool
	}{
		{name: "exact", str: "ENC[handle]", want: true},
		{name: "leading space", str: " ENC[handle]", want: true},
		{name: "trailing tab", str: "ENC[handle]\t", want: true},
		{name: "spaces and tabs", str: " \tENC[handle]\t ", want: true},
		{name: "empty handle", str: "ENC[]", want: true},
		{name: "literal", str: "0123456789abcdef", want: false},
		{name: "empty", str: "", want: false},
		{name: "whitespace only", str: " \t ", want: false},
		{name: "lowercase", str: "enc[handle]", want: false},
		{name: "missing closing bracket", str: " ENC[handle", want: false},
		{name: "missing prefix", str: "handle] ", want: false},
		{name: "newline not trimmed", str: "\nENC[handle]", want: false},
		{name: "space inside prefix", str: "ENC [handle]", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsEnc(tt.str); got != tt.want {
				t.Errorf("IsEnc(%q) = %v, want %v", tt.str, got, tt.want)
			}
		})
	}
}
