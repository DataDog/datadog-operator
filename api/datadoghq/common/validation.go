// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"fmt"
	"strings"
)

// ValidationRule is one CEL check: an expression that must evaluate to true,
// and the message reported when it does not.
//
// A rule is the single definition of a check. The API server runs it through a
// ValidatingAdmissionPolicy the operator creates at runtime, and the operator
// runs the same string at reconcile. There is no second implementation in Go.
//
// Rules are plain data on purpose. Evaluating CEL needs cel-go and
// k8s.io/apiserver, and this module is imported by other projects, so the
// evaluator lives in the operator module and only the definitions live here,
// next to the types they validate.
//
// Expression and Message are the only fields on purpose. The operator runs the
// rules through the API server's own validating-admission code, but only
// connects the validation parts of it: policy variables, matchConditions,
// messageExpression and audit annotations are deliberately unavailable, so
// that a rule means the same thing on both sides.
type ValidationRule struct {
	// Expression is a CEL expression over `object`, which is the resource as
	// submitted. It must evaluate to a bool, and must not error on any input:
	// guard every field access with has().
	Expression string
	// Message is reported verbatim when Expression is false.
	Message string
}

// ValidationGuard builds the leading `!has(a) || !has(b) || ` of a rule, so the
// rule holds vacuously when the fields it inspects are absent. Absence is a
// different rule's concern; this keeps one bad spec from producing several
// redundant messages, and keeps the expression from erroring on a missing
// parent.
func ValidationGuard(paths ...string) string {
	terms := make([]string, 0, len(paths))
	for _, p := range paths {
		terms = append(terms, fmt.Sprintf("!has(%s)", p))
	}
	return strings.Join(terms, " || ")
}

// ValidationOr joins a guard prefix and the rule body.
func ValidationOr(guard, body string) string {
	if guard == "" {
		return body
	}
	return guard + " || " + body
}

// ValidationAbsent asserts that field is not set on base.
func ValidationAbsent(base, field string) string {
	return fmt.Sprintf("!has(%s.%s)", base, EscapeCELProperty(field))
}

// EscapeCELProperty applies the property-name escaping Kubernetes CEL requires.
// None of the current field names need it, but a future field with a dash or a
// CEL keyword would, and silently emitting an invalid expression would be worse
// than handling it here.
func EscapeCELProperty(name string) string {
	reserved := map[string]struct{}{
		"true": {}, "false": {}, "null": {}, "in": {}, "as": {}, "break": {}, "const": {},
		"continue": {}, "else": {}, "for": {}, "function": {}, "if": {}, "import": {},
		"let": {}, "loop": {}, "package": {}, "namespace": {}, "return": {},
	}
	if _, ok := reserved[name]; ok {
		return "__" + name + "__"
	}
	r := strings.NewReplacer("__", "__underscores__", ".", "__dot__", "-", "__dash__", "/", "__slash__")
	return r.Replace(name)
}

// QuotedCELList renders items as a CEL list body: 'a', 'b'.
func QuotedCELList(items []string) string {
	q := make([]string, 0, len(items))
	for _, i := range items {
		q = append(q, "'"+i+"'")
	}
	return strings.Join(q, ", ")
}

// HumanList renders items for a message: "a, b and c".
func HumanList(items []string) string {
	switch len(items) {
	case 0:
		return "(none)"
	case 1:
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

// AnnotationValueRule requires an annotation, where present, to hold one of
// allowed.
//
// The rule holds when the annotation is absent: whether a feature must be
// configured at all is a different question. It only rejects a value the
// reader would not understand.
func AnnotationValueRule(key string, allowed []string, message string) ValidationRule {
	const annotations = "object.metadata.annotations"
	return ValidationRule{
		Expression: fmt.Sprintf(
			"!has(object.metadata) || !has(%s) || !('%s' in %s) || %s['%s'] in [%s]",
			annotations, key, annotations, annotations, key, QuotedCELList(allowed),
		),
		Message: message,
	}
}

// BoolAnnotationValues are the only values a feature enable/disable annotation
// may hold. HasFeatureEnableAnnotation compares against "true" exactly, so
// anything else silently leaves the feature off.
var BoolAnnotationValues = []string{"true", "false"}

// ParseBoolAnnotationValues are the values strconv.ParseBool accepts, for the
// annotations read that way. Narrower than this would reject configurations
// that work today.
var ParseBoolAnnotationValues = []string{
	"1", "t", "T", "TRUE", "true", "True",
	"0", "f", "F", "FALSE", "false", "False",
}

// EnableAnnotationRule is the Decided 16 rule for a feature enable/disable
// annotation.
func EnableAnnotationRule(key string) ValidationRule {
	return AnnotationValueRule(key, BoolAnnotationValues,
		fmt.Sprintf("annotation %s must be exactly \"true\" or \"false\"; any other value leaves the feature off", key))
}
