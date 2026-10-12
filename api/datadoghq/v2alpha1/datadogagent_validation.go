// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v2alpha1

import (
	"fmt"
	"sort"
	"strings"

	k8svalidation "k8s.io/apimachinery/pkg/util/validation"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
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

// Object paths the rules inspect.
const (
	pathSpec         = "object.spec"
	pathGlobal       = "object.spec.global"
	pathCredentials  = "object.spec.global.credentials"
	pathCommonLabels = "object.spec.global.commonLabels"
)

// DatadogAgentValidationRules returns the CEL rules a DatadogAgent must satisfy.
//
// These are the checks that can be phrased on the DatadogAgent as the user
// wrote it. The operator evaluates them at the top of its reconcile, before
// defaulting, so both evaluators see the same object. A check that depends on a
// defaulted value cannot be a rule here, because the API server never sees that
// value.
func DatadogAgentValidationRules() []common.ValidationRule {
	// The credentials rule, then one per reserved prefix.
	rules := make([]common.ValidationRule, 0, 1+len(reservedExtraLabelPrefixes))
	rules = append(rules, common.ValidationRule{
		Expression: fmt.Sprintf("has(%s) && has(%s) && has(%s)", pathSpec, pathGlobal, pathCredentials),
		Message:    "credentials not configured in the DatadogAgent, can't reconcile",
	})

	// One rule per reserved prefix, so the message names the prefix that was
	// matched. A single rule over all three could not, without
	// messageExpression.
	guard := common.ValidationGuard(pathSpec, pathGlobal, pathCommonLabels)
	for _, prefix := range reservedExtraLabelPrefixes {
		rules = append(rules, common.ValidationRule{
			Expression: common.ValidationOr(guard,
				fmt.Sprintf("%s.all(k, !k.startsWith('%s'))", pathCommonLabels, prefix)),
			Message: fmt.Sprintf("spec.global.commonLabels contains a reserved key under prefix %q, "+
				"which is owned by the operator; remove it to avoid interfering with "+
				"operator-internal label-based control flow", prefix),
		})
	}

	return rules
}

// ValidateCommonLabelSyntax returns an error if any key or value in commonLabels
// is not a valid Kubernetes label key/value. Invalid entries would otherwise
// cause the API server to reject the generated resources rather than producing a
// clear DatadogAgent error.
//
// This stays in Go rather than becoming a CEL rule: the check is
// k8svalidation.IsQualifiedName and IsValidLabelValue, whose semantics (optional
// DNS-subdomain prefix, per-segment length limits, charset rules) are more than
// a regex. Approximating them in CEL would make the rule either laxer or
// stricter than the API server's own label validation, with no way to tell
// which. The reserved-prefix half of the check is a CEL rule; see
// DatadogAgentValidationRules.
func ValidateCommonLabelSyntax(commonLabels map[string]string) error {
	// Sort for a deterministic error when several keys are invalid.
	keys := make([]string, 0, len(commonLabels))
	for key := range commonLabels {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		if errs := k8svalidation.IsQualifiedName(key); len(errs) > 0 {
			return fmt.Errorf("spec.global.commonLabels contains invalid label key %q: %s",
				key, strings.Join(errs, "; "))
		}
		if errs := k8svalidation.IsValidLabelValue(commonLabels[key]); len(errs) > 0 {
			return fmt.Errorf("spec.global.commonLabels contains invalid value %q for key %q: %s",
				commonLabels[key], key, strings.Join(errs, "; "))
		}
	}
	return nil
}
