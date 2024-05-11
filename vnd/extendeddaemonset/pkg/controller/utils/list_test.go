// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContainsString(t *testing.T) {
	list := []string{
		"hello",
		"goodbye",
		"see you",
		"hi!",
	}
	s := "hi!"
	containsString := ContainsString(list, s)
	assert.True(t, containsString)

	s = "see you?"
	containsString = ContainsString(list, s)
	assert.False(t, containsString)

	s = "see"
	containsString = ContainsString(list, s)
	assert.False(t, containsString)

	s = "see you"
	containsString = ContainsString(list, s)
	assert.True(t, containsString)
}

func TestRemoveString(t *testing.T) {
	list := []string{
		"hello",
		"goodbye",
		"see you",
		"hi!",
	}
	s := "hi!"
	result := RemoveString(list, s)
	assert.NotContains(t, result, s)

	s = "see you?"
	result = RemoveString(list, s)
	assert.Equal(t, list, result)
}
