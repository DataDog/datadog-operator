// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	"fmt"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
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

	if spec.LayoutType != DASHBOARDLAYOUTTYPE_ORDERED && spec.ReflowType != nil {
		errs = append(errs, fmt.Errorf("spec.ReflowType should only be set if layout type is 'ordered'"))
	}

	if spec.ReflowType != nil && !spec.ReflowType.IsValid() {
		errs = append(errs, fmt.Errorf("spec.ReflowType must be one of the values: %s or %s", datadogV1.DASHBOARDREFLOWTYPE_AUTO, datadogV1.DASHBOARDREFLOWTYPE_FIXED))
	}

	return utilserrors.NewAggregate(errs)
}