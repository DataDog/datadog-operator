package kubernetes

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_createPlatformInfoFromAPIObjects(t *testing.T) {
	tests := []struct {
		name                string
		tag                 string
		apiGroups           []*v1.APIGroup
		apiResourceList     []*v1.APIResourceList
		useV1Beta1PDB       bool
		pdbPreferredVersion string
		pdbOtherVersion     string
	}{
		{
			name: "v1 preferred, PDB v1 prferred, PDB v1beta1 not proferred",
			apiGroups: []*v1.APIGroup{
				newApiGroupPointer(
					v1.APIGroup{
						Name: "policy",
						Versions: []v1.GroupVersionForDiscovery{
							{
								GroupVersion: "policy/v1",
							},
							{
								GroupVersion: "policy/v1beta1",
							},
						},
						PreferredVersion: v1.GroupVersionForDiscovery{
							GroupVersion: "policy/v1",
						},
					},
				),
			},
			apiResourceList:     createDefaultApiResourceList(),
			useV1Beta1PDB:       false,
			pdbPreferredVersion: "policy/v1",
			pdbOtherVersion:     "policy/v1beta1",
		},
		{
			name: "v1beta1 preferred, PDB PDB v1 not proferred",
			tag:  "tag 1",
			apiGroups: []*v1.APIGroup{
				newApiGroupPointer(
					v1.APIGroup{
						Name: "policy",
						Versions: []v1.GroupVersionForDiscovery{
							{
								GroupVersion: "policy/v1",
							},
							{
								GroupVersion: "policy/v1beta1",
							},
						},
						PreferredVersion: v1.GroupVersionForDiscovery{
							GroupVersion: "policy/v1beta1",
						},
					},
				),
			},
			apiResourceList:     createDefaultApiResourceList(),
			useV1Beta1PDB:       true,
			pdbPreferredVersion: "policy/v1beta1",
			pdbOtherVersion:     "policy/v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			platformInfo := NewPlatformInfo(nil, tt.apiGroups, tt.apiResourceList)
			assert.Equal(t, tt.useV1Beta1PDB, platformInfo.UseV1Beta1PDB())
			assert.Equal(t, tt.pdbPreferredVersion, platformInfo.apiPreferredVersions["PodDisruptionBudget"])
			assert.Equal(t, tt.pdbOtherVersion, platformInfo.apiOtherVersions["PodDisruptionBudget"])
		})
	}
}

func Test_getPDBFlag(t *testing.T) {
	tests := []struct {
		name          string
		preferred     map[string]string
		other         map[string]string
		useV1Beta1PDB bool
	}{
		{
			name: "Chooses preferred version of PodDisruptionBudget",
			preferred: map[string]string{
				"PodDisruptionBudget": "policy/v1",
			},
			other: map[string]string{
				"PodDisruptionBudget": "policy/v1beta1",
			},
			useV1Beta1PDB: false,
		},
		{
			name: "Chooses preferred version of PodDisruptionBudget",
			preferred: map[string]string{
				"PodDisruptionBudget": "policy/v1beta1",
			},
			other: map[string]string{
				"PodDisruptionBudget": "policy/v1",
			},
			useV1Beta1PDB: true,
		},
		{
			name: "Unrecognized preferred version, defaults to v1",
			preferred: map[string]string{
				"PodDisruptionBudget": "xyz",
			},
			other:         map[string]string{},
			useV1Beta1PDB: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			platformInfo := NewPlatformInfoFromVersionMaps(nil, tt.preferred, tt.other)
			assert.Equal(t, tt.useV1Beta1PDB, platformInfo.UseV1Beta1PDB())
		})
	}
}

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

func createDefaultApiResourceList() []*v1.APIResourceList {
	return []*v1.APIResourceList{
		newApiResourceListPointer(
			v1.APIResourceList{
				GroupVersion: "policy/v1",
				APIResources: []v1.APIResource{
					{
						Kind: "PodDisruptionBudget",
					},
				},
			},
		),
		newApiResourceListPointer(
			v1.APIResourceList{
				GroupVersion: "policy/v1beta1",
				APIResources: []v1.APIResource{
					{
						Kind: "PodDisruptionBudget",
					},
				},
			},
		),
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

func containsObjectKind(list []ObjectKind, s ObjectKind) bool {
	return slices.Contains(list, s)
}
