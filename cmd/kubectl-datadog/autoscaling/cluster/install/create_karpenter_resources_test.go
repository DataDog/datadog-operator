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
