package datadogdashboard

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"
)

// Property 1: YAML Array Widget Acceptance
// Validates: Requirements 1.1, 1.3, 2.2
func TestProperty_YAMLArrayWidgetAcceptance(t *testing.T) {
	tests := []struct {
		name        string
		widgets     []v1alpha2.DashboardWidget
		expectValid bool
		description string
	}{
		{
			name:        "empty_widgets_array",
			widgets:     []v1alpha2.DashboardWidget{},
			expectValid: true,
			description: "Empty widgets array should be accepted",
		},
		{
			name: "single_valid_widget",
			widgets: []v1alpha2.DashboardWidget{
				{
					Definition: runtime.RawExtension{
						Raw: []byte(`{"type": "timeseries", "title": "CPU Usage"}`),
					},
					Layout: &v1alpha2.WidgetLayout{
						X: 0, Y: 0, Width: 4, Height: 2,
					},
				},
			},
			expectValid: true,
			description: "Single valid widget should be accepted",
		},
		{
			name: "multiple_valid_widgets",
			widgets: []v1alpha2.DashboardWidget{
				{
					Definition: runtime.RawExtension{
						Raw: []byte(`{"type": "timeseries", "title": "CPU Usage"}`),
					},
					Layout: &v1alpha2.WidgetLayout{X: 0, Y: 0, Width: 4, Height: 2},
				},
				{
					Definition: runtime.RawExtension{
						Raw: []byte(`{"type": "query_value", "title": "Memory Usage"}`),
					},
					Layout: &v1alpha2.WidgetLayout{X: 4, Y: 0, Width: 4, Height: 2},
				},
			},
			expectValid: true,
			description: "Multiple valid widgets should be accepted",
		},
		{
			name: "widget_without_layout",
			widgets: []v1alpha2.DashboardWidget{
				{
					Definition: runtime.RawExtension{
						Raw: []byte(`{"type": "timeseries", "title": "CPU Usage"}`),
					},
				},
			},
			expectValid: true,
			description: "Widget without layout should be accepted (layout is optional)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dashboard := &v1alpha2.DatadogDashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dashboard",
					Namespace: "default",
				},
				Spec: v1alpha2.DatadogDashboardSpec{
					Title:      "Test Dashboard",
					LayoutType: "ordered",
					Widgets:    tt.widgets,
				},
			}

			processor := NewV1Alpha2WidgetProcessor(testLogger)

			// Test widget processing
			widgets, err := processor.ProcessWidgets(dashboard)
			if tt.expectValid {
				assert.NoError(t, err, tt.description)
				assert.Len(t, widgets, len(tt.widgets), "Processed widgets count should match input")
			} else {
				assert.Error(t, err, tt.description)
			}

			// Test validation
			err = processor.ValidateWidgets(dashboard)
			if tt.expectValid {
				assert.NoError(t, err, tt.description)
			} else {
				assert.Error(t, err, tt.description)
			}
		})
	}
}

