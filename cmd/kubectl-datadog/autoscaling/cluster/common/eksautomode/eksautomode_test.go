package eksautomode

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakediscovery "k8s.io/client-go/discovery/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestIsEnabled(t *testing.T) {
	for _, tc := range []struct {
		name      string
		resources []*metav1.APIResourceList
		expected  bool
	}{
		{
			name: "EKS auto-mode is active",
			resources: []*metav1.APIResourceList{
				{
					GroupVersion: "eks.amazonaws.com/v1",
					APIResources: []metav1.APIResource{
						{Name: "nodeclasses", Kind: "NodeClass"},
					},
				},
			},
			expected: true,
		},
		{
			name:      "EKS auto-mode is not active (group not found)",
			resources: []*metav1.APIResourceList{},
			expected:  false,
		},
		{
			name: "EKS auto-mode is not active (group exists but no nodeclasses)",
			resources: []*metav1.APIResourceList{
				{
					GroupVersion: "eks.amazonaws.com/v1",
					APIResources: []metav1.APIResource{
						{Name: "nodeconfigs", Kind: "NodeConfig"},
					},
				},
			},
			expected: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fakeDiscovery := &fakediscovery.FakeDiscovery{
				Fake: &k8stesting.Fake{
					Resources: tc.resources,
				},
			}

			result, err := IsEnabled(fakeDiscovery)
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}
