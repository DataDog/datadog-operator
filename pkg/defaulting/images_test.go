// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2012 Datadog, Inc.

package defaulting

import "testing"

func TestGetLatestAgentImage(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{
			name: "default",
			want: "gcr.io/datadoghq/agent:7.30.0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetLatestAgentImage(); got != tt.want {
				t.Errorf("GetLatestAgentImage() = %v, want %v", got, tt.want)
			}
		})
	}
}
