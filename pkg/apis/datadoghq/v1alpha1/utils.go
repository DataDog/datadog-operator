// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package v1alpha1

// NewInt32Pointer returns pointer on a new int32 value instance
func NewInt32Pointer(i int32) *int32 {
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
