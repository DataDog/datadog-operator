// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogslo

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

func buildSLO(crdSLO *v1alpha1.DatadogSLO) (*datadogV1.ServiceLevelObjectiveRequest, *datadogV1.ServiceLevelObjective) {
	sloType := datadogV1.SLOType(crdSLO.Spec.Type)

	// Used for SLO creation
	sloReq := datadogV1.NewServiceLevelObjectiveRequest(crdSLO.Spec.Name, buildThreshold(crdSLO.Spec), sloType)
	{
		if crdSLO.Spec.Description != nil {
			sloReq.SetDescription(*crdSLO.Spec.Description)
		} else {
			sloReq.SetDescriptionNil()
		}
		sloReq.SetTags(crdSLO.Spec.Tags)
		if crdSLO.Spec.Type == v1alpha1.DatadogSLOTypeMetric {
			sloReq.SetQuery(datadogV1.ServiceLevelObjectiveQuery{
				Denominator: crdSLO.Spec.Query.Denominator,
				Numerator:   crdSLO.Spec.Query.Numerator,
			})
		}
		if crdSLO.Spec.Type == v1alpha1.DatadogSLOTypeMonitor {
			sloReq.SetMonitorIds(crdSLO.Spec.MonitorIDs)
			sloReq.SetGroups(crdSLO.Spec.Groups)
		}
	}

	// Used for SLO updates
	slo := datadogV1.NewServiceLevelObjective(crdSLO.Spec.Name, buildThreshold(crdSLO.Spec), sloType)
	{
		if crdSLO.Spec.Description != nil {
			slo.SetDescription(*crdSLO.Spec.Description)
		} else {
			slo.SetDescriptionNil()
		}
		slo.SetTags(crdSLO.Spec.Tags)
		if crdSLO.Spec.Type == v1alpha1.DatadogSLOTypeMetric {
			slo.SetQuery(datadogV1.ServiceLevelObjectiveQuery{
				Denominator: crdSLO.Spec.Query.Denominator,
				Numerator:   crdSLO.Spec.Query.Numerator,
			})
		}
		if crdSLO.Spec.Type == v1alpha1.DatadogSLOTypeMonitor {
			slo.SetMonitorIds(crdSLO.Spec.MonitorIDs)
			slo.SetGroups(crdSLO.Spec.Groups)
		}
	}

	return sloReq, slo
}

func buildThreshold(sloSpec v1alpha1.DatadogSLOSpec) []datadogV1.SLOThreshold {
	// Convert DatadogSLOSpec Timeframe, TargetThreshold, and WarningThreshold to datadogV1.SLOThreshold
	// (returned as a single-item list) for backwards compatibility.

	timeframe, _ := datadogV1.NewSLOTimeframeFromValue(string(sloSpec.Timeframe))

	var warningThreshold *float64
	if sloSpec.WarningThreshold != nil {
		convertedFloat, _ := strconv.ParseFloat(sloSpec.WarningThreshold.AsDec().String(), 64)
		warningThreshold = &convertedFloat
	}

	convertedFloat, _ := strconv.ParseFloat(sloSpec.TargetThreshold.AsDec().String(), 64)
	threshold := datadogV1.SLOThreshold{
		Target:    convertedFloat,
		Timeframe: *timeframe,
		Warning:   warningThreshold,
	}
	return []datadogV1.SLOThreshold{threshold}
}

func createSLO(auth context.Context, client *datadogV1.ServiceLevelObjectivesApi, crdSLO *v1alpha1.DatadogSLO) (datadogV1.ServiceLevelObjective, error) {
	sloReq, _ := buildSLO(crdSLO)
	slo, _, err := client.CreateSLO(auth, *sloReq)
	if err != nil {
		return datadogV1.ServiceLevelObjective{}, translateClientError(err, "error creating SLO")
	}

	return slo.Data[0], nil
}

func getSLO(auth context.Context, client *datadogV1.ServiceLevelObjectivesApi, sloId string) (*datadogV1.SLOResponseData, error) {
	slo, _, err := client.GetSLO(auth, sloId, datadogV1.GetSLOOptionalParameters{})
	if err != nil {
		return &datadogV1.SLOResponseData{}, translateClientError(err, "error getting SLO")
	}

	return slo.Data, nil
}

func updateSLO(auth context.Context, client *datadogV1.ServiceLevelObjectivesApi, crdSLO *v1alpha1.DatadogSLO) (datadogV1.SLOListResponse, error) {
	_, slo := buildSLO(crdSLO)
	sloListResponse, _, err := client.UpdateSLO(auth, crdSLO.Status.ID, *slo)
	if err != nil {
		return datadogV1.SLOListResponse{}, translateClientError(err, "error updating SLO")
	}
	return sloListResponse, nil
}

func deleteSLO(auth context.Context, client *datadogV1.ServiceLevelObjectivesApi, sloID string) (int, error) {
	force := "false"
	optionalParams := datadogV1.DeleteSLOOptionalParameters{
		Force: &force,
	}
	if _, localVarHTTPResponse, err := client.DeleteSLO(auth, sloID, optionalParams); err != nil {
		return localVarHTTPResponse.StatusCode, translateClientError(err, "error deleting SLO")
	}
	return 200, nil
}

func translateClientError(err error, msg string) error {
	if msg == "" {
		msg = "an error occurred"
	}

	var apiErr datadogapi.GenericOpenAPIError
	var errURL *url.Error
	if errors.As(err, &apiErr) {
		return fmt.Errorf(msg+": %w: %s", err, apiErr.Body())
	}

	if errors.As(err, &errURL) {
		return fmt.Errorf(msg+" (url.Error): %s", errURL)
	}

	return fmt.Errorf(msg+": %w", err)
}
