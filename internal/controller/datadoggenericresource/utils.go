package datadoggenericresource

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

// mockSubresource is used to mock the subresource in tests
const mockSubresource = "mock_resource"

type MockHandler struct{}

func (h *MockHandler) createResourcefunc(r *Reconciler, logger logr.Logger, instance *v1alpha1.DatadogGenericResource, status *v1alpha1.DatadogGenericResourceStatus, now metav1.Time, hash string) error {
	status.Id = "mock-id"
	status.Created = &now
	status.LastForceSyncTime = &now
	status.Creator = "mock-creator"
	status.SyncStatus = v1alpha1.DatadogSyncStatusOK
	status.CurrentHash = hash
	return nil
}

func (h *MockHandler) getResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericResource) error {
	return nil
}
func (h *MockHandler) updateResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericResource) error {
	return nil
}
func (h *MockHandler) deleteResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericResource) error {
	return nil
}

func apiDelete(r *Reconciler, instance *v1alpha1.DatadogGenericResource) error {
	return getHandler(instance.Spec.Type).deleteResourcefunc(r, instance)
}

func apiGet(r *Reconciler, instance *v1alpha1.DatadogGenericResource) error {
	return getHandler(instance.Spec.Type).getResourcefunc(r, instance)
}

func apiUpdate(r *Reconciler, instance *v1alpha1.DatadogGenericResource) error {
	return getHandler(instance.Spec.Type).updateResourcefunc(r, instance)
}

func apiCreateAndUpdateStatus(r *Reconciler, logger logr.Logger, instance *v1alpha1.DatadogGenericResource, status *v1alpha1.DatadogGenericResourceStatus, now metav1.Time, hash string) error {
	return getHandler(instance.Spec.Type).createResourcefunc(r, logger, instance, status, now, hash)
}

func getHandler(resourceType v1alpha1.SupportedResourcesType) ResourceHandler {
	switch resourceType {
	case v1alpha1.Notebook:
		return &NotebookHandler{}
	case v1alpha1.SyntheticsAPITest:
		return &SyntheticsAPITestHandler{}
	case v1alpha1.SyntheticsBrowserTest:
		return &SyntheticsBrowserTestHandler{}
	case mockSubresource:
		return &MockHandler{}
	default:
		panic(unsupportedInstanceType(resourceType))
	}
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

func unsupportedInstanceType(resourceType v1alpha1.SupportedResourcesType) error {
	return fmt.Errorf("unsupported type: %s", resourceType)
}

func updateStatusFromSyntheticsTest(createdTest interface{ GetPublicId() string }, additionalProperties map[string]interface{}, status *v1alpha1.DatadogGenericResourceStatus, logger logr.Logger, hash string) error {
	// All synthetic test types share this method
	status.Id = createdTest.GetPublicId()

	// Parse Created Time
	createdTimeString, ok := additionalProperties["created_at"].(string)
	if !ok {
		logger.Error(nil, "missing or invalid created_at field, using current time")
		createdTimeString = time.Now().Format(time.RFC3339)
	}

	createdTimeParsed, err := time.Parse(time.RFC3339, createdTimeString)
	if err != nil {
		logger.Error(err, "error parsing created time, using current time")
		createdTimeParsed = time.Now()
	}
	createdTime := metav1.NewTime(createdTimeParsed)

	// Update status fields
	status.Created = &createdTime
	status.LastForceSyncTime = &createdTime

	// Update Creator
	if createdBy, ok := additionalProperties["created_by"].(map[string]interface{}); ok {
		if handle, ok := createdBy["handle"].(string); ok {
			status.Creator = handle
		} else {
			logger.Error(nil, "missing handle field in created_by")
			status.Creator = ""
		}
	} else {
		logger.Error(nil, "missing or invalid created_by field")
		status.Creator = ""
	}

	// Update Sync Status and Hash
	status.SyncStatus = v1alpha1.DatadogSyncStatusOK
	status.CurrentHash = hash

	return nil
}
