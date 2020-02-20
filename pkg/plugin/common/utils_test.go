// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package common

import "testing"

func TestHasImagePattern(t *testing.T) {
	tests := []struct {
		name  string
		image string
		want  bool
	}{
		{
			name:  "nominal case",
			image: "datadog/agent:latest",
			want:  true,
		},
		{
			name:  "no tag 1",
			image: "datadog/agent",
			want:  false,
		},
		{
			name:  "no tag 2",
			image: "datadog/agent:",
			want:  false,
		},
		{
			name:  "no repo 1",
			image: "datadog/:latest",
			want:  false,
		},
		{
			name:  "no repo 2",
			image: "datadog:latest",
			want:  false,
		},
		{
			name:  "multiple tags",
			image: "datadog/agent:tag1:tag2",
			want:  true,
		},
		{
			name:  "no account",
			image: "/agent:latest",
			want:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasImagePattern(tt.image); got != tt.want {
				t.Errorf("HasImagePattern() = %v, want %v", got, tt.want)
			}
		})
	}
}
