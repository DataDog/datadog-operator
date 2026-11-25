// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"fmt"
	"strings"
	"testing"
)

func BenchmarkHashConfigMapContents(b *testing.B) {
	yamlChunk := strings.Repeat("  - name: check_instance\n    url: http://localhost:8080/metrics\n    tags:\n      - env:prod\n      - team:platform\n", 30)
	contents := make(map[string]configMapContent, 5)
	for i := range 5 {
		data := make(map[string]string, 4)
		for j := range 4 {
			data[fmt.Sprintf("config-%d-%d.yaml", i, j)] = yamlChunk
		}
		contents[fmt.Sprintf("datadog-custom-config-%d", i)] = configMapContent{
			Data: data,
			BinaryData: map[string][]byte{
				"cert.der": []byte(strings.Repeat("0123456789abcdef", 64)),
			},
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = hashConfigMapContents(contents)
	}
}
