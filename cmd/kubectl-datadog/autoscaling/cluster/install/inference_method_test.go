package install

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInferenceMethod_String(t *testing.T) {
	for _, tc := range []struct {
		name     string
		method   InferenceMethod
		expected string
	}{
		{
			name:     "Nodes method",
			method:   InferenceMethodNodes,
			expected: "nodes",
		},
		{
			name:     "NodeGroups method",
			method:   InferenceMethodNodeGroups,
			expected: "nodegroups",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.method.String()
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestInferenceMethod_Set(t *testing.T) {
	for _, tc := range []struct {
		name        string
		input       string
		expected    InferenceMethod
		expectError bool
	}{
		{
			name:        "Set to nodes",
			input:       "nodes",
			expected:    InferenceMethodNodes,
			expectError: false,
		},
		{
			name:        "Set to nodegroups",
			input:       "nodegroups",
			expected:    InferenceMethodNodeGroups,
			expectError: false,
		},
		{
			name:        "Invalid value",
			input:       "invalid",
			expectError: true,
		},
		{
			name:        "Empty value",
			input:       "",
			expectError: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var method InferenceMethod
			err := method.Set(tc.input)

			if tc.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "inference-method must be one of nodes or nodegroups")
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, method)
			}
		})
	}
}

func TestInferenceMethod_Type(t *testing.T) {
	var method InferenceMethod
	assert.Equal(t, "InferenceMethod", method.Type())
}
