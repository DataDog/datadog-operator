// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"unicode"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// DatadogAgentProfileFeatureAllowlist is the set of spec.config.features fields a
// DatadogAgentProfile may set.
var DatadogAgentProfileFeatureAllowlist = map[string]struct{}{
	"gpu": {},
	"apm": {},
}

// DatadogAgentProfileComponentOverrideAllowlist is the set of component override
// fields a DatadogAgentProfile may set.
var DatadogAgentProfileComponentOverrideAllowlist = map[string]struct{}{
	"containers":        {},
	"priorityClassName": {},
	"runtimeClassName":  {},
	"updateStrategy":    {},
	"labels":            {},
	"volumes":           {},
}

// DatadogAgentProfileContainerOverrideAllowlist is the set of container override
// fields a DatadogAgentProfile may set.
var DatadogAgentProfileContainerOverrideAllowlist = map[string]struct{}{
	"resources":    {},
	"env":          {},
	"volumeMounts": {},
}

// DatadogAgentProfileSupportedContainers is the set of container names a
// DatadogAgentProfile may override.
var DatadogAgentProfileSupportedContainers = map[common.AgentContainerName]struct{}{
	common.CoreAgentContainerName:        {},
	common.TraceAgentContainerName:       {},
	common.ProcessAgentContainerName:     {},
	common.SecurityAgentContainerName:    {},
	common.SystemProbeContainerName:      {},
	common.OtelAgent:                     {},
	common.AgentDataPlaneContainerName:   {},
	common.AgentCheckRunnerContainerName: {},
}

// Object paths the rules inspect. Guards use full paths rather than policy
// variables: variables are evaluated lazily and memoized by the API server, and
// reproducing that in the operator's evaluator is the API server behaviour we
// deliberately do not re-create.
const (
	pathSpec         = "object.spec"
	pathProfAffinity = "object.spec.profileAffinity"
	pathNodeAffinity = "object.spec.profileAffinity.profileNodeAffinity"
	pathConfig       = "object.spec.config"
	pathFeatures     = "object.spec.config.features"
	pathGlobal       = "object.spec.config.global"
	pathOverride     = "object.spec.config.override"
	pathNodeAgent    = "object.spec.config.override.nodeAgent"
	pathContainers   = "object.spec.config.override.nodeAgent.containers"
)

// DatadogAgentProfileValidationRules returns the CEL rules a DatadogAgentProfile must
// satisfy. They are generated in the order the previous Go validator evaluated
// its checks, so the first failing rule reports the message that validator
// reported.
//
// The allowlist rules are emitted one per disallowed field rather than one per
// struct. CEL cannot enumerate an object's field names, so an allowlist has to
// become the explicit set of fields that must be absent; emitting one rule per
// field is what lets each message name the offending field the way the Go
// validator did, without messageExpression.
//
// The disallowed sets are derived by reflection from the same exported
// allowlists and structs, so adding a field to DatadogFeatures,
// DatadogAgentComponentOverride or DatadogAgentGenericContainer produces a rule
// for it automatically. Hand-maintaining that inverted list is the one way this
// single copy could silently go stale.
func DatadogAgentProfileValidationRules() []common.ValidationRule {
	// Three fixed rules, then one per disallowed field of each of the three structs.
	rules := make([]common.ValidationRule, 0, 3)
	rules = append(rules,
		// validateProfileAffinity: profileAffinity must be defined.
		common.ValidationRule{
			Expression: fmt.Sprintf("has(%s) && has(%s)", pathSpec, pathProfAffinity),
			Message:    "profileAffinity must be defined",
		},
		// validateProfileAffinity: at least one node affinity requirement.
		//
		// This covers both of the previous validator's cases, an absent
		// profileNodeAffinity and an empty one. They cannot be told apart in
		// both evaluators: the field is omitempty, so an empty list reaches the
		// API server as `profileNodeAffinity: []` but reaches the operator's
		// evaluator as an absent field.
		common.ValidationRule{
			Expression: common.ValidationOr(
				common.ValidationGuard(pathSpec, pathProfAffinity),
				fmt.Sprintf("(has(%s) && size(%s) >= 1)", pathNodeAffinity, pathNodeAffinity),
			),
			Message: "profileNodeAffinity must have at least 1 requirement",
		},
		// validateConfig: config must be defined.
		common.ValidationRule{
			Expression: fmt.Sprintf("has(%s) && has(%s)", pathSpec, pathConfig),
			Message:    "config must be defined",
		},
	)

	// validateFeatures: only allowlisted features may be set.
	featureGuard := common.ValidationGuard(pathSpec, pathConfig, pathFeatures)
	for _, field := range disallowedFields(reflect.TypeFor[v2alpha1.DatadogFeatures](), DatadogAgentProfileFeatureAllowlist) {
		rules = append(rules, common.ValidationRule{
			Expression: common.ValidationOr(featureGuard, common.ValidationAbsent(pathFeatures, field.jsonName)),
			Message:    unsupportedMessage(field.jsonName),
		})
	}

	rules = append(rules,
		// validateConfig: global is not supported.
		common.ValidationRule{
			Expression: common.ValidationOr(common.ValidationGuard(pathSpec, pathConfig), fmt.Sprintf("!has(%s)", pathGlobal)),
			Message:    unsupportedMessage("global"),
		},
		// validateOverride: only the node agent component may be overridden.
		common.ValidationRule{
			Expression: common.ValidationOr(
				common.ValidationGuard(pathSpec, pathConfig, pathOverride),
				fmt.Sprintf("%s.all(c, c == '%s')", pathOverride, v2alpha1.NodeAgentComponentName),
			),
			Message: "only node agent componentoverrides are supported",
		},
	)

	// validateOverride: only allowlisted component override fields may be set.
	componentGuard := common.ValidationGuard(pathSpec, pathConfig, pathOverride, pathNodeAgent)
	for _, field := range disallowedFields(reflect.TypeFor[v2alpha1.DatadogAgentComponentOverride](), DatadogAgentProfileComponentOverrideAllowlist) {
		rules = append(rules, common.ValidationRule{
			Expression: common.ValidationOr(componentGuard, common.ValidationAbsent(pathNodeAgent, field.jsonName)),
			Message:    unsupportedMessage("component " + field.splitName),
		})
	}

	// validateContainerOverride: only supported containers may be overridden.
	//
	// The message lists the supported containers rather than naming the
	// offending one: the container name is a map key, and reporting it would
	// need messageExpression.
	containerGuard := common.ValidationGuard(pathSpec, pathConfig, pathOverride, pathNodeAgent, pathContainers)
	supported := supportedContainerNames()
	rules = append(rules, common.ValidationRule{
		Expression: common.ValidationOr(
			containerGuard,
			fmt.Sprintf("%s.all(n, n in [%s])", pathContainers, common.QuotedCELList(supported)),
		),
		Message: fmt.Sprintf("only the %s containers may be overridden", common.HumanList(supported)),
	})

	// validateContainerOverride: only allowlisted container override fields may be set.
	for _, field := range disallowedFields(reflect.TypeFor[v2alpha1.DatadogAgentGenericContainer](), DatadogAgentProfileContainerOverrideAllowlist) {
		rules = append(rules, common.ValidationRule{
			Expression: common.ValidationOr(
				containerGuard,
				fmt.Sprintf("%s.all(n, !has(%s[n].%s))", pathContainers, pathContainers, common.EscapeCELProperty(field.jsonName)),
			),
			Message: unsupportedMessage("container " + field.splitName),
		})
	}

	return rules
}

