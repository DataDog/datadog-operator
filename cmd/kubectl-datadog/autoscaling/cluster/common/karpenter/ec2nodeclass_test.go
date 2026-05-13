package karpenter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAmiFamilyToAlias(t *testing.T) {
	for _, tc := range []struct {
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
	} {
		t.Run(tc.amiFamily, func(t *testing.T) {
			assert.Equal(t, tc.expected, amiFamilyToAlias(tc.amiFamily))
		})
	}
}
