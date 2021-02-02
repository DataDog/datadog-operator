// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidDatadogMonitor(t *testing.T) {
	// Test cases are missing each of the required parameters
	minimumValid := &DatadogMonitorSpec{
		Query:   "avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.05",
		Type:    "metric alert",
		Name:    "Test Monitor",
		Message: "Something is wrong",
	}
	missingQuery := &DatadogMonitorSpec{
		Type:    "metric alert",
		Name:    "Test Monitor",
		Message: "Something is wrong",
	}
	missingType := &DatadogMonitorSpec{
		Query:   "avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.05",
		Name:    "Test Monitor",
		Message: "Something is wrong",
	}
	missingName := &DatadogMonitorSpec{
		Query:   "avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.05",
		Type:    "metric alert",
		Message: "Something is wrong",
	}
	missingMessage := &DatadogMonitorSpec{
		Query: "avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.05",
		Type:  "metric alert",
		Name:  "Test Monitor",
	}

	testCases := []struct {
		name  string
		spec  *DatadogMonitorSpec
		isErr bool
	}{
		{
			name:  "minimum valid monitor",
			spec:  minimumValid,
			isErr: false,
		},
		{
			name:  "monitor missing query",
			spec:  missingQuery,
			isErr: true,
		},
		{
			name:  "monitor missing type",
			spec:  missingType,
			isErr: true,
		},
		{
			name:  "monitor missing name",
			spec:  missingName,
			isErr: true,
		},
		{
			name:  "monitor missing message",
			spec:  missingMessage,
			isErr: true,
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			result := IsValidDatadogMonitor(test.spec)
			if test.isErr {
				assert.Error(t, result)
			} else {
				assert.NoError(t, result)
			}
		})
	}
}
