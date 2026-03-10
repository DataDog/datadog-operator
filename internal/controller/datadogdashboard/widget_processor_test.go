package datadogdashboard

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"
)

func TestV1Alpha1WidgetProcessor_ProcessWidgets(t *testing.T) {
	logger := logr.Discard()
	processor := NewV1Alpha1WidgetProcessor(logger)

	tests := []struct {
		name        string
		dashboard   *v1alpha1.DatadogDashboard
		expectError bool
		expectCount int
	}{
		{
			name: "empty widgets",
			dashboard: &v1alpha1.DatadogDashboard{
				Spec: v1alpha1.DatadogDashboardSpec{
					Widgets: "",
				},
			},
			expectError: false,
			expectCount: 0,
		},
		{
			name: "single widget",
			dashboard: &v1alpha1.DatadogDashboard{
				Spec: v1alpha1.DatadogDashboardSpec{
					Widgets: `[{"definition": {"type": "timeseries", "title": "Test"}}]`,
				},
			},
			expectError: false,
			expectCount: 1,
		},
		{
			name: "invalid JSON",
			dashboard: &v1alpha1.DatadogDashboard{
				Spec: v1alpha1.DatadogDashboardSpec{
					Widgets: `[{"definition": {"type": "timeseries", "title": "Test"}`,
				},
			},
			expectError: true,
			expectCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			widgets, err := processor.ProcessWidgets(tt.dashboard)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, widgets, tt.expectCount)
			}
		})
	}
}

func TestV1Alpha2WidgetProcessor_ProcessWidgets(t *testing.T) {
	logger := logr.Discard()
	processor := NewV1Alpha2WidgetProcessor(logger)

	tests := []struct {
		name        string
		dashboard   *v1alpha2.DatadogDashboard
		expectError bool
		expectCount int
	}{
		{
			name: "empty widgets",
			dashboard: &v1alpha2.DatadogDashboard{
				Spec: v1alpha2.DatadogDashboardSpec{
					Widgets: []v1alpha2.DashboardWidget{},
				},
			},
			expectError: false,
			expectCount: 0,
		},
		{
			name: "single widget with layout",
			dashboard: &v1alpha2.DatadogDashboard{
				Spec: v1alpha2.DatadogDashboardSpec{
					Widgets: []v1alpha2.DashboardWidget{
						{
							Definition: runtime.RawExtension{
								Raw: []byte(`{"type": "timeseries", "title": "Test Widget"}`),
							},
							Layout: &v1alpha2.WidgetLayout{
								X:      0,
								Y:      0,
								Width:  4,
								Height: 2,
							},
						},
					},
				},
			},
			expectError: false,
			expectCount: 1,
		},
		{
			name: "widget with ID",
			dashboard: &v1alpha2.DatadogDashboard{
				Spec: v1alpha2.DatadogDashboardSpec{
					Widgets: []v1alpha2.DashboardWidget{
						{
							ID: func() *int64 { id := int64(12345); return &id }(),
							Definition: runtime.RawExtension{
								Raw: []byte(`{"type": "query_value", "title": "Memory Usage"}`),
							},
						},
					},
				},
			},
			expectError: false,
			expectCount: 1,
		},
		{
			name: "invalid widget definition",
			dashboard: &v1alpha2.DatadogDashboard{
				Spec: v1alpha2.DatadogDashboardSpec{
					Widgets: []v1alpha2.DashboardWidget{
						{
							Definition: runtime.RawExtension{
								Raw: []byte(`{"type": "timeseries", "title": "Test"`),
							},
						},
					},
				},
			},
			expectError: true,
			expectCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			widgets, err := processor.ProcessWidgets(tt.dashboard)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, widgets, tt.expectCount)

				// Verify widget properties for successful cases
				if len(widgets) > 0 {
					widget := widgets[0]

					// Check if ID was set correctly
					if tt.dashboard.Spec.Widgets[0].ID != nil {
						assert.Equal(t, *tt.dashboard.Spec.Widgets[0].ID, widget.GetId())
					}

					// Check if definition was set
					assert.NotNil(t, widget.Definition)

					// Check if layout was set correctly
					if tt.dashboard.Spec.Widgets[0].Layout != nil {
						assert.NotNil(t, widget.Layout)
					}
				}
			}
		})
	}
}

func TestV1Alpha1WidgetProcessor_ValidateWidgets(t *testing.T) {
	logger := logr.Discard()
	processor := NewV1Alpha1WidgetProcessor(logger)

	tests := []struct {
		name        string
		dashboard   *v1alpha1.DatadogDashboard
		expectError bool
	}{
		{
			name: "valid dashboard",
			dashboard: &v1alpha1.DatadogDashboard{
				Spec: v1alpha1.DatadogDashboardSpec{
					Title:      "Test Dashboard",
					LayoutType: "ordered",
					Widgets:    `[{"definition": {"type": "timeseries"}}]`,
				},
			},
			expectError: false,
		},
		{
			name: "missing title",
			dashboard: &v1alpha1.DatadogDashboard{
				Spec: v1alpha1.DatadogDashboardSpec{
					LayoutType: "ordered",
					Widgets:    `[{"definition": {"type": "timeseries"}}]`,
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := processor.ValidateWidgets(tt.dashboard)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestV1Alpha2WidgetProcessor_ValidateWidgets(t *testing.T) {
	logger := logr.Discard()
	processor := NewV1Alpha2WidgetProcessor(logger)

	tests := []struct {
		name        string
		dashboard   *v1alpha2.DatadogDashboard
		expectError bool
	}{
		{
			name: "valid dashboard",
			dashboard: &v1alpha2.DatadogDashboard{
				Spec: v1alpha2.DatadogDashboardSpec{
					Title:      "Test Dashboard",
					LayoutType: "ordered",
					Widgets: []v1alpha2.DashboardWidget{
						{
							Definition: runtime.RawExtension{
								Raw: []byte(`{"type": "timeseries", "title": "Test"}`),
							},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "missing title",
			dashboard: &v1alpha2.DatadogDashboard{
				Spec: v1alpha2.DatadogDashboardSpec{
					LayoutType: "ordered",
					Widgets: []v1alpha2.DashboardWidget{
						{
							Definition: runtime.RawExtension{
								Raw: []byte(`{"type": "timeseries", "title": "Test"}`),
							},
						},
					},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := processor.ValidateWidgets(tt.dashboard)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetWidgetProcessor(t *testing.T) {
	logger := logr.Discard()

	tests := []struct {
		name            string
		dashboard       interface{}
		expectedVersion string
		expectError     bool
	}{
		{
			name:            "v1alpha1 dashboard",
			dashboard:       &v1alpha1.DatadogDashboard{},
			expectedVersion: "v1alpha1",
			expectError:     false,
		},
		{
			name:            "v1alpha2 dashboard",
			dashboard:       &v1alpha2.DatadogDashboard{},
			expectedVersion: "v1alpha2",
			expectError:     false,
		},
		{
			name:        "unsupported type",
			dashboard:   "invalid",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			processor, err := GetWidgetProcessor(tt.dashboard, logger)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, processor)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, processor)
				assert.Equal(t, tt.expectedVersion, processor.GetAPIVersion())
			}
		})
	}
}
