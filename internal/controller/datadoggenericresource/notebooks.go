package datadoggenericresource

import (
	"context"
	"encoding/json"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

type NotebookHandler struct{}

func (h *NotebookHandler) createResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericResource) (CreateResult, error) {
	createdNotebook, err := createNotebook(r.datadogAuth, r.datadogNotebooksClient, instance)
	if err != nil {
		return CreateResult{}, err
	}
	createdTime := metav1.NewTime(*createdNotebook.Data.GetAttributes().Created)
	return CreateResult{
		ID:          resourceInt64ToStringID(createdNotebook.Data.GetId()),
		CreatedTime: &createdTime,
		Creator:     *createdNotebook.Data.GetAttributes().Author.Handle,
	}, nil
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

func getNotebook(auth context.Context, client *datadogV1.NotebooksApi, notebookStringID string) (datadogV1.NotebookResponse, error) {
	notebookID, err := resourceStringToInt64ID(notebookStringID)
	if err != nil {
		return datadogV1.NotebookResponse{}, err
	}
	notebook, _, err := client.GetNotebook(auth, notebookID)
	if err != nil {
		return datadogV1.NotebookResponse{}, translateClientError(err, "error getting notebook")
	}
	return notebook, nil
}

func deleteNotebook(auth context.Context, client *datadogV1.NotebooksApi, notebookStringID string) error {
	notebookID, err := resourceStringToInt64ID(notebookStringID)
	if err != nil {
		return err
	}
	if _, err := client.DeleteNotebook(auth, notebookID); err != nil {
		return translateClientError(err, "error deleting notebook")
	}
	return nil
}

func createNotebook(auth context.Context, client *datadogV1.NotebooksApi, instance *v1alpha1.DatadogGenericResource) (datadogV1.NotebookResponse, error) {
	notebookCreateData := &datadogV1.NotebookCreateRequest{}
	json.Unmarshal([]byte(instance.Spec.JsonSpec), notebookCreateData)
	notebook, _, err := client.CreateNotebook(auth, *notebookCreateData)
	if err != nil {
		return datadogV1.NotebookResponse{}, translateClientError(err, "error creating notebook")
	}
	return notebook, nil
}

func updateNotebook(auth context.Context, client *datadogV1.NotebooksApi, instance *v1alpha1.DatadogGenericResource) (datadogV1.NotebookResponse, error) {
	notebookUpdateData := &datadogV1.NotebookUpdateRequest{}
	json.Unmarshal([]byte(instance.Spec.JsonSpec), notebookUpdateData)
	notebookID, err := resourceStringToInt64ID(instance.Status.Id)
	if err != nil {
		return datadogV1.NotebookResponse{}, err
	}
	notebookUpdated, _, err := client.UpdateNotebook(auth, notebookID, *notebookUpdateData)
	if err != nil {
		return datadogV1.NotebookResponse{}, translateClientError(err, "error updating notebook")
	}
	return notebookUpdated, nil
}
