// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	"fmt"

	utilserrors "k8s.io/apimachinery/pkg/util/errors"
)

var allowedCustomResourcesEnumMap = map[SupportedResourcesType]struct{}{
	SyntheticsBrowserTest: {},
	Notebook:              {},
}

func IsValidDatadogGenericCR(spec *DatadogGenericCRSpec) error {
	var errs []error
	if _, ok := allowedCustomResourcesEnumMap[spec.Type]; !ok {
		errs = append(errs, fmt.Errorf("spec.Type must be a supported resource type"))
	}

	if spec.JsonSpec == "" {
		errs = append(errs, fmt.Errorf("spec.JsonSpec must be defined"))
	}

	return utilserrors.NewAggregate(errs)
}