// fieldName pairs the JSON name of a field with the spaced form used in
// component and container messages (nodeSelector -> "node selector").
type fieldName struct {
	jsonName  string
	splitName string
}

// disallowedFields returns the fields of t that are not in allowlist, sorted by
// JSON name so rule generation is deterministic.
func disallowedFields(t reflect.Type, allowlist map[string]struct{}) []fieldName {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	out := make([]fieldName, 0, t.NumField())
	for f := range t.Fields() {
		if f.PkgPath != "" {
			continue // unexported, never serialized
		}
		name := jsonFieldName(f)
		if name == "" {
			continue
		}
		if _, ok := allowlist[name]; ok {
			continue
		}
		out = append(out, fieldName{jsonName: name, splitName: splitJSONFieldName(f)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].jsonName < out[j].jsonName })
	return out
}

func supportedContainerNames() []string {
	out := make([]string, 0, len(DatadogAgentProfileSupportedContainers))
	for k := range DatadogAgentProfileSupportedContainers {
		out = append(out, string(k))
	}
	sort.Strings(out)
	return out
}

func unsupportedMessage(config string) string {
	return fmt.Sprintf("%s override is not supported", config)
}

func jsonFieldName(field reflect.StructField) string {
	name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
	if name == "" || name == "-" {
		return field.Name
	}

	return name
}

func splitJSONFieldName(field reflect.StructField) string {
	name := []rune(jsonFieldName(field))
	var words []rune
	for i, r := range name {
		if i > 0 && unicode.IsUpper(r) {
			previous := name[i-1]
			hasNext := i+1 < len(name)
			if unicode.IsLower(previous) || hasNext && unicode.IsLower(name[i+1]) {
				words = append(words, ' ')
			}
		}
		words = append(words, unicode.ToLower(r))
	}

	return string(words)
}

// ValidateOverridesDefined rejects an override that is present but null.
//
// This is not a CEL rule because the two evaluators would not agree on it. A
// null map value reaches the API server as an explicit null, and reaches the
// operator as a nil pointer that the unstructured converter writes back as null
// too - but has() on a null-valued field is presence in one and absence in the
// other depending on the schema, and getting that wrong either misses the case
// or errors inside the rule. The check is cheap and unambiguous in Go, so it
// stays in Go.
//
// Unlike the CEL rules, this runs after them, so a profile with both a null
// override and a disallowed field reports the disallowed field first. The
// previous Go validator reported them in the other order.
func ValidateOverridesDefined(profile *DatadogAgentProfile) error {
	if profile.Spec.Config == nil {
		return nil
	}
	for _, override := range profile.Spec.Config.Override {
		if override == nil {
			return fmt.Errorf("component override must be defined")
		}
		for name, container := range override.Containers {
			if container == nil {
				return fmt.Errorf("container %s override must be defined", name)
			}
		}
	}
	return nil
}
