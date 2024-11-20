package datadoggenericcrd

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
)

func getNotebook(auth context.Context, client *datadogV1.NotebooksApi, notebookStringID string) (datadogV1.NotebookResponse, error) {
	notebookID, err := notebookStringToInt64(notebookStringID)
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
	notebookID, err := notebookStringToInt64(notebookStringID)
	if err != nil {
		return err
	}
	if _, err := client.DeleteNotebook(auth, notebookID); err != nil {
		return translateClientError(err, "error deleting notebook")
	}
	return nil
}

func createNotebook(auth context.Context, client *datadogV1.NotebooksApi, instance *v1alpha1.DatadogGenericCR) (datadogV1.NotebookResponse, error) {
	notebookCreateData := &datadogV1.NotebookCreateRequest{}
	json.Unmarshal([]byte(instance.Spec.JsonSpec), notebookCreateData)
	notebook, _, err := client.CreateNotebook(auth, *notebookCreateData)
	if err != nil {
		return datadogV1.NotebookResponse{}, translateClientError(err, "error creating notebook")
	}
	return notebook, nil
}

func updateNotebook(auth context.Context, client *datadogV1.NotebooksApi, instance *v1alpha1.DatadogGenericCR) (datadogV1.NotebookResponse, error) {
	notebookUpdateData := &datadogV1.NotebookUpdateRequest{}
	json.Unmarshal([]byte(instance.Spec.JsonSpec), notebookUpdateData)
	notebookID, err := notebookStringToInt64(instance.Status.Id)
	if err != nil {
		return datadogV1.NotebookResponse{}, err
	}
	notebookUpdated, _, err := client.UpdateNotebook(auth, notebookID, *notebookUpdateData)
	if err != nil {
		return datadogV1.NotebookResponse{}, translateClientError(err, "error updating browser test")
	}
	return notebookUpdated, nil
}

func notebookStringToInt64(notebookStringID string) (int64, error) {
	notebookID, err := strconv.ParseInt(notebookStringID, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("error parsing notebook Id: %w", err)
	}
	return notebookID, nil
}

func notebookInt64ToString(notebookID int64) string {
	return strconv.FormatInt(notebookID, 10)
}
