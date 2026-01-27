// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha2

import (
	"encoding/json"
	"fmt"

	utilserrors "k8s.io/apimachinery/pkg/util/errors"
)

// ValidationError represents a detailed validation error
type ValidationError struct {
	Field   string   `json:"field"`
	Widget  int      `json:"widget,omitempty"`
	Message string   `json:"message"`
	Allowed []string `json:"allowed,omitempty"`
}

func (e *ValidationError) Error() string {
	if e.Widget >= 0 {
		return fmt.Sprintf("Widget %d: field '%s' %s", e.Widget, e.Field, e.Message)
	}
	return fmt.Sprintf("field '%s' %s", e.Field, e.Message)
}

// IsValidDatadogDashboard validates a DatadogDashboardSpec for v1alpha2
func IsValidDatadogDashboard(spec *DatadogDashboardSpec) error {
	var errs []error

	// Validate required fields
	if spec.Title == "" {
		errs = append(errs, &ValidationError{
			Field:   "title",
			Widget:  -1,
			Message: "is required",
		})
	}

	if spec.LayoutType == "" {
		errs = append(errs, &ValidationError{
			Field:   "layoutType",
			Widget:  -1,
			Message: "is required",
			Allowed: []string{"ordered", "free"},
		})
	} else {
		// Validate layoutType is one of the allowed values
		validLayoutTypes := []string{"ordered", "free"}
		isValid := false
		for _, validType := range validLayoutTypes {
			if string(spec.LayoutType) == validType {
				isValid = true
				break
			}
		}
		if !isValid {
			errs = append(errs, &ValidationError{
				Field:   "layoutType",
				Widget:  -1,
				Message: fmt.Sprintf("'%s' is not a valid layout type", spec.LayoutType),
				Allowed: validLayoutTypes,
			})
		}
	}

	// Validate widgets array
	if err := validateWidgets(spec.Widgets); err != nil {
		errs = append(errs, err)
	}

	return utilserrors.NewAggregate(errs)
}

// validateWidgets validates the widgets array structure and content
func validateWidgets(widgets []DashboardWidget) error {
	var errs []error

	for i, widget := range widgets {
		if err := validateWidget(widget, i); err != nil {
			errs = append(errs, err)
		}
	}

	return utilserrors.NewAggregate(errs)
}

// validateWidget validates a single widget
func validateWidget(widget DashboardWidget, index int) error {
	var errs []error

	// Validate that definition is not empty
	if len(widget.Definition.Raw) == 0 {
		errs = append(errs, &ValidationError{
			Field:   "definition",
			Widget:  index,
			Message: "is required and cannot be empty",
		})
		return utilserrors.NewAggregate(errs)
	}

	// Try to unmarshal the definition to validate it's valid JSON
	var definition map[string]interface{}
	if err := json.Unmarshal(widget.Definition.Raw, &definition); err != nil {
		errs = append(errs, &ValidationError{
			Field:   "definition",
			Widget:  index,
			Message: fmt.Sprintf("must be valid JSON: %v", err),
		})
		return utilserrors.NewAggregate(errs)
	}

	// Validate that type field exists in definition
	widgetType, exists := definition["type"]
	if !exists {
		errs = append(errs, &ValidationError{
			Field:   "definition.type",
			Widget:  index,
			Message: "is required",
			Allowed: getSupportedWidgetTypes(),
		})
	} else {
		// Validate widget type is supported
		if typeStr, ok := widgetType.(string); ok {
			if !isValidWidgetType(typeStr) {
				errs = append(errs, &ValidationError{
					Field:   "definition.type",
					Widget:  index,
					Message: fmt.Sprintf("'%s' is not a supported widget type", typeStr),
					Allowed: getSupportedWidgetTypes(),
				})
			}
		} else {
			errs = append(errs, &ValidationError{
				Field:   "definition.type",
				Widget:  index,
				Message: "must be a string",
				Allowed: getSupportedWidgetTypes(),
			})
		}
	}

	// Validate layout if present
	if widget.Layout != nil {
		if err := validateLayout(*widget.Layout, index); err != nil {
			errs = append(errs, err)
		}
	}

	return utilserrors.NewAggregate(errs)
}

// validateLayout validates widget layout
func validateLayout(layout WidgetLayout, widgetIndex int) error {
	var errs []error

	if layout.Width <= 0 {
		errs = append(errs, &ValidationError{
			Field:   "layout.width",
			Widget:  widgetIndex,
			Message: "must be greater than 0",
		})
	}

	if layout.Height <= 0 {
		errs = append(errs, &ValidationError{
			Field:   "layout.height",
			Widget:  widgetIndex,
			Message: "must be greater than 0",
		})
	}

	if layout.X < 0 {
		errs = append(errs, &ValidationError{
			Field:   "layout.x",
			Widget:  widgetIndex,
			Message: "must be greater than or equal to 0",
		})
	}

	if layout.Y < 0 {
		errs = append(errs, &ValidationError{
			Field:   "layout.y",
			Widget:  widgetIndex,
			Message: "must be greater than or equal to 0",
		})
	}

	return utilserrors.NewAggregate(errs)
}

// getSupportedWidgetTypes returns a list of supported widget types
func getSupportedWidgetTypes() []string {
	return []string{
		"alert_graph",
		"alert_value",
		"change",
		"check_status",
		"distribution",
		"event_stream",
		"event_timeline",
		"free_text",
		"funnel",
		"geomap",
		"heatmap",
		"hostmap",
		"iframe",
		"image",
		"log_stream",
		"manage_status",
		"note",
		"query_value",
		"scatter_plot",
		"servicemap",
		"service_summary",
		"sunburst",
		"table",
		"timeseries",
		"toplist",
		"trace_service",
		"treemap",
	}
}

// isValidWidgetType checks if a widget type is supported
func isValidWidgetType(widgetType string) bool {
	supportedTypes := getSupportedWidgetTypes()
	for _, supportedType := range supportedTypes {
		if widgetType == supportedType {
			return true
		}
	}
	return false
}

// RejectJSONStringWidgets validates that widgets field is not a JSON string (v1alpha1 format)
func RejectJSONStringWidgets(spec interface{}) error {
	// This function is called during validation to ensure v1alpha2 doesn't accept JSON strings
	// Since we're using []DashboardWidget type, JSON strings will be rejected at the API level
	// This function serves as documentation and can be used in tests
	return nil
}
