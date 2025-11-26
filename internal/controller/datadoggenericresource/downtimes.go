package datadoggenericresource

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

type DowntimeHandler struct{}

func (h *DowntimeHandler) createResourcefunc(r *Reconciler, logger logr.Logger, instance *v1alpha1.DatadogGenericResource, status *v1alpha1.DatadogGenericResourceStatus, now metav1.Time, hash string) error {
	createdDowntime, err := createDowntime(r.datadogAuth, r.datadogDowntimesClient, instance)
	if err != nil {
		logger.Error(err, "error creating downtime")
		updateErrStatus(status, now, v1alpha1.DatadogSyncStatusCreateError, "CreatingCustomResource", err)
		return err
	}
	logger.Info("created a new downtime", "downtime Id", createdDowntime.Data.GetId())
	status.Id = createdDowntime.Data.GetId()

	// Extract created time from attributes
	if createdDowntime.Data.Attributes != nil && createdDowntime.Data.Attributes.Created != nil {
		createdTime := metav1.NewTime(*createdDowntime.Data.Attributes.Created)
		status.Created = &createdTime
		status.LastForceSyncTime = &createdTime
	} else {
		status.Created = &now
		status.LastForceSyncTime = &now
	}

	// Extract creator from relationships
	if createdDowntime.Data.Relationships != nil &&
		createdDowntime.Data.Relationships.CreatedBy != nil &&
		createdDowntime.Data.Relationships.CreatedBy.Data.IsSet() {
		createdByData := createdDowntime.Data.Relationships.CreatedBy.Data.Get()
		if createdByData != nil {
			status.Creator = createdByData.GetId()
		} else {
			status.Creator = "unknown"
		}
	} else {
		status.Creator = "unknown"
	}

	status.SyncStatus = v1alpha1.DatadogSyncStatusOK
	status.CurrentHash = hash
	return nil
}

func (h *DowntimeHandler) getResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericResource) error {
	_, err := getDowntime(r.datadogAuth, r.datadogDowntimesClient, instance.Status.Id)
	return err
}

func (h *DowntimeHandler) updateResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericResource) error {
	_, err := updateDowntime(r.datadogAuth, r.datadogDowntimesClient, instance)
	return err
}

func (h *DowntimeHandler) deleteResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericResource) error {
	return deleteDowntime(r.datadogAuth, r.datadogDowntimesClient, instance.Status.Id)
}

func getDowntime(auth context.Context, client *datadogV2.DowntimesApi, downtimeID string) (datadogV2.DowntimeResponse, error) {
	if downtimeID == "" {
		return datadogV2.DowntimeResponse{}, fmt.Errorf("cannot get downtime: downtimeID is empty")
	}
	downtime, _, err := client.GetDowntime(auth, downtimeID)
	if err != nil {
		return datadogV2.DowntimeResponse{}, translateClientError(err, "error getting downtime")
	}
	return downtime, nil
}

func deleteDowntime(auth context.Context, client *datadogV2.DowntimesApi, downtimeID string) error {
	if downtimeID == "" {
		return fmt.Errorf("cannot delete downtime: downtimeID is empty")
	}
	// Note: Downtimes canceled through the API are no longer active, but are retained for approximately two days before being permanently removed.
	// The downtime may still appear in search results until it is permanently removed.
	if _, err := client.CancelDowntime(auth, downtimeID); err != nil {
		return translateClientError(err, "error deleting downtime")
	}
	return nil
}

func createDowntime(auth context.Context, client *datadogV2.DowntimesApi, instance *v1alpha1.DatadogGenericResource) (datadogV2.DowntimeResponse, error) {
	if instance.Spec.JsonSpec == "" {
		return datadogV2.DowntimeResponse{}, fmt.Errorf("cannot create downtime: spec.jsonSpec is empty")
	}

	downtimeBody := &datadogV2.DowntimeCreateRequest{}
	if err := json.Unmarshal([]byte(instance.Spec.JsonSpec), downtimeBody); err != nil {
		return datadogV2.DowntimeResponse{}, translateClientError(err, "error unmarshalling downtime spec")
	}

	downtime, _, err := client.CreateDowntime(auth, *downtimeBody)
	if err != nil {
		return datadogV2.DowntimeResponse{}, translateClientError(err, "error creating downtime")
	}
	return downtime, nil
}

func updateDowntime(auth context.Context, client *datadogV2.DowntimesApi, instance *v1alpha1.DatadogGenericResource) (datadogV2.DowntimeResponse, error) {
	// Validate the presence of a target downtime ID
	if instance.Status.Id == "" {
		return datadogV2.DowntimeResponse{}, fmt.Errorf("cannot update downtime: status.id is empty")
	}

	if instance.Spec.JsonSpec == "" {
		return datadogV2.DowntimeResponse{}, fmt.Errorf("cannot update downtime: spec.jsonSpec is empty")
	}

	// Unmarshal the user-provided spec into the typed update request to preserve field fidelity
	var updateReq datadogV2.DowntimeUpdateRequest
	if err := json.Unmarshal([]byte(instance.Spec.JsonSpec), &updateReq); err != nil {
		return datadogV2.DowntimeResponse{}, translateClientError(err, "error unmarshalling downtime spec")
	}

	// Enforce correct resource type and ID regardless of what's in the spec
	// This prevents accidental cross-resource updates if the spec contains incorrect values
	updateReq.Data.Type = datadogV2.DOWNTIMERESOURCETYPE_DOWNTIME
	updateReq.Data.Id = instance.Status.Id

	// Call update using the status ID as the path parameter
	downtimeUpdated, _, err := client.UpdateDowntime(auth, instance.Status.Id, updateReq)
	if err != nil {
		return datadogV2.DowntimeResponse{}, translateClientError(err, "error updating downtime")
	}
	return downtimeUpdated, nil
}
