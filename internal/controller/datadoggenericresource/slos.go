// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoggenericresource

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

type SLOHandler struct {
	client *datadogV1.ServiceLevelObjectivesApi
}

func (h *SLOHandler) createResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) (CreateResult, error) {
	createdSLO, err := createSLO(auth, h.client, instance)
	if err != nil {
		return CreateResult{}, err
	}

	var createdTime *metav1.Time
	if createdSLO.CreatedAt != nil {
		ct := metav1.Unix(createdSLO.GetCreatedAt(), 0)
		createdTime = &ct
	}

	creator := ""
	if creatorHandle := createdSLO.GetCreator().Handle; creatorHandle != nil {
		creator = *creatorHandle
	}

	return CreateResult{
		ID:          createdSLO.GetId(),
		CreatedTime: createdTime,
		Creator:     creator,
	}, nil
}

func (h *SLOHandler) getResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error {
	_, err := getSLO(auth, h.client, instance.Status.Id)
	return err
}

func (h *SLOHandler) updateResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error {
	_, err := updateSLO(auth, h.client, instance)
	return err
}

func (h *SLOHandler) deleteResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error {
	return deleteSLO(auth, h.client, instance.Status.Id)
}

func (h *SLOHandler) refreshState(auth context.Context, instance *v1alpha1.DatadogGenericResource) (*string, error) {
	sloName, err := getSLONameFromSpec(instance)
	if err != nil {
		return nil, err
	}
	return getSLOState(auth, h.client, instance.Status.Id, sloName)
}

func createSLO(auth context.Context, client *datadogV1.ServiceLevelObjectivesApi, instance *v1alpha1.DatadogGenericResource) (datadogV1.ServiceLevelObjective, error) {
	sloCreateData := &datadogV1.ServiceLevelObjectiveRequest{}
	if err := json.Unmarshal([]byte(instance.Spec.JsonSpec), sloCreateData); err != nil {
		return datadogV1.ServiceLevelObjective{}, translateClientError(err, "error unmarshalling SLO spec")
	}
	slo, _, err := client.CreateSLO(auth, *sloCreateData)
	if err != nil {
		return datadogV1.ServiceLevelObjective{}, translateClientError(err, "error creating SLO")
	}

	data := slo.GetData()
	if len(data) == 0 {
		return datadogV1.ServiceLevelObjective{}, fmt.Errorf("error creating SLO: empty response data")
	}
	return data[0], nil
}

func getSLO(auth context.Context, client *datadogV1.ServiceLevelObjectivesApi, sloID string) (*datadogV1.SLOResponseData, error) {
	slo, _, err := client.GetSLO(auth, sloID, datadogV1.GetSLOOptionalParameters{})
	if err != nil {
		return nil, translateClientError(err, "error getting SLO")
	}
	return slo.Data, nil
}

func updateSLO(auth context.Context, client *datadogV1.ServiceLevelObjectivesApi, instance *v1alpha1.DatadogGenericResource) (datadogV1.SLOListResponse, error) {
	sloUpdateData := &datadogV1.ServiceLevelObjective{}
	if err := json.Unmarshal([]byte(instance.Spec.JsonSpec), sloUpdateData); err != nil {
		return datadogV1.SLOListResponse{}, translateClientError(err, "error unmarshalling SLO spec")
	}
	sloUpdated, _, err := client.UpdateSLO(auth, instance.Status.Id, *sloUpdateData)
	if err != nil {
		return datadogV1.SLOListResponse{}, translateClientError(err, "error updating SLO")
	}
	return sloUpdated, nil
}

func deleteSLO(auth context.Context, client *datadogV1.ServiceLevelObjectivesApi, sloID string) error {
	force := "false"
	optionalParams := datadogV1.DeleteSLOOptionalParameters{
		Force: &force,
	}
	_, httpResponse, err := client.DeleteSLO(auth, sloID, optionalParams)
	if err != nil {
		// Deletion is idempotent for finalization: if the SLO was already removed
		// in Datadog (for example from the UI), allow the Kubernetes finalizer to clear.
		// Retry other errors (e.g. 400, 401, 429, 5XX).
		if httpResponse != nil && httpResponse.StatusCode == 404 {
			return nil
		}
		return translateClientError(err, "error deleting SLO")
	}
	return nil
}

func getSLOState(auth context.Context, client *datadogV1.ServiceLevelObjectivesApi, sloID, sloName string) (*string, error) {
	pageSize := int64(100)
	searchParams := datadogV1.SearchSLOOptionalParameters{
		Query:    &sloName,
		PageSize: &pageSize,
	}

	response, _, err := client.SearchSLO(auth, searchParams)
	if err != nil {
		return nil, translateClientError(err, "error searching SLO")
	}

	state, err := extractSLOState(response, sloID)
	if err != nil {
		return nil, err
	}
	return &state, nil
}

func extractSLOState(response datadogV1.SearchSLOResponse, sloID string) (string, error) {
	data := response.GetData()
	attributes := data.GetAttributes()
	for _, slo := range attributes.GetSlos() {
		sloData := slo.GetData()
		if sloData.GetId() != sloID {
			continue
		}

		sloAttributes := sloData.GetAttributes()
		for _, status := range sloAttributes.GetOverallStatus() {
			if state, ok := status.GetStateOk(); ok {
				return string(*state), nil
			}
		}
		if status, ok := sloAttributes.GetStatusOk(); ok {
			if state, ok := status.GetStateOk(); ok {
				return string(*state), nil
			}
		}

		return "", fmt.Errorf("error getting SLO state: SLO %s does not include state", sloID)
	}

	return "", fmt.Errorf("error getting SLO state: SLO %s not found", sloID)
}

func getSLONameFromSpec(instance *v1alpha1.DatadogGenericResource) (string, error) {
	sloSpec := struct {
		Name string `json:"name"`
	}{}
	if err := json.Unmarshal([]byte(instance.Spec.JsonSpec), &sloSpec); err != nil {
		return "", translateClientError(err, "error unmarshalling SLO spec")
	}
	if sloSpec.Name == "" {
		return "", fmt.Errorf("error getting SLO state: SLO spec does not include name")
	}
	return sloSpec.Name, nil
}
