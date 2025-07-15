// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metadata

import (
	"os"
	"testing"
)

func Test_getURL(t *testing.T) {
	tests := []struct {
		name     string
		loadFunc func()
		wantURL  string
	}{
		{
			name: "default case",
			loadFunc: func() {
			},
			wantURL: "https://app.datadoghq.com/api/v1/metadata",
		},
		{
			name: "set DD_SITE",
			loadFunc: func() {
				os.Clearenv()
				os.Setenv("DD_SITE", "datad0g.com")
			},
			wantURL: "https://app.datad0g.com/api/v1/metadata",
		},
		{
			name: "set DD_URL",
			loadFunc: func() {
				os.Clearenv()
				os.Setenv("DD_URL", "https://app.datad0g.com")
			},
			wantURL: "https://app.datad0g.com/api/v1/metadata",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.loadFunc()

			u := getURL()

			if u != tt.wantURL {
				t.Errorf("getURL() url = %v, want %v", u, tt.wantURL)

			}
		})
	}
}
