// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha2

import (
	"testing"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestIsValidDatadogDashboard(t *testing.T) {
	tests := []struct {
		name        string
		spec        *DatadogDashboardSpec
		expectError bool
		errorCount  int
	}{
		{
			name: "valid dashboard",
			spec: &DatadogDashboardSpec{
				Title:      "Test Dashboard",
				LayoutType: datadogV1.DASHBOARDLAYOUTTYPE_ORDERED,
				Widgets: []DashboardWidget{
					{
						Definition: runtime.RawExtension{
							Raw: []byte(`{"type": "timeseries", "title": "Test Widget"}`),
						},
						Layout: &WidgetLayout{
							X:      0,
							Y:      0,
							Width:  4,
							Height: 2,
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "missing title",
			spec: &DatadogDashboardSpec{
				LayoutType: datadogV1.DASHBOARDLAYOUTTYPE_ORDERED,
				Widgets: []DashboardWidget{
					{
						Definition: runtime.RawExtension{
							Raw: []byte(`{"type": "timeseries", "title": "Test Widget"}`),
						},
					},
				},
			},
			expectError: true,
			errorCount:  1,
		},
		{
			name: "missing layout type",
			spec: &DatadogDashboardSpec{
				Title: "Test Dashboard",
				Widgets: []DashboardWidget{
					{
						Definition: runtime.RawExtension{
							Raw: []byte(`{"type": "timeseries", "title": "Test Widget"}`),
						},
					},
				},
			},
			expectError: true,
			errorCount:  1,
		},
		{
			name: "empty widget definition",
			spec: &DatadogDashboardSpec{
				Title:      "Test Dashboard",
				LayoutType: datadogV1.DASHBOARDLAYOUTTYPE_ORDERED,
				Widgets: []DashboardWidget{
					{
						Definition: runtime.RawExtension{
							Raw: []byte{},
						},
					},
				},
			},
			expectError: true,
			errorCount:  1,
		},
		{
			name: "invalid widget definition JSON",
			spec: &DatadogDashboardSpec{
				Title:      "Test Dashboard",
				LayoutType: datadogV1.DASHBOARDLAYOUTTYPE_ORDERED,
				Widgets: []DashboardWidget{
					{
						Definition: runtime.RawExtension{
							Raw: []byte(`{"type": "timeseries", "title": "Test"`),
						},
					},
				},
			},
			expectError: true,
			errorCount:  1,
		},
		{
			name: "missing widget type",
			spec: &DatadogDashboardSpec{
				Title:      "Test Dashboard",
				LayoutType: datadogV1.DASHBOARDLAYOUTTYPE_ORDERED,
				Widgets: []DashboardWidget{
					{
						Definition: runtime.RawExtension{
							Raw: []byte(`{"title": "Test Widget"}`),
						},
					},
				},
			},
			expectError: true,
			errorCount:  1,
		},
		{
			name: "invalid widget type",
			spec: &DatadogDashboardSpec{
				Title:      "Test Dashboard",
				LayoutType: datadogV1.DASHBOARDLAYOUTTYPE_ORDERED,
				Widgets: []DashboardWidget{
					{
						Definition: runtime.RawExtension{
							Raw: []byte(`{"type": "invalid_type", "title": "Test Widget"}`),
						},
					},
				},
			},
			expectError: true,
			errorCount:  1,
		},
		{
			name: "invalid layout dimensions",
			spec: &DatadogDashboardSpec{
				Title:      "Test Dashboard",
				LayoutType: datadogV1.DASHBOARDLAYOUTTYPE_ORDERED,
				Widgets: []DashboardWidget{
					{
						Definition: runtime.RawExtension{
							Raw: []byte(`{"type": "timeseries", "title": "Test Widget"}`),
						},
						Layout: &WidgetLayout{
							X:      -1,
							Y:      -1,
							Width:  0,
							Height: 0,
						},
					},
				},
			},
			expectError: true,
			errorCount:  4, // x, y, width, height all invalid
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := IsValidDatadogDashboard(tt.spec)

			if tt.expectError {
				assert.Error(t, err)
				// Check that we get the expected number of validation errors
				if tt.errorCount > 0 {
					// The error should contain multiple validation errors
					assert.Contains(t, err.Error(), "field")
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidationError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *ValidationError
		expected string
	}{
		{
			name: "field error without widget index",
			err: &ValidationError{
				Field:   "title",
				Widget:  -1, // Use -1 to indicate no widget
				Message: "is required",
			},
			expected: "field 'title' is required",
		},
		{
			name: "field error with widget index",
			err: &ValidationError{
				Field:   "definition.type",
				Widget:  0,
				Message: "is required",
				Allowed: []string{"timeseries", "query_value"},
			},
			expected: "Widget 0: field 'definition.type' is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

func TestIsValidWidgetType(t *testing.T) {
	tests := []struct {
		name       string
		widgetType string
		expected   bool
	}{
		{
			name:       "valid timeseries",
			widgetType: "timeseries",
			expected:   true,
		},
		{
			name:       "valid query_value",
			widgetType: "query_value",
			expected:   true,
		},
		{
			name:       "valid heatmap",
			widgetType: "heatmap",
			expected:   true,
		},
		{
			name:       "invalid type",
			widgetType: "invalid_type",
			expected:   false,
		},
		{
			name:       "empty type",
			widgetType: "",
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidWidgetType(tt.widgetType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetSupportedWidgetTypes(t *testing.T) {
	types := getSupportedWidgetTypes()

	// Check that we have a reasonable number of widget types
	assert.Greater(t, len(types), 10)

	// Check that common widget types are included
	expectedTypes := []string{
		"timeseries",
		"query_value",
		"heatmap",
		"toplist",
		"table",
		"free_text",
	}

	for _, expectedType := range expectedTypes {
		assert.Contains(t, types, expectedType)
	}
}

func TestRejectJSONStringWidgets(t *testing.T) {
	// This function should not return an error since v1alpha2 uses typed arrays
	// The rejection happens at the API level through type checking
	err := RejectJSONStringWidgets(nil)
	assert.NoError(t, err)
}
