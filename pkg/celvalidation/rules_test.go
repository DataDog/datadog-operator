// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package celvalidation

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/cel-go/cel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/cel/environment"
	"sigs.k8s.io/yaml"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// repoRoot is this package's directory, two levels below the repository root.
const repoRoot = "../.."

// exampleDirs are searched for manifests to run the rules against. Every
// DatadogAgent and DatadogAgentProfile found is used; other kinds are skipped.
var exampleDirs = []string{
	"config/samples",
	"test/e2e/manifests",
	"internal/controller/testutils/renderer/testdata",
}

func ddaRules(t *testing.T) *CompiledRules {
	t.Helper()
	rules, err := CompileRules(
		v2alpha1.DatadogAgentValidationRules(),
		v2alpha1.GroupVersion.WithKind("DatadogAgent"),
		v2alpha1.GroupVersion.WithResource("datadogagents"),
	)
	require.NoError(t, err)
	return rules
}

func dapRules(t *testing.T) *CompiledRules {
	t.Helper()
	rules, err := CompileRules(
		v1alpha1.DatadogAgentProfileValidationRules(),
		v1alpha1.GroupVersion.WithKind("DatadogAgentProfile"),
		v1alpha1.GroupVersion.WithResource("datadogagentprofiles"),
	)
	require.NoError(t, err)
	return rules
}

// TestRulesCompile is the guard that every generated expression is valid CEL in
// the pinned environment, uses only functions available there, and references
// only variables the operator binds.
//
// It does not check what the API server checks when the policy object is
// created: schema-based type checking of each expression against the CRD, and
// the cost budget. Those need envtest.
func TestRulesCompile(t *testing.T) {
	require.NotEmpty(t, ddaRules(t).Rules())
	require.NotEmpty(t, dapRules(t).Rules())
}

// TestCompileSkipsOnlyTheBrokenRule pins the behaviour a compile failure must
// have: the broken rule is named and dropped, and every other rule still runs.
// Returning an error from every reconcile instead would be the same failure
// mode failurePolicy: Fail was rejected for on the API server side, where one
// bad rule blocks every edit of the kind.
func TestCompileSkipsOnlyTheBrokenRule(t *testing.T) {
	rules := []apicommon.ValidationRule{
		{Expression: "has(object.spec)", Message: "good rule"},
		{Expression: "object.spec.this is not CEL", Message: "broken rule"},
		// Valid CEL, but uses a function newer than the pin: ip() arrived in
		// 1.30. Compiling with NewExpressions is what rejects it, and is what
		// stops a rule the oldest supported cluster would refuse from shipping.
		{Expression: "ip('10.0.0.1').isLoopback()", Message: "rule past the pin"},
	}

	compiled, err := CompileRules(rules,
		v2alpha1.GroupVersion.WithKind("DatadogAgent"),
		v2alpha1.GroupVersion.WithResource("datadogagents"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "broken rule")
	assert.Contains(t, err.Error(), "rule past the pin")

	require.NotNil(t, compiled, "the surviving rules must still be usable")
	require.Len(t, compiled.Rules(), 1)
	assert.Equal(t, "good rule", compiled.Rules()[0].Message)

	// And the survivor still evaluates.
	failures, err := compiled.Evaluate(context.Background(),
		&unstructured.Unstructured{Object: map[string]any{}}, "ns", "name")
	require.NoError(t, err)
	assert.Equal(t, []string{"good rule"}, failures)
}

// TestRulesAlwaysAnswer is the doc's agreed safeguard: a rule must evaluate to
// true or false on any input, never error.
//
// The distinction matters because the two sides disagree about what an error
// means. A rule that answers false reports our message on both. A rule that
// cannot run at all - `object.spec.global.credentials != null` on a spec with
// no global, say - is skipped by the policy under failurePolicy: Ignore, so
// kubectl apply shows nothing, while the operator's reconcile fails with an
// internal CEL message that is hard to read and worded differently across
// versions. Guarding every field access with has() is what prevents that.
func TestRulesAlwaysAnswer(t *testing.T) {
	cases := []struct {
		kind  string
		rules *CompiledRules
	}{
		{"DatadogAgent", ddaRules(t)},
		{"DatadogAgentProfile", dapRules(t)},
	}

	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			objects := map[string]runtime.Object{
				"empty object": &unstructured.Unstructured{Object: map[string]any{}},
				"empty spec": &unstructured.Unstructured{Object: map[string]any{
					"spec": map[string]any{},
				}},
			}
			for name, obj := range loadExamples(t, tc.kind) {
				objects[name] = obj
			}
			// The examples are the point of this test; an empty corpus would
			// make it pass for the wrong reason.
			require.Greater(t, len(objects), 2, "no %s examples found", tc.kind)

			for name, obj := range objects {
				t.Run(name, func(t *testing.T) {
					// Rule failures are expected and fine. Only an evaluation
					// error fails this test.
					_, err := tc.rules.Evaluate(context.Background(), obj, "ns", "name")
					assert.NoError(t, err)
				})
			}
		})
	}
}

