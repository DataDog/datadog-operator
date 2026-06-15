// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetMax(t *testing.T) {
	// Val1 is bigger, val2 is bigger, or same

	testCases := []struct {
		name           string
		val1           int64
		val2           int64
		expectedResult int64
	}{
		{
			name:           "val1 is bigger",
			val1:           int64(100),
			val2:           int64(50),
			expectedResult: int64(100),
		},
		{
			name:           "val2 is bigger",
			val1:           int64(100),
			val2:           int64(150),
			expectedResult: int64(150),
		},
		{
			name:           "val1 is same as val2",
			val1:           int64(100),
			val2:           int64(100),
			expectedResult: int64(100),
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			result := GetMax(test.val1, test.val2)
			assert.Equal(t, test.expectedResult, result)
		})
	}
}

func TestGetTagFromImageName(t *testing.T) {
	testCases := []struct {
		name      string
		imageName string
		want      string
	}{
		{
			name:      "registry, name and tag",
			imageName: "gcr.io/datadoghq/agent:7.64.0",
			want:      "7.64.0",
		},
		{
			name:      "name and tag",
			imageName: "agent:7.64.0",
			want:      "7.64.0",
		},
		{
			name:      "no tag",
			imageName: "gcr.io/datadoghq/agent",
			want:      "latest",
		},
		{
			name:      "registry with port and no tag",
			imageName: "localhost:5000/agent",
			want:      "latest",
		},
		{
			name:      "tag and digest",
			imageName: "gcr.io/datadoghq/agent:7.64.0@sha256:e8370b31d516e2c878bc4af696d6d06484465e6311d506ecd2b5ebb42c1ec8a1",
			want:      "7.64.0",
		},
		{
			name:      "digest only",
			imageName: "gcr.io/datadoghq/agent@sha256:e8370b31d516e2c878bc4af696d6d06484465e6311d506ecd2b5ebb42c1ec8a1",
			want:      "latest",
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, GetTagFromImageName(test.imageName))
		})
	}
}
