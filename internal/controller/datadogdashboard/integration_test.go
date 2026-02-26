package datadogdashboard

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"
)

func TestIntegration_V1Alpha1_V1Alpha2_Coexistence(t *testing.T) {
	// Test that both v1alpha1 and v1alpha2 resources can coexist in the same cluster
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	require.NoError(t, v1alpha2.AddToScheme(scheme))

	// Create fake client with both API versions
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	ctx := context.Background()

	// Create a v1alpha1 dashboard
	v1alpha1Dashboard := &v1alpha1.DatadogDashboard{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dashboard-v1alpha1",
			Namespace: "default",
		},
		Spec: v1alpha1.DatadogDashboardSpec{
			Title:      "Test Dashboard v1alpha1",
			LayoutType: "ordered",
			Widgets:    `[{"definition": {"type": "timeseries", "title": "CPU Usage"}}]`,
		},
	}

	err := fakeClient.Create(ctx, v1alpha1Dashboard)
	require.NoError(t, err)

	// Create a v1alpha2 dashboard
	v1alpha2Dashboard := &v1alpha2.DatadogDashboard{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dashboard-v1alpha2",
			Namespace: "default",
		},
		Spec: v1alpha2.DatadogDashboardSpec{
			Title:      "Test Dashboard v1alpha2",
			LayoutType: "ordered",
			Widgets: []v1alpha2.DashboardWidget{
				{
					Definition: runtime.RawExtension{
						Raw: []byte(`{"type": "timeseries", "title": "CPU Usage"}`),
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
	}

	err = fakeClient.Create(ctx, v1alpha2Dashboard)
	require.NoError(t, err)

	// Verify both resources exist and can be retrieved
	var retrievedV1Alpha1 v1alpha1.DatadogDashboard
	err = fakeClient.Get(ctx, types.NamespacedName{
		Name:      "test-dashboard-v1alpha1",
		Namespace: "default",
	}, &retrievedV1Alpha1)
	require.NoError(t, err)
	assert.Equal(t, "Test Dashboard v1alpha1", retrievedV1Alpha1.Spec.Title)

	var retrievedV1Alpha2 v1alpha2.DatadogDashboard
	err = fakeClient.Get(ctx, types.NamespacedName{
		Name:      "test-dashboard-v1alpha2",
		Namespace: "default",
	}, &retrievedV1Alpha2)
	require.NoError(t, err)
	assert.Equal(t, "Test Dashboard v1alpha2", retrievedV1Alpha2.Spec.Title)
	assert.Len(t, retrievedV1Alpha2.Spec.Widgets, 1)
}

func TestIntegration_WidgetProcessorCompatibility(t *testing.T) {
	// Test that widget processors can handle their respective API versions correctly

	// Test v1alpha1 widget processor
	v1alpha1Dashboard := &v1alpha1.DatadogDashboard{
		Spec: v1alpha1.DatadogDashboardSpec{
			Title:      "Test Dashboard",
			LayoutType: "ordered",
			Widgets:    `[{"definition": {"type": "timeseries", "title": "CPU Usage"}, "layout": {"x": 0, "y": 0, "width": 4, "height": 2}}]`,
		},
	}

	v1alpha1Processor := NewV1Alpha1WidgetProcessor(testLogger)
	v1alpha1Widgets, err := v1alpha1Processor.ProcessWidgets(v1alpha1Dashboard)
	require.NoError(t, err)
	assert.Len(t, v1alpha1Widgets, 1)

	// Test v1alpha2 widget processor
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
						X:      0,
						Y:      0,
						Width:  4,
						Height: 2,
					},
				},
			},
		},
	}

	v1alpha2Processor := NewV1Alpha2WidgetProcessor(testLogger)
	v1alpha2Widgets, err := v1alpha2Processor.ProcessWidgets(v1alpha2Dashboard)
	require.NoError(t, err)
	assert.Len(t, v1alpha2Widgets, 1)

	// Both processors should produce equivalent widget structures
	assert.Equal(t, len(v1alpha1Widgets), len(v1alpha2Widgets))
}

func TestIntegration_ValidationConsistency(t *testing.T) {
	// Test that validation works consistently across both API versions

	// Test v1alpha1 validation
	v1alpha1Dashboard := &v1alpha1.DatadogDashboard{
		Spec: v1alpha1.DatadogDashboardSpec{
			// Missing required fields
		},
	}

	v1alpha1Processor := NewV1Alpha1WidgetProcessor(testLogger)
	err := v1alpha1Processor.ValidateWidgets(v1alpha1Dashboard)
	assert.Error(t, err, "v1alpha1 validation should fail for missing required fields")

	// Test v1alpha2 validation
	v1alpha2Dashboard := &v1alpha2.DatadogDashboard{
		Spec: v1alpha2.DatadogDashboardSpec{
			// Missing required fields
		},
	}

	v1alpha2Processor := NewV1Alpha2WidgetProcessor(testLogger)
	err = v1alpha2Processor.ValidateWidgets(v1alpha2Dashboard)
	assert.Error(t, err, "v1alpha2 validation should fail for missing required fields")
}

func TestIntegration_GetWidgetProcessorRouting(t *testing.T) {
	// Test that GetWidgetProcessor correctly routes to the right processor

	v1alpha1Dashboard := &v1alpha1.DatadogDashboard{}
	processor, err := GetWidgetProcessor(v1alpha1Dashboard, testLogger)
	require.NoError(t, err)
	assert.Equal(t, "v1alpha1", processor.GetAPIVersion())

	v1alpha2Dashboard := &v1alpha2.DatadogDashboard{}
	processor, err = GetWidgetProcessor(v1alpha2Dashboard, testLogger)
	require.NoError(t, err)
	assert.Equal(t, "v1alpha2", processor.GetAPIVersion())
}

func TestIntegration_CRDSchemaValidation(t *testing.T) {
	// Test that the CRD schema properly validates v1alpha2 resources
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha2.AddToScheme(scheme))

	// Test valid v1alpha2 dashboard
	validDashboard := &v1alpha2.DatadogDashboard{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "valid-dashboard",
			Namespace: "default",
		},
		Spec: v1alpha2.DatadogDashboardSpec{
			Title:      "Valid Dashboard",
			LayoutType: "ordered",
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
	}

	// Validate the dashboard using the v1alpha2 validation
	err := v1alpha2.IsValidDatadogDashboard(&validDashboard.Spec)
	assert.NoError(t, err, "Valid v1alpha2 dashboard should pass validation")

	// Test invalid v1alpha2 dashboard
	invalidDashboard := &v1alpha2.DatadogDashboard{
		Spec: v1alpha2.DatadogDashboardSpec{
			// Missing required title and layoutType
			Widgets: []v1alpha2.DashboardWidget{
				{
					Definition: runtime.RawExtension{
						Raw: []byte(`{"type": "invalid_type"}`),
					},
				},
			},
		},
	}

	err = v1alpha2.IsValidDatadogDashboard(&invalidDashboard.Spec)
	assert.Error(t, err, "Invalid v1alpha2 dashboard should fail validation")
}