// Property 2: Widget Validation Completeness
// Validates: Requirements 1.2, 2.3, 5.1, 5.2, 5.3, 5.4, 6.4
func TestProperty_WidgetValidationCompleteness(t *testing.T) {
	tests := []struct {
		name        string
		spec        v1alpha2.DatadogDashboardSpec
		expectValid bool
		description string
	}{
		{
			name: "missing_title",
			spec: v1alpha2.DatadogDashboardSpec{
				LayoutType: "ordered",
				Widgets: []v1alpha2.DashboardWidget{
					{
						Definition: runtime.RawExtension{
							Raw: []byte(`{"type": "timeseries", "title": "Test"}`),
						},
					},
				},
			},
			expectValid: false,
			description: "Dashboard without title should fail validation",
		},
		{
			name: "missing_layout_type",
			spec: v1alpha2.DatadogDashboardSpec{
				Title: "Test Dashboard",
				Widgets: []v1alpha2.DashboardWidget{
					{
						Definition: runtime.RawExtension{
							Raw: []byte(`{"type": "timeseries", "title": "Test"}`),
						},
					},
				},
			},
			expectValid: false,
			description: "Dashboard without layoutType should fail validation",
		},
		{
			name: "invalid_widget_definition",
			spec: v1alpha2.DatadogDashboardSpec{
				Title:      "Test Dashboard",
				LayoutType: "ordered",
				Widgets: []v1alpha2.DashboardWidget{
					{
						Definition: runtime.RawExtension{
							Raw: []byte(`invalid json`),
						},
					},
				},
			},
			expectValid: false,
			description: "Widget with invalid JSON definition should fail validation",
		},
		{
			name: "widget_without_type",
			spec: v1alpha2.DatadogDashboardSpec{
				Title:      "Test Dashboard",
				LayoutType: "ordered",
				Widgets: []v1alpha2.DashboardWidget{
					{
						Definition: runtime.RawExtension{
							Raw: []byte(`{"title": "Test Widget"}`),
						},
					},
				},
			},
			expectValid: false,
			description: "Widget without type should fail validation",
		},
		{
			name: "invalid_widget_type",
			spec: v1alpha2.DatadogDashboardSpec{
				Title:      "Test Dashboard",
				LayoutType: "ordered",
				Widgets: []v1alpha2.DashboardWidget{
					{
						Definition: runtime.RawExtension{
							Raw: []byte(`{"type": "invalid_type", "title": "Test"}`),
						},
					},
				},
			},
			expectValid: false,
			description: "Widget with invalid type should fail validation",
		},
		{
			name: "invalid_layout_dimensions",
			spec: v1alpha2.DatadogDashboardSpec{
				Title:      "Test Dashboard",
				LayoutType: "ordered",
				Widgets: []v1alpha2.DashboardWidget{
					{
						Definition: runtime.RawExtension{
							Raw: []byte(`{"type": "timeseries", "title": "Test"}`),
						},
						Layout: &v1alpha2.WidgetLayout{
							X: -1, Y: -1, Width: 0, Height: 0,
						},
					},
				},
			},
			expectValid: false,
			description: "Widget with invalid layout dimensions should fail validation",
		},
		{
			name: "valid_complete_dashboard",
			spec: v1alpha2.DatadogDashboardSpec{
				Title:       "Test Dashboard",
				LayoutType:  "ordered",
				Description: "A test dashboard",
				Widgets: []v1alpha2.DashboardWidget{
					{
						Definition: runtime.RawExtension{
							Raw: []byte(`{"type": "timeseries", "title": "CPU Usage"}`),
						},
						Layout: &v1alpha2.WidgetLayout{
							X: 0, Y: 0, Width: 4, Height: 2,
						},
					},
				},
			},
			expectValid: true,
			description: "Valid complete dashboard should pass validation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v1alpha2.IsValidDatadogDashboard(&tt.spec)
			if tt.expectValid {
				assert.NoError(t, err, tt.description)
			} else {
				assert.Error(t, err, tt.description)
			}
		})
	}
}

// Property 3: YAML Array Round-trip Consistency
// Validates: Requirements 1.4
func TestProperty_YAMLArrayRoundTripConsistency(t *testing.T) {
	originalWidgets := []v1alpha2.DashboardWidget{
		{
			ID: func() *int64 { id := int64(123); return &id }(),
			Definition: runtime.RawExtension{
				Raw: []byte(`{"type": "timeseries", "title": "CPU Usage", "requests": [{"q": "avg:system.cpu.user{*}"}]}`),
			},
			Layout: &v1alpha2.WidgetLayout{
				X: 0, Y: 0, Width: 4, Height: 2,
			},
		},
		{
			Definition: runtime.RawExtension{
				Raw: []byte(`{"type": "query_value", "title": "Memory Usage", "requests": [{"q": "avg:system.mem.used{*}"}]}`),
			},
			Layout: &v1alpha2.WidgetLayout{
				X: 4, Y: 0, Width: 4, Height: 2,
			},
		},
	}

	dashboard := &v1alpha2.DatadogDashboard{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dashboard",
			Namespace: "default",
		},
		Spec: v1alpha2.DatadogDashboardSpec{
			Title:      "Test Dashboard",
			LayoutType: "ordered",
			Widgets:    originalWidgets,
		},
	}

	processor := NewV1Alpha2WidgetProcessor(testLogger)

	// Process widgets to Datadog format
	processedWidgets, err := processor.ProcessWidgets(dashboard)
	require.NoError(t, err, "Processing widgets should not fail")

	// Verify the processed widgets maintain the essential properties
	assert.Len(t, processedWidgets, len(originalWidgets), "Processed widgets count should match original")

	for i, processedWidget := range processedWidgets {
		originalWidget := originalWidgets[i]

		// Check ID preservation
		if originalWidget.ID != nil {
			assert.True(t, processedWidget.HasId(), "Widget ID should be preserved")
			assert.Equal(t, *originalWidget.ID, processedWidget.GetId(), "Widget ID should match")
		}

		// Check definition preservation
		definition := processedWidget.GetDefinition()
		assert.NotNil(t, definition, "Widget should have definition")

		// Check layout preservation
		if originalWidget.Layout != nil {
			assert.True(t, processedWidget.HasLayout(), "Widget should have layout")
			layout := processedWidget.GetLayout()
			assert.Equal(t, int64(originalWidget.Layout.X), layout.GetX(), "Layout X should match")
			assert.Equal(t, int64(originalWidget.Layout.Y), layout.GetY(), "Layout Y should match")
			assert.Equal(t, int64(originalWidget.Layout.Width), layout.GetWidth(), "Layout Width should match")
			assert.Equal(t, int64(originalWidget.Layout.Height), layout.GetHeight(), "Layout Height should match")
		}
	}
}

