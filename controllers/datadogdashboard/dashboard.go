package datadogdashboard

// build dashboards - build dashoard spec
// get dashboards (why? idk, but perhaps to update status if it's not reachable. Tells you something)
// validate if the fields of a dashboard are correctly written
// create dashboard -- use dashboard spec to create dashbaord
// update dashboard -- based on CRD change, I'm assuming?
// delete dashboard -- delete dashboard
// kubernetes automatically detects changes? how to deleting/updating

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/go-logr/logr"
)

// Dashboard
func buildDashboard(logger logr.Logger, ddb *v1alpha1.DatadogDashboard) *datadogV1.Dashboard {
	// create a dashboard
	layoutType := datadogV1.DashboardLayoutType(ddb.Spec.LayoutType)
	widgets := &[]datadogV1.Widget{}

	// NOTE: for now, pass in empty widgetlist
	dashboard := datadogV1.NewDashboard(layoutType, ddb.Spec.Title, *widgets)

	if ddb.Spec.Description != "" {
		dashboard.SetDescription(ddb.Spec.Description)
	} else {
		dashboard.SetDescriptionNil()
	}

	if ddb.Spec.ReflowType != nil {
		dashboard.SetReflowType(*ddb.Spec.ReflowType)
	}

	if ddb.Spec.TemplateVariablePresets != nil {
		dbTemplateVariablePresets := []datadogV1.DashboardTemplateVariablePreset{}
		for _, variablePreset := range ddb.Spec.TemplateVariablePresets {
			dbTemplateVariablePreset := datadogV1.DashboardTemplateVariablePreset{}
			// Name is required
			dbTemplateVariablePreset.SetName(*variablePreset.Name)
			dbTemplateVariablePresetValues := []datadogV1.DashboardTemplateVariablePresetValue{}
			for _, presetValue := range variablePreset.TemplateVariables {
				dbTemplateVariablePresetValue := datadogV1.DashboardTemplateVariablePresetValue{}
				// Name is required
				dbTemplateVariablePresetValue.SetName(*presetValue.Name)
				// NOTE: is it possible to assign nil here anyways?
				if presetValue.Values != nil {
					dbTemplateVariablePresetValue.SetValues(presetValue.Values)
				}
				dbTemplateVariablePresetValues = append(dbTemplateVariablePresetValues, dbTemplateVariablePresetValue)
			}
			dbTemplateVariablePreset.SetTemplateVariables(dbTemplateVariablePresetValues)
			dbTemplateVariablePresets = append(dbTemplateVariablePresets, dbTemplateVariablePreset)
		}
		dashboard.SetTemplateVariablePresets(dbTemplateVariablePresets)
	}

	if ddb.Spec.TemplateVariables != nil {
		dbTemplateVariables := []datadogV1.DashboardTemplateVariable{}
		for _, templateVariable := range ddb.Spec.TemplateVariables {
			dbTemplateVariable := datadogV1.DashboardTemplateVariable{}
			dbTemplateVariable.SetName(templateVariable.Name)

			if dbTemplateVariable.Defaults != nil {
				dbTemplateVariable.SetDefaults(templateVariable.Defaults)
			}
			// NOTE: Unsure about this nullableList behavior
			if templateVariable.AvailableValues.Value != nil {
				// availableValues := datadog.NullableList[string]{}
				// availableValues.Set(dbTemplateVariable.AvailableValues.Get())
				dbTemplateVariable.SetAvailableValues(*templateVariable.AvailableValues.Value)
			}
			// NOTE: since we can just set nullableString/List like so, perhaps change types to just make it a string?
			if templateVariable.Prefix.Value != nil {
				dbTemplateVariable.SetPrefix(*templateVariable.Prefix.Value)
			}
			dbTemplateVariables = append(dbTemplateVariables, dbTemplateVariable)

		}
		dashboard.SetTemplateVariables(dbTemplateVariables)
	}

	tags := ddb.Spec.Tags
	sort.Strings(tags)
	dashboard.SetTags(tags)

	return dashboard
}

func getDashboard(auth context.Context, client *datadogV1.DashboardsApi, dashboardID string) (datadogV1.Dashboard, error) {
	dashboard, _, err := client.GetDashboard(auth, dashboardID)
	if err != nil {
		return datadogV1.Dashboard{}, translateClientError(err, "error creating Dashboard")
	}
	return dashboard, nil
}

func createDashboard(logger logr.Logger, auth context.Context, client *datadogV1.DashboardsApi, ddb *v1alpha1.DatadogDashboard) (datadogV1.Dashboard, error) {
	db := buildDashboard(logger, ddb)
	dbCreated, _, err := client.CreateDashboard(auth, *db)
	if err != nil {
		return datadogV1.Dashboard{}, translateClientError(err, "error creating dashboard")
	}

	return dbCreated, nil
}

func updateDashboard(logger logr.Logger, auth context.Context, client *datadogV1.DashboardsApi, ddb *v1alpha1.DatadogDashboard) (datadogV1.Dashboard, error) {
	dashboard := buildDashboard(logger, ddb)
	dbUpdated, _, err := client.UpdateDashboard(auth, ddb.Status.ID, *dashboard)
	if err != nil {
		return datadogV1.Dashboard{}, translateClientError(err, "error updating SLO")
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
