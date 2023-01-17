package kubernetes

import (
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
		pspPreferredVersion string
		pdbOtherVersion     string
		pspOtherVersion     string
	}{
		{
			name: "v1 preferred, PDB v1 prferred, PSP, PDB v1beta1 not proferred",
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
			pspPreferredVersion: "",
			pdbOtherVersion:     "policy/v1beta1",
			pspOtherVersion:     "policy/v1beta1",
		},
		{
			name: "v1beta1 preferred, PDB, PSP v1beta1 prferred, PDB v1 not proferred",
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
			pspPreferredVersion: "policy/v1beta1",
			pdbOtherVersion:     "policy/v1",
			pspOtherVersion:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			platformInfo := NewPlatformInfo(nil, tt.apiGroups, tt.apiResourceList)
			assert.Equal(t, tt.useV1Beta1PDB, platformInfo.UseV1Beta1PDB())
			assert.Equal(t, tt.pdbPreferredVersion, platformInfo.apiPreferredVersions["PodDisruptionBudget"])
			assert.Equal(t, tt.pspPreferredVersion, platformInfo.apiPreferredVersions["PodSecurityPolicy"])
			assert.Equal(t, tt.pdbOtherVersion, platformInfo.apiOtherVersions["PodDisruptionBudget"])
			assert.Equal(t, tt.pspOtherVersion, platformInfo.apiOtherVersions["PodSecurityPolicy"])
		})
	}
}

func Test_getPDBFlag(t *testing.T) {
	tests := []struct {
		name          string
		preferred     map[string]string
		other         map[string]string
		useV1Beta1PDB bool
		supportsPSP   bool
	}{
		{
			name: "Chooses preferred version of PodDisruptionBudget",
			preferred: map[string]string{
				"PodDisruptionBudget": "policy/v1",
				"PodSecurityPolicy":   "anything",
			},
			other: map[string]string{
				"PodDisruptionBudget": "policy/v1beta1",
			},
			useV1Beta1PDB: false,
			supportsPSP:   true,
		},
		{
			name: "Chooses preferred version of PodDisruptionBudget",
			preferred: map[string]string{
				"PodDisruptionBudget": "policy/v1beta1",
			},
			other: map[string]string{
				"PodDisruptionBudget": "policy/v1",
				"PodSecurityPolicy":   "anything",
			},
			useV1Beta1PDB: true,
			supportsPSP:   true,
		},
		{
			name: "Unrecognized preferred version, defaults to v1",
			preferred: map[string]string{
				"PodDisruptionBudget": "xyz",
			},
			other:         map[string]string{},
			useV1Beta1PDB: false,
			supportsPSP:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			platformInfo := NewPlatformInfoFromVersionMaps(nil, tt.preferred, tt.other)
			assert.Equal(t, tt.useV1Beta1PDB, platformInfo.UseV1Beta1PDB())
			assert.Equal(t, tt.supportsPSP, platformInfo.supportsPSP())
			assert.Equal(t, tt.supportsPSP, containsObjectKind(platformInfo.GetAgentResourcesKind(false), PodSecurityPoliciesKind))
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
					{
						Kind: "PodSecurityPolicy",
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
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
