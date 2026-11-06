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

type OnCallEscalationPolicyHandler struct {
	client *datadogV2.OnCallApi
}

func (h *OnCallEscalationPolicyHandler) createResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) (CreateResult, error) {
	created, err := createOnCallEscalationPolicy(auth, h.client, instance)
	if err != nil {
		return CreateResult{}, err
	}

	id := ""
	if created.Data != nil && created.Data.Id != nil {
		id = *created.Data.Id
	}
	return CreateResult{ID: id}, nil
}

func (h *OnCallEscalationPolicyHandler) getResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error {
	_, err := getOnCallEscalationPolicy(auth, h.client, instance.Status.Id)
	return err
}

func (h *OnCallEscalationPolicyHandler) updateResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error {
	_, err := updateOnCallEscalationPolicy(auth, h.client, instance)
	return err
}

func (h *OnCallEscalationPolicyHandler) deleteResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error {
	return deleteOnCallEscalationPolicy(auth, h.client, instance.Status.Id)
}

func (h *OnCallEscalationPolicyHandler) refreshState(_ context.Context, _ *v1alpha1.DatadogGenericResource) (*string, error) {
	return nil, nil
}

func getOnCallEscalationPolicy(auth context.Context, client *datadogV2.OnCallApi, policyID string) (datadogV2.EscalationPolicy, error) {
	if policyID == "" {
		return datadogV2.EscalationPolicy{}, fmt.Errorf("cannot get On-Call escalation policy: policyID is empty")
	}

	resource, httpResp, err := client.GetOnCallEscalationPolicy(auth, policyID)
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return datadogV2.EscalationPolicy{}, translateClientError(err, httpResp, "error getting On-Call escalation policy")
	}
	return resource, nil
}

func deleteOnCallEscalationPolicy(auth context.Context, client *datadogV2.OnCallApi, policyID string) error {
	if policyID == "" {
		return fmt.Errorf("cannot delete On-Call escalation policy: policyID is empty")
	}

	httpResp, err := client.DeleteOnCallEscalationPolicy(auth, policyID)
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == 404 {
			return nil
		}
		return translateClientError(err, httpResp, "error deleting On-Call escalation policy")
	}
	return nil
}

func createOnCallEscalationPolicy(auth context.Context, client *datadogV2.OnCallApi, instance *v1alpha1.DatadogGenericResource) (datadogV2.EscalationPolicy, error) {
	if instance.Spec.JsonSpec == "" {
		return datadogV2.EscalationPolicy{}, fmt.Errorf("cannot create On-Call escalation policy: spec.jsonSpec is empty")
	}

	var body datadogV2.EscalationPolicyCreateRequest
	if err := json.Unmarshal([]byte(instance.Spec.JsonSpec), &body); err != nil {
		return datadogV2.EscalationPolicy{}, translateUnmarshalError(err, "error unmarshalling On-Call escalation policy spec")
	}

	resource, httpResp, err := client.CreateOnCallEscalationPolicy(auth, body)
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return datadogV2.EscalationPolicy{}, translateClientError(err, httpResp, "error creating On-Call escalation policy")
	}
	return resource, nil
}

func updateOnCallEscalationPolicy(auth context.Context, client *datadogV2.OnCallApi, instance *v1alpha1.DatadogGenericResource) (datadogV2.EscalationPolicy, error) {
	if instance.Status.Id == "" {
		return datadogV2.EscalationPolicy{}, fmt.Errorf("cannot update On-Call escalation policy: status.id is empty")
	}
	if instance.Spec.JsonSpec == "" {
		return datadogV2.EscalationPolicy{}, fmt.Errorf("cannot update On-Call escalation policy: spec.jsonSpec is empty")
	}

	var specData struct {
		Data struct {
			Attributes    *datadogV2.EscalationPolicyUpdateRequestDataAttributes    `json:"attributes"`
			Relationships *datadogV2.EscalationPolicyUpdateRequestDataRelationships `json:"relationships,omitempty"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(instance.Spec.JsonSpec), &specData); err != nil {
		return datadogV2.EscalationPolicy{}, translateUnmarshalError(err, "error unmarshalling On-Call escalation policy spec")
	}
	if specData.Data.Attributes == nil {
		return datadogV2.EscalationPolicy{}, fmt.Errorf("cannot update On-Call escalation policy: spec.jsonSpec.data.attributes is missing")
	}

	// jsonSpec uses the create shape; the update API also requires the ID
	// recorded in status. Preserve relationships when constructing its payload.
	updateData := datadogV2.NewEscalationPolicyUpdateRequestData(*specData.Data.Attributes, instance.Status.Id, datadogV2.ESCALATIONPOLICYUPDATEREQUESTDATATYPE_POLICIES)
	updateData.Relationships = specData.Data.Relationships
	body := datadogV2.NewEscalationPolicyUpdateRequest(*updateData)

	resource, httpResp, err := client.UpdateOnCallEscalationPolicy(auth, instance.Status.Id, *body)
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return datadogV2.EscalationPolicy{}, translateClientError(err, httpResp, "error updating On-Call escalation policy")
	}
	return resource, nil
}
