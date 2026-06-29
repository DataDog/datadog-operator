// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	"fmt"

	utilserrors "k8s.io/apimachinery/pkg/util/errors"
)

// IsValidDatadogSLO use to check if a DatadogSLOSpec is valid by checking
// that the required fields are defined
func IsValidDatadogSLO(spec *DatadogSLOSpec) error {
	var errs []error
	if spec.Name == "" {
		errs = append(errs, fmt.Errorf("spec.Name must be defined"))
	}

	if spec.Type == "" {
		errs = append(errs, fmt.Errorf("spec.Type must be defined"))
	}

	if spec.Type != "" && !spec.Type.IsValid() {
		errs = append(errs, fmt.Errorf("spec.Type must be one of the values: %s, %s, or %s", DatadogSLOTypeMonitor, DatadogSLOTypeMetric, DatadogSLOTypeTimeSlice))
	}

	if spec.Type == DatadogSLOTypeMetric && spec.Query == nil {
		errs = append(errs, fmt.Errorf("spec.Query must be defined when spec.Type is metric"))
	}

	if spec.Type == DatadogSLOTypeMonitor && len(spec.MonitorIDs) == 0 {
		errs = append(errs, fmt.Errorf("spec.MonitorIDs must be defined when spec.Type is monitor"))
	}

	if spec.Type == DatadogSLOTypeTimeSlice {
		if spec.TimeSlice == nil {
			errs = append(errs, fmt.Errorf("spec.TimeSlice must be defined when spec.Type is time_slice"))
		} else {
			if spec.TimeSlice.Query == "" {
				errs = append(errs, fmt.Errorf("spec.TimeSlice.Query must be defined"))
			}
		}
	}

	// Cross-field validation: reject fields that don't belong to the specified type.
	if spec.Type == DatadogSLOTypeMetric && spec.TimeSlice != nil {
		errs = append(errs, fmt.Errorf("spec.TimeSlice must not be defined when spec.Type is metric"))
	}
	if spec.Type == DatadogSLOTypeMonitor && spec.TimeSlice != nil {
		errs = append(errs, fmt.Errorf("spec.TimeSlice must not be defined when spec.Type is monitor"))
	}
	if spec.Type == DatadogSLOTypeTimeSlice && spec.Query != nil {
		errs = append(errs, fmt.Errorf("spec.Query must not be defined when spec.Type is time_slice"))
	}
	if spec.Type == DatadogSLOTypeTimeSlice && len(spec.MonitorIDs) > 0 {
		errs = append(errs, fmt.Errorf("spec.MonitorIDs must not be defined when spec.Type is time_slice"))
	}

	if spec.TargetThreshold.AsApproximateFloat64() <= 0 || spec.TargetThreshold.AsApproximateFloat64() >= 100 {
		errs = append(errs, fmt.Errorf("spec.TargetThreshold must be greater than 0 and less than 100"))
	}

	if spec.WarningThreshold != nil && (spec.WarningThreshold.AsApproximateFloat64() <= 0 || spec.WarningThreshold.AsApproximateFloat64() >= 100) {
		errs = append(errs, fmt.Errorf("spec.WarningThreshold must be greater than 0 and less than 100"))
	}

	switch spec.Timeframe {
	case DatadogSLOTimeFrame7d, DatadogSLOTimeFrame30d, DatadogSLOTimeFrame90d:
		break
	default:
		errs = append(errs, fmt.Errorf("spec.Timeframe must be defined as one of the values: 7d, 30d, or 90d"))
	}

	return utilserrors.NewAggregate(errs)
}
