// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2023 Datadog, Inc.

package datadogslo

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"

	datadogapiclientv1 "github.com/DataDog/datadog-api-client-go/api/v1/datadog"
)

func buildSLO(crdSLO *datadoghqv2alpha1.DatadogSLO) (*datadogapiclientv1.ServiceLevelObjectiveRequest, *datadogapiclientv1.ServiceLevelObjective) {
	sloType := datadogapiclientv1.SLOType(crdSLO.Spec.Type)
	req := datadogapiclientv1.NewServiceLevelObjectiveRequest(crdSLO.Spec.Name, buildThreshold(crdSLO.Spec.Thresholds), sloType)
	{
		if crdSLO.Spec.Description != nil {
			req.SetDescription(*crdSLO.Spec.Description)
		} else {
			req.SetDescriptionNil()
		}
		req.SetTags(crdSLO.Spec.Tags)
		req.SetQuery(datadogapiclientv1.ServiceLevelObjectiveQuery{
			Denominator: crdSLO.Spec.Query.Denominator,
			Numerator:   crdSLO.Spec.Query.Numerator,
		})
		req.SetMonitorIds(crdSLO.Spec.MonitorIDs)
		req.SetGroups(crdSLO.Spec.Groups)
	}
	ddSlo := datadogapiclientv1.NewServiceLevelObjective(crdSLO.Spec.Name, buildThreshold(crdSLO.Spec.Thresholds), sloType)
	{
		if crdSLO.Spec.Description != nil {
			ddSlo.SetDescription(*crdSLO.Spec.Description)
		} else {
			ddSlo.SetDescriptionNil()
		}
		ddSlo.SetTags(crdSLO.Spec.Tags)
		ddSlo.SetQuery(datadogapiclientv1.ServiceLevelObjectiveQuery{
			Denominator: crdSLO.Spec.Query.Denominator,
			Numerator:   crdSLO.Spec.Query.Numerator,
		})
		ddSlo.SetMonitorIds(crdSLO.Spec.MonitorIDs)
		ddSlo.SetGroups(crdSLO.Spec.Groups)
	}

	return req, ddSlo
}

func buildThreshold(thresholds []datadoghqv2alpha1.DatadogSLOThreshold) []datadogapiclientv1.SLOThreshold {
	var thresholdArray []datadogapiclientv1.SLOThreshold
	for _, t := range thresholds {
		timeFrame, _ := datadogapiclientv1.NewSLOTimeframeFromValue(string(t.Timeframe))
		var warningDisplay *string
		if t.WarningDisplay != "" {
			warningDisplay = stringPtr(t.WarningDisplay)
		}

		var warning *float64
		if t.Warning != nil {
			approxFloat := t.Warning.AsApproximateFloat64()
			warning = &approxFloat
		}

		threshold := datadogapiclientv1.SLOThreshold{
			Target:         t.Target.AsApproximateFloat64(),
			TargetDisplay:  stringPtr(t.TargetDisplay),
			Timeframe:      *timeFrame,
			Warning:        warning,
			WarningDisplay: warningDisplay,
		}
		thresholdArray = append(thresholdArray, threshold)
	}
	return thresholdArray
}

func createSLO(auth context.Context, client *datadogapiclientv1.APIClient, crdSLO *datadoghqv2alpha1.DatadogSLO) (datadogapiclientv1.ServiceLevelObjective, error) {
	sloReq, _ := buildSLO(crdSLO)
	createSLO, _, err := client.ServiceLevelObjectivesApi.CreateSLO(auth, *sloReq)
	if err != nil {
		return datadogapiclientv1.ServiceLevelObjective{}, translateClientError(err, "error creating monitor")
	}

	return createSLO.Data[0], nil
}

func updateSLO(auth context.Context, client *datadogapiclientv1.APIClient, crdSLO *datadoghqv2alpha1.DatadogSLO) (datadogapiclientv1.SLOListResponse, error) {
	_, slo := buildSLO(crdSLO)
	updateSLO, _, err := client.ServiceLevelObjectivesApi.UpdateSLO(auth, crdSLO.Status.ID, *slo)
	if err != nil {
		return datadogapiclientv1.SLOListResponse{}, translateClientError(err, "error updating monitor")
	}
	return updateSLO, nil
}

func deleteSLO(auth context.Context, client *datadogapiclientv1.APIClient, sloID string) error {
	force := "false"
	optionalParams := datadogapiclientv1.DeleteSLOOptionalParameters{
		Force: &force,
	}
	if _, _, err := client.ServiceLevelObjectivesApi.DeleteSLO(auth, sloID, optionalParams); err != nil {
		return translateClientError(err, "error deleting monitor")
	}
	return nil
}

func translateClientError(err error, msg string) error {
	if msg == "" {
		msg = "an error occurred"
	}

	var apiErr datadogapiclientv1.GenericOpenAPIError
	var errURL *url.Error
	if errors.As(err, &apiErr) {
		return fmt.Errorf(msg+": %w: %s", err, apiErr.Body())
	}

	if errors.As(err, &errURL) {
		return fmt.Errorf(msg+" (url.Error): %s", errURL)
	}

	return fmt.Errorf(msg+": %w", err)
}

func stringPtr(s string) *string {
	return &s
}
