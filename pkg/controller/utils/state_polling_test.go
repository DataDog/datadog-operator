// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"os"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
)

func TestStatePollingEnabledFromEnv(t *testing.T) {
	tests := []struct {
		name  string
		value *string
		want  bool
	}{
		{name: "unset", value: nil, want: true},
		{name: "true", value: new("true"), want: true},
		{name: "false", value: new("false"), want: false},
		{name: "zero", value: new("0"), want: false},
		{name: "invalid", value: new("nope"), want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setenv restores any host value after the test.
			t.Setenv(StatePollingEnabledEnvVar, "")
			if tt.value != nil {
				t.Setenv(StatePollingEnabledEnvVar, *tt.value)
			} else {
				os.Unsetenv(StatePollingEnabledEnvVar)
			}
			assert.Equal(t, tt.want, StatePollingEnabledFromEnv(logr.Discard()))
		})
	}
}
