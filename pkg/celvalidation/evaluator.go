// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package celvalidation compiles and evaluates CEL validation rules inside the
// operator, using the API server's own validating-admission code.
//
// The API server evaluates the same rule strings through the
// ValidatingAdmissionPolicy that pkg/admissionpolicy creates, but a policy only
// sees new CREATE and UPDATE requests. It never sees an object created before
// the policy existed, a request made while the policy was missing, a request
// where a rule errored under failurePolicy: Ignore, or anything at all on a
// cluster that does not serve ValidatingAdmissionPolicy. So the operator runs
// the rules too. The rule text has one copy; it is evaluated in two places.
//
// This package knows nothing about DatadogAgent or DatadogAgentProfile: it
// compiles and runs the rules it is given, and the callers decide which rules
// apply to what. The only Datadog import is the rule type itself.
package celvalidation

import (
	"context"
	"errors"
	"fmt"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/version"
	"k8s.io/apiserver/pkg/admission"
	plugincel "k8s.io/apiserver/pkg/admission/plugin/cel"
	"k8s.io/apiserver/pkg/admission/plugin/policy/validating"
	celconfig "k8s.io/apiserver/pkg/apis/cel"
	"k8s.io/apiserver/pkg/cel/environment"
	"k8s.io/utils/ptr"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
)

// CompatibilityVersion pins the CEL environment to a Kubernetes version. The
// operator must only use CEL functions that every cluster it installs a policy
// on will accept, and the set of available functions grows with each release.
//
// It is 1.29, one below the 1.30 that ValidatingAdmissionPolicy reached v1 in,
// because a cluster accepts a *new* policy against its compatibility version,
// which is one minor below the cluster version. On a 1.30 cluster that is 1.29
// (DefaultCompatibilityVersion in k8s.io/apiserver@v0.30.0); from 1.31 on it is
// the cluster version minus one. So pinning to 1.30 would let a rule compile
// here, pass CI, and have the oldest supported cluster reject the whole policy
// object.
//
// Raise this with the minimum Kubernetes version pkg/admissionpolicy installs
// on, not with the operator's k8s.io dependency.
var CompatibilityVersion = version.MajorMinor(1, 29)

// CompiledRules is a rule set compiled once and evaluated many times.
//
// The zero value and a nil pointer are both safe: ValidateObject on nil admits
// everything. That is what lets a caller use the result of a CompileRules that
// failed outright without a nil check.
type CompiledRules struct {
	// rules are the rules that compiled, in order. Rules that did not are left
	// out, so an index here is not an index into the caller's slice.
	rules     []common.ValidationRule
	validator validating.Validator
	gvk       schema.GroupVersionKind
	gvr       schema.GroupVersionResource
}

// CompileRules compiles rules with the API server's own validating-admission
// code, at CompatibilityVersion.
//
// Using the API server's code rather than our own loop around cel-go is what
// makes the two evaluations the same one: object conversion, error handling and
// the per-request cost budget all come from the same place, and when those
// internals change the build breaks instead of the results quietly drifting.
//
// Only the validation parts are connected. Policy variables, matchConditions,
// messageExpression and audit annotations are left out, which is the contract
// common.ValidationRule describes: a rule is an expression and a static
// message, and nothing else.
//
// A rule that fails to compile is skipped rather than fatal, and named in the
// returned error. The caller logs that error and keeps the returned rule set:
// the rules are compiled into the binary, so a compile failure is an operator
// bug rather than anything a user did, and refusing to reconcile every object
// in the cluster over one bad rule is the same failure mode that
// failurePolicy: Fail was rejected for on the API server side. The returned
// *CompiledRules is usable in every case, including alongside a non-nil error;
// it is nil only if the compiler itself could not be built, and ValidateObject
// tolerates that too.
func CompileRules(rules []common.ValidationRule, gvk schema.GroupVersionKind, gvr schema.GroupVersionResource) (*CompiledRules, error) {
	envSet, err := envSet()
	if err != nil {
		return nil, err
	}

	// Compile each rule on its own first. The API server keeps a compile error
	// per expression and only surfaces it when that expression runs, so
	// compiling the set in one go would report nothing here and then fail every
	// evaluation. Compiling singly is how the broken ones are identified at
	// startup so the rest can keep running.
	var errs []error
	good := make([]common.ValidationRule, 0, len(rules))
	for _, rule := range rules {
		if compileErr := compileOne(envSet, rule); compileErr != nil {
			errs = append(errs, fmt.Errorf("skipping rule %q: %w", rule.Message, compileErr))
			continue
		}
		good = append(good, rule)
	}

	validator, _, err := newValidator(envSet, good)
	if err != nil {
		return nil, errors.Join(append(errs, err)...)
	}
	return &CompiledRules{rules: good, validator: validator, gvk: gvk, gvr: gvr}, errors.Join(errs...)
}

// compileOne reports whether a single rule compiles.
//
// The compile error has to be read off the condition evaluator rather than the
// validator: Validator.CompileError only reports a failure to build the
// compiler at all, while a rule whose expression does not parse or type-check
// is kept inside the filter and surfaces as an evaluation error on every object.
// That is the asymmetry that makes compiling rules one at a time necessary.
func compileOne(set *environment.EnvSet, rule common.ValidationRule) error {
	_, validations, err := newValidator(set, []common.ValidationRule{rule})
	if err != nil {
		return err
	}
	return errors.Join(validations.CompilationErrors()...)
}