// loadExamples returns every manifest of the given kind under exampleDirs,
// keyed by a readable name. They are loaded as unstructured, which is what the
// API server evaluates the rules against.
func loadExamples(t *testing.T, kind string) map[string]runtime.Object {
	t.Helper()

	out := map[string]runtime.Object{}
	for _, dir := range exampleDirs {
		root := filepath.Join(repoRoot, dir)
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			if ext := filepath.Ext(path); ext != ".yaml" && ext != ".yml" {
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for i, doc := range strings.Split(string(raw), "\n---") {
				if strings.TrimSpace(doc) == "" {
					continue
				}
				obj := map[string]any{}
				if err := yaml.Unmarshal([]byte(doc), &obj); err != nil {
					// Not every file here is a manifest; skip what will not parse.
					continue
				}
				u := &unstructured.Unstructured{Object: obj}
				if u.GetKind() != kind || u.GroupVersionKind().Group != "datadoghq.com" {
					continue
				}
				name := filepath.Join(dir, d.Name())
				if i > 0 {
					name += "#" + strconv.Itoa(i)
				}
				out[name] = u
			}
			return nil
		})
		// A missing directory is not a failure: the corpus is a convenience,
		// and the require on its size is what guards against it going empty.
		if err != nil && !os.IsNotExist(err) {
			require.NoError(t, err)
		}
	}
	return out
}

// objectOnlyEnv builds a CEL environment at the pinned version declaring only
// `object`.
//
// The API server's compiler declares `oldObject`, `request` and
// `namespaceObject` as well (plugin/cel/compile.go:254-257), and gives no way
// to turn that off, so CompileRules cannot reject a rule that reaches for one.
// This environment can.
func objectOnlyEnv(t *testing.T) *cel.Env {
	t.Helper()

	base, err := envSet()
	require.NoError(t, err)
	extended, err := base.Extend(environment.VersionedOptions{
		IntroducedVersion: CompatibilityVersion,
		EnvOptions:        []cel.EnvOption{cel.Variable("object", cel.DynType)},
	})
	require.NoError(t, err)
	env, err := extended.Env(environment.NewExpressions)
	require.NoError(t, err)
	return env
}

// TestRulesDeclareOnlyObject fails a rule that references any variable other
// than `object`.
//
// This is the doc's agreed safeguard, and under option C it is the only one.
// The operator binds no old object, no admission request and no namespace, so
// a rule mentioning one of those compiles cleanly, evaluates against an empty
// value and returns a wrong answer rather than an error - which the policy and
// the operator would both report as a plain rule failure. Catching it at
// compile time here makes it a red CI check instead.
func TestRulesDeclareOnlyObject(t *testing.T) {
	env := objectOnlyEnv(t)

	cases := []struct {
		kind  string
		rules []apicommon.ValidationRule
	}{
		{"DatadogAgent", v2alpha1.DatadogAgentValidationRules()},
		{"DatadogAgentProfile", v1alpha1.DatadogAgentProfileValidationRules()},
	}

	// Prove the environment rejects what it is here to catch, so that a change
	// making it permissive shows up as a failure rather than a silent pass.
	t.Run("guard works", func(t *testing.T) {
		for _, expression := range []string{
			"has(oldObject.spec)",
			"request.operation == 'CREATE'",
			"has(namespaceObject.metadata)",
		} {
			_, issues := env.Compile(expression)
			assert.Errorf(t, issues.Err(), "%q should not compile here", expression)
		}
	})

	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			require.NotEmpty(t, tc.rules)
			for _, rule := range tc.rules {
				// Reported by rule message, never by index: the rule sets are
				// generated, so an index moves when a field is added.
				_, issues := env.Compile(rule.Expression)
				assert.NoErrorf(t, issues.Err(),
					"rule %q must reference only `object`", rule.Message)
			}
		})
	}
}
