package datadoggenericresource

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
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
	case v1alpha1.Downtime:
		return &DowntimeHandler{}
	case v1alpha1.Monitor:
		return &MonitorHandler{}
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

// resourceStringToInt64ID converts a string ID to an int64 ID
func resourceStringToInt64ID(resourceStringID string) (int64, error) {
	int64ID, err := strconv.ParseInt(resourceStringID, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("error parsing resource ID: %w", err)
	}
	return int64ID, nil
}

// resourceInt64ToStringID converts an int64 ID to a string ID
// This is used to store the ID in the status (some resources use int64 IDs while others use string IDs)
func resourceInt64ToStringID(resourceInt64ID int64) string {
	return strconv.FormatInt(resourceInt64ID, 10)
}
