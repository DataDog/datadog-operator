// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoggenericresource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

// SLOCorrectionHandler manages SLO Corrections through the datadogV1 SLO Corrections API.
type SLOCorrectionHandler struct {
	client *datadogV1.ServiceLevelObjectiveCorrectionsApi
}

// createResource creates an SLO correction and extracts its ID, creation time, and creator for the status.
func (h *SLOCorrectionHandler) createResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) (CreateResult, error) {
	created, err := createSLOCorrection(auth, h.client, instance)
	if err != nil {
		return CreateResult{}, err
	}

	var createdTime *metav1.Time
	if created.Data != nil && created.Data.Attributes != nil {
		if ts := created.Data.Attributes.CreatedAt.Get(); ts != nil {
			ct := metav1.NewTime(time.Unix(*ts, 0))
			createdTime = &ct
		}
	}

	creator := ""
	if created.Data != nil && created.Data.Attributes != nil && created.Data.Attributes.Creator != nil && created.Data.Attributes.Creator.Handle != nil {
		creator = *created.Data.Attributes.Creator.Handle
	}

	id := ""
	if created.Data != nil && created.Data.Id != nil {
		id = *created.Data.Id
	}

	return CreateResult{
		ID:          id,
		CreatedTime: createdTime,
		Creator:     creator,
	}, nil
}

// getResource fetches the SLO correction to confirm it still exists.
func (h *SLOCorrectionHandler) getResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error {
	_, err := getSLOCorrection(auth, h.client, instance.Status.Id)
	return err
}

// updateResource updates the SLO correction's attributes.
func (h *SLOCorrectionHandler) updateResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error {
	_, err := updateSLOCorrection(auth, h.client, instance)
	return err
}

// deleteResource deletes the SLO correction.
func (h *SLOCorrectionHandler) deleteResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error {
	return deleteSLOCorrection(auth, h.client, instance.Status.Id)
}

// refreshState is a no-op: SLO corrections expose no live state to poll.
func (h *SLOCorrectionHandler) refreshState(_ context.Context, _ *v1alpha1.DatadogGenericResource) (*string, error) {
	return nil, nil
}

// getSLOCorrection fetches an SLO correction by ID.
func getSLOCorrection(auth context.Context, client *datadogV1.ServiceLevelObjectiveCorrectionsApi, sloCorrectionID string) (datadogV1.SLOCorrectionResponse, error) {
	if sloCorrectionID == "" {
		return datadogV1.SLOCorrectionResponse{}, fmt.Errorf("cannot get SLO correction: sloCorrectionID is empty")
	}
	correction, _, err := client.GetSLOCorrection(auth, sloCorrectionID)
	if err != nil {
		return datadogV1.SLOCorrectionResponse{}, translateClientError(err, "error getting SLO correction")
	}
	return correction, nil
}

// deleteSLOCorrection deletes an SLO correction, treating a 404 as already-deleted so finalization is idempotent.
func deleteSLOCorrection(auth context.Context, client *datadogV1.ServiceLevelObjectiveCorrectionsApi, sloCorrectionID string) error {
	if sloCorrectionID == "" {
		return fmt.Errorf("cannot delete SLO correction: sloCorrectionID is empty")
	}
	httpResponse, err := client.DeleteSLOCorrection(auth, sloCorrectionID)
	if err != nil {
		// Deletion is idempotent for finalization: if the correction was already
		// removed in Datadog, allow the Kubernetes finalizer to clear.
		// Retry other errors (e.g. 400, 401, 429, 5XX).
		if httpResponse != nil && httpResponse.StatusCode == 404 {
			return nil
		}
		return translateClientError(err, "error deleting SLO correction")
	}
	return nil
}

