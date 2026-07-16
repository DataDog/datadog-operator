// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v2alpha1

import (
	"fmt"
	"strings"

	k8svalidation "k8s.io/apimachinery/pkg/util/validation"
)

// reservedExtraLabelPrefixes holds label-key prefixes that are owned by the
// operator. Users must not set commonLabels keys under these prefixes because
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

	if err := validateCommonLabels(dda.Spec.Global.CommonLabels); err != nil {
		return err
	}

	return nil
}

// validateCommonLabels returns an error if any key in commonLabels uses a
// reserved operator-owned prefix, or if any key or value is not a valid
// Kubernetes label key/value (invalid entries would cause API server
// rejection of generated resources rather than a clear DatadogAgent error).
func validateCommonLabels(commonLabels map[string]string) error {
	for key, value := range commonLabels {
		// Check reserved operator prefixes first.
		for _, prefix := range reservedExtraLabelPrefixes {
			if strings.HasPrefix(key, prefix) {
				return fmt.Errorf(
					"spec.global.commonLabels contains reserved key %q (prefix %q is owned by the operator); "+
						"remove it to avoid interfering with operator-internal label-based control flow",
					key, prefix,
				)
			}
		}

		// Validate Kubernetes label key format.
		if errs := k8svalidation.IsQualifiedName(key); len(errs) > 0 {
			return fmt.Errorf("spec.global.commonLabels contains invalid label key %q: %s",
				key, strings.Join(errs, "; "))
		}

		// Validate Kubernetes label value format.
		if errs := k8svalidation.IsValidLabelValue(value); len(errs) > 0 {
			return fmt.Errorf("spec.global.commonLabels contains invalid value %q for key %q: %s",
				value, key, strings.Join(errs, "; "))
		}
	}
	return nil
}
