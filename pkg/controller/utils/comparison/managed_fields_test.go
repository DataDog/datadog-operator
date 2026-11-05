// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package comparison

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagedFieldsMatch(t *testing.T) {
	tests := []struct {
		name, desired, actual string
		want                  bool
	}{
		{"server defaults", `{"options":{"thresholds":{"critical":80}}}`, `{"id":1,"options":{"thresholds":{"critical":80.0,"warning":70},"include_tags":true}}`, true},
		{"nested drift", `{"options":{"thresholds":{"critical":80}}}`, `{"options":{"thresholds":{"critical":90}}}`, false},
		{"missing field", `{"message":"alert"}`, `{}`, false},
		{"explicit false", `{"enabled":false}`, `{"enabled":true}`, false},
		{"false is not absent", `{"enabled":false}`, `{}`, false},
		{"explicit zero", `{"delay":0}`, `{"delay":300}`, false},
		{"zero is not absent", `{"delay":0}`, `{}`, false},
		{"explicit null", `{"priority":null}`, `{"priority":1}`, false},
		{"null and absent", `{"priority":null}`, `{}`, true},
		{"empty string is managed", `{"message":""}`, `{"message":"alert"}`, false},
		{"empty list is managed", `{"tags":[]}`, `{"tags":["extra"]}`, false},
		{"empty object has no managed fields", `{"options":{}}`, `{"options":{"enabled":true}}`, true},
		{"empty object absent", `{"options":{"thresholds":{}}}`, `{}`, true},
		{"object type mismatch", `{"options":{}}`, `{"options":false}`, false},
		{"nested list defaults", `{"items":[{"rrule":"rule"}]}`, `{"items":[{"rrule":"rule","start":"date"}]}`, true},
		{"extra list item", `{"items":[1]}`, `{"items":[1,2]}`, false},
		{"list order", `{"items":[1,2]}`, `{"items":[2,1]}`, false},
		{"list type mismatch", `{"items":[]}`, `{"items":{}}`, false},
		{"numeric notation", `{"n":8e1}`, `{"n":80.0}`, true},
		{"large integer drift", `{"n":9007199254740992}`, `{"n":9007199254740993}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match, err := ManagedFieldsMatch(json.RawMessage(tt.desired), json.RawMessage(tt.actual), nil)
			require.NoError(t, err)
			assert.Equal(t, tt.want, match)
		})
	}
}

func TestManagedFieldsMatchErrors(t *testing.T) {
	_, err := ManagedFieldsMatch(json.RawMessage(`{`), map[string]any{}, nil)
	require.Error(t, err)
	_, err = ManagedFieldsMatch(map[string]any{}, make(chan int), nil)
	require.Error(t, err)
}
