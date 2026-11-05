package datadoggenericresource

import (
	"context"
	"encoding/json"
	"net/http"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/utils"
)

type MonitorHandler struct {
	client *datadogV1.MonitorsApi
}

func (h *MonitorHandler) createResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) (CreateResult, error) {
	createdMonitor, err := createMonitor(auth, h.client, instance)
	if err != nil {
		return CreateResult{}, err
	}
	createdTime := metav1.NewTime(*createdMonitor.Created)
	return CreateResult{
		ID:          resourceInt64ToStringID(createdMonitor.GetId()),
		CreatedTime: &createdTime,
		Creator:     *createdMonitor.GetCreator().Handle,
	}, nil
}

func (h *MonitorHandler) getResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error {
	_, err := getMonitor(auth, h.client, instance.Status.Id)
	return err
}

// remoteMatchesSpec compares the effective update payload. Decoding into the same
// zero-value request type as updateMonitor preserves supplied fields without SDK
// constructor defaults, and omits values (such as tags:null) the SDK cannot send.
func (h *MonitorHandler) remoteMatchesSpec(auth context.Context, instance *v1alpha1.DatadogGenericResource) (bool, error) {
	desired, parseErr := buildMonitorUpdateRequest(instance)
	comparisonClient := h.client
	if parseErr == nil {
		comparisonClient = monitorComparisonClient(h.client, desired)
	}
	monitor, err := getMonitor(auth, comparisonClient, instance.Status.Id, *datadogV1.NewGetMonitorOptionalParameters().WithWithAssets(true))
	if err != nil {
		return false, err
	}
	if parseErr != nil {
		// Let the update path report invalid specs and apply its error backoff.
		return false, nil //nolint:nilerr // Parse errors must follow the existing update error path.
	}
	matches, compareErr := utils.MonitorMatchesSpec(desired, monitor)
	// An inconclusive comparison must not suppress an update.
	return compareErr == nil && matches, nil
}

// The SDK exposes assets as a GET option, but restriction_policy is still an
// additional JSON property and its GET option is not exposed. A scoped client
// adds that option while preserving SDK authentication, retries, and decoding.
// Never change the shared client/configuration during a reconciliation.
func monitorComparisonClient(api *datadogV1.MonitorsApi, desired *datadogV1.MonitorUpdateRequest) *datadogV1.MonitorsApi {
	_, withPolicy := desired.AdditionalProperties["restriction_policy"]
	_, withUnparsedPolicy := desired.UnparsedObject["restriction_policy"]
	if !withPolicy && !withUnparsedPolicy {
		return api
	}
	cfg := *api.Client.GetConfig()
	httpClient := *cfg.HTTPClient
	transport := httpClient.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	httpClient.Transport = monitorRestrictionPolicyTransport{base: transport}
	cfg.HTTPClient = &httpClient
	return datadogV1.NewMonitorsApi(datadogapi.NewAPIClient(&cfg))
}

type monitorRestrictionPolicyTransport struct {
	base http.RoundTripper
}

func (t monitorRestrictionPolicyTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	request = request.Clone(request.Context())
	query := request.URL.Query()
	query.Set("with_restriction_policy", "true")
	request.URL.RawQuery = query.Encode()
	return t.base.RoundTrip(request)
}

func (h *MonitorHandler) updateResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error {
	_, err := updateMonitor(auth, h.client, instance)
	return err
}
func (h *MonitorHandler) deleteResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error {
	return deleteMonitor(auth, h.client, instance.Status.Id)
}

// refreshState fetches the live Datadog-side state of the monitor.
// The returned value mirrors the overall state reported by Datadog
// (OK, Alert, Warn, No Data, Skipped, Ignored, Unknown). Per-group TriggeredState
// is intentionally not surfaced in DDGR (see internal/controller/datadogmonitor/
// controller.go::convertStateToStatus for the per-group detail available on the
// dedicated DatadogMonitor CRD).
func (h *MonitorHandler) refreshState(auth context.Context, instance *v1alpha1.DatadogGenericResource) (*string, error) {
	m, err := getMonitor(auth, h.client, instance.Status.Id)
	if err != nil {
		return nil, err
	}
	state := string(m.GetOverallState())
	return &state, nil
}

func getMonitor(auth context.Context, client *datadogV1.MonitorsApi, monitorStringID string, options ...datadogV1.GetMonitorOptionalParameters) (datadogV1.Monitor, error) {
	monitorID, err := resourceStringToInt64ID(monitorStringID)
	if err != nil {
		return datadogV1.Monitor{}, err
	}
	monitor, httpResp, err := client.GetMonitor(auth, monitorID, options...)
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return datadogV1.Monitor{}, translateClientError(err, httpResp, "error getting monitor")
	}
	return monitor, nil
}

func deleteMonitor(auth context.Context, client *datadogV1.MonitorsApi, monitorStringID string) error {
	monitorID, err := resourceStringToInt64ID(monitorStringID)
	if err != nil {
		return err
	}
	_, httpResponse, err := client.DeleteMonitor(auth, monitorID)
	if httpResponse != nil {
		defer httpResponse.Body.Close()
	}
	if err != nil {
		// Deletion is idempotent for finalization: if the monitor was already removed
		// in Datadog (for example from the UI), allow the Kubernetes finalizer to clear.
		// Retry other errors (e.g. 400, 401, 429, 5XX).
		if httpResponse != nil && httpResponse.StatusCode == 404 {
			return nil
		}
		return translateClientError(err, httpResponse, "error deleting monitor")
	}
	return nil
}

func createMonitor(auth context.Context, client *datadogV1.MonitorsApi, instance *v1alpha1.DatadogGenericResource) (datadogV1.Monitor, error) {
	monitorBody := &datadogV1.Monitor{}
	if err := json.Unmarshal([]byte(instance.Spec.JsonSpec), monitorBody); err != nil {
		return datadogV1.Monitor{}, translateUnmarshalError(err, "error unmarshalling monitor spec")
	}
	monitor, httpResp, err := client.CreateMonitor(auth, *monitorBody)
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return datadogV1.Monitor{}, translateClientError(err, httpResp, "error creating monitor")
	}
	return monitor, nil
}

func buildMonitorUpdateRequest(instance *v1alpha1.DatadogGenericResource) (*datadogV1.MonitorUpdateRequest, error) {
	monitorUpdateData := &datadogV1.MonitorUpdateRequest{}
	if err := json.Unmarshal([]byte(instance.Spec.JsonSpec), monitorUpdateData); err != nil {
		return nil, translateUnmarshalError(err, "error unmarshalling monitor spec")
	}
	return monitorUpdateData, nil
}

func updateMonitor(auth context.Context, client *datadogV1.MonitorsApi, instance *v1alpha1.DatadogGenericResource) (datadogV1.Monitor, error) {
	monitorUpdateData, err := buildMonitorUpdateRequest(instance)
	if err != nil {
		return datadogV1.Monitor{}, err
	}
	monitorID, err := resourceStringToInt64ID(instance.Status.Id)
	if err != nil {
		return datadogV1.Monitor{}, err
	}
	monitorUpdated, httpResp, err := client.UpdateMonitor(auth, monitorID, *monitorUpdateData)
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return datadogV1.Monitor{}, translateClientError(err, httpResp, "error updating monitor")
	}
	return monitorUpdated, nil
}
