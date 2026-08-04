// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	"fmt"
	"reflect"
	"strings"
	"unicode"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

var datadogAgentProfileFeatureAllowlist = map[string]struct{}{
	"gpu": {},
	"apm": {},
}

var datadogAgentProfileComponentOverrideAllowlist = map[string]struct{}{
	"containers":        {},
	"priorityClassName": {},
	"runtimeClassName":  {},
	"updateStrategy":    {},
	"labels":            {},
	"volumes":           {},
}

var datadogAgentProfileContainerOverrideAllowlist = map[string]struct{}{
	"resources":    {},
	"env":          {},
	"volumeMounts": {},
}

// ValidateDatadogAgentProfileSpec is used to check if a DatadogAgentProfileSpec is valid
func ValidateDatadogAgentProfileSpec(spec *DatadogAgentProfileSpec) error {
	if err := validateProfileAffinity(spec.ProfileAffinity); err != nil {
		return err
	}
	if err := validateConfig(spec.Config); err != nil {
		return err
	}

	return nil
}

func validateProfileAffinity(profileAffinity *ProfileAffinity) error {
	if profileAffinity == nil {
		return undefinedError("profileAffinity")
	}
	if profileAffinity.ProfileNodeAffinity == nil {
		return undefinedError("profileNodeAffinity")
	}
	if len(profileAffinity.ProfileNodeAffinity) < 1 {
		return fmt.Errorf("profileNodeAffinity must have at least 1 requirement")
	}

	return nil
}

func validateConfig(spec *v2alpha1.DatadogAgentSpec) error {
	if spec == nil {
		return undefinedError("config")
	}
	if err := validateFeatures(spec.Features); err != nil {
		return err
	}
	// global is not supported
	if spec.Global != nil {
		return unsupportedError("global")
	}
	for component, override := range spec.Override {
		if err := validateOverride(component, override); err != nil {
			return err
		}
	}

	return nil
}

func validateFeatures(features *v2alpha1.DatadogFeatures) error {
	if features == nil {
		return nil
	}

	return validateAllowlistedFields(features, datadogAgentProfileFeatureAllowlist, jsonFieldName)
}

func validateOverride(component v2alpha1.ComponentName, override *v2alpha1.DatadogAgentComponentOverride) error {
	if component != v2alpha1.NodeAgentComponentName {
		return fmt.Errorf("only node agent componentoverrides are supported")
	}
	if override == nil {
		return undefinedError("component override")
	}

	if err := validateAllowlistedFields(override, datadogAgentProfileComponentOverrideAllowlist, prefixedJSONFieldName("component")); err != nil {
		return err
	}
	for name, override := range override.Containers {
		if err := validateContainerOverride(name, override); err != nil {
			return err
		}
	}

	return nil
}

func validateContainerOverride(name common.AgentContainerName, override *v2alpha1.DatadogAgentGenericContainer) error {
	supportedContainers := map[common.AgentContainerName]struct{}{
		common.CoreAgentContainerName:      {},
		common.TraceAgentContainerName:     {},
		common.ProcessAgentContainerName:   {},
		common.SecurityAgentContainerName:  {},
		common.SystemProbeContainerName:    {},
		common.OtelAgent:                   {},
		common.AgentDataPlaneContainerName: {},
	}
	if _, ok := supportedContainers[name]; !ok {
		return unsupportedError(fmt.Sprintf("container %s", name))
	}
	if override == nil {
		return undefinedError(fmt.Sprintf("container %s", name))
	}

	return validateAllowlistedFields(override, datadogAgentProfileContainerOverrideAllowlist, prefixedJSONFieldName("container"))
}

// For every set field in a struct/pointer, read the JSON name
// (nodeSelector from json:"nodeSelector,omitempty") and check
// the name is in the allowlist. If not in the list, error
func validateAllowlistedFields(value any, allowlist map[string]struct{}, unsupportedFieldName func(reflect.StructField) string) error {
	structValue := reflect.ValueOf(value)
	if structValue.Kind() == reflect.Ptr {
		if structValue.IsNil() {
			return nil
		}
		structValue = structValue.Elem()
	}

	structType := structValue.Type()
	for i := 0; i < structValue.NumField(); i++ {
		if isEmptyOverrideValue(structValue.Field(i)) {
			continue
		}

		field := structType.Field(i)
		if _, ok := allowlist[jsonFieldName(field)]; !ok {
			return unsupportedError(unsupportedFieldName(field))
		}
	}

	return nil
}

func isEmptyOverrideValue(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return value.IsNil()
	default:
		return value.IsZero()
	}
}

func jsonFieldName(field reflect.StructField) string {
	name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
	if name == "" || name == "-" {
		return field.Name
	}

	return name
}

func prefixedJSONFieldName(prefix string) func(reflect.StructField) string {
	return func(field reflect.StructField) string {
		return fmt.Sprintf("%s %s", prefix, splitJSONFieldName(field))
	}
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

func unsupportedError(config string) error {
	return fmt.Errorf("%s override is not supported", config)
}

func undefinedError(config string) error {
	return fmt.Errorf("%s must be defined", config)
}
