// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	"fmt"

	utilserrors "k8s.io/apimachinery/pkg/util/errors"
)

// IsValidDatadogMonitor use to check if a DatadogMonitorSpec is valid by checking
// that the required fields are defined
// Deprecated: Use CRD validation rules for input validation
// https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/#validation-rules
func IsValidDatadogMonitor(spec *DatadogMonitorSpec) error {
	var errs []error
	if spec.Query == "" {
		errs = append(errs, fmt.Errorf("spec.Query must be defined"))
	}

	if spec.Type == "" {
		errs = append(errs, fmt.Errorf("spec.Type must be defined"))
	}

	if spec.Name == "" {
		errs = append(errs, fmt.Errorf("spec.Name must be defined"))
	}

	if spec.Message == "" {
		errs = append(errs, fmt.Errorf("spec.Message must be defined"))
	}

	return utilserrors.NewAggregate(errs)
}
