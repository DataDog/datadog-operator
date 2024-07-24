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

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/go-logr/logr"
)

// NOTE: normalize all the names crd/ddb/dashboard/etc
// NOTE: double check that I don't need request struct
// Dashboard
func buildDashboard(logger logr.Logger, crd *v1alpha1.DatadogDashboard) *datadogV1.Dashboard {
	// create a dashboard
	layoutType := datadogV1.DashboardLayoutType(crd.Spec.LayoutType)
	widgets := &[]datadogV1.Widget{}

	// NOTE: for now, pass in empty widgetlist
	dashboard := datadogV1.NewDashboard(layoutType, crd.Spec.Title, *widgets)

	// fmu: why is this bracketed tho in SLOs?
	if crd.Spec.Description != "" {
		dashboard.SetDescription(crd.Spec.Description)
	} else {
		dashboard.SetDescriptionNil()
	}
	// NOTE: sort tags like in monitors?
	dashboard.SetTags(crd.Spec.Tags)

	return dashboard
}

func getDashboard(auth context.Context, client *datadogV1.DashboardsApi, dashboardID string) (datadogV1.Dashboard, error) {
	// NOTE: no optional params like monitors/SLOs
	dashboard, _, err := client.GetDashboard(auth, dashboardID)
	if err != nil {
		return datadogV1.Dashboard{}, translateClientError(err, "error creating Dashboard")
	}
	return dashboard, nil
}

// no validation fo dashboards
// func validateDashboard(auth context.Context, logger logr.Logger, client *datadogV1.DashboardsApi, dm *datadoghqv1alpha1.DatadogMonitor) error {
// 	// m, _ := buildMonitor(logger, dm)
// 	// if _, _, err := client.ValidateMonitor(auth, *m); err != nil {
// 	// 	return translateClientError(err, "error validating monitor")
// 	// }
// 	client
// 	// return nil
// 	return nil
// }

// NOTE: remove logger after finishing debugging
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
