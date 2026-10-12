// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package admissionpolicy builds and maintains the ValidatingAdmissionPolicy
// objects that run the operator's CR validation rules at admission time.
//
// The rules themselves live next to the types they validate and are evaluated
// by the operator too, through pkg/celvalidation. There is one copy of each rule;
// this package is what hands that copy to the API server.
//
// The CRDs cannot carry the rules instead: the OLM bundle installs every CRD
// unconditionally and the CSV declares minKubeVersion 1.16.0, well below the
// 1.25 that x-kubernetes-validations requires. A ValidatingAdmissionPolicy is a
// separate object created at runtime, so the operator can check whether the
// cluster serves admissionregistration.k8s.io/v1 first and do nothing when it
// does not.
package admissionpolicy

import (
	admregv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

const (
	// FieldOwner identifies the operator in managedFields for server-side apply.
	FieldOwner = "datadog-operator-admission-policy"

	// DAPPolicyName and DDAPolicyName name each policy and its binding.
	//
	// Both are cluster-scoped singletons: one policy governs every object of
	// its kind in the cluster, so there is no per-object instance and no owner
	// to attach them to. The names are fixed, which means two operator installs
	// in one cluster write the same two objects; see the Controller.
	DAPPolicyName = "datadogagentprofiles.validation.datadoghq.com"
	DDAPolicyName = "datadogagents.validation.datadoghq.com"

	datadoghqGroup = "datadoghq.com"
)

// Target is one kind and the rules that apply to it. Each kind needs its own
// policy: the expressions are written against that kind's paths.
type Target struct {
	name       string
	apiVersion string
	resource   string
	rules      []common.ValidationRule
}

// Targets returns the policies to maintain, in a stable order.
//
// Only the one served version of each CRD is matched. A policy that matched a
// version the rules were not written against would evaluate expressions over
// paths that do not exist there.
func Targets() []Target {
	return []Target{
		{
			name:       DDAPolicyName,
			apiVersion: "v2alpha1",
			resource:   "datadogagents",
			rules:      v2alpha1.DatadogAgentValidationRules(),
		},
		{
			name:       DAPPolicyName,
			apiVersion: "v1alpha1",
			resource:   "datadogagentprofiles",
			rules:      v1alpha1.DatadogAgentProfileValidationRules(),
		},
	}
}

// policyLabels marks both objects as operator-managed. They carry no owner
// reference: they are cluster-scoped while the operator is namespaced, and an
// owner reference from a namespaced object to a cluster-scoped one makes the
// garbage collector delete the dependent immediately.
func policyLabels() map[string]string {
	return map[string]string{
		kubernetes.AppKubernetesManageByLabelKey: "datadog-operator",
		kubernetes.AppKubernetesPartOfLabelKey:   "datadog-operator",
	}
}

// matchConditions returns the conditions that must hold for the policy to be
// evaluated at all.
//
// The spec-changed condition is load-bearing. Naming the bare resource in
// matchConstraints already excludes the /status subresource, but it does not
// exclude writes to the main resource that the operator itself makes, such as
// adding or removing a finalizer. Without this condition, a Deny binding would
// make an already-invalid object impossible to delete. It is kept under Warn so
// the policy is safe to promote.
//
// It compares metadata.generation rather than the specs themselves. Both CRDs
// declare a status subresource, so the API server bumps generation on a spec
// change and leaves it alone for metadata-only writes, which is what this
// condition is trying to tell apart. Comparing the specs would do the same job
// at a cost proportional to the size of the object: match conditions have their
// own budget, RuntimeCELCostBudgetMatchConditions, which is a quarter of the one
// the validations share, and a deep comparison of two DatadogAgent specs is the
// one expression here whose cost grows with how much the user configured. Going
// over it is an evaluation error, and under failurePolicy: Ignore that silently
// skips the policy for exactly the large configurations that most need checking.
//
// Note this also means a metadata-only edit is not re-checked. That was already
// true of the spec comparison, but it matters more once rules read annotations:
// such a rule would not fire on an annotation-only update.
//
// It reads oldObject and request, which rules themselves do not: a rule looks at
// one object and never at the change.
func matchConditions() []admregv1.MatchCondition {
	return []admregv1.MatchCondition{
		{
			Name:       "spec-changed",
			Expression: "request.operation == 'CREATE' || object.metadata.generation != oldObject.metadata.generation",
		},
	}
}

// validations converts the shared rules into policy validations. The expression
// and message are carried across unchanged; that is the whole contract between
// the two evaluators.
func validations(rules []common.ValidationRule) []admregv1.Validation {
	reason := metav1.StatusReasonInvalid
	out := make([]admregv1.Validation, 0, len(rules))
	for _, rule := range rules {
		out = append(out, admregv1.Validation{
			Expression: rule.Expression,
			Message:    rule.Message,
			Reason:     &reason,
		})
	}
	return out
}

// BuildPolicy returns the ValidatingAdmissionPolicy for one target.
//
// failurePolicy is Ignore: a compile, type-check or runtime error in an
// expression skips the policy rather than affecting the request. Under a Warn
// binding Fail would only produce a warning, but Ignore keeps the blast radius
// at zero if the binding is later promoted to Deny, and keeps one broken rule
// from blocking every edit of the kind.
func BuildPolicy(t Target) *admregv1.ValidatingAdmissionPolicy {
	return &admregv1.ValidatingAdmissionPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: admregv1.SchemeGroupVersion.String(),
			Kind:       "ValidatingAdmissionPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   t.name,
			Labels: policyLabels(),
		},
		Spec: admregv1.ValidatingAdmissionPolicySpec{
			FailurePolicy: ptr.To(admregv1.Ignore),
			MatchConstraints: &admregv1.MatchResources{
				ResourceRules: []admregv1.NamedRuleWithOperations{
					{
						RuleWithOperations: admregv1.RuleWithOperations{
							Operations: []admregv1.OperationType{admregv1.Create, admregv1.Update},
							Rule: admregv1.Rule{
								APIGroups:   []string{datadoghqGroup},
								APIVersions: []string{t.apiVersion},
								// The bare resource name deliberately excludes
								// subresources, so the operator's own status
								// writes are never matched.
								Resources: []string{t.resource},
							},
						},
					},
				},
			},
			MatchConditions: matchConditions(),
			Validations:     validations(t.rules),
		},
	}
}

// BuildBinding returns the binding for one target.
//
// validationActions is Warn only. Deny and Warn are mutually exclusive
// upstream, so promoting a policy is a swap rather than an addition, and that
// promotion is a deliberate behaviour change: configurations the operator
// accepts today would start being rejected at apply.
func BuildBinding(t Target) *admregv1.ValidatingAdmissionPolicyBinding {
	return &admregv1.ValidatingAdmissionPolicyBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: admregv1.SchemeGroupVersion.String(),
			Kind:       "ValidatingAdmissionPolicyBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   t.name,
			Labels: policyLabels(),
		},
		Spec: admregv1.ValidatingAdmissionPolicyBindingSpec{
			PolicyName:        t.name,
			ValidationActions: []admregv1.ValidationAction{admregv1.Warn},
		},
	}
}
