package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_getDatadogAgentVersions(t *testing.T) {
	tests := []struct {
		name            string
		apiGroups       []*v1.APIGroup
		apiResourceList []*v1.APIResourceList
		preferred       string
		other           string
	}{
		{
			name: "v2 preferred, v1 other",
			apiGroups: []*v1.APIGroup{
				newApiGroupPointer(
					v1.APIGroup{
						Name: "datadoghq",
						Versions: []v1.GroupVersionForDiscovery{
							{
								GroupVersion: "datadoghq/v1alpha1",
							},
							{
								GroupVersion: "datadoghq/v2alpha1",
							},
						},
						PreferredVersion: v1.GroupVersionForDiscovery{
							GroupVersion: "datadoghq/v2alpha1",
						},
					},
				),
			},
			apiResourceList: createDatadogAgentResourceList(),
			preferred:       "datadoghq/v2alpha1",
			other:           "datadoghq/v1alpha1",
		},
		{
			name: "v2 only, v2 preferred, other empty",
			apiGroups: []*v1.APIGroup{
				newApiGroupPointer(
					v1.APIGroup{
						Name: "datadoghq",
						Versions: []v1.GroupVersionForDiscovery{
							{
								GroupVersion: "datadoghq/v2alpha1",
							},
						},
						PreferredVersion: v1.GroupVersionForDiscovery{
							GroupVersion: "datadoghq/v2alpha1",
						},
					},
				),
			},
			apiResourceList: []*v1.APIResourceList{
				newApiResourceListPointer(
					v1.APIResourceList{
						GroupVersion: "datadoghq/v2alpha1",
						APIResources: []v1.APIResource{
							{
								Kind: "DatadogAgent",
							},
						},
					},
				)},
			preferred: "datadoghq/v2alpha1",
			other:     "",
		},
		{
			name: "No API groups and resources, versions empty",
			apiGroups: []*v1.APIGroup{
				newApiGroupPointer(
					v1.APIGroup{},
				),
			},
			apiResourceList: []*v1.APIResourceList{},
			preferred:       "",
			other:           "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			platformInfo := NewPlatformInfo(nil, tt.apiGroups, tt.apiResourceList)
			preffered, other := platformInfo.GetApiVersions("DatadogAgent")
			assert.Equal(t, tt.preferred, preffered)
			assert.Equal(t, tt.other, other)
		})
	}
}

func createDatadogAgentResourceList() []*v1.APIResourceList {
	return []*v1.APIResourceList{
		newApiResourceListPointer(
			v1.APIResourceList{
				GroupVersion: "datadoghq/v1alpha1",
				APIResources: []v1.APIResource{
					{
						Kind: "DatadogAgent",
					},
				},
			},
		),
		newApiResourceListPointer(
			v1.APIResourceList{
				GroupVersion: "datadoghq/v2alpha1",
				APIResources: []v1.APIResource{
					{
						Kind: "DatadogAgent",
					},
				},
			},
		),
	}
}

func newApiGroupPointer(apiGroup v1.APIGroup) *v1.APIGroup {
	return &apiGroup
}

func newApiResourceListPointer(apiResourceList v1.APIResourceList) *v1.APIResourceList {
	return &apiResourceList
}
