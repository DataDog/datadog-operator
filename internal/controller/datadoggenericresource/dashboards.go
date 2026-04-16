// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoggenericresource

import (
	"context"
	"encoding/json"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

type DashboardHandler struct{}

func (h *DashboardHandler) createResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericResource) (CreateResult, error) {
	createdDashboard, err := createDashboard(r.datadogAuth, r.datadogDashboardsClient, instance)
	if err != nil {
		return CreateResult{}, err
	}
	createdTime := metav1.NewTime(createdDashboard.GetCreatedAt())
	return CreateResult{
		ID:          createdDashboard.GetId(),
		CreatedTime: &createdTime,
		Creator:     createdDashboard.GetAuthorHandle(),
	}, nil
}

func (h *DashboardHandler) getResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericResource) error {
	_, err := getDashboard(r.datadogAuth, r.datadogDashboardsClient, instance.Status.Id)
	return err
}

func (h *DashboardHandler) updateResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericResource) error {
	_, err := updateDashboard(r.datadogAuth, r.datadogDashboardsClient, instance)
	return err
}

func (h *DashboardHandler) deleteResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericResource) error {
	return deleteDashboard(r.datadogAuth, r.datadogDashboardsClient, instance.Status.Id)
}

func getDashboard(auth context.Context, client *datadogV1.DashboardsApi, dashboardID string) (datadogV1.Dashboard, error) {
	dashboard, _, err := client.GetDashboard(auth, dashboardID)
	if err != nil {
		return datadogV1.Dashboard{}, translateClientError(err, "error getting dashboard")
	}
	return dashboard, nil
}

func createDashboard(auth context.Context, client *datadogV1.DashboardsApi, instance *v1alpha1.DatadogGenericResource) (datadogV1.Dashboard, error) {
	dashboardCreateData := &datadogV1.Dashboard{}
	if err := json.Unmarshal([]byte(instance.Spec.JsonSpec), dashboardCreateData); err != nil {
		return datadogV1.Dashboard{}, translateClientError(err, "error unmarshalling dashboard spec")
	}
	dashboard, _, err := client.CreateDashboard(auth, *dashboardCreateData)
	if err != nil {
		return datadogV1.Dashboard{}, translateClientError(err, "error creating dashboard")
	}
	return dashboard, nil
}

func updateDashboard(auth context.Context, client *datadogV1.DashboardsApi, instance *v1alpha1.DatadogGenericResource) (datadogV1.Dashboard, error) {
	dashboardUpdateData := &datadogV1.Dashboard{}
	if err := json.Unmarshal([]byte(instance.Spec.JsonSpec), dashboardUpdateData); err != nil {
		return datadogV1.Dashboard{}, translateClientError(err, "error unmarshalling dashboard spec")
	}
	dashboardUpdated, _, err := client.UpdateDashboard(auth, instance.Status.Id, *dashboardUpdateData)
	if err != nil {
		return datadogV1.Dashboard{}, translateClientError(err, "error updating dashboard")
	}
	return dashboardUpdated, nil
}

func deleteDashboard(auth context.Context, client *datadogV1.DashboardsApi, dashboardID string) error {
	if _, _, err := client.DeleteDashboard(auth, dashboardID); err != nil {
		return translateClientError(err, "error deleting dashboard")
	}
	return nil
}
