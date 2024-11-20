package datadoggenericcrd

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
	operationDelete operation = "delete"
	operationGet    operation = "get"
	operationUpdate operation = "update"
)

type apiHandlerKey struct {
	Type string
	op   operation
}

// Delete, Get and Update operations share the same signature
type apiHandlerFunc func(r *Reconciler, instance *v1alpha1.DatadogGenericCRD) error

var apiHandlers = map[apiHandlerKey]apiHandlerFunc{
	{"synthetics_browser_test", operationDelete}: func(r *Reconciler, instance *v1alpha1.DatadogGenericCRD) error {
		return deleteSyntheticTest(r.datadogAuth, r.datadogSyntheticsClient, instance.Status.ID)
	},
	{"notebook", operationDelete}: func(r *Reconciler, instance *v1alpha1.DatadogGenericCRD) error {
		return deleteNotebook(r.datadogAuth, r.datadogNotebooksClient, instance.Status.ID)
	},
	{"synthetics_browser_test", operationGet}: func(r *Reconciler, instance *v1alpha1.DatadogGenericCRD) error {
		_, err := getSyntheticsTest(r.datadogAuth, r.datadogSyntheticsClient, instance.Status.ID)
		return err
	},
	{"notebook", operationGet}: func(r *Reconciler, instance *v1alpha1.DatadogGenericCRD) error {
		_, err := getNotebook(r.datadogAuth, r.datadogNotebooksClient, instance.Status.ID)
		return err
	},
	{"synthetics_browser_test", operationUpdate}: func(r *Reconciler, instance *v1alpha1.DatadogGenericCRD) error {
		_, err := updateSyntheticsBrowserTest(r.datadogAuth, r.datadogSyntheticsClient, instance)
		return err
	},
	{"notebook", operationUpdate}: func(r *Reconciler, instance *v1alpha1.DatadogGenericCRD) error {
		_, err := updateNotebook(r.datadogAuth, r.datadogNotebooksClient, instance)
		return err
	},
}

// Common handler executor (delete, get and update)
func executeHandler(operation operation, r *Reconciler, instance *v1alpha1.DatadogGenericCRD) error {
	key := apiHandlerKey{Type: instance.Spec.Type, op: operation}
	if handler, exists := apiHandlers[key]; exists {
		return handler(r, instance)
	}
	return unsupportedInstanceType(instance)
}

// Create is handled separately due to the dynamic signature and need to extract/update status based on the returned struct
type createHandlerFunc func(r *Reconciler, logger logr.Logger, instance *v1alpha1.DatadogGenericCRD, status *v1alpha1.DatadogGenericCRDStatus, now metav1.Time, hash string) error

var createHandlers = map[string]createHandlerFunc{
	"synthetics_browser_test": func(r *Reconciler, logger logr.Logger, instance *v1alpha1.DatadogGenericCRD, status *v1alpha1.DatadogGenericCRDStatus, now metav1.Time, hash string) error {
		createdTest, err := createSyntheticBrowserTest(r.datadogAuth, r.datadogSyntheticsClient, instance)
		if err != nil {
			logger.Error(err, "error creating browser test")
			updateErrStatus(status, now, v1alpha1.DatadogSyncStatusCreateError, "CreatingCustomResource", err)
			return err
		}
		status.ID = createdTest.GetPublicId()
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
	"notebook": func(r *Reconciler, logger logr.Logger, instance *v1alpha1.DatadogGenericCRD, status *v1alpha1.DatadogGenericCRDStatus, now metav1.Time, hash string) error {
		createdNotebook, err := createNotebook(r.datadogAuth, r.datadogNotebooksClient, instance)
		if err != nil {
			logger.Error(err, "error creating notebook")
			updateErrStatus(status, now, v1alpha1.DatadogSyncStatusCreateError, "CreatingCustomResource", err)
			return err
		}
		logger.Info("created a new notebook", "notebook ID", createdNotebook.Data.GetId())
		status.ID = notebookInt64ToString(createdNotebook.Data.GetId())
		createdTime := metav1.NewTime(*createdNotebook.Data.GetAttributes().Created)
		status.Created = &createdTime
		status.LastForceSyncTime = &createdTime
		status.Creator = *createdNotebook.Data.GetAttributes().Author.Handle
		status.SyncStatus = v1alpha1.DatadogSyncStatusOK
		status.CurrentHash = hash
		return nil
	},
}

func executeCreateHandler(r *Reconciler, logger logr.Logger, instance *v1alpha1.DatadogGenericCRD, status *v1alpha1.DatadogGenericCRDStatus, now metav1.Time, hash string) error {
	if handler, exists := createHandlers[instance.Spec.Type]; exists {
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

// TODO: add validation on the DatadogGenericCRD type so we can't encounter unsupported types
func unsupportedInstanceType(instance *v1alpha1.DatadogGenericCRD) error {
	return fmt.Errorf("unsupported type: %s", instance.Spec.Type)
}

func apiDelete(r *Reconciler, instance *v1alpha1.DatadogGenericCRD) error {
	return executeHandler(operationDelete, r, instance)
}

func apiGet(r *Reconciler, instance *v1alpha1.DatadogGenericCRD) error {
	return executeHandler(operationGet, r, instance)
}

func apiUpdate(r *Reconciler, instance *v1alpha1.DatadogGenericCRD) error {
	return executeHandler(operationUpdate, r, instance)
}

func apiCreateAndUpdateStatus(r *Reconciler, logger logr.Logger, instance *v1alpha1.DatadogGenericCRD, status *v1alpha1.DatadogGenericCRDStatus, now metav1.Time, hash string) error {
	return executeCreateHandler(r, logger, instance, status, now, hash)
}
