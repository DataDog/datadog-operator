package aws

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestExtractEC2InstanceID(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		wantID   string
		wantOK   bool
	}{
		{name: "ec2", provider: "aws:///eu-west-3a/i-0123456789abcdef0", wantID: "i-0123456789abcdef0", wantOK: true},
		{name: "ec2 short id", provider: "aws:///us-east-1b/i-abc123", wantID: "i-abc123", wantOK: true},
		{name: "fargate", provider: "aws:///eu-west-3a/fargate-ip-10-0-1-2", wantOK: false},
		{name: "gcp", provider: "gce://project/zone/instance", wantOK: false},
		{name: "empty", provider: "", wantOK: false},
		{name: "missing prefix", provider: "i-0123456789abcdef0", wantOK: false},
		{name: "missing AZ", provider: "aws:////i-0123456789abcdef0", wantOK: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			node := &corev1.Node{Spec: corev1.NodeSpec{ProviderID: tc.provider}}
			id, ok := ExtractEC2InstanceID(node)
			assert.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				assert.Equal(t, tc.wantID, id)
			}
		})
	}
	t.Run("nil node", func(t *testing.T) {
		_, ok := ExtractEC2InstanceID(nil)
		assert.False(t, ok)
	})
}
