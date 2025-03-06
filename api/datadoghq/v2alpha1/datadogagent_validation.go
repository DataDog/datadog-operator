// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v2alpha1

import (
	"fmt"
)

// ValidateDatadogAgentProfileSpec is used to check if a DatadogAgentProfileSpec is valid
func ValidateDatadogAgent(dda *DatadogAgent) error {
	// global
	if dda.Spec.Global == nil {
		return fmt.Errorf("global not configured in the DatadogAgent, can't reconcile")
	}

	// creds
	// add more validation from features
	if dda.Spec.Global.Credentials == nil {
		return fmt.Errorf("credentials not configured in the DatadogAgent, can't reconcile")
	}

	// token
	// add more validation from features
	if dda.Spec.Global.ClusterAgentToken == nil || dda.Spec.Global.ClusterAgentTokenSecret == nil {
		return fmt.Errorf("credentials not configured in the DatadogAgent, can't reconcile")
	}

	return nil
}
