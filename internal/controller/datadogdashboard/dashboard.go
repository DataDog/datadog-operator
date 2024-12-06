package datadogdashboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-operator/api/crds/datadoghq/v1alpha1"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/go-logr/logr"
)

// Transform v1alpha1 dashboard into a datadogV1 Dashboard
func buildDashboard(logger logr.Logger, ddb *v1alpha1.DatadogDashboard) *datadogV1.Dashboard {
	layoutType := ddb.Spec.LayoutType
	widgetList := &[]datadogV1.Widget{}
	json.Unmarshal([]byte(ddb.Spec.Widgets), widgetList)

	dashboard := datadogV1.NewDashboard(layoutType, ddb.Spec.Title, *widgetList)

	if ddb.Spec.Description != "" {
		dashboard.SetDescription(ddb.Spec.Description)
	} else {
		dashboard.SetDescriptionNil()
	}
	if ddb.Spec.ReflowType != nil {
		dashboard.SetReflowType(*ddb.Spec.ReflowType)
	}
	if ddb.Spec.TemplateVariablePresets != nil {
		dbTemplateVariablePresets := convertTempVarPresets(ddb.Spec.TemplateVariablePresets)
		dashboard.SetTemplateVariablePresets(dbTemplateVariablePresets)
	}

	dashboard.SetWidgets(*widgetList)

	tags := ddb.Spec.Tags
	sort.Strings(tags)
	dashboard.SetTags(tags)

	dashboard.SetTitle(ddb.Spec.Title)

	if ddb.Spec.NotifyList != nil {
		dashboard.SetNotifyList(ddb.Spec.NotifyList)
	}
	if ddb.Spec.TemplateVariables != nil {
		dashboard.SetTemplateVariables(convertTempVars(ddb.Spec.TemplateVariables))
	}

	return dashboard
}

func getDashboard(auth context.Context, client *datadogV1.DashboardsApi, dashboardID string) (datadogV1.Dashboard, error) {
	dashboard, _, err := client.GetDashboard(auth, dashboardID)
	if err != nil {
		return datadogV1.Dashboard{}, translateClientError(err, "error creating Dashboard")
	}
	return dashboard, nil
}

func createDashboard(auth context.Context, logger logr.Logger, client *datadogV1.DashboardsApi, ddb *v1alpha1.DatadogDashboard) (datadogV1.Dashboard, error) {
	db := buildDashboard(logger, ddb)
	dbCreated, _, err := client.CreateDashboard(auth, *db)
	if err != nil {
		return datadogV1.Dashboard{}, translateClientError(err, "error creating dashboard")
	}

	return dbCreated, nil
}

func updateDashboard(auth context.Context, logger logr.Logger, client *datadogV1.DashboardsApi, ddb *v1alpha1.DatadogDashboard) (datadogV1.Dashboard, error) {
	dashboard := buildDashboard(logger, ddb)
	dbUpdated, _, err := client.UpdateDashboard(auth, ddb.Status.ID, *dashboard)
	if err != nil {
		return datadogV1.Dashboard{}, translateClientError(err, "error updating dashboard")
	}

	return dbUpdated, nil
}

func deleteDashboard(auth context.Context, client *datadogV1.DashboardsApi, dashboardID string) error {
	if _, _, err := client.DeleteDashboard(auth, dashboardID); err != nil {
		return translateClientError(err, "error deleting Dashboard")
	}

	return nil
}

func translateClientError(err error, msg string) error {
	if msg == "" {
		msg = "an error occurred"
	}

	var apiErr datadogapi.GenericOpenAPIError
	var errURL *url.Error
	if errors.As(err, &apiErr) {
		return fmt.Errorf(msg+": %w: %s", err, apiErr.Body())
	}

	if errors.As(err, &errURL) {
		return fmt.Errorf(msg+" (url.Error): %s", errURL)
	}

	return fmt.Errorf(msg+": %w", err)
}

func convertTempVarPresets(tempVarPresets []v1alpha1.DashboardTemplateVariablePreset) []datadogV1.DashboardTemplateVariablePreset {
	dbTemplateVariablePresets := []datadogV1.DashboardTemplateVariablePreset{}
	for _, variablePreset := range tempVarPresets {
		dbTemplateVariablePreset := datadogV1.DashboardTemplateVariablePreset{}
		// Note: Name is required
		dbTemplateVariablePreset.SetName(*variablePreset.Name)
		dbTemplateVariablePresetValues := []datadogV1.DashboardTemplateVariablePresetValue{}
		for _, presetValue := range variablePreset.TemplateVariables {
			dbTemplateVariablePresetValue := datadogV1.DashboardTemplateVariablePresetValue{}
			dbTemplateVariablePresetValue.SetName(*presetValue.Name)
			if presetValue.Values != nil {
				dbTemplateVariablePresetValue.SetValues(presetValue.Values)
			}
			dbTemplateVariablePresetValues = append(dbTemplateVariablePresetValues, dbTemplateVariablePresetValue)
		}
		dbTemplateVariablePreset.SetTemplateVariables(dbTemplateVariablePresetValues)
		dbTemplateVariablePresets = append(dbTemplateVariablePresets, dbTemplateVariablePreset)
	}
	return dbTemplateVariablePresets
}

func convertTempVars(tempVars []v1alpha1.DashboardTemplateVariable) []datadogV1.DashboardTemplateVariable {
	dbTemplateVariables := []datadogV1.DashboardTemplateVariable{}
	for _, templateVariable := range tempVars {
		dbTemplateVariable := datadogV1.DashboardTemplateVariable{}
		dbTemplateVariable.SetName(templateVariable.Name)

		if templateVariable.Defaults != nil {
			dbTemplateVariable.SetDefaults(templateVariable.Defaults)
		}
		if templateVariable.AvailableValues != nil {
			dbTemplateVariable.SetAvailableValues(*templateVariable.AvailableValues)
		}
		if templateVariable.Prefix != nil {
			dbTemplateVariable.SetPrefix(*templateVariable.Prefix)
		}
		dbTemplateVariables = append(dbTemplateVariables, dbTemplateVariable)

	}
	return dbTemplateVariables
}
