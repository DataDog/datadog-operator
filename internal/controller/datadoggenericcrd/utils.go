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
	switch instance.Spec.Type {
	case "synthetics_browser_test":
		return deleteSyntheticTest(r.datadogAuth, r.datadogSyntheticsClient, instance.Status.ID)
	case "notebook":
		return deleteNotebook(r.datadogAuth, r.datadogNotebooksClient, instance.Status.ID)
	}
	return unsupportedInstanceType(instance)
}

func apiGet(r *Reconciler, instance *v1alpha1.DatadogGenericCRD) error {
	switch instance.Spec.Type {
	case "synthetics_browser_test":
		_, err := getSyntheticsTest(r.datadogAuth, r.datadogSyntheticsClient, instance.Status.ID)
		return err
	case "notebook":
		_, err := getNotebook(r.datadogAuth, r.datadogNotebooksClient, instance.Status.ID)
		return err
	}
	return unsupportedInstanceType(instance)
}

func apiUpdate(r *Reconciler, instance *v1alpha1.DatadogGenericCRD) error {
	switch instance.Spec.Type {
	case "synthetics_browser_test":
		_, err := updateSyntheticsBrowserTest(r.datadogAuth, r.datadogSyntheticsClient, instance)
		// err will be nil if successful
		return err
	case "notebook":
		_, err := updateNotebook(r.datadogAuth, r.datadogNotebooksClient, instance)
		// err will be nil if successful
		return err
	}
	// Default if we do not exit previously
	return unsupportedInstanceType(instance)
}

func apiCreateAndUpdateStatus(r *Reconciler, logger logr.Logger, instance *v1alpha1.DatadogGenericCRD, status *v1alpha1.DatadogGenericCRDStatus, now metav1.Time, hash string) error {
	switch instance.Spec.Type {
	case "synthetics_browser_test":
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
	case "notebook":
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
	}
	// Default if we do not exit previously
	return unsupportedInstanceType(instance)
}
