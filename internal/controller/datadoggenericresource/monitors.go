package datadoggenericresource

import (
	"context"
	"encoding/json"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

type MonitorHandler struct {
	client *datadogV1.MonitorsApi
}

func (h *MonitorHandler) createResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) (CreateResult, error) {
	createdMonitor, err := createMonitor(auth, h.client, instance)
	if err != nil {
		return CreateResult{}, err
	}
	createdTime := metav1.NewTime(*createdMonitor.Created)
	return CreateResult{
		ID:          resourceInt64ToStringID(createdMonitor.GetId()),
		CreatedTime: &createdTime,
		Creator:     *createdMonitor.GetCreator().Handle,
	}, nil
}

func (h *MonitorHandler) getResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error {
	_, err := getMonitor(auth, h.client, instance.Status.Id)
	return err
}
func (h *MonitorHandler) updateResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error {
	_, err := updateMonitor(auth, h.client, instance)
	return err
}
func (h *MonitorHandler) deleteResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error {
	return deleteMonitor(auth, h.client, instance.Status.Id)
}

func getMonitor(auth context.Context, client *datadogV1.MonitorsApi, monitorStringID string) (datadogV1.Monitor, error) {
	monitorID, err := resourceStringToInt64ID(monitorStringID)
	if err != nil {
		return datadogV1.Monitor{}, err
	}
	monitor, _, err := client.GetMonitor(auth, monitorID)
	if err != nil {
		return datadogV1.Monitor{}, translateClientError(err, "error getting monitor")
	}
	return monitor, nil
}

func deleteMonitor(auth context.Context, client *datadogV1.MonitorsApi, monitorStringID string) error {
	monitorID, err := resourceStringToInt64ID(monitorStringID)
	if err != nil {
		return err
	}
	_, httpResponse, err := client.DeleteMonitor(auth, monitorID)
	if err != nil {
		// Deletion is idempotent for finalization: if the monitor was already removed
		// in Datadog (for example from the UI), allow the Kubernetes finalizer to clear.
		// Retry other errors (e.g. 400, 401, 429, 5XX).
		if httpResponse != nil && httpResponse.StatusCode == 404 {
			return nil
		}
		return translateClientError(err, "error deleting monitor")
	}
	return nil
}

func createMonitor(auth context.Context, client *datadogV1.MonitorsApi, instance *v1alpha1.DatadogGenericResource) (datadogV1.Monitor, error) {
	monitorBody := &datadogV1.Monitor{}
	if err := json.Unmarshal([]byte(instance.Spec.JsonSpec), monitorBody); err != nil {
		return datadogV1.Monitor{}, translateClientError(err, "error unmarshalling monitor spec")
	}
	monitor, _, err := client.CreateMonitor(auth, *monitorBody)
	if err != nil {
		return datadogV1.Monitor{}, translateClientError(err, "error creating monitor")
	}
	return monitor, nil
}

func updateMonitor(auth context.Context, client *datadogV1.MonitorsApi, instance *v1alpha1.DatadogGenericResource) (datadogV1.Monitor, error) {
	monitorUpdateData := &datadogV1.MonitorUpdateRequest{}
	if err := json.Unmarshal([]byte(instance.Spec.JsonSpec), monitorUpdateData); err != nil {
		return datadogV1.Monitor{}, translateClientError(err, "error unmarshalling monitor spec")
	}
	monitorID, err := resourceStringToInt64ID(instance.Status.Id)
	if err != nil {
		return datadogV1.Monitor{}, err
	}
	monitorUpdated, _, err := client.UpdateMonitor(auth, monitorID, *monitorUpdateData)
	if err != nil {
		return datadogV1.Monitor{}, translateClientError(err, "error updating monitor")
	}
	return monitorUpdated, nil
}
