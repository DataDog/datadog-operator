// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import "fmt"

// IsValidDatadogPodCheck validates the DatadogPodCheck spec.
func IsValidDatadogPodCheck(spec *DatadogPodCheckSpec) error {
	if len(spec.Selector.MatchLabels) == 0 && len(spec.Selector.MatchAnnotations) == 0 {
		return fmt.Errorf("spec.selector must have at least one of matchLabels or matchAnnotations set")
	}
	return nil
}
