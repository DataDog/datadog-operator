// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetesstatecore

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIndentWriter(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		spaces   int
		expected string
	}{
		{
			name:     "single line with 2 spaces",
			input:    "hello world",
			spaces:   2,
			expected: "  hello world",
		},
		{
			name:     "single line with 4 spaces",
			input:    "hello world",
			spaces:   4,
			expected: "    hello world",
		},
		{
			name:     "multiple lines with 2 spaces",
			input:    "line1\nline2\nline3",
			spaces:   2,
			expected: "  line1\n  line2\n  line3",
		},
		{
			name:     "multiple lines with 4 spaces",
			input:    "line1\nline2\nline3",
			spaces:   4,
			expected: "    line1\n    line2\n    line3",
		},
		{
			name:     "empty lines preserved",
			input:    "line1\n\nline3",
			spaces:   2,
			expected: "  line1\n\n  line3",
		},
		{
			name:     "yaml-like content with 4 spaces",
			input:    "- item1\n  field: value\n- item2",
			spaces:   4,
			expected: "    - item1\n      field: value\n    - item2",
		},
		{
			name:     "yaml nested structure with 10 spaces",
			input:    "- groupVersionKind:\n    group: apps\n    version: v1\n    kind: Deployment",
			spaces:   10,
			expected: "          - groupVersionKind:\n              group: apps\n              version: v1\n              kind: Deployment",
		},
		{
			name:     "zero spaces (no indentation)",
			input:    "line1\nline2",
			spaces:   0,
			expected: "line1\nline2",
		},
		{
			name:     "empty input",
			input:    "",
			spaces:   2,
			expected: "",
		},
		{
			name:     "single newline",
			input:    "\n",
			spaces:   2,
			expected: "\n",
		},
		{
			name:     "multiple consecutive newlines",
			input:    "line1\n\n\nline2",
			spaces:   3,
			expected: "   line1\n\n\n   line2",
		},
		{
			name:     "line ending with newline",
			input:    "line1\nline2\n",
			spaces:   2,
			expected: "  line1\n  line2\n",
		},
		{
			name:     "mixed content with tabs",
			input:    "line1\n\ttabbed\nline3",
			spaces:   2,
			expected: "  line1\n  \ttabbed\n  line3",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			writer := newIndentWriter(&buf, tc.spaces)

			n, err := writer.Write([]byte(tc.input))
			require.NoError(t, err, "Write failed")

			assert.Equal(t, len(tc.input), n, "Write returned wrong byte count")

			result := buf.String()
			assert.Equal(t, tc.expected, result, "Indentation mismatch")
		})
	}
}

func TestIndentWriterMultipleWrites(t *testing.T) {
	testCases := []struct {
		name     string
		writes   []string
		spaces   int
		expected string
	}{
		{
			name:     "multiple writes on same line",
			writes:   []string{"hello", " ", "world"},
			spaces:   2,
			expected: "  hello world",
		},
		{
			name:     "multiple writes with newlines",
			writes:   []string{"line1\n", "line2\n", "line3"},
			spaces:   3,
			expected: "   line1\n   line2\n   line3",
		},
		{
			name:     "write by character",
			writes:   []string{"h", "e", "l", "l", "o", "\n", "w", "o", "r", "l", "d"},
			spaces:   4,
			expected: "    hello\n    world",
		},
		{
			name:     "partial line writes",
			writes:   []string{"part1", "part2\n", "next"},
			spaces:   2,
			expected: "  part1part2\n  next",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			writer := newIndentWriter(&buf, tc.spaces)

			totalBytes := 0
			for _, write := range tc.writes {
				n, err := writer.Write([]byte(write))
				require.NoError(t, err, "Write failed")
				assert.Equal(t, len(write), n, "Write returned wrong byte count")
				totalBytes += n
			}

			result := buf.String()
			assert.Equal(t, tc.expected, result, "Indentation mismatch")
		})
	}
}

func TestIndentWriterLargeContent(t *testing.T) {
	// Test with a larger YAML-like structure
	input := `resources:
  - groupVersionKind:
      group: apps
      version: v1
      kind: Deployment
    metrics:
      - name: deployment_replicas
        help: Number of replicas
  - groupVersionKind:
      group: batch
      version: v1
      kind: Job
    metrics:
      - name: job_status
        help: Job status`

	var buf bytes.Buffer
	writer := newIndentWriter(&buf, 6)

	n, err := writer.Write([]byte(input))
	require.NoError(t, err, "Write failed")

	assert.Equal(t, len(input), n, "Write returned wrong byte count")

	result := buf.String()
	lines := strings.Split(result, "\n")

	// Check that each non-empty line starts with 6 spaces
	for i, line := range lines {
		if line != "" {
			assert.True(t, strings.HasPrefix(line, "      "),
				"Line %d does not have correct indentation: %q", i+1, line)
		}
	}

	// Verify the first few lines have correct indentation
	expectedPrefix := "      resources:"
	assert.True(t, strings.HasPrefix(result, expectedPrefix),
		"Output should start with %q", expectedPrefix)
}

func BenchmarkIndentWriter(b *testing.B) {
	content := strings.Repeat("This is a line of text\n", 100)
	spaces := 4

	b.ResetTimer()
	for b.Loop() {
		var buf bytes.Buffer
		writer := newIndentWriter(&buf, spaces)
		_, err := writer.Write([]byte(content))
		require.NoError(b, err)
	}
}
