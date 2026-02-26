package datadogdashboard

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/go-logr/logr"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"
)

// WidgetProcessor defines the interface for processing widgets from different API versions
type WidgetProcessor interface {
	ProcessWidgets(dashboard interface{}) ([]datadogV1.Widget, error)
	ValidateWidgets(dashboard interface{}) error
	GetAPIVersion() string
}

// V1Alpha1WidgetProcessor processes widgets from v1alpha1 DatadogDashboard
type V1Alpha1WidgetProcessor struct {
	logger logr.Logger
}

// NewV1Alpha1WidgetProcessor creates a new V1Alpha1WidgetProcessor
func NewV1Alpha1WidgetProcessor(logger logr.Logger) *V1Alpha1WidgetProcessor {
	return &V1Alpha1WidgetProcessor{
		logger: logger,
	}
}

// ProcessWidgets processes widgets from v1alpha1 format (JSON string)
func (p *V1Alpha1WidgetProcessor) ProcessWidgets(dashboard interface{}) ([]datadogV1.Widget, error) {
	ddb, ok := dashboard.(*v1alpha1.DatadogDashboard)
	if !ok {
		return nil, fmt.Errorf("expected v1alpha1.DatadogDashboard, got %T", dashboard)
	}

	widgetList := &[]datadogV1.Widget{}
	if ddb.Spec.Widgets != "" {
		if err := json.Unmarshal([]byte(ddb.Spec.Widgets), widgetList); err != nil {
			return nil, fmt.Errorf("failed to unmarshal v1alpha1 widgets JSON: %w", err)
		}
	}

	return *widgetList, nil
}

// ValidateWidgets validates widgets for v1alpha1
func (p *V1Alpha1WidgetProcessor) ValidateWidgets(dashboard interface{}) error {
	ddb, ok := dashboard.(*v1alpha1.DatadogDashboard)
	if !ok {
		return fmt.Errorf("expected v1alpha1.DatadogDashboard, got %T", dashboard)
	}

	return v1alpha1.IsValidDatadogDashboard(&ddb.Spec)
}

// GetAPIVersion returns the API version
func (p *V1Alpha1WidgetProcessor) GetAPIVersion() string {
	return "v1alpha1"
}

// V1Alpha2WidgetProcessor processes widgets from v1alpha2 DatadogDashboard
type V1Alpha2WidgetProcessor struct {
	logger logr.Logger
}

// NewV1Alpha2WidgetProcessor creates a new V1Alpha2WidgetProcessor
func NewV1Alpha2WidgetProcessor(logger logr.Logger) *V1Alpha2WidgetProcessor {
	return &V1Alpha2WidgetProcessor{
		logger: logger,
	}
}

// ProcessWidgets processes widgets from v1alpha2 format (YAML array)
func (p *V1Alpha2WidgetProcessor) ProcessWidgets(dashboard interface{}) ([]datadogV1.Widget, error) {
	ddb, ok := dashboard.(*v1alpha2.DatadogDashboard)
	if !ok {
		return nil, fmt.Errorf("expected v1alpha2.DatadogDashboard, got %T", dashboard)
	}

	widgets := make([]datadogV1.Widget, 0, len(ddb.Spec.Widgets))

	for i, widget := range ddb.Spec.Widgets {
		// Convert the runtime.RawExtension to a datadogV1.Widget
		var definition map[string]interface{}
		if err := json.Unmarshal(widget.Definition.Raw, &definition); err != nil {
			return nil, fmt.Errorf("failed to unmarshal widget %d definition: %w", i, err)
		}

		// Create a datadogV1.Widget
		ddWidget := datadogV1.Widget{}

		// Set the ID if provided
		if widget.ID != nil {
			ddWidget.SetId(*widget.ID)
		}

		// Set the definition - convert to proper type
		var widgetDef datadogV1.WidgetDefinition
		defBytes, err := json.Marshal(definition)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal widget %d definition: %w", i, err)
		}
		if err := json.Unmarshal(defBytes, &widgetDef); err != nil {
			return nil, fmt.Errorf("failed to unmarshal widget %d definition to WidgetDefinition: %w", i, err)
		}
		ddWidget.SetDefinition(widgetDef)

		// Set the layout if provided
		if widget.Layout != nil {
			var widgetLayout datadogV1.WidgetLayout
			layoutBytes, err := json.Marshal(map[string]interface{}{
				"x":      widget.Layout.X,
				"y":      widget.Layout.Y,
				"width":  widget.Layout.Width,
				"height": widget.Layout.Height,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to marshal widget %d layout: %w", i, err)
			}
			if err := json.Unmarshal(layoutBytes, &widgetLayout); err != nil {
				return nil, fmt.Errorf("failed to unmarshal widget %d layout to WidgetLayout: %w", i, err)
			}
			ddWidget.SetLayout(widgetLayout)
		}

		widgets = append(widgets, ddWidget)
	}

	return widgets, nil
}

// ValidateWidgets validates widgets for v1alpha2
func (p *V1Alpha2WidgetProcessor) ValidateWidgets(dashboard interface{}) error {
	ddb, ok := dashboard.(*v1alpha2.DatadogDashboard)
	if !ok {
		return fmt.Errorf("expected v1alpha2.DatadogDashboard, got %T", dashboard)
	}

	return v1alpha2.IsValidDatadogDashboard(&ddb.Spec)
}

// GetAPIVersion returns the API version
func (p *V1Alpha2WidgetProcessor) GetAPIVersion() string {
	return "v1alpha2"
}

// GetWidgetProcessor returns the appropriate widget processor for the given dashboard
func GetWidgetProcessor(dashboard interface{}, logger logr.Logger) (WidgetProcessor, error) {
	switch dashboard.(type) {
	case *v1alpha1.DatadogDashboard:
		return NewV1Alpha1WidgetProcessor(logger), nil
	case *v1alpha2.DatadogDashboard:
		return NewV1Alpha2WidgetProcessor(logger), nil
	default:
		return nil, fmt.Errorf("unsupported dashboard type: %T", dashboard)
	}
}
