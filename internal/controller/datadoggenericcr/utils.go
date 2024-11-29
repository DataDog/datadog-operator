package datadoggenericcr

import (
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/go-logr/logr"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type operation string

const (
	// mockSubresource is used to mock the subresource in tests
	mockSubresource           = "mock_resource"
	operationDelete operation = "delete"
	operationGet    operation = "get"
	operationUpdate operation = "update"
)

type apiHandlerKey struct {
	resourceType v1alpha1.SupportedResourcesType
	op           operation
}

// Delete, Get and Update operations share the same signature
type apiHandlerFunc func(r *Reconciler, instance *v1alpha1.DatadogGenericCR) error

var apiHandlers = map[apiHandlerKey]apiHandlerFunc{
	{v1alpha1.SyntheticsBrowserTest, operationGet}: func(r *Reconciler, instance *v1alpha1.DatadogGenericCR) error {
		_, err := getSyntheticsTest(r.datadogAuth, r.datadogSyntheticsClient, instance.Status.Id)
		return err
	},
	{v1alpha1.SyntheticsBrowserTest, operationUpdate}: func(r *Reconciler, instance *v1alpha1.DatadogGenericCR) error {
		_, err := updateSyntheticsBrowserTest(r.datadogAuth, r.datadogSyntheticsClient, instance)
		return err
	},
	{v1alpha1.SyntheticsBrowserTest, operationDelete}: func(r *Reconciler, instance *v1alpha1.DatadogGenericCR) error {
		return deleteSyntheticTest(r.datadogAuth, r.datadogSyntheticsClient, instance.Status.Id)
	},
	{v1alpha1.Notebook, operationGet}: func(r *Reconciler, instance *v1alpha1.DatadogGenericCR) error {
		_, err := getNotebook(r.datadogAuth, r.datadogNotebooksClient, instance.Status.Id)
		return err
	},
	{v1alpha1.Notebook, operationUpdate}: func(r *Reconciler, instance *v1alpha1.DatadogGenericCR) error {
		_, err := updateNotebook(r.datadogAuth, r.datadogNotebooksClient, instance)
		return err
	},
	{v1alpha1.Notebook, operationDelete}: func(r *Reconciler, instance *v1alpha1.DatadogGenericCR) error {
		return deleteNotebook(r.datadogAuth, r.datadogNotebooksClient, instance.Status.Id)
	},
	{mockSubresource, operationGet}: func(r *Reconciler, instance *v1alpha1.DatadogGenericCR) error {
		return nil
	},
	{mockSubresource, operationUpdate}: func(r *Reconciler, instance *v1alpha1.DatadogGenericCR) error {
		return nil
	},
	{mockSubresource, operationDelete}: func(r *Reconciler, instance *v1alpha1.DatadogGenericCR) error {
		return nil
	},
}

// Common handler executor (delete, get and update)
func executeHandler(operation operation, r *Reconciler, instance *v1alpha1.DatadogGenericCR) error {
	key := apiHandlerKey{resourceType: instance.Spec.Type, op: operation}
	if handler, found := apiHandlers[key]; found {
		return handler(r, instance)
	}
	return unsupportedInstanceType(instance)
}

// Create is handled separately due to the dynamic signature and need to extract/update status based on the returned struct
type createHandlerFunc func(r *Reconciler, logger logr.Logger, instance *v1alpha1.DatadogGenericCR, status *v1alpha1.DatadogGenericCRStatus, now metav1.Time, hash string) error

var createHandlers = map[v1alpha1.SupportedResourcesType]createHandlerFunc{
	v1alpha1.SyntheticsBrowserTest: func(r *Reconciler, logger logr.Logger, instance *v1alpha1.DatadogGenericCR, status *v1alpha1.DatadogGenericCRStatus, now metav1.Time, hash string) error {
		createdTest, err := createSyntheticBrowserTest(r.datadogAuth, r.datadogSyntheticsClient, instance)
		if err != nil {
			logger.Error(err, "error creating browser test")
			updateErrStatus(status, now, v1alpha1.DatadogSyncStatusCreateError, "CreatingCustomResource", err)
			return err
		}
		status.Id = createdTest.GetPublicId()
		createdTimeString := createdTest.AdditionalProperties["created_at"].(string)
		createdTimeParsed, err := time.Parse(time.RFC3339, createdTimeString)
		if err != nil {
			logger.Error(err, "error parsing created time")
			createdTimeParsed = time.Now()
		}
		createdTime := metav1.NewTime(createdTimeParsed)
		status.Created = &createdTime
		status.LastForceSyncTime = &createdTime
		status.Creator = createdTest.AdditionalProperties["created_by"].(map[string]interface{})["handle"].(string)
		status.SyncStatus = v1alpha1.DatadogSyncStatusOK
		status.CurrentHash = hash
		return nil
	},
	v1alpha1.Notebook: func(r *Reconciler, logger logr.Logger, instance *v1alpha1.DatadogGenericCR, status *v1alpha1.DatadogGenericCRStatus, now metav1.Time, hash string) error {
		createdNotebook, err := createNotebook(r.datadogAuth, r.datadogNotebooksClient, instance)
		if err != nil {
			logger.Error(err, "error creating notebook")
			updateErrStatus(status, now, v1alpha1.DatadogSyncStatusCreateError, "CreatingCustomResource", err)
			return err
		}
		logger.Info("created a new notebook", "notebook Id", createdNotebook.Data.GetId())
		status.Id = notebookInt64ToString(createdNotebook.Data.GetId())
		createdTime := metav1.NewTime(*createdNotebook.Data.GetAttributes().Created)
		status.Created = &createdTime
		status.LastForceSyncTime = &createdTime
		status.Creator = *createdNotebook.Data.GetAttributes().Author.Handle
		status.SyncStatus = v1alpha1.DatadogSyncStatusOK
		status.CurrentHash = hash
		return nil
	},
	mockSubresource: func(r *Reconciler, logger logr.Logger, instance *v1alpha1.DatadogGenericCR, status *v1alpha1.DatadogGenericCRStatus, now metav1.Time, hash string) error {
		status.Id = "mock-id"
		status.Created = &now
		status.LastForceSyncTime = &now
		status.Creator = "mock-creator"
		status.SyncStatus = v1alpha1.DatadogSyncStatusOK
		status.CurrentHash = hash
		return nil
	},
}

func executeCreateHandler(r *Reconciler, logger logr.Logger, instance *v1alpha1.DatadogGenericCR, status *v1alpha1.DatadogGenericCRStatus, now metav1.Time, hash string) error {
	if handler, found := createHandlers[instance.Spec.Type]; found {
		return handler(r, logger, instance, status, now, hash)
	}
	return unsupportedInstanceType(instance)
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

func unsupportedInstanceType(instance *v1alpha1.DatadogGenericCR) error {
	return fmt.Errorf("unsupported type: %s", instance.Spec.Type)
}

func apiDelete(r *Reconciler, instance *v1alpha1.DatadogGenericCR) error {
	return executeHandler(operationDelete, r, instance)
}

func apiGet(r *Reconciler, instance *v1alpha1.DatadogGenericCR) error {
	return executeHandler(operationGet, r, instance)
}

func apiUpdate(r *Reconciler, instance *v1alpha1.DatadogGenericCR) error {
	return executeHandler(operationUpdate, r, instance)
}

func apiCreateAndUpdateStatus(r *Reconciler, logger logr.Logger, instance *v1alpha1.DatadogGenericCR, status *v1alpha1.DatadogGenericCRStatus, now metav1.Time, hash string) error {
	return executeCreateHandler(r, logger, instance, status, now, hash)
}
