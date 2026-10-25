// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package comparison

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagedFieldsMatch(t *testing.T) {
	opts := ManagedFieldsOptions{
		UnorderedLists: []string{"tags", "nested.set"},
		Normalize: func(path string, value any) any {
			if s, ok := value.(string); ok && path == "trimmed" {
				return strings.TrimSpace(s)
			}
			return value
		},
	}

	tests := []struct {
		name    string
		desired string
		actual  string
		want    bool
	}{
		{
			name:    "fields only present in actual are ignored",
			desired: `{"name":"a","options":{"thresholds":{"critical":1}}}`,
			actual:  `{"name":"a","id":1,"options":{"thresholds":{"critical":1,"ok":0},"notify_audit":false}}`,
			want:    true,
		},
		{
			name:    "scalar difference",
			desired: `{"name":"a"}`,
			actual:  `{"name":"b"}`,
			want:    false,
		},
		{
			name:    "nested scalar difference",
			desired: `{"options":{"thresholds":{"critical":1}}}`,
			actual:  `{"options":{"thresholds":{"critical":2}}}`,
			want:    false,
		},
		{
			name:    "numbers compare by value",
			desired: `{"delay":300}`,
			actual:  `{"delay":300.0}`,
			want:    true,
		},
		{
			name:    "desired field missing from actual",
			desired: `{"name":"a","message":"m"}`,
			actual:  `{"name":"a"}`,
			want:    false,
		},
		{
			name:    "false is not equivalent to a missing field",
			desired: `{"enabled":false}`,
			actual:  `{}`,
			want:    false,
		},
		{
			name:    "null, empty string, empty list and missing field are equivalent",
			desired: `{"a":null,"b":"","c":[],"d":null}`,
			actual:  `{"a":[],"b":null,"d":""}`,
			want:    true,
		},
		{
			name:    "empty desired object matches any object",
			desired: `{"thresholds":{}}`,
			actual:  `{"thresholds":{"critical":1}}`,
			want:    true,
		},
		{
			name:    "explicit null does not match a value",
			desired: `{"restricted_roles":null}`,
			actual:  `{"restricted_roles":["role"]}`,
			want:    false,
		},
		{
			name:    "type mismatch",
			desired: `{"thresholds":{"critical":1}}`,
			actual:  `{"thresholds":"critical"}`,
			want:    false,
		},
		{
			name:    "unordered lists compare as sets",
			desired: `{"tags":["b","a","a"],"nested":{"set":["y","x"]}}`,
			actual:  `{"tags":["a","b"],"nested":{"set":["x","y"]}}`,
			want:    true,
		},
		{
			name:    "unordered list with an extra element",
			desired: `{"tags":["a"]}`,
			actual:  `{"tags":["a","b"]}`,
			want:    false,
		},
		{
			name:    "ordered lists keep their order",
			desired: `{"list":["a","b"]}`,
			actual:  `{"list":["b","a"]}`,
			want:    false,
		},
		{
			name:    "list elements compare desired fields only",
			desired: `{"list":[{"rrule":"r"}]}`,
			actual:  `{"list":[{"rrule":"r","start":"s"}]}`,
			want:    true,
		},
		{
			name:    "normalized values are equivalent",
			desired: `{"trimmed":"text\n"}`,
			actual:  `{"trimmed":" text"}`,
			want:    true,
		},
		{
			name:    "normalization only applies to its path",
			desired: `{"other":"text\n"}`,
			actual:  `{"other":"text"}`,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ManagedFieldsMatch(json.RawMessage(tt.desired), json.RawMessage(tt.actual), opts)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestManagedFieldsMatch_MarshalError(t *testing.T) {
	_, err := ManagedFieldsMatch(map[string]any{"f": func() {}}, map[string]any{}, ManagedFieldsOptions{})
	assert.Error(t, err)
}
