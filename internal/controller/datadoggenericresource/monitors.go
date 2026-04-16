package datadoggenericresource

import (
	"context"
	"encoding/json"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

type MonitorHandler struct{}

func (h *MonitorHandler) createResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericResource) (CreateResult, error) {
	createdMonitor, err := createMonitor(r.datadogAuth, r.datadogMonitorsClient, instance)
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

func (h *MonitorHandler) getResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericResource) error {
	_, err := getMonitor(r.datadogAuth, r.datadogMonitorsClient, instance.Status.Id)
	return err
}
func (h *MonitorHandler) updateResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericResource) error {
	_, err := updateMonitor(r.datadogAuth, r.datadogMonitorsClient, instance)
	return err
}
func (h *MonitorHandler) deleteResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericResource) error {
	return deleteMonitor(r.datadogAuth, r.datadogMonitorsClient, instance.Status.Id)
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
	if _, _, err := client.DeleteMonitor(auth, monitorID); err != nil {
		return translateClientError(err, "error deleting monitor")
	}
	return nil
}

func createMonitor(auth context.Context, client *datadogV1.MonitorsApi, instance *v1alpha1.DatadogGenericResource) (datadogV1.Monitor, error) {
	monitorBody := &datadogV1.Monitor{}
	json.Unmarshal([]byte(instance.Spec.JsonSpec), monitorBody)
	monitor, _, err := client.CreateMonitor(auth, *monitorBody)
	if err != nil {
		return datadogV1.Monitor{}, translateClientError(err, "error creating monitor")
	}
	return monitor, nil
}

func updateMonitor(auth context.Context, client *datadogV1.MonitorsApi, instance *v1alpha1.DatadogGenericResource) (datadogV1.Monitor, error) {
	monitorUpdateData := &datadogV1.MonitorUpdateRequest{}
	json.Unmarshal([]byte(instance.Spec.JsonSpec), monitorUpdateData)
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
