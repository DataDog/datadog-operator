package apply

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInstallMode_String(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mode     InstallMode
		expected string
	}{
		{
			name:     "Fargate mode",
			mode:     InstallModeFargate,
			expected: "fargate",
		},
		{
			name:     "Existing-nodes mode",
			mode:     InstallModeExistingNodes,
			expected: "existing-nodes",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.mode.String())
		})
	}
}

func TestInstallMode_Set(t *testing.T) {
	for _, tc := range []struct {
		name        string
		input       string
		expected    InstallMode
		expectError bool
	}{
		{
			name:        "Set to fargate",
			input:       "fargate",
			expected:    InstallModeFargate,
			expectError: false,
		},
		{
			name:        "Set to existing-nodes",
			input:       "existing-nodes",
			expected:    InstallModeExistingNodes,
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
			var mode InstallMode
			err := mode.Set(tc.input)

			if tc.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "install-mode must be one of fargate or existing-nodes")
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, mode)
			}
		})
	}
}

func TestInstallMode_Type(t *testing.T) {
	var mode InstallMode
	assert.Equal(t, "InstallMode", mode.Type())
}
