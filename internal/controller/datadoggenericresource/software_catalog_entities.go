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
	"net/http"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

// SoftwareCatalogEntityHandler manages Software Catalog entities (services,
// datastores, queues, systems, APIs) through the DDGR CRD.
//
// The Software Catalog API is upsert-shaped: UpsertCatalogEntity handles both
// creation and update of an entity, keyed by its kind/namespace/name rather
// than by a client-supplied ID. createResource and updateResource both call
// the same upsertSoftwareCatalogEntity helper; status.id is populated from
// the ID Datadog assigns in the upsert response and is then used as the
// stable identifier for get/delete calls. There is no dedicated get-by-id
// endpoint, so getResource uses ListCatalogEntity filtered by that ID.
type SoftwareCatalogEntityHandler struct {
	client *datadogV2.SoftwareCatalogApi
}

func (h *SoftwareCatalogEntityHandler) createResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) (CreateResult, error) {
	upserted, err := upsertSoftwareCatalogEntity(auth, h.client, instance)
	if err != nil {
		return CreateResult{}, err
	}

	if len(upserted.Data) == 0 {
		return CreateResult{}, errors.New("cannot create software catalog entity: upsert response contained no entity data")
	}
	entity := upserted.Data[0]

	var createdTime *metav1.Time
	if entity.Meta != nil && entity.Meta.CreatedAt != nil {
		if parsed, err := time.Parse(time.RFC3339, *entity.Meta.CreatedAt); err == nil {
			ct := metav1.NewTime(parsed)
			createdTime = &ct
		}
	}

	return CreateResult{
		ID:          entity.GetId(),
		CreatedTime: createdTime,
	}, nil
}

func (h *SoftwareCatalogEntityHandler) getResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error {
	_, err := getSoftwareCatalogEntity(auth, h.client, instance.Status.Id)
	return err
}

func (h *SoftwareCatalogEntityHandler) updateResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error {
	_, err := upsertSoftwareCatalogEntity(auth, h.client, instance)
	return err
}

func (h *SoftwareCatalogEntityHandler) deleteResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error {
	return deleteSoftwareCatalogEntity(auth, h.client, instance.Status.Id)
}

func (h *SoftwareCatalogEntityHandler) refreshState(_ context.Context, _ *v1alpha1.DatadogGenericResource) (*string, error) {
	return nil, nil
}

func getSoftwareCatalogEntity(auth context.Context, client *datadogV2.SoftwareCatalogApi, entityID string) (datadogV2.EntityData, error) {
	if entityID == "" {
		return datadogV2.EntityData{}, fmt.Errorf("cannot get software catalog entity: entityID is empty")
	}

	params := datadogV2.NewListCatalogEntityOptionalParameters().WithFilterId(entityID)
	resp, httpResp, err := client.ListCatalogEntity(auth, *params)
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return datadogV2.EntityData{}, translateClientError(err, httpResp, "error getting software catalog entity")
	}

	if len(resp.Data) == 0 {
		return datadogV2.EntityData{}, translateClientError(errors.New("404 Not Found"), &http.Response{StatusCode: http.StatusNotFound}, "error getting software catalog entity")
	}

	return resp.Data[0], nil
}

func deleteSoftwareCatalogEntity(auth context.Context, client *datadogV2.SoftwareCatalogApi, entityID string) error {
	if entityID == "" {
		return fmt.Errorf("cannot delete software catalog entity: entityID is empty")
	}
	httpResponse, err := client.DeleteCatalogEntity(auth, entityID)
	if httpResponse != nil {
		defer httpResponse.Body.Close()
	}
	if err != nil {
		if httpResponse != nil && httpResponse.StatusCode == 404 {
			return nil
		}
		return translateClientError(err, httpResponse, "error deleting software catalog entity")
	}
	return nil
}

func upsertSoftwareCatalogEntity(auth context.Context, client *datadogV2.SoftwareCatalogApi, instance *v1alpha1.DatadogGenericResource) (datadogV2.UpsertCatalogEntityResponse, error) {
	if instance.Spec.JsonSpec == "" {
		return datadogV2.UpsertCatalogEntityResponse{}, fmt.Errorf("cannot upsert software catalog entity: spec.jsonSpec is empty")
	}

	body := &datadogV2.UpsertCatalogEntityRequest{}
	if err := json.Unmarshal([]byte(instance.Spec.JsonSpec), body); err != nil {
		return datadogV2.UpsertCatalogEntityResponse{}, translateUnmarshalError(err, "error unmarshalling software catalog entity spec")
	}

	upserted, httpResp, err := client.UpsertCatalogEntity(auth, *body)
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return datadogV2.UpsertCatalogEntityResponse{}, translateClientError(err, httpResp, "error upserting software catalog entity")
	}
	return upserted, nil
}
