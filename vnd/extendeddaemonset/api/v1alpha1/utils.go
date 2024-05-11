// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package v1alpha1

// NewInt32 returns pointer on a new int32 value instance.
func NewInt32(i int32) *int32 {
	return &i
}

// NewBool returns pointer to a new bool value instance.
func NewBool(b bool) *bool {
	return &b
}