// Property 4: JSON String Rejection
// Validates: Requirements 2.1
func TestProperty_JSONStringRejection(t *testing.T) {
	// Test that v1alpha2 API rejects JSON string format (compile-time check)
	// This is enforced by the type system - []DashboardWidget cannot accept string

	// Verify that attempting to use JSON string in v1alpha2 fails at unmarshaling level
	jsonString := `{"spec": {"title": "Test", "layoutType": "ordered", "widgets": "[{\"definition\": {\"type\": \"timeseries\"}}]"}}`

	var dashboard v1alpha2.DatadogDashboard
	err := json.Unmarshal([]byte(jsonString), &dashboard)

	// This should fail because widgets expects an array, not a string
	assert.Error(t, err, "v1alpha2 should reject JSON string format for widgets")
	assert.Contains(t, err.Error(), "cannot unmarshal string into Go struct field", "Error should indicate type mismatch")
}

// Property 5: Widget Type Support
// Validates: Requirements 1.5
func TestProperty_WidgetTypeSupport(t *testing.T) {
	supportedTypes := []string{
		"alert_graph", "alert_value", "change", "check_status", "distribution",
		"event_stream", "event_timeline", "free_text", "funnel", "geomap",
		"heatmap", "hostmap", "iframe", "image", "log_stream", "manage_status",
		"note", "query_value", "scatter_plot", "servicemap", "service_summary",
		"sunburst", "table", "timeseries", "toplist", "trace_service", "treemap",
	}

	for _, widgetType := range supportedTypes {
		t.Run(fmt.Sprintf("widget_type_%s", widgetType), func(t *testing.T) {
			widget := v1alpha2.DashboardWidget{
				Definition: runtime.RawExtension{
					Raw: []byte(fmt.Sprintf(`{"type": "%s", "title": "Test Widget"}`, widgetType)),
				},
				Layout: &v1alpha2.WidgetLayout{
					X: 0, Y: 0, Width: 4, Height: 2,
				},
			}

			dashboard := &v1alpha2.DatadogDashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dashboard",
					Namespace: "default",
				},
				Spec: v1alpha2.DatadogDashboardSpec{
					Title:      "Test Dashboard",
					LayoutType: "ordered",
					Widgets:    []v1alpha2.DashboardWidget{widget},
				},
			}

			processor := NewV1Alpha2WidgetProcessor(testLogger)

			// Test validation accepts the widget type
			err := processor.ValidateWidgets(dashboard)
			assert.NoError(t, err, "Supported widget type %s should pass validation", widgetType)

			// Test processing works for the widget type
			widgets, err := processor.ProcessWidgets(dashboard)
			assert.NoError(t, err, "Supported widget type %s should process successfully", widgetType)
			assert.Len(t, widgets, 1, "Should process exactly one widget")
		})
	}

	// Test unsupported widget type
	t.Run("unsupported_widget_type", func(t *testing.T) {
		widget := v1alpha2.DashboardWidget{
			Definition: runtime.RawExtension{
				Raw: []byte(`{"type": "unsupported_type", "title": "Test Widget"}`),
			},
		}

		dashboard := &v1alpha2.DatadogDashboard{
			Spec: v1alpha2.DatadogDashboardSpec{
				Title:      "Test Dashboard",
				LayoutType: "ordered",
				Widgets:    []v1alpha2.DashboardWidget{widget},
			},
		}

		processor := NewV1Alpha2WidgetProcessor(testLogger)
		err := processor.ValidateWidgets(dashboard)
		assert.Error(t, err, "Unsupported widget type should fail validation")
		assert.Contains(t, err.Error(), "unsupported_type", "Error should mention the unsupported type")
	})
}

