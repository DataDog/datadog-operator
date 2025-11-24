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
			name:     "None method",
			method:   InferenceMethodNone,
			expected: "none",
		},
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
			name:        "Set to none",
			input:       "none",
			expected:    InferenceMethodNone,
			expectError: false,
		},
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
				assert.Contains(t, err.Error(), "inference-method must be one of none, nodes or nodegroups")
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

func TestValidate(t *testing.T) {
	for _, tc := range []struct {
		name            string
		args            []string
		inferenceMethod InferenceMethod
		expectError     bool
		errorContains   string
	}{
		{
			name:            "Valid with no arguments and valid inference method",
			args:            []string{},
			inferenceMethod: InferenceMethodNone,
			expectError:     false,
		},
		{
			name:            "Valid with nodes inference method",
			args:            []string{},
			inferenceMethod: InferenceMethodNodes,
			expectError:     false,
		},
		{
			name:            "Valid with nodegroups inference method",
			args:            []string{},
			inferenceMethod: InferenceMethodNodeGroups,
			expectError:     false,
		},
		{
			name:            "Invalid with arguments",
			args:            []string{"arg1"},
			inferenceMethod: InferenceMethodNone,
			expectError:     true,
			errorContains:   "no arguments are allowed",
		},
		{
			name:            "Invalid with invalid inference method",
			args:            []string{},
			inferenceMethod: InferenceMethod("invalid"),
			expectError:     true,
			errorContains:   "inference-method must be one of none, nodes or nodegroups",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Save and restore the global inferenceMethod
			oldMethod := inferenceMethod
			inferenceMethod = tc.inferenceMethod
			defer func() { inferenceMethod = oldMethod }()

			o := &options{
				args: tc.args,
			}

			err := o.validate()

			if tc.expectError {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
