// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v2alpha1

import "fmt"

// ValidateDatadogAgent is used to check if a DatadogAgent is valid
func ValidateDatadogAgent(dda *DatadogAgent) error {
	// TODO
	// Ensure required credentials are configured.
	if dda.Spec.Global == nil || dda.Spec.Global.Credentials == nil {
		return fmt.Errorf("credentials not configured in the DatadogAgent, can't reconcile")
	}
	return nil
}
