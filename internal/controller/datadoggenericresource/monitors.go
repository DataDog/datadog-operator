package datadoggenericresource

import (
	"context"
	"encoding/json"

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
		ID:                   resourceInt64ToStringID(createdMonitor.GetId()),
		CreatedTime:          &createdTime,
		Creator:              *createdMonitor.GetCreator().Handle,
		RemoteDefinitionHash: monitorDefinitionHash(instance, createdMonitor),
	}, nil
}

func (h *MonitorHandler) getResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error {
	_, err := getMonitor(auth, h.client, instance.Status.Id)
	return err
}

func (h *MonitorHandler) getRemoteDefinitionHash(auth context.Context, instance *v1alpha1.DatadogGenericResource) (string, error) {
	// Assets are part of the hashed definition and are only returned on request.
	monitor, err := getMonitor(auth, h.client, instance.Status.Id, *datadogV1.NewGetMonitorOptionalParameters().WithWithAssets(true))
	if err != nil {
		return "", err
	}
	return monitorDefinitionHash(instance, monitor), nil
}

func (h *MonitorHandler) updateResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error {
	_, err := updateMonitor(auth, h.client, instance)
	return err
}

func (h *MonitorHandler) updateResourceWithHash(auth context.Context, instance *v1alpha1.DatadogGenericResource) (string, error) {
	monitor, err := updateMonitor(auth, h.client, instance)
	if err != nil {
		return "", err
	}
	return monitorDefinitionHash(instance, monitor), nil
}

// monitorDefinitionHash returns the definition hash of a monitor returned by the
// API, or "" when force syncs must keep updating it: a restriction policy in the
// spec is not part of the monitor definition that the hash covers.
func monitorDefinitionHash(instance *v1alpha1.DatadogGenericResource, monitor datadogV1.Monitor) string {
	var spec map[string]json.RawMessage
	if err := json.Unmarshal([]byte(instance.Spec.JsonSpec), &spec); err != nil {
		return ""
	}
	if _, ok := spec["restriction_policy"]; ok {
		return ""
	}
	hash, err := utils.MonitorDefinitionHash(monitor, "assets")
	if err != nil {
		return ""
	}
	return hash
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

func getMonitor(auth context.Context, client *datadogV1.MonitorsApi, monitorStringID string, params ...datadogV1.GetMonitorOptionalParameters) (datadogV1.Monitor, error) {
	monitorID, err := resourceStringToInt64ID(monitorStringID)
	if err != nil {
		return datadogV1.Monitor{}, err
	}
	monitor, httpResp, err := client.GetMonitor(auth, monitorID, params...)
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

func updateMonitor(auth context.Context, client *datadogV1.MonitorsApi, instance *v1alpha1.DatadogGenericResource) (datadogV1.Monitor, error) {
	monitorUpdateData := &datadogV1.MonitorUpdateRequest{}
	if err := json.Unmarshal([]byte(instance.Spec.JsonSpec), monitorUpdateData); err != nil {
		return datadogV1.Monitor{}, translateUnmarshalError(err, "error unmarshalling monitor spec")
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
