package datadoggenericresource

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

// testHandlers allows tests to register additional handlers without coupling
// production code to test-only types.
var testHandlers map[v1alpha1.SupportedResourcesType]ResourceHandler

func apiDelete(r *Reconciler, instance *v1alpha1.DatadogGenericResource) error {
	return getHandler(instance.Spec.Type).deleteResourcefunc(r, instance)
}

func apiGet(r *Reconciler, instance *v1alpha1.DatadogGenericResource) error {
	return getHandler(instance.Spec.Type).getResourcefunc(r, instance)
}

func apiUpdate(r *Reconciler, instance *v1alpha1.DatadogGenericResource) error {
	return getHandler(instance.Spec.Type).updateResourcefunc(r, instance)
}

func apiCreateAndUpdateStatus(r *Reconciler, ctx context.Context, instance *v1alpha1.DatadogGenericResource, status *v1alpha1.DatadogGenericResourceStatus, now metav1.Time, hash string) error {
	result, err := getHandler(instance.Spec.Type).createResourcefunc(r, instance)
	if err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "error creating resource", "type", instance.Spec.Type)
		updateErrStatus(status, now, v1alpha1.DatadogSyncStatusCreateError, "CreatingCustomResource", err)
		return err
	}
	createdTime := result.CreatedTime
	if createdTime == nil {
		createdTime = &now
	}
	status.Id = result.ID
	status.Created = createdTime
	status.LastForceSyncTime = createdTime
	status.Creator = result.Creator
	status.SyncStatus = v1alpha1.DatadogSyncStatusOK
	status.CurrentHash = hash
	return nil
}

func getHandler(resourceType v1alpha1.SupportedResourcesType) ResourceHandler {
	if h, ok := testHandlers[resourceType]; ok {
		return h
	}
	switch resourceType {
	case v1alpha1.Dashboard:
		return &DashboardHandler{}
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
