// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"encoding/json"
	"math/rand/v2"

	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"
)

// BoolValue return the boolean value, false if nil
func BoolValue(b *bool) bool {
	return ptr.Deref(b, false)
}

// StringValue return the string value, "" if nil
func StringValue(s *string) string {
	return ptr.Deref(s, "")
}

// BoolToString return "true" if b == true, else "false"
func BoolToString(b *bool) string {
	if BoolValue(b) {
		return "true"
	}

	return "false"
}

// DefaultBooleanIfUnset sets default value d of a boolean if unset
func DefaultBooleanIfUnset(valPtr **bool, d bool) {
	if *valPtr == nil {
		*valPtr = &d
	}
}

// DefaultInt32IfUnset sets default value d of an int32 if unset
func DefaultInt32IfUnset(valPtr **int32, d int32) {
	if *valPtr == nil {
		*valPtr = &d
	}
}

// DefaultIntIfUnset sets value val of an int if unset
func DefaultIntIfUnset(ptr **int, val int) {
	if *ptr == nil {
		*ptr = &val
	}
}

// DefaultStringIfUnset sets default value d of a string if unset
func DefaultStringIfUnset(valPtr **string, d string) {
	if *valPtr == nil {
		*valPtr = &d
	}
}

// IsEqualStruct is a util function that returns whether 2 structures are the same
// We compare the marshaled results to avoid traversing all fields and be agnostic of the struct.
func IsEqualStruct(in any, cmp any) bool {
	if in == nil {
		return true
	}
	inJSON, err := json.Marshal(in)
	if err != nil {
		return false
	}
	cmpJSON, err := json.Marshal(cmp)
	if err != nil {
		return false
	}
	return string(inJSON) == string(cmpJSON)
}

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

// GenerateRandomString use to generate random string with a define size
func GenerateRandomString(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.IntN(len(letterRunes))]
	}
	return string(b)
}

// YAMLToJSONString converts a YAML string to a JSON string
func YAMLToJSONString(yamlConfigs string) string {
	jsonValue, err := yaml.YAMLToJSON([]byte(yamlConfigs))
	if err != nil {
		return ""
	}
	return string(jsonValue)
}

// NewIntOrStringPointer converts a string value to an IntOrString pointer
func NewIntOrStringPointer(str string) *intstr.IntOrString {
	val := intstr.Parse(str)
	return &val
}
