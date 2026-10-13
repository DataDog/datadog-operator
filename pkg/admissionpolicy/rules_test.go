// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package admissionpolicy

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	// Blank-imported for its init(): it is the package that registers every
	// feature, so the feature rules below are the same set the operator has.
	// Listing the feature packages here instead would be a second list to keep
	// in step, and a feature missing from it would drop out of the comparison
	// silently - which is the failure this file exists to catch.
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
)

// testDDARules and testDAPRules assemble the rule sets exactly as cmd/main.go
// does, so the tests build the policies the operator would.
func testDDARules() []common.ValidationRule {
	return slices.Concat(v2alpha1.DatadogAgentValidationRules(), feature.ValidationRules())
}

func testDAPRules() []common.ValidationRule {
	return v1alpha1.DatadogAgentProfileValidationRules()
}

func testTargets() []Target {
	return Targets(testDDARules(), testDAPRules())
}

// TestPolicyCarriesEveryRule is the regression test for the policies shipping a
// subset of the rules the operator evaluates.
//
// Both sides are meant to run one copy of each rule (doc Decided 7). Before
// this test, Targets sourced the DatadogAgent rules itself and reached only
// v2alpha1.DatadogAgentValidationRules, so the 15 feature annotation rules of
// Decided 16 were evaluated at reconcile and never at admission. Nothing failed:
// the policies applied cleanly, every rule in them worked, and the missing ones
// were visible only as a warning that never arrived.
//
// Comparing expressions rather than counts is deliberate. A count would pass if
// a rule were swapped for another.
func TestPolicyCarriesEveryRule(t *testing.T) {
	ddaRules, dapRules := testDDARules(), testDAPRules()

	// Guards the premise: if no feature registered a rule, the DatadogAgent
	// case below would be comparing the non-feature rules against themselves
	// and would pass no matter what Targets did with its arguments.
	require.NotEmpty(t, feature.ValidationRules(), "no feature rules registered, so this test proves nothing")

	byName := map[string][]common.ValidationRule{
		DDAPolicyName: ddaRules,
		DAPPolicyName: dapRules,
	}

	targets := Targets(ddaRules, dapRules)
	require.Len(t, targets, len(byName))

	for _, target := range targets {
		t.Run(target.name, func(t *testing.T) {
			want, ok := byName[target.name]
			require.True(t, ok, "unexpected policy %s", target.name)

			got := BuildPolicy(target).Spec.Validations
			require.Len(t, got, len(want), "policy carries a different number of rules than the operator evaluates")

			for i, rule := range want {
				assert.Equal(t, rule.Expression, got[i].Expression)
				assert.Equal(t, rule.Message, got[i].Message)
			}
		})
	}
}
