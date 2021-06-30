// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	"encoding/json"
)

// NewInt32Pointer returns pointer on a new int32 value instance
func NewInt32Pointer(i int32) *int32 {
	return &i
}

// NewInt64Pointer returns pointer on a new int32 value instance
func NewInt64Pointer(i int64) *int64 {
	return &i
}

// NewStringPointer returns pointer on a new string value instance
func NewStringPointer(s string) *string {
	return &s
}

// NewBoolPointer returns pointer on a new bool value instance
func NewBoolPointer(b bool) *bool {
	return &b
}

// BoolValue return the boolean value, false if nil
func BoolValue(b *bool) bool {
	if b == nil {
		return false
	}

	return *b
}

// BoolToString return "true" if b == true, else "false"
func BoolToString(b *bool) string {
	if BoolValue(b) {
		return "true"
	}

	return "false"
}

// IsEqualStruct is a util fonction that returns whether 2 structures are the same
// We compare the marchaled results to avoid traversing all fields and be agnostic of the struct.
func IsEqualStruct(in interface{}, cmp interface{}) bool {
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
