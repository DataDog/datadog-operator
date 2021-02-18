// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

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
