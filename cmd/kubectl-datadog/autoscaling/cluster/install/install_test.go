package install

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateKarpenterResources_String(t *testing.T) {
	for _, tc := range []struct {
		name     string
		resource CreateKarpenterResources
		expected string
	}{
		{
			name:     "None resources",
			resource: CreateKarpenterResourcesNone,
			expected: "none",
		},
		{
			name:     "EC2NodeClass only",
			resource: CreateKarpenterResourcesEC2NodeClass,
			expected: "ec2nodeclass",
		},
		{
			name:     "All resources",
			resource: CreateKarpenterResourcesAll,
			expected: "all",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.resource.String()
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestCreateKarpenterResources_Set(t *testing.T) {
	for _, tc := range []struct {
		name        string
		input       string
		expected    CreateKarpenterResources
		expectError bool
	}{
		{
			name:        "Set to none",
			input:       "none",
			expected:    CreateKarpenterResourcesNone,
			expectError: false,
		},
		{
			name:        "Set to ec2nodeclass",
			input:       "ec2nodeclass",
			expected:    CreateKarpenterResourcesEC2NodeClass,
			expectError: false,
		},
		{
			name:        "Set to all",
			input:       "all",
			expected:    CreateKarpenterResourcesAll,
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
			var resource CreateKarpenterResources
			err := resource.Set(tc.input)

			if tc.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "create-karpenter-resources must be one of none, ec2nodeclass or all")
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, resource)
			}
		})
	}
}

func TestCreateKarpenterResources_Type(t *testing.T) {
	var resource CreateKarpenterResources
	assert.Equal(t, "CreateKarpenterResources", resource.Type())
}

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

func TestValidate(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		args                     []string
		createKarpenterResources CreateKarpenterResources
		inferenceMethod          InferenceMethod
		expectError              bool
		errorContains            string
	}{
		{
			name:                     "Valid with nodes inference method",
			args:                     []string{},
			createKarpenterResources: CreateKarpenterResourcesEC2NodeClass,
			inferenceMethod:          InferenceMethodNodes,
			expectError:              false,
		},
		{
			name:                     "Valid with nodegroups inference method",
			args:                     []string{},
			createKarpenterResources: CreateKarpenterResourcesEC2NodeClass,
			inferenceMethod:          InferenceMethodNodeGroups,
			expectError:              false,
		},
		{
			name:                     "Valid with create none",
			args:                     []string{},
			createKarpenterResources: CreateKarpenterResourcesNone,
			inferenceMethod:          InferenceMethodNodes,
			expectError:              false,
		},
		{
			name:                     "Valid with create all",
			args:                     []string{},
			createKarpenterResources: CreateKarpenterResourcesAll,
			inferenceMethod:          InferenceMethodNodes,
			expectError:              false,
		},
		{
			name:                     "Invalid with arguments",
			args:                     []string{"arg1"},
			createKarpenterResources: CreateKarpenterResourcesEC2NodeClass,
			inferenceMethod:          InferenceMethodNodes,
			expectError:              true,
			errorContains:            "no arguments are allowed",
		},
		{
			name:                     "Invalid with invalid inference method",
			args:                     []string{},
			createKarpenterResources: CreateKarpenterResourcesEC2NodeClass,
			inferenceMethod:          InferenceMethod("invalid"),
			expectError:              true,
			errorContains:            "inference-method must be one of nodes or nodegroups",
		},
		{
			name:                     "Invalid with invalid create resources",
			args:                     []string{},
			createKarpenterResources: CreateKarpenterResources("invalid"),
			inferenceMethod:          InferenceMethodNodes,
			expectError:              true,
			errorContains:            "create-karpenter-resources must be one of none, ec2nodeclass or all",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Save and restore the global variables
			oldCreate := createKarpenterResources
			oldMethod := inferenceMethod
			createKarpenterResources = tc.createKarpenterResources
			inferenceMethod = tc.inferenceMethod
			defer func() {
				createKarpenterResources = oldCreate
				inferenceMethod = oldMethod
			}()

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