// newValidator wires the pieces of the API server's compilePolicy that apply to
// a bare expression-and-message rule. It returns the validations filter
// alongside the validator so the caller can read compilation errors from it.
func newValidator(set *environment.EnvSet, rules []common.ValidationRule) (validating.Validator, plugincel.ConditionEvaluator, error) {
	compiler, err := plugincel.NewCompositedCompiler(set)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to build the CEL compiler: %w", err)
	}

	conditions := make([]plugincel.ExpressionAccessor, 0, len(rules))
	for i := range rules {
		conditions = append(conditions, &validating.ValidationCondition{
			Expression: rules[i].Expression,
			Message:    rules[i].Message,
		})
	}

	// No params and no authorizer: a rule that reaches for either would have
	// nothing to read here, so not declaring them turns that into a compile
	// error at startup instead of a surprise at evaluation.
	decls := plugincel.OptionalVariableDeclarations{}

	// NewExpressions, not StoredExpressions, is what makes CompatibilityVersion
	// mean anything. StoredExpressions holds the libraries of every Kubernetes
	// version the operator's k8s.io knows, so that a policy a cluster already
	// accepted keeps evaluating after an upgrade. NewExpressions holds only
	// those introduced at or before the pin, which is the set a cluster checks
	// a new policy against. The API server uses the permissive one here because
	// by then the policy has already been accepted; we are on the other side of
	// that check, so we use the strict one.
	validations := compiler.CompileCondition(conditions, decls, environment.NewExpressions)

	// The message and audit-annotation filters must exist even though no rule
	// uses either: Validator.Validate calls both unconditionally, so a nil one
	// panics rather than being treated as absent.
	//
	// A slice of nil accessors, one per rule, is exactly what the API server
	// passes for validations that set no messageExpression, and it leaves every
	// message as the rule's own. No rule has an audit annotation, so that one is
	// empty.
	messages := compiler.CompileCondition(make([]plugincel.ExpressionAccessor, len(rules)), decls, environment.NewExpressions)
	auditAnnotations := compiler.CompileCondition(nil, decls, environment.NewExpressions)

	// Fail, not Ignore: a rule that cannot be evaluated is reported rather than
	// skipped. The policy on the API server side uses Ignore, because there one
	// broken rule would block every edit of the kind; here the operator is the
	// backstop, so silently admitting an object a rule could not judge is the
	// worse of the two.
	return validating.NewValidator(validations, nil, auditAnnotations, messages, ptr.To(admissionregistrationv1.Fail), nil), validations, nil
}

// envSet builds the CEL environment: the Kubernetes base environment at the
// pinned version, exactly as the API server builds its own. Nothing is added to
// it, so the functions available to a rule here are the functions available to
// it there.
func envSet() (*environment.EnvSet, error) {
	set := environment.MustBaseEnvSet(CompatibilityVersion)
	if set == nil {
		return nil, fmt.Errorf("unable to build the CEL environment at %s", CompatibilityVersion)
	}
	return set, nil
}

// Rules returns the rules that compiled, in order. Tests report by rule rather
// than by index: the rule sets are generated, so an index moves whenever a
// field is added to one of the structs.
func (c *CompiledRules) Rules() []common.ValidationRule {
	if c == nil {
		return nil
	}
	return c.rules
}

// Evaluate runs the rules against obj as if it were a CREATE request, and
// returns the messages of every rule that is not true, in rule order.
//
// A rule that cannot be evaluated is an error rather than a failure. A rule
// must not error on any input: every field access is guarded, and the cost
// ceiling is checked by TestRuleCostsStayWellBelowLimits. So reaching that is a
// bug in the rule, and naming the rule is the useful part of the message.
//
// The attributes record is not an API request. NewAttributesRecord only fills
// in a struct that the validating code reads; inside the API server the same
// struct is populated from an incoming request. Nothing is sent to the control
// plane.
func (c *CompiledRules) Evaluate(ctx context.Context, obj runtime.Object, namespace, name string) ([]string, error) {
	if c == nil || len(c.rules) == 0 {
		return nil, nil
	}

	attributes := admission.NewAttributesRecord(obj, nil, c.gvk, namespace, name, c.gvr, "", admission.Create, nil, false, nil)
	versioned := &admission.VersionedAttributes{
		Attributes:      attributes,
		VersionedObject: admission.NewLazyObject(obj),
		VersionedKind:   c.gvk,
	}

	result := c.validator.Validate(ctx, c.gvr, versioned, nil, nil, celconfig.RuntimeCELCostBudget, nil)

	var failed []string
	for i, decision := range result.Decisions {
		if decision.Action != validating.ActionDeny {
			continue
		}
		if decision.Evaluation == validating.EvalError {
			return nil, fmt.Errorf("rule %q could not be evaluated: %s", c.ruleName(i), decision.Message)
		}
		failed = append(failed, decision.Message)
	}
	return failed, nil
}

// ValidateObject returns the messages of every rule that fails, joined.
//
// Every failure is reported rather than just the first, so that the operator
// and the API server say the same thing about the same object: a policy
// evaluates all of its validations and surfaces a warning for each one. The Go
// validators this replaced returned on the first failure, so an object that
// breaks several rules now produces several lines where it used to produce one.
func (c *CompiledRules) ValidateObject(ctx context.Context, obj runtime.Object, namespace, name string) error {
	failed, err := c.Evaluate(ctx, obj, namespace, name)
	if err != nil {
		return err
	}
	errs := make([]error, 0, len(failed))
	for _, message := range failed {
		errs = append(errs, errors.New(message))
	}
	return errors.Join(errs...)
}

// ruleName names the rule a decision came from. Decisions are returned in rule
// order, but a short set can be returned when evaluation stops early, so the
// index is checked.
func (c *CompiledRules) ruleName(i int) string {
	if i < 0 || i >= len(c.rules) {
		return "(unknown)"
	}
	return c.rules[i].Message
}
