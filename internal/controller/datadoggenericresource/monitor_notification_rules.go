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

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

type MonitorNotificationRuleHandler struct {
	client *datadogV2.MonitorsApi
}

func (h *MonitorNotificationRuleHandler) createResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) (CreateResult, error) {
	created, err := createMonitorNotificationRule(auth, h.client, instance)
	if err != nil {
		return CreateResult{}, err
	}

	var createdTime *metav1.Time
	if created.Data != nil && created.Data.Attributes != nil && created.Data.Attributes.Created != nil {
		ct := metav1.NewTime(*created.Data.Attributes.Created)
		createdTime = &ct
	}

	creator := ""
	if created.Data != nil &&
		created.Data.Relationships != nil &&
		created.Data.Relationships.CreatedBy != nil &&
		created.Data.Relationships.CreatedBy.Data.IsSet() {
		if createdByData := created.Data.Relationships.CreatedBy.Data.Get(); createdByData != nil && createdByData.Id != nil {
			creator = *createdByData.Id
		}
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

func (h *MonitorNotificationRuleHandler) getResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error {
	_, err := getMonitorNotificationRule(auth, h.client, instance.Status.Id)
	return err
}

func (h *MonitorNotificationRuleHandler) updateResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error {
	_, err := updateMonitorNotificationRule(auth, h.client, instance)
	return err
}

func (h *MonitorNotificationRuleHandler) deleteResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error {
	return deleteMonitorNotificationRule(auth, h.client, instance.Status.Id)
}

func (h *MonitorNotificationRuleHandler) refreshState(_ context.Context, _ *v1alpha1.DatadogGenericResource) (*string, error) {
	return nil, nil
}

func getMonitorNotificationRule(auth context.Context, client *datadogV2.MonitorsApi, ruleID string) (datadogV2.MonitorNotificationRuleResponse, error) {
	if ruleID == "" {
		return datadogV2.MonitorNotificationRuleResponse{}, fmt.Errorf("cannot get monitor notification rule: ruleID is empty")
	}
	rule, _, err := client.GetMonitorNotificationRule(auth, ruleID)
	if err != nil {
		return datadogV2.MonitorNotificationRuleResponse{}, translateClientError(err, "error getting monitor notification rule")
	}
	return rule, nil
}

func deleteMonitorNotificationRule(auth context.Context, client *datadogV2.MonitorsApi, ruleID string) error {
	if ruleID == "" {
		return fmt.Errorf("cannot delete monitor notification rule: ruleID is empty")
	}
	httpResponse, err := client.DeleteMonitorNotificationRule(auth, ruleID)
	if err != nil {
		if httpResponse != nil && httpResponse.StatusCode == 404 {
			return nil
		}
		return translateClientError(err, "error deleting monitor notification rule")
	}
	return nil
}

func createMonitorNotificationRule(auth context.Context, client *datadogV2.MonitorsApi, instance *v1alpha1.DatadogGenericResource) (datadogV2.MonitorNotificationRuleResponse, error) {
	if instance.Spec.JsonSpec == "" {
		return datadogV2.MonitorNotificationRuleResponse{}, fmt.Errorf("cannot create monitor notification rule: spec.jsonSpec is empty")
	}

	body := &datadogV2.MonitorNotificationRuleCreateRequest{}
	if err := json.Unmarshal([]byte(instance.Spec.JsonSpec), body); err != nil {
		return datadogV2.MonitorNotificationRuleResponse{}, translateClientError(err, "error unmarshalling monitor notification rule spec")
	}

	rule, _, err := client.CreateMonitorNotificationRule(auth, *body)
	if err != nil {
		return datadogV2.MonitorNotificationRuleResponse{}, translateClientError(err, "error creating monitor notification rule")
	}
	return rule, nil
}

func updateMonitorNotificationRule(auth context.Context, client *datadogV2.MonitorsApi, instance *v1alpha1.DatadogGenericResource) (datadogV2.MonitorNotificationRuleResponse, error) {
	if instance.Status.Id == "" {
		return datadogV2.MonitorNotificationRuleResponse{}, errors.New("cannot update monitor notification rule: status.id is empty")
	}

	if instance.Spec.JsonSpec == "" {
		return datadogV2.MonitorNotificationRuleResponse{}, errors.New("cannot update monitor notification rule: spec.jsonSpec is empty")
	}

	var specData struct {
		Data struct {
			Attributes *datadogV2.MonitorNotificationRuleAttributes
		}
	}
	if err := json.Unmarshal([]byte(instance.Spec.JsonSpec), &specData); err != nil {
		return datadogV2.MonitorNotificationRuleResponse{}, translateClientError(err, "error unmarshalling monitor notification rule spec")
	}

	updateData := datadogV2.NewMonitorNotificationRuleUpdateRequestData(*specData.Data.Attributes, instance.Status.Id)
	updateReq := datadogV2.NewMonitorNotificationRuleUpdateRequest(*updateData)

	updated, _, err := client.UpdateMonitorNotificationRule(auth, instance.Status.Id, *updateReq)
	if err != nil {
		return datadogV2.MonitorNotificationRuleResponse{}, translateClientError(err, "error updating monitor notification rule")
	}
	return updated, nil
}