// createSLOCorrection unmarshals the spec's data.attributes envelope and creates the SLO correction.
func createSLOCorrection(auth context.Context, client *datadogV1.ServiceLevelObjectiveCorrectionsApi, instance *v1alpha1.DatadogGenericResource) (datadogV1.SLOCorrectionResponse, error) {
	if instance.Spec.JsonSpec == "" {
		return datadogV1.SLOCorrectionResponse{}, fmt.Errorf("cannot create SLO correction: spec.jsonSpec is empty")
	}

	body := &datadogV1.SLOCorrectionCreateRequest{}
	if err := json.Unmarshal([]byte(instance.Spec.JsonSpec), body); err != nil {
		return datadogV1.SLOCorrectionResponse{}, translateClientError(err, "error unmarshalling SLO correction spec")
	}

	correction, _, err := client.CreateSLOCorrection(auth, *body)
	if err != nil {
		return datadogV1.SLOCorrectionResponse{}, translateClientError(err, "error creating SLO correction")
	}
	return correction, nil
}

// updateSLOCorrection updates the SLO correction's attributes; the ID travels only in the URL path, not the request body.
// - unmarshals the attributes to update, plus slo_id separately since the update API has no slo_id field
// - rejects the update if slo_id changed, since the API would otherwise silently keep the correction on its original SLO
func updateSLOCorrection(auth context.Context, client *datadogV1.ServiceLevelObjectiveCorrectionsApi, instance *v1alpha1.DatadogGenericResource) (datadogV1.SLOCorrectionResponse, error) {
	if instance.Status.Id == "" {
		return datadogV1.SLOCorrectionResponse{}, errors.New("cannot update SLO correction: status.id is empty")
	}

	if instance.Spec.JsonSpec == "" {
		return datadogV1.SLOCorrectionResponse{}, errors.New("cannot update SLO correction: spec.jsonSpec is empty")
	}

	// Unmarshal just the attributes portion from the user's spec.
	// ID is retrieved from the status and travels only in the URL path.
	var specData struct {
		Data struct {
			Attributes *datadogV1.SLOCorrectionUpdateRequestAttributes `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(instance.Spec.JsonSpec), &specData); err != nil {
		return datadogV1.SLOCorrectionResponse{}, translateClientError(err, "error unmarshalling SLO correction spec")
	}

	if specData.Data.Attributes == nil {
		return datadogV1.SLOCorrectionResponse{}, errors.New("cannot update SLO correction: spec.jsonSpec.data.attributes is missing")
	}

	// slo_id is create-only: SLOCorrectionUpdateRequestAttributes has no slo_id field, so the update
	// API silently keeps the correction on its original SLO if the spec's slo_id changed. Unmarshal it
	// separately and compare against the live correction to catch that instead of reporting sync as OK.
	var sloIDSpec struct {
		Data struct {
			Attributes struct {
				SloID string `json:"slo_id"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(instance.Spec.JsonSpec), &sloIDSpec); err != nil {
		return datadogV1.SLOCorrectionResponse{}, translateClientError(err, "error unmarshalling SLO correction spec")
	}

	if sloIDSpec.Data.Attributes.SloID != "" {
		current, err := getSLOCorrection(auth, client, instance.Status.Id)
		if err != nil {
			return datadogV1.SLOCorrectionResponse{}, err
		}
		if current.Data != nil && current.Data.Attributes != nil {
			if currentSloID := current.Data.Attributes.GetSloId(); currentSloID != "" && currentSloID != sloIDSpec.Data.Attributes.SloID {
				return datadogV1.SLOCorrectionResponse{}, fmt.Errorf("cannot update SLO correction: slo_id cannot be changed from %q to %q; delete and recreate the resource to target a different SLO", currentSloID, sloIDSpec.Data.Attributes.SloID)
			}
		}
	}

	updateData := datadogV1.NewSLOCorrectionUpdateData()
	updateData.SetAttributes(*specData.Data.Attributes)
	updateReq := datadogV1.NewSLOCorrectionUpdateRequest()
	updateReq.SetData(*updateData)

	updated, _, err := client.UpdateSLOCorrection(auth, instance.Status.Id, *updateReq)
	if err != nil {
		return datadogV1.SLOCorrectionResponse{}, translateClientError(err, "error updating SLO correction")
	}
	return updated, nil
}
