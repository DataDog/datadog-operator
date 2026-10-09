// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package celvalidation

import (
	"fmt"
	"testing"

	"github.com/google/cel-go/cel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	celconfig "k8s.io/apiserver/pkg/apis/cel"
	"k8s.io/apiserver/pkg/cel/environment"
	"k8s.io/utils/ptr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// perRuleCostCeiling is the share of the API server's per-expression limit one
// rule may use on a deliberately oversized object.
//
// The margin is the point. How much a CEL function costs has changed between
// library versions before, so a rule that merely fits today could stop fitting
// after a dependency bump. Holding every rule to a small fraction of the limit
// means a cost change has to be large before a rule starts erroring on a
// cluster, where failurePolicy: Ignore would hide it.
const perRuleCostCeiling = celconfig.PerCallLimit / 100

// totalCostCeiling is the share of the budget the API server gives all the
// validations of one request. The rule sets are checked against a fraction of
// it for the same reason.
const totalCostCeiling = celconfig.RuntimeCELCostBudget / 10

// TestRuleCostsStayWellBelowLimits evaluates each rule set against an object
// larger than a real one and checks every rule, and the set as a whole, against
// the API server's limits.
//
// Two caveats on what this number is. The validating code the operator runs the
// rules through does not report cost: ValidateResult carries decisions and
// nothing else, and the remaining budget stays inside the validator. So the
// measurement here compiles the same expressions in the same pinned environment
// and runs them directly through cel-go, which is a second path and not the one
// production uses. And it evaluates an unstructured map declared as DynType,
// while the API server charges a schema-typed object. Both make this a strong
// signal rather than the real figure; envtest is what would produce that.
func TestRuleCostsStayWellBelowLimits(t *testing.T) {
	t.Run("DatadogAgent", func(t *testing.T) {
		assertRuleCosts(t, v2alpha1.DatadogAgentValidationRules(), largeDatadogAgent())
	})

	t.Run("DatadogAgentProfile", func(t *testing.T) {
		assertRuleCosts(t, v1alpha1.DatadogAgentProfileValidationRules(), largeDatadogAgentProfile())
	})
}

func assertRuleCosts(t *testing.T, rules []apicommon.ValidationRule, obj runtime.Object) {
	t.Helper()

	costs := measureRuleCosts(t, rules, obj)
	require.Len(t, costs, len(rules))

	var total, worst uint64
	worstRule := "(none)"
	for i, cost := range costs {
		total += cost
		if cost > worst {
			worst, worstRule = cost, rules[i].Message
		}
		// Report by rule message, never by index: the rule sets are generated,
		// so an index moves whenever a field is added to one of the structs.
		assert.LessOrEqualf(t, cost, uint64(perRuleCostCeiling),
			"rule %q cost %d, which is over the %d ceiling (%d%% of PerCallLimit)",
			rules[i].Message, cost, perRuleCostCeiling, 100*perRuleCostCeiling/celconfig.PerCallLimit)
	}

	t.Logf("%d rules, total cost %d (ceiling %d, API server budget %d); most expensive %d on %q",
		len(rules), total, totalCostCeiling, celconfig.RuntimeCELCostBudget, worst, worstRule)

	assert.LessOrEqualf(t, total, uint64(totalCostCeiling),
		"the rule set cost %d in total, over the %d ceiling", total, totalCostCeiling)
}

// measureRuleCosts runs each rule against obj through cel-go directly and
// returns what it cost. See the caveats on TestRuleCostsStayWellBelowLimits for
// why this does not go through CompiledRules.
func measureRuleCosts(t *testing.T, rules []apicommon.ValidationRule, obj runtime.Object) []uint64 {
	t.Helper()

	base, err := envSet()
	require.NoError(t, err)
	// Declare `object` the way the API server's compiler declares it. Only
	// `object` is declared here, so this doubles as the check that no rule
	// reaches for oldObject, request or namespaceObject, which the validating
	// code does declare but leaves empty in the operator.
	extended, err := base.Extend(environment.VersionedOptions{
		IntroducedVersion: CompatibilityVersion,
		EnvOptions:        []cel.EnvOption{cel.Variable("object", cel.DynType)},
	})
	require.NoError(t, err)
	env, err := extended.Env(environment.NewExpressions)
	require.NoError(t, err)

	asMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	require.NoError(t, err)
	activation := map[string]any{"object": asMap}

	costs := make([]uint64, len(rules))
	for i, rule := range rules {
		ast, issues := env.Compile(rule.Expression)
		require.NoErrorf(t, issues.Err(), "rule %q", rule.Message)

		// CostLimit both applies the API server's per-expression ceiling and
		// turns on the cost tracking that ActualCost reads.
		program, err := env.Program(ast, cel.CostLimit(celconfig.PerCallLimit))
		require.NoErrorf(t, err, "rule %q", rule.Message)

		_, details, err := program.Eval(activation)
		require.NoErrorf(t, err, "rule %q", rule.Message)
		if cost := details.ActualCost(); cost != nil {
			costs[i] = *cost
		}
	}
	return costs
}

// largeDatadogAgent returns a DatadogAgent with far more commonLabels than a
// real one, since the reserved-prefix rules iterate that map.
func largeDatadogAgent() *v2alpha1.DatadogAgent {
	labels := make(map[string]string, 200)
	for i := 0; i < 200; i++ {
		labels[fmt.Sprintf("example.com/label-with-a-fairly-long-key-%d", i)] = fmt.Sprintf("value-%d", i)
	}
	return &v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Global: &v2alpha1.GlobalConfig{
				Credentials:  &v2alpha1.DatadogCredentials{APIKey: ptr.To("key")},
				CommonLabels: labels,
			},
		},
	}
}

// largeDatadogAgentProfile returns a profile that exercises every map the rules
// iterate: node affinity requirements, component overrides, and one container
// override per supported container, each with many env vars and volume mounts.
func largeDatadogAgentProfile() *v1alpha1.DatadogAgentProfile {
	requirements := make([]corev1.NodeSelectorRequirement, 0, 20)
	for i := 0; i < 20; i++ {
		requirements = append(requirements, corev1.NodeSelectorRequirement{
			Key:      fmt.Sprintf("example.com/node-label-%d", i),
			Operator: corev1.NodeSelectorOpIn,
			Values:   []string{fmt.Sprintf("value-%d", i)},
		})
	}

	containers := map[apicommon.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{}
	for name := range v1alpha1.DatadogAgentProfileSupportedContainers {
		env := make([]corev1.EnvVar, 0, 100)
		mounts := make([]corev1.VolumeMount, 0, 50)
		for i := 0; i < 100; i++ {
			env = append(env, corev1.EnvVar{
				Name:  fmt.Sprintf("DD_SOME_REASONABLY_LONG_VARIABLE_NAME_%d", i),
				Value: fmt.Sprintf("value-%d", i),
			})
		}
		for i := 0; i < 50; i++ {
			mounts = append(mounts, corev1.VolumeMount{
				Name:      fmt.Sprintf("volume-%d", i),
				MountPath: fmt.Sprintf("/var/run/datadog/path-%d", i),
			})
		}
		containers[name] = &v2alpha1.DatadogAgentGenericContainer{Env: env, VolumeMounts: mounts}
	}

	return &v1alpha1.DatadogAgentProfile{
		Spec: v1alpha1.DatadogAgentProfileSpec{
			ProfileAffinity: &v1alpha1.ProfileAffinity{ProfileNodeAffinity: requirements},
			Config: &v2alpha1.DatadogAgentSpec{
				Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
					v2alpha1.NodeAgentComponentName: {Containers: containers},
				},
			},
		},
	}
}
