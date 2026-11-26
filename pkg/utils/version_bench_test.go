// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"
)

func BenchmarkIsAboveMinVersion(b *testing.B) {
	defaultTrue := true
	defaultFalse := false
	cases := []struct {
		version    string
		minVersion string
		def        *bool
	}{
		{"7.78.0", "7.76.0-0", nil},
		{"7.78.0-jmx", "7.65.0-0", &defaultTrue},
		{"7-78-0", "7.50.0", nil},
		{"v1.31.2-gke.100", "1.30.0-0", &defaultFalse},
		{"latest", "7.78.0-0", nil},
		{"7.60.1-rc.1", "7.65.0-0", nil},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		for _, c := range cases {
			_ = IsAboveMinVersion(c.version, c.minVersion, c.def)
		}
	}
}
