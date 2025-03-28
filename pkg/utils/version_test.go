// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsAboveMinVersion(t *testing.T) {
	testCases := []struct {
		version    string
		minVersion string
		expected   bool
	}{
		{
			version:    "7.27.0",
			minVersion: "7.26.0-rc.5",
			expected:   true,
		},
		{
			version:    "7.29.0-rc.5",
			minVersion: "7.27.0-0",
			expected:   true,
		},
		{
			version:    "7.30.0",
			minVersion: "7.27.1-0",
			expected:   true,
		},
		{
			version:    "7.25.0",
			minVersion: "7.27.1-0",
			expected:   false,
		},
		{
			version:    "6.27.0",
			minVersion: "6.28.0-0",
			expected:   false,
		},
		{
			version:    "6.28.1",
			minVersion: "6.28.0-0",
			expected:   true,
		},
		{
			version:    "6.27.0",
			minVersion: "6.28.0",
			expected:   false,
		},
		{
			version:    "foobar",
			minVersion: "7.28.0",
			expected:   true,
		},
		{
			version:    "1.23.4",
			minVersion: "1.22-0",
			expected:   true,
		},
		{
			version:    "7.27.0-beta",
			minVersion: "7.26.0-0",
			expected:   true,
		},
		{
			version:    "7.27.0-beta",
			minVersion: "7.26.0",
			expected:   false,
		},
		{
			version:    "7.25.0-beta",
			minVersion: "7.26.0-0",
			expected:   false,
		},
		{
			version:    "7.25.0-beta",
			minVersion: "7.26.0",
			expected:   false,
		},
		{
			version:    "7.27.0-rc-3-jmx",
			minVersion: "7.26.0-0",
			expected:   true,
		},
		{
			version:    "7.27.0-rc-3-jmx",
			minVersion: "7.26.0",
			expected:   false,
		},
		{
			version:    "7.25.0-rc-3-jmx",
			minVersion: "7.26.0-0",
			expected:   false,
		},
		{
			version:    "7.25.0-rc-3-jmx",
			minVersion: "7.26.0",
			expected:   false,
		},
		{
			version:    "main",
			minVersion: "7.99.0",
			expected:   true,
		},
		{
			version:    "latest",
			minVersion: "7.99.0",
			expected:   true,
		},
		{
			version:    "latest-foo",
			minVersion: "7.99.0",
			expected:   true,
		},
	}
	for _, test := range testCases {
		t.Run(test.version, func(t *testing.T) {
			result := IsAboveMinVersion(test.version, test.minVersion)
			assert.Equal(t, test.expected, result)
		})
	}
}

func Test_formatVersionTag(t *testing.T) {
	testCases := []struct {
		version  string
		expected string
	}{
		{
			version:  "7.46.0",
			expected: "7.46.0",
		},
		{
			version:  "7.46.0-jmx",
			expected: "7.46.0-jmx",
		},
		{
			version:  "7.46.0-rc.3",
			expected: "7.46.0-rc.3",
		},
		{
			version:  "7.46.0-rc.3-jmx",
			expected: "7.46.0-rc.3-jmx",
		},
		{
			version:  "7.46.0-beta",
			expected: "7.46.0-beta",
		},
		// dashes ("-") in lieu of periods (".")
		{
			version:  "7-46-0",
			expected: "7.46.0",
		},
		{
			version:  "7-46-0-jmx",
			expected: "7.46.0-jmx",
		},
		{
			version:  "7-46-0-rc-3-jmx",
			expected: "7.46.0-rc-3-jmx",
		},
		{
			version:  "7-46-0-rc.3.jmx",
			expected: "7.46.0-rc.3.jmx",
		},
		{
			version:  "7-46-0-beta",
			expected: "7.46.0-beta",
		},
		// other string formats
		{
			version:  "customImage",
			expected: "",
		},
		{
			version:  "custom.image",
			expected: "",
		},
		{
			version:  "custom-image",
			expected: "",
		},
		{
			version:  "custom-image-long-name",
			expected: "",
		},
	}
	for _, test := range testCases {
		t.Run(test.version, func(t *testing.T) {
			result := formatVersionTag(test.version)
			assert.Equal(t, test.expected, result)
		})
	}
}
