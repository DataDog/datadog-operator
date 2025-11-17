// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"

	apiutils "github.com/DataDog/datadog-operator/api/utils"
)

func TestIsAboveMinVersion(t *testing.T) {
	testCases := []struct {
		name         string
		version      string
		minVersion   string
		defaultValue *bool
		expected     bool
		description  string
	}{
		{
			name:         "7.27.0 >= 7.26.0-rc.5",
			version:      "7.27.0",
			minVersion:   "7.26.0-rc.5",
			defaultValue: nil,
			expected:     true,
		},
		{
			name:         "7.29.0-rc.5 >= 7.27.0-0",
			version:      "7.29.0-rc.5",
			minVersion:   "7.27.0-0",
			defaultValue: nil,
			expected:     true,
		},
		{
			name:         "7.30.0 >= 7.27.1-0",
			version:      "7.30.0",
			minVersion:   "7.27.1-0",
			defaultValue: nil,
			expected:     true,
		},
		{
			name:         "7.25.0 < 7.27.1-0",
			version:      "7.25.0",
			minVersion:   "7.27.1-0",
			defaultValue: nil,
			expected:     false,
		},
		{
			name:         "6.27.0 < 6.28.0-0",
			version:      "6.27.0",
			minVersion:   "6.28.0-0",
			defaultValue: nil,
			expected:     false,
		},
		{
			name:         "6.28.1 >= 6.28.0-0",
			version:      "6.28.1",
			minVersion:   "6.28.0-0",
			defaultValue: nil,
			expected:     true,
		},
		{
			name:         "6.27.0 < 6.28.0",
			version:      "6.27.0",
			minVersion:   "6.28.0",
			defaultValue: nil,
			expected:     false,
		},
		{
			name:         "foobar unparseable with nil default",
			version:      "foobar",
			minVersion:   "7.28.0",
			defaultValue: nil,
			expected:     true,
			description:  "unparseable versions return true with nil default",
		},
		{
			name:         "1.23.4 >= 1.22-0",
			version:      "1.23.4",
			minVersion:   "1.22-0",
			defaultValue: nil,
			expected:     true,
		},
		{
			name:         "7.27.0-beta >= 7.26.0-0",
			version:      "7.27.0-beta",
			minVersion:   "7.26.0-0",
			defaultValue: nil,
			expected:     true,
		},
		{
			name:         "7.27.0-beta < 7.26.0",
			version:      "7.27.0-beta",
			minVersion:   "7.26.0",
			defaultValue: nil,
			expected:     false,
		},
		{
			name:         "7.25.0-beta < 7.26.0-0",
			version:      "7.25.0-beta",
			minVersion:   "7.26.0-0",
			defaultValue: nil,
			expected:     false,
		},
		{
			name:         "7.25.0-beta < 7.26.0",
			version:      "7.25.0-beta",
			minVersion:   "7.26.0",
			defaultValue: nil,
			expected:     false,
		},
		{
			name:         "7.27.0-rc-3-jmx >= 7.26.0-0",
			version:      "7.27.0-rc-3-jmx",
			minVersion:   "7.26.0-0",
			defaultValue: nil,
			expected:     true,
		},
		{
			name:         "7.27.0-rc-3-jmx < 7.26.0",
			version:      "7.27.0-rc-3-jmx",
			minVersion:   "7.26.0",
			defaultValue: nil,
			expected:     false,
		},
		{
			name:         "7.25.0-rc-3-jmx < 7.26.0-0",
			version:      "7.25.0-rc-3-jmx",
			minVersion:   "7.26.0-0",
			defaultValue: nil,
			expected:     false,
		},
		{
			name:         "7.25.0-rc-3-jmx < 7.26.0",
			version:      "7.25.0-rc-3-jmx",
			minVersion:   "7.26.0",
			defaultValue: nil,
			expected:     false,
		},
		{
			name:         "main unparseable with nil default",
			version:      "main",
			minVersion:   "7.99.0",
			defaultValue: nil,
			expected:     true,
		},
		{
			name:         "latest unparseable with nil default",
			version:      "latest",
			minVersion:   "7.99.0",
			defaultValue: nil,
			expected:     true,
		},
		{
			name:         "latest-foo unparseable with nil default",
			version:      "latest-foo",
			minVersion:   "7.99.0",
			defaultValue: nil,
			expected:     true,
		},
		// Test cases with false default
		{
			name:         "foobar unparseable with false default",
			version:      "foobar",
			minVersion:   "7.28.0",
			defaultValue: apiutils.NewBoolPointer(false),
			expected:     false,
			description:  "unparseable versions return false when defaultValue is false",
		},
		{
			name:         "main unparseable with false default",
			version:      "main",
			minVersion:   "7.99.0",
			defaultValue: apiutils.NewBoolPointer(false),
			expected:     false,
		},
		// Test cases with explicit true default
		{
			name:         "foobar with explicit true default",
			version:      "foobar",
			minVersion:   "7.28.0",
			defaultValue: apiutils.NewBoolPointer(true),
			expected:     true,
		},
		{
			name:         "valid version with explicit true default",
			version:      "7.30.0",
			minVersion:   "7.27.0",
			defaultValue: apiutils.NewBoolPointer(true),
			expected:     true,
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			result := IsAboveMinVersion(test.version, test.minVersion, test.defaultValue)
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
