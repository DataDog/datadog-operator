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
func IsValidDatadogDashboard(spec *DatadogDashboardSpec) error {
	var errs []error
	if spec.Title == "" {
		errs = append(errs, fmt.Errorf("spec.Title must be defined"))
	}

	if spec.LayoutType == "" {
		errs = append(errs, fmt.Errorf("spec.LayoutType must be defined"))
	}

	if spec.LayoutType != "" && !spec.LayoutType.isValid() {
		errs = append(errs, fmt.Errorf("spec.LayoutType must be one of the values: %s or %s", DASHBOARDLAYOUTTYPE_FREE, DASHBOARDLAYOUTTYPE_ORDERED))
	}

	
	// if spec.Type == "" {
	// 	errs = append(errs, fmt.Errorf("spec.Type must be defined"))
	// }

	// if spec.Name == "" {
	// 	errs = append(errs, fmt.Errorf("spec.Name must be defined"))
	// }

	// if spec.Message == "" {
	// 	errs = append(errs, fmt.Errorf("spec.Message must be defined"))
	// }

	return utilserrors.NewAggregate(errs)
}
