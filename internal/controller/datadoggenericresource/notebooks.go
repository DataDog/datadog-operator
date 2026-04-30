package datadoggenericresource

import (
	"context"
	"encoding/json"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

type NotebookHandler struct {
	client *datadogV1.NotebooksApi
}

func (h *NotebookHandler) createResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) (CreateResult, error) {
	createdNotebook, err := createNotebook(auth, h.client, instance)
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

func (h *NotebookHandler) getResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error {
	_, err := getNotebook(auth, h.client, instance.Status.Id)
	return err
}
func (h *NotebookHandler) updateResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error {
	_, err := updateNotebook(auth, h.client, instance)
	return err
}
func (h *NotebookHandler) deleteResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error {
	return deleteNotebook(auth, h.client, instance.Status.Id)
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
	httpResponse, err := client.DeleteNotebook(auth, notebookID)
	if err != nil {
		// Deletion is idempotent for finalization: if the notebook was already removed
		// in Datadog (for example from the UI), allow the Kubernetes finalizer to clear.
		// Retry other errors (e.g. 400, 401, 429, 5XX).
		if httpResponse != nil && httpResponse.StatusCode == 404 {
			return nil
		}
		return translateClientError(err, "error deleting notebook")
	}
	return nil
}

func createNotebook(auth context.Context, client *datadogV1.NotebooksApi, instance *v1alpha1.DatadogGenericResource) (datadogV1.NotebookResponse, error) {
	notebookCreateData := &datadogV1.NotebookCreateRequest{}
	if err := json.Unmarshal([]byte(instance.Spec.JsonSpec), notebookCreateData); err != nil {
		return datadogV1.NotebookResponse{}, translateClientError(err, "error unmarshalling notebook spec")
	}
	notebook, _, err := client.CreateNotebook(auth, *notebookCreateData)
	if err != nil {
		return datadogV1.NotebookResponse{}, translateClientError(err, "error creating notebook")
	}
	return notebook, nil
}

func updateNotebook(auth context.Context, client *datadogV1.NotebooksApi, instance *v1alpha1.DatadogGenericResource) (datadogV1.NotebookResponse, error) {
	notebookUpdateData := &datadogV1.NotebookUpdateRequest{}
	if err := json.Unmarshal([]byte(instance.Spec.JsonSpec), notebookUpdateData); err != nil {
		return datadogV1.NotebookResponse{}, translateClientError(err, "error unmarshalling notebook spec")
	}
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
