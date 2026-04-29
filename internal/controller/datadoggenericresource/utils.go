package datadoggenericresource

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/datadogclient"
)

// buildHandlers creates a handler for each supported resource type, each holding
// its own API client. Auth is passed per-call via the ResourceHandler methods.
func buildHandlers(clients *datadogclient.GenericClients) map[v1alpha1.SupportedResourcesType]ResourceHandler {
	return map[v1alpha1.SupportedResourcesType]ResourceHandler{
		v1alpha1.Dashboard:             &DashboardHandler{client: clients.DashboardsClient},
		v1alpha1.Downtime:              &DowntimeHandler{client: clients.DowntimesClient},
		v1alpha1.Monitor:               &MonitorHandler{client: clients.MonitorsClient},
		v1alpha1.Notebook:              &NotebookHandler{client: clients.NotebooksClient},
		v1alpha1.SyntheticsAPITest:     &SyntheticsAPITestHandler{client: clients.SyntheticsClient},
		v1alpha1.SyntheticsBrowserTest: &SyntheticsBrowserTestHandler{client: clients.SyntheticsClient},
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
