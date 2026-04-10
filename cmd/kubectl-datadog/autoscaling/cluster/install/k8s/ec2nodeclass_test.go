package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAmiFamilyToAlias(t *testing.T) {
	tests := []struct {
		amiFamily string
		expected  string
	}{
		{"AL2", "al2@latest"},
		{"AL2023", "al2023@latest"},
		{"Bottlerocket", "bottlerocket@latest"},
		{"Windows2019", "windows2019@latest"},
		{"Windows2022", "windows2022@latest"},
		{"Windows2025", "windows2025@latest"},
		{"Custom", ""},
		{"Unknown", ""},
	}

	for _, tt := range tests {
		t.Run(tt.amiFamily, func(t *testing.T) {
			assert.Equal(t, tt.expected, amiFamilyToAlias(tt.amiFamily))
		})
	}
}
