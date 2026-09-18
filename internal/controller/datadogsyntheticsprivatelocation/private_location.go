// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogsyntheticsprivatelocation

import (
	"context"
	"net/http"
	"slices"
	"sort"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	ctrutils "github.com/DataDog/datadog-operator/pkg/controller/utils"
)

// applyRequiredTags returns the spec tags, appending the generated:kubernetes
// marker tag unless the user already set it or disabled it.
func applyRequiredTags(instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation) []string {
	if instance.Spec.ControllerOptions != nil && instance.Spec.ControllerOptions.DisableRequiredTags {
		return instance.Spec.Tags
	}
	tags := make([]string, 0, len(instance.Spec.Tags)+1)
	tags = append(tags, instance.Spec.Tags...)
	if slices.Contains(tags, datadoghqv1alpha1.DatadogSPLRequiredTag) {
		return tags
	}
	return append(tags, datadoghqv1alpha1.DatadogSPLRequiredTag)
}

func buildPrivateLocation(instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation) *datadogV1.SyntheticsPrivateLocation {
	tags := applyRequiredTags(instance)
	sort.Strings(tags)
	pl := datadogV1.NewSyntheticsPrivateLocation(instance.Spec.Description, instance.Spec.Name, tags)
	if instance.Status.ID != "" {
		pl.SetId(instance.Status.ID)
	}
	return pl
}

func createPrivateLocation(auth context.Context, ddClientSynthetics *datadogV1.SyntheticsApi, instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation) (*datadogV1.SyntheticsPrivateLocationCreationResponse, error) {
	created, httpResp, err := ddClientSynthetics.CreatePrivateLocation(auth, *buildPrivateLocation(instance))
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return nil, ctrutils.TranslateClientError(err, httpResp, "error creating private location")
	}

	return &created, nil
}

// getPrivateLocation returns the private location, or (nil, nil) when it does
// not exist in Datadog anymore (404) so the caller can distinguish drift from
// transient errors.
func getPrivateLocation(auth context.Context, ddClientSynthetics *datadogV1.SyntheticsApi, locationID string) (*datadogV1.SyntheticsPrivateLocation, error) {
	pl, httpResp, err := ddClientSynthetics.GetPrivateLocation(auth, locationID)
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		return nil, ctrutils.TranslateClientError(err, httpResp, "error getting private location")
	}

	return &pl, nil
}

func updatePrivateLocation(auth context.Context, ddClientSynthetics *datadogV1.SyntheticsApi, instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation) (*datadogV1.SyntheticsPrivateLocation, error) {
	updated, httpResp, err := ddClientSynthetics.UpdatePrivateLocation(auth, instance.Status.ID, *buildPrivateLocation(instance))
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return nil, ctrutils.TranslateClientError(err, httpResp, "error updating private location")
	}

	return &updated, nil
}

func deletePrivateLocation(auth context.Context, ddClientSynthetics *datadogV1.SyntheticsApi, locationID string) error {
	httpResp, err := ddClientSynthetics.DeletePrivateLocation(auth, locationID)
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		// Deletion is idempotent for finalization: if the private location was
		// already removed in Datadog (for example from the UI), allow the
		// Kubernetes finalizer to clear.
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			return nil
		}
		return ctrutils.TranslateClientError(err, httpResp, "error deleting private location")
	}

	return nil
}
