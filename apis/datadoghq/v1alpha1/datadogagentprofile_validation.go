// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	"fmt"
)

// ValidateDatadogAgentProfileSpec is used to check if a DatadogAgentProfileSpec is valid
func ValidateDatadogAgentProfileSpec(spec *DatadogAgentProfileSpec) error {
	// check that profileAffinity contains a set of requirements
	if spec.ProfileAffinity == nil {
		return fmt.Errorf("profileAffinity must be defined")
	}
	if spec.ProfileAffinity.ProfileNodeAffinity == nil {
		return fmt.Errorf("profileNodeAffinity must be defined")
	}
	if len(spec.ProfileAffinity.ProfileNodeAffinity) < 1 {
		return fmt.Errorf("profileNodeAffinity must have at least 1 requirement")
	}

	// validate config
	if spec.Config == nil {
		return fmt.Errorf("config must be defined")
	}
	if spec.Config.Override == nil {
		return fmt.Errorf("config override must be defined")
	}

	return nil
}
