// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

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
