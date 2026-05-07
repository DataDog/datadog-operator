package guess

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectAMIFamilyFromImage(t *testing.T) {
	for _, tc := range []struct {
		imageName string
		expected  string
	}{
		{"amazon-linux-2023-x86_64-standard", "AL2023"},
		{"al2023-ami-2023.7.20250512.0-kernel-6.1-x86_64", "AL2023"},
		{"amazon-linux-2-x86_64-gpu", "AL2"},
		{"amzn2-ami-hvm-2.0.20250101.0-x86_64-gp2", "AL2"},
		{"bottlerocket-aws-k8s-1.30-x86_64-v1.25.0", "Bottlerocket"},
		{"BOTTLEROCKET-aws-k8s-1.30-aarch64-v1.25.0", "Bottlerocket"},
		{"Windows_Server-2025-English-Full-EKS_Optimized-1.30", "Windows2025"},
		{"Windows_Server-2022-English-Full-EKS_Optimized-1.30", "Windows2022"},
		{"Windows_Server-2019-English-Full-EKS_Optimized-1.30", "Windows2019"},
		{"some-custom-ami-name", "Custom"},
	} {
		t.Run(tc.imageName, func(t *testing.T) {
			assert.Equal(t, tc.expected, detectAMIFamilyFromImage(tc.imageName))
		})
	}
}