// Property 6: Controller Processing Consistency
// Validates: Requirements 6.1, 6.2, 6.3, 6.5
func TestProperty_ControllerProcessingConsistency(t *testing.T) {
	// Test that both v1alpha1 and v1alpha2 processors produce consistent results
	// for equivalent widget configurations

	// Create equivalent widgets in both formats
	v1alpha1Dashboard := &v1alpha1.DatadogDashboard{
		Spec: v1alpha1.DatadogDashboardSpec{
			Title:      "Test Dashboard",
			LayoutType: "ordered",
			Widgets:    `[{"definition": {"type": "timeseries", "title": "CPU Usage"}, "layout": {"x": 0, "y": 0, "width": 4, "height": 2}}]`,
		},
	}

	v1alpha2Dashboard := &v1alpha2.DatadogDashboard{
		Spec: v1alpha2.DatadogDashboardSpec{
			Title:      "Test Dashboard",
			LayoutType: "ordered",
			Widgets: []v1alpha2.DashboardWidget{
				{
					Definition: runtime.RawExtension{
						Raw: []byte(`{"type": "timeseries", "title": "CPU Usage"}`),
					},
					Layout: &v1alpha2.WidgetLayout{
						X: 0, Y: 0, Width: 4, Height: 2,
					},
				},
			},
		},
	}

	// Process with both processors
	v1alpha1Processor := NewV1Alpha1WidgetProcessor(testLogger)
	v1alpha2Processor := NewV1Alpha2WidgetProcessor(testLogger)

	v1alpha1Widgets, err := v1alpha1Processor.ProcessWidgets(v1alpha1Dashboard)
	require.NoError(t, err, "v1alpha1 processing should succeed")

	v1alpha2Widgets, err := v1alpha2Processor.ProcessWidgets(v1alpha2Dashboard)
	require.NoError(t, err, "v1alpha2 processing should succeed")

	// Verify consistency
	assert.Len(t, v1alpha1Widgets, len(v1alpha2Widgets), "Both processors should produce same number of widgets")

	for i := range v1alpha1Widgets {
		v1Widget := v1alpha1Widgets[i]
		v2Widget := v1alpha2Widgets[i]

		// Both should have definitions
		v1Definition := v1Widget.GetDefinition()
		v2Definition := v2Widget.GetDefinition()
		assert.NotNil(t, v1Definition, "v1alpha1 widget should have definition")
		assert.NotNil(t, v2Definition, "v1alpha2 widget should have definition")

		// Both should have layouts
		assert.True(t, v1Widget.HasLayout(), "v1alpha1 widget should have layout")
		assert.True(t, v2Widget.HasLayout(), "v1alpha2 widget should have layout")

		// Layout values should match
		v1Layout := v1Widget.GetLayout()
		v2Layout := v2Widget.GetLayout()
		assert.Equal(t, v1Layout.GetX(), v2Layout.GetX(), "Layout X should match")
		assert.Equal(t, v1Layout.GetY(), v2Layout.GetY(), "Layout Y should match")
		assert.Equal(t, v1Layout.GetWidth(), v2Layout.GetWidth(), "Layout Width should match")
		assert.Equal(t, v1Layout.GetHeight(), v2Layout.GetHeight(), "Layout Height should match")
	}
}

