// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoggenericresource

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

type OnCallScheduleHandler struct {
	client *datadogV2.OnCallApi
}

func (h *OnCallScheduleHandler) createResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) (CreateResult, error) {
	created, err := createOnCallSchedule(auth, h.client, instance)
	if err != nil {
		return CreateResult{}, err
	}

	id := ""
	if created.Data != nil && created.Data.Id != nil {
		id = *created.Data.Id
	}
	return CreateResult{ID: id}, nil
}

func (h *OnCallScheduleHandler) getResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error {
	_, err := getOnCallSchedule(auth, h.client, instance.Status.Id)
	return err
}

func (h *OnCallScheduleHandler) updateResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error {
	_, err := updateOnCallSchedule(auth, h.client, instance)
	return err
}

func (h *OnCallScheduleHandler) deleteResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error {
	return deleteOnCallSchedule(auth, h.client, instance.Status.Id)
}

func (h *OnCallScheduleHandler) refreshState(_ context.Context, _ *v1alpha1.DatadogGenericResource) (*string, error) {
	return nil, nil
}

func getOnCallSchedule(auth context.Context, client *datadogV2.OnCallApi, scheduleID string) (datadogV2.Schedule, error) {
	if scheduleID == "" {
		return datadogV2.Schedule{}, fmt.Errorf("cannot get On-Call schedule: scheduleID is empty")
	}

	resource, httpResp, err := client.GetOnCallSchedule(auth, scheduleID)
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return datadogV2.Schedule{}, translateClientError(err, httpResp, "error getting On-Call schedule")
	}
	return resource, nil
}

func deleteOnCallSchedule(auth context.Context, client *datadogV2.OnCallApi, scheduleID string) error {
	if scheduleID == "" {
		return fmt.Errorf("cannot delete On-Call schedule: scheduleID is empty")
	}

	httpResp, err := client.DeleteOnCallSchedule(auth, scheduleID)
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == 404 {
			return nil
		}
		return translateClientError(err, httpResp, "error deleting On-Call schedule")
	}
	return nil
}

func createOnCallSchedule(auth context.Context, client *datadogV2.OnCallApi, instance *v1alpha1.DatadogGenericResource) (datadogV2.Schedule, error) {
	if instance.Spec.JsonSpec == "" {
		return datadogV2.Schedule{}, fmt.Errorf("cannot create On-Call schedule: spec.jsonSpec is empty")
	}

	var body datadogV2.ScheduleCreateRequest
	if err := json.Unmarshal([]byte(instance.Spec.JsonSpec), &body); err != nil {
		return datadogV2.Schedule{}, translateUnmarshalError(err, "error unmarshalling On-Call schedule spec")
	}

	resource, httpResp, err := client.CreateOnCallSchedule(auth, body)
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return datadogV2.Schedule{}, translateClientError(err, httpResp, "error creating On-Call schedule")
	}
	return resource, nil
}

func updateOnCallSchedule(auth context.Context, client *datadogV2.OnCallApi, instance *v1alpha1.DatadogGenericResource) (datadogV2.Schedule, error) {
	if instance.Status.Id == "" {
		return datadogV2.Schedule{}, fmt.Errorf("cannot update On-Call schedule: status.id is empty")
	}
	if instance.Spec.JsonSpec == "" {
		return datadogV2.Schedule{}, fmt.Errorf("cannot update On-Call schedule: spec.jsonSpec is empty")
	}

	var specData struct {
		Data struct {
			Attributes    *datadogV2.ScheduleUpdateRequestDataAttributes    `json:"attributes"`
			Relationships *datadogV2.ScheduleUpdateRequestDataRelationships `json:"relationships,omitempty"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(instance.Spec.JsonSpec), &specData); err != nil {
		return datadogV2.Schedule{}, translateUnmarshalError(err, "error unmarshalling On-Call schedule spec")
	}
	if specData.Data.Attributes == nil {
		return datadogV2.Schedule{}, fmt.Errorf("cannot update On-Call schedule: spec.jsonSpec.data.attributes is missing")
	}

	// jsonSpec uses the create shape; the update API also requires the ID
	// recorded in status. Preserve relationships when constructing its payload.
	updateData := datadogV2.NewScheduleUpdateRequestData(*specData.Data.Attributes, instance.Status.Id, datadogV2.SCHEDULEUPDATEREQUESTDATATYPE_SCHEDULES)
	updateData.Relationships = specData.Data.Relationships
	body := datadogV2.NewScheduleUpdateRequest(*updateData)

	resource, httpResp, err := client.UpdateOnCallSchedule(auth, instance.Status.Id, *body)
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return datadogV2.Schedule{}, translateClientError(err, httpResp, "error updating On-Call schedule")
	}
	return resource, nil
}
