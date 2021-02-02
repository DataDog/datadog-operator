// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package v1alpha1

import (
	"fmt"

	utilserrors "k8s.io/apimachinery/pkg/util/errors"
)

// IsValidDatadogMonitor use to check if a DatadogMonitorSpec is valid by checking
// that the required fields are defined
func IsValidDatadogMonitor(spec *DatadogMonitorSpec) error {
	var errs []error
	var err error
	if spec.Query == "" {
		errs = append(errs, fmt.Errorf("invalid spec.Query, err: %v", err))
	}

	if spec.Type == "" {
		errs = append(errs, fmt.Errorf("invalid spec.Type, err: %v", err))
	}

	if spec.Name == "" {
		errs = append(errs, fmt.Errorf("invalid spec.Name, err: %v", err))
	}

	if spec.Message == "" {
		errs = append(errs, fmt.Errorf("invalid spec.Message, err: %v", err))
	}

	return utilserrors.NewAggregate(errs)
}
