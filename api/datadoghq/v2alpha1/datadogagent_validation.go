// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v2alpha1

import (
	"fmt"
	"strings"
)

// reservedExtraLabelPrefixes holds label-key prefixes that are owned by the
// operator. Users must not set extraLabels keys under these prefixes because
// the operator uses them to drive internal control-flow (profile routing,
// store ownership, DDAI identity, …).
var reservedExtraLabelPrefixes = []string{
	"agent.datadoghq.com/",
	"operator.datadoghq.com/",
	"datadoghq.com/",
}

// ValidateDatadogAgent is used to check if a DatadogAgent is valid
func ValidateDatadogAgent(dda *DatadogAgent) error {
	// TODO
	// Ensure required credentials are configured.
	if dda.Spec.Global == nil || dda.Spec.Global.Credentials == nil {
		return fmt.Errorf("credentials not configured in the DatadogAgent, can't reconcile")
	}

	if err := validateExtraLabels(dda.Spec.Global.ExtraLabels); err != nil {
		return err
	}

	return nil
}

// validateExtraLabels returns an error if any key in extraLabels uses a
// reserved operator-owned prefix.
func validateExtraLabels(extraLabels map[string]string) error {
	for key := range extraLabels {
		for _, prefix := range reservedExtraLabelPrefixes {
			if strings.HasPrefix(key, prefix) {
				return fmt.Errorf(
					"spec.global.extraLabels contains reserved key %q (prefix %q is owned by the operator); "+
						"remove it to avoid interfering with operator-internal label-based control flow",
					key, prefix,
				)
			}
		}
	}
	return nil
}
