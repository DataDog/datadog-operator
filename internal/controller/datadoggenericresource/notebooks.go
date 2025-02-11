package datadoggenericresource

import (
	"context"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
)

type NotebookHandler struct{}

func (h *NotebookHandler) createResourcefunc(r *Reconciler, logger logr.Logger, instance *v1alpha1.DatadogGenericResource, status *v1alpha1.DatadogGenericResourceStatus, now metav1.Time, hash string) error {
	createdNotebook, err := createNotebook(r.datadogAuth, r.datadogNotebooksClient, instance)
	if err != nil {
		logger.Error(err, "error creating notebook")
		updateErrStatus(status, now, v1alpha1.DatadogSyncStatusCreateError, "CreatingCustomResource", err)
		return err
	}
	logger.Info("created a new notebook", "notebook Id", createdNotebook.Data.GetId())
	status.Id = resourceInt64ToStringID(createdNotebook.Data.GetId())
	createdTime := metav1.NewTime(*createdNotebook.Data.GetAttributes().Created)
	status.Created = &createdTime
	status.LastForceSyncTime = &createdTime
	status.Creator = *createdNotebook.Data.GetAttributes().Author.Handle
	status.SyncStatus = v1alpha1.DatadogSyncStatusOK
	status.CurrentHash = hash
	return nil
}

func (h *NotebookHandler) getResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericResource) error {
	_, err := getNotebook(r.datadogAuth, r.datadogNotebooksClient, instance.Status.Id)
	return err
}
func (h *NotebookHandler) updateResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericResource) error {
	_, err := updateNotebook(r.datadogAuth, r.datadogNotebooksClient, instance)
	return err
}
func (h *NotebookHandler) deleteResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericResource) error {
	return deleteNotebook(r.datadogAuth, r.datadogNotebooksClient, instance.Status.Id)
}

type NotebookHandler struct{}
type NotebookCRUDClient struct {
	client *datadogV1.NotebooksApi
}

func (h *NotebookHandler) createResourcefunc(r *Reconciler, logger logr.Logger, instance *v1alpha1.DatadogGenericResource, status *v1alpha1.DatadogGenericResourceStatus, now metav1.Time, hash string) error {
	resource, err := CreateResource(r.datadogAuth, &NotebookCRUDClient{client: r.datadogNotebooksClient}, instance)
	if err != nil {
		logger.Error(err, "error creating notebook")
		updateErrStatus(status, now, v1alpha1.DatadogSyncStatusCreateError, "CreatingCustomResource", err)
		return err
	}
	createdNotebook := resource.(datadogV1.NotebookResponse)
	logger.Info("created a new notebook", "notebook Id", createdNotebook.Data.GetId())
	status.Id = resourceInt64ToStringID(createdNotebook.Data.GetId())
	createdTime := metav1.NewTime(*createdNotebook.Data.GetAttributes().Created)
	status.Created = &createdTime
	status.LastForceSyncTime = &createdTime
	status.Creator = *createdNotebook.Data.GetAttributes().Author.Handle
	status.SyncStatus = v1alpha1.DatadogSyncStatusOK
	status.CurrentHash = hash
	return nil
}

func (h *NotebookHandler) getResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericResource) error {
	return GetResource(r.datadogAuth, &NotebookCRUDClient{client: r.datadogNotebooksClient}, instance)
}
func (h *NotebookHandler) updateResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericResource) error {
	_, err := UpdateResource(r.datadogAuth, &NotebookCRUDClient{client: r.datadogNotebooksClient}, instance)
	return err
}
func (h *NotebookHandler) deleteResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericResource) error {
	return DeleteResource(r.datadogAuth, &NotebookCRUDClient{client: r.datadogNotebooksClient}, instance)
}

func (c *NotebookCRUDClient) createResource(auth context.Context, unmarshaledSpec any) (any, error) {
	notebookCreateRequest := unmarshaledSpec.(*datadogV1.NotebookCreateRequest)
	notebook, _, err := c.client.CreateNotebook(auth, *notebookCreateRequest)
	if err != nil {
		return nil, err
	}
	return notebook, nil
}

func (c *NotebookCRUDClient) getResource(auth context.Context, resourceStringID string) error {
	notebookID, err := resourceStringToInt64ID(resourceStringID)
	if err != nil {
		return err
	}
	_, _, err = c.client.GetNotebook(auth, notebookID)
	if err != nil {
		return translateClientError(err, "error getting notebook")
	}
	return nil
}

func (c *NotebookCRUDClient) updateResource(auth context.Context, resourceStringID string, unmarshaledSpec any) (any, error) {
	notebookUpdateData := unmarshaledSpec.(*datadogV1.NotebookUpdateRequest)
	notebookID, err := resourceStringToInt64ID(resourceStringID)
	if err != nil {
		return nil, err
	}
	notebookUpdated, _, err := c.client.UpdateNotebook(auth, notebookID, *notebookUpdateData)
	if err != nil {
		return nil, err
	}
	return notebookUpdated, nil
}

func (c *NotebookCRUDClient) deleteResource(auth context.Context, resourceStringID string) error {
	notebookID, err := resourceStringToInt64ID(resourceStringID)
	if err != nil {
		return err
	}
	if _, err := c.client.DeleteNotebook(auth, notebookID); err != nil {
		return translateClientError(err, "error deleting notebook")
	}
	return nil
}
