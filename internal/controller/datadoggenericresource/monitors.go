package datadoggenericresource

import (
	"context"
	"encoding/json"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/utils"
	ctrutils "github.com/DataDog/datadog-operator/pkg/controller/utils"
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
	_, err := h.read(auth, instance, instance.Status.Etag)
	return err
}

func (h *MonitorHandler) readRemoteDefinition(auth context.Context, instance *v1alpha1.DatadogGenericResource) (remoteRead, error) {
	return h.read(auth, instance, instance.Status.Etag)
}

// read reads the monitor, sending etag in If-None-Match when it is not empty.
// Force syncs and state polls use the same request, so the ETag of one can
// match the other.
func (h *MonitorHandler) read(auth context.Context, instance *v1alpha1.DatadogGenericResource, etag string) (remoteRead, error) {
	read, err := getMonitor(auth, h.client, instance.Status.Id, etag)
	if err != nil {
		return remoteRead{}, err
	}
	if read.NotModified {
		return remoteRead{ETag: read.ETag, NotModified: true}, nil
	}
	state := string(read.Monitor.GetOverallState())
	return remoteRead{
		Hash:  monitorDefinitionHash(instance, read.Monitor),
		ETag:  read.ETag,
		State: &state,
	}, nil
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
// spec is not part of the monitor definition that the hash covers, and supplied
// fields missing from the response cannot be monitored for drift.
func monitorDefinitionHash(instance *v1alpha1.DatadogGenericResource, monitor datadogV1.Monitor) string {
	var spec map[string]json.RawMessage
	if err := json.Unmarshal([]byte(instance.Spec.JsonSpec), &spec); err != nil {
		return ""
	}
	if _, ok := spec["restriction_policy"]; ok {
		return ""
	}
	data, err := json.Marshal(monitor)
	if err != nil {
		return ""
	}
	var remoteFields map[string]json.RawMessage
	if err = json.Unmarshal(data, &remoteFields); err != nil {
		return ""
	}
	// The SDK preserves unknown properties, so jsonSpec can manage fields that
	// are not in the shared definition list, even without an SDK upgrade.
	// Response-only fields such as "state" are deliberately not filtered out:
	// hashing one only costs extra updates, while a filter list that wrongly
	// excluded a writable field would hide drift. The docs ask users to keep
	// response-only fields out of jsonSpec.
	fields := []string{"assets"}
	for field := range spec {
		if _, returned := remoteFields[field]; !returned {
			return ""
		}
		fields = append(fields, field)
	}
	hash, err := utils.MonitorDefinitionHash(monitor, fields...)
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
func (h *MonitorHandler) refreshState(auth context.Context, instance *v1alpha1.DatadogGenericResource) (remoteRead, error) {
	// Without a polled state, for example after a create or while polling was
	// disabled, a Not Modified response would leave the state unknown.
	etag := instance.Status.Etag
	if instance.Status.State == "" || instance.Status.State == ctrutils.StatePollingDisabledState {
		etag = ""
	}
	return h.read(auth, instance, etag)
}

// getMonitor reads a monitor with its assets, which are part of the hashed
// definition and are only returned on request. It reports a monitor that has
// not changed since the read that returned etag as NotModified.
func getMonitor(auth context.Context, client *datadogV1.MonitorsApi, monitorStringID string, etag string) (utils.MonitorRead, error) {
	monitorID, err := resourceStringToInt64ID(monitorStringID)
	if err != nil {
		return utils.MonitorRead{}, err
	}
	params := *datadogV1.NewGetMonitorOptionalParameters().WithWithAssets(true)
	read, httpResp, err := utils.GetMonitorIfChanged(auth, client, monitorID, etag, params)
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return utils.MonitorRead{}, translateClientError(err, httpResp, "error getting monitor")
	}
	return read, nil
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