// Property 7: CRD Schema Generation
// Validates: Requirements 3.4
func TestProperty_CRDSchemaGeneration(t *testing.T) {
	// Test that the CRD schema properly validates v1alpha2 resources
	// This is tested through the validation functions

	validSpecs := []v1alpha2.DatadogDashboardSpec{
		{
			Title:      "Valid Dashboard 1",
			LayoutType: "ordered",
			Widgets: []v1alpha2.DashboardWidget{
				{
					Definition: runtime.RawExtension{
						Raw: []byte(`{"type": "timeseries", "title": "Test"}`),
					},
				},
			},
		},
		{
			Title:       "Valid Dashboard 2",
			LayoutType:  "free",
			Description: "A test dashboard",
			Widgets: []v1alpha2.DashboardWidget{
				{
					Definition: runtime.RawExtension{
						Raw: []byte(`{"type": "query_value", "title": "Test"}`),
					},
					Layout: &v1alpha2.WidgetLayout{X: 0, Y: 0, Width: 4, Height: 2},
				},
			},
		},
	}

	invalidSpecs := []v1alpha2.DatadogDashboardSpec{
		{
			// Missing title
			LayoutType: "ordered",
		},
		{
			Title: "Invalid Dashboard",
			// Missing layoutType
		},
		{
			Title:      "Invalid Dashboard",
			LayoutType: "invalid_layout",
		},
	}

	// Test valid specs pass validation
	for i, spec := range validSpecs {
		t.Run(fmt.Sprintf("valid_spec_%d", i), func(t *testing.T) {
			err := v1alpha2.IsValidDatadogDashboard(&spec)
			assert.NoError(t, err, "Valid spec should pass validation")
		})
	}

	// Test invalid specs fail validation
	for i, spec := range invalidSpecs {
		t.Run(fmt.Sprintf("invalid_spec_%d", i), func(t *testing.T) {
			err := v1alpha2.IsValidDatadogDashboard(&spec)
			assert.Error(t, err, "Invalid spec should fail validation")
		})
	}
}

// Property 8: Error Handling Robustness
// Validates: Requirements 3.5
func TestProperty_ErrorHandlingRobustness(t *testing.T) {
	processor := NewV1Alpha2WidgetProcessor(testLogger)

	errorCases := []struct {
		name        string
		dashboard   interface{}
		expectError bool
		description string
	}{
		{
			name:        "nil_dashboard",
			dashboard:   nil,
			expectError: true,
			description: "Nil dashboard should be handled gracefully",
		},
		{
			name:        "wrong_type_dashboard",
			dashboard:   "not a dashboard",
			expectError: true,
			description: "Wrong type should be handled gracefully",
		},
		{
			name: "dashboard_with_malformed_widget",
			dashboard: &v1alpha2.DatadogDashboard{
				Spec: v1alpha2.DatadogDashboardSpec{
					Title:      "Test Dashboard",
					LayoutType: "ordered",
					Widgets: []v1alpha2.DashboardWidget{
						{
							Definition: runtime.RawExtension{
								Raw: []byte(`{malformed json`),
							},
						},
					},
				},
			},
			expectError: true,
			description: "Malformed widget JSON should be handled gracefully",
		},
		{
			name: "dashboard_with_empty_widget_definition",
			dashboard: &v1alpha2.DatadogDashboard{
				Spec: v1alpha2.DatadogDashboardSpec{
					Title:      "Test Dashboard",
					LayoutType: "ordered",
					Widgets: []v1alpha2.DashboardWidget{
						{
							Definition: runtime.RawExtension{
								Raw: []byte{},
							},
						},
					},
				},
			},
			expectError: true,
			description: "Empty widget definition should be handled gracefully",
		},
	}

	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test ProcessWidgets error handling
			_, err := processor.ProcessWidgets(tc.dashboard)
			if tc.expectError {
				assert.Error(t, err, tc.description)
				assert.NotEmpty(t, err.Error(), "Error message should not be empty")
			} else {
				assert.NoError(t, err, tc.description)
			}

			// Test ValidateWidgets error handling
			err = processor.ValidateWidgets(tc.dashboard)
			if tc.expectError {
				assert.Error(t, err, tc.description)
				assert.NotEmpty(t, err.Error(), "Error message should not be empty")
			} else {
				assert.NoError(t, err, tc.description)
			}
		})
	}
}
