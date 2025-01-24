package datadoggenericresource

import (
	"context"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type MonitorHandler struct{}
type MonitorCRUDClient struct {
	client *datadogV1.MonitorsApi
}

func (c *MonitorCRUDClient) createResource(auth context.Context, unmarshaledSpec any) (any, error) {
	v := unmarshaledSpec.(*datadogV1.Monitor)
	monitor, _, err := c.client.CreateMonitor(auth, *v)
	return monitor, err
}

func (c *MonitorCRUDClient) getResource(auth context.Context, resourceStringID string) error {
	monitorID, err := resourceStringToInt64ID(resourceStringID)
	if err != nil {
		return err
	}
	_, _, err = c.client.GetMonitor(auth, monitorID)
	if err != nil {
		return translateClientError(err, "error getting monitor")
	}
	return nil
}

func (c *MonitorCRUDClient) updateResource(auth context.Context, resourceStringID string, unmarshaledSpec any) (any, error) {
	monitorID, err := resourceStringToInt64ID(resourceStringID)
	if err != nil {
		return nil, err
	}
	v := unmarshaledSpec.(*datadogV1.MonitorUpdateRequest)
	monitor, _, err := c.client.UpdateMonitor(auth, monitorID, *v)
	return monitor, err
}

func (c *MonitorCRUDClient) deleteResource(auth context.Context, resourceStringID string) error {
	monitorID, err := resourceStringToInt64ID(resourceStringID)
	if err != nil {
		return err
	}
	if _, _, err = c.client.DeleteMonitor(auth, monitorID); err != nil {
		return translateClientError(err, "error deleting monitor")
	}
	return nil
}

func (h *MonitorHandler) createResourcefunc(r *Reconciler, logger logr.Logger, instance *v1alpha1.DatadogGenericResource, status *v1alpha1.DatadogGenericResourceStatus, now metav1.Time, hash string) error {
	resource, err := CreateResource(r.datadogAuth, &MonitorCRUDClient{client: r.datadogMonitorsClient}, instance)
	if err != nil {
		logger.Error(err, "error creating monitor")
		updateErrStatus(status, now, v1alpha1.DatadogSyncStatusCreateError, "CreatingCustomResource", err)
		return err
	}
	createdMonitor := resource.(datadogV1.Monitor)
	logger.Info("created a new monitor", "monitor Id", createdMonitor.GetId())
	status.Id = resourceInt64ToStringID(createdMonitor.GetId())
	createdTime := metav1.NewTime(*createdMonitor.Created)
	status.Created = &createdTime
	status.LastForceSyncTime = &createdTime
	status.Creator = *createdMonitor.GetCreator().Handle
	status.SyncStatus = v1alpha1.DatadogSyncStatusOK
	status.CurrentHash = hash
	return nil
}

func (h *MonitorHandler) getResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericResource) error {
	return GetResource(r.datadogAuth, &MonitorCRUDClient{client: r.datadogMonitorsClient}, instance)
}
func (h *MonitorHandler) updateResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericResource) error {
	_, err := UpdateResource(r.datadogAuth, &MonitorCRUDClient{client: r.datadogMonitorsClient}, instance)
	return err
}
func (h *MonitorHandler) deleteResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericResource) error {
	return DeleteResource(r.datadogAuth, &MonitorCRUDClient{client: r.datadogMonitorsClient}, instance)
}
