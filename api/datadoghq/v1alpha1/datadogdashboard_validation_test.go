// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	"testing"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/stretchr/testify/assert"
)

func TestIsValidDatadogDashboard(t *testing.T) {
	// Test cases are missing each of the required parameters
	minimumValid := &DatadogDashboardSpec{
		LayoutType: datadogV1.DASHBOARDLAYOUTTYPE_ORDERED,
		Title:      "test",
	}
	missingTitle := &DatadogDashboardSpec{
		LayoutType: datadogV1.DASHBOARDLAYOUTTYPE_ORDERED,
	}
	missingLayoutType := &DatadogDashboardSpec{
		Title: "test",
	}

	testCases := []struct {
		name    string
		spec    *DatadogDashboardSpec
		wantErr string
	}{
		{
			name: "minimum valid dashboard",
			spec: minimumValid,
		},
		{
			name:    "dashboard missing title",
			spec:    missingTitle,
			wantErr: "spec.Title must be defined",
		},
		{
			name:    "dashboard missing layout type",
			spec:    missingLayoutType,
			wantErr: "spec.LayoutType must be defined",
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			result := IsValidDatadogDashboard(test.spec)
			if test.wantErr != "" {
				assert.Error(t, result)
				assert.EqualError(t, result, test.wantErr)
			} else {
				assert.NoError(t, result)
			}
		})
	}
}
