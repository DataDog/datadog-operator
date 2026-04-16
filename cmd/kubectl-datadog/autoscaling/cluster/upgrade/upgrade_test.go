package upgrade

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidate(t *testing.T) {
	for _, tc := range []struct {
		name        string
		args        []string
		expectError bool
	}{
		{
			name:        "No arguments",
			args:        []string{},
			expectError: false,
		},
		{
			name:        "With arguments",
			args:        []string{"arg1"},
			expectError: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := &options{args: tc.args}
			err := o.validate()

			if tc.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "no arguments are allowed")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestExtractClusterName(t *testing.T) {
	for _, tc := range []struct {
		name     string
		config   map[string]any
		expected string
	}{
		{
			name:     "Nil config",
			config:   nil,
			expected: "",
		},
		{
			name:     "Empty config",
			config:   map[string]any{},
			expected: "",
		},
		{
			name: "Missing settings key",
			config: map[string]any{
				"additionalLabels": map[string]any{},
			},
			expected: "",
		},
		{
			name: "Settings is wrong type",
			config: map[string]any{
				"settings": "not-a-map",
			},
			expected: "",
		},
		{
			name: "Missing clusterName in settings",
			config: map[string]any{
				"settings": map[string]any{
					"interruptionQueue": "my-cluster",
				},
			},
			expected: "",
		},
		{
			name: "clusterName is wrong type",
			config: map[string]any{
				"settings": map[string]any{
					"clusterName": 42,
				},
			},
			expected: "",
		},
		{
			name: "Valid clusterName",
			config: map[string]any{
				"settings": map[string]any{
					"clusterName":       "my-eks-cluster",
					"interruptionQueue": "my-eks-cluster",
				},
			},
			expected: "my-eks-cluster",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := extractClusterName(tc.config)
			assert.Equal(t, tc.expected, result)
		})
	}
}
