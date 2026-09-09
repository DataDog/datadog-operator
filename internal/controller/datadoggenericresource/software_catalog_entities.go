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
	ctrutils "github.com/DataDog/datadog-operator/pkg/controller/utils"
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
	// UpsertCatalogEntity is keyed by kind/namespace/name, not by instance.Status.Id: if the
	// spec's identity fields changed, the upsert would create a second entity under the new
	// identity rather than update the one we're tracking, orphaning the old entity and leaving
	// the new one unmanaged. Reject identity changes instead of silently leaking a duplicate.
	current, err := getSoftwareCatalogEntity(auth, h.client, instance.Status.Id)
	if err != nil {
		return err
	}

	newIdentity, err := parseEntityIdentity(instance.Spec.JsonSpec)
	if err != nil {
		return translateUnmarshalError(err, "error parsing software catalog entity identity")
	}

	currentAttrs := current.GetAttributes()
	if newIdentity.kind != currentAttrs.GetKind() || newIdentity.name != currentAttrs.GetName() || newIdentity.namespace != currentAttrs.GetNamespace() {
		// Not a live API error, but treated as one of the same shape (permanent, 400-equivalent) so the
		// reconciler backs off to forceSyncPeriod instead of retrying every few seconds forever: this
		// condition never resolves on its own until the user edits the spec back or deletes the CR.
		immutableErr := fmt.Errorf("cannot update software catalog entity: identity fields kind/metadata.name/metadata.namespace are immutable (was %s/%s/%s, spec now has %s/%s/%s); delete and recreate the resource instead",
			currentAttrs.GetNamespace(), currentAttrs.GetKind(), currentAttrs.GetName(), newIdentity.namespace, newIdentity.kind, newIdentity.name)
		return ctrutils.NewAPIError(immutableErr, &http.Response{StatusCode: http.StatusBadRequest})
	}

	_, err = upsertSoftwareCatalogEntity(auth, h.client, instance)
	return err
}

// entityIdentity is the kind/namespace/name tuple that UpsertCatalogEntity uses to key an
// entity; it is read generically off spec.jsonSpec since Entity V3's oneOf kinds (service,
// datastore, queue, system, api) all share this same top-level shape.
type entityIdentity struct {
	kind      string
	name      string
	namespace string
}

func parseEntityIdentity(jsonSpec string) (entityIdentity, error) {
	var parsed struct {
		Kind     string `json:"kind"`
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal([]byte(jsonSpec), &parsed); err != nil {
		return entityIdentity{}, err
	}

	namespace := parsed.Metadata.Namespace
	if namespace == "" {
		namespace = "default"
	}
	return entityIdentity{kind: parsed.Kind, name: parsed.Metadata.Name, namespace: namespace}, nil
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
