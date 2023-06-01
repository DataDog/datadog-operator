// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2023 Datadog, Inc.

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

	if spec.Query == nil {
		errs = append(errs, fmt.Errorf("spec.Query must be defined"))
	}

	if spec.Type == "" {
		errs = append(errs, fmt.Errorf("spec.Type must be defined"))
	}

	if spec.Type != "" && !spec.Type.IsValid() {
		errs = append(errs, fmt.Errorf("spec.Type must be one of the values: %s or %s", DatadogSLOTypeMonitor, DatadogSLOTypeMetric))
	}

	if len(spec.Thresholds) < 1 {
		errs = append(errs, fmt.Errorf("spec.Thresholds must be defined"))
	}

	if spec.Type == DatadogSLOTypeMonitor && len(spec.MonitorIDs) < 1 {
		errs = append(errs, fmt.Errorf("spec.MonitorIDs must be defined when spec.Type is monitor"))
	}

	for _, threshold := range spec.Thresholds {
		if threshold.Target.Value() <= 0 {
			errs = append(errs, fmt.Errorf("spec.Thresholds.Target must be defined and greater than 0"))
		}

		if threshold.Warning != nil {
			if threshold.Warning.Value() <= 0 {
				errs = append(errs, fmt.Errorf("spec.Thresholds.Warning must be greater than 0"))
			}
		}

		switch threshold.Timeframe {
		case DatadogSLOTimeFrame7d, DatadogSLOTimeFrame30d, DatadogSLOTimeFrame90d, DatadogSLOTimeFrameCustom:
			break
		default:
			errs = append(errs, fmt.Errorf("spec.Thresholds.Timeframe must be defined as one of the values: 7d, 30d, 90d, or custom"))
		}
	}

	return utilserrors.NewAggregate(errs)
}
