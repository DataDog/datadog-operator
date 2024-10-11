package kubernetes

import (
	policyv1 "k8s.io/api/policy/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PlatformInfo struct {
	versionInfo          *version.Info
	apiPreferredVersions map[string]string
	apiOtherVersions     map[string]string
}

func NewPlatformInfo(versionInfo *version.Info, groups []*v1.APIGroup, resources []*v1.APIResourceList) PlatformInfo {
	preferredGroupVersions := make(map[string]struct{})

	for _, group := range groups {
		preferredGroupVersions[group.PreferredVersion.GroupVersion] = struct{}{}
	}

	preferred := make([]*v1.APIResourceList, 0, len(resources))
	others := make([]*v1.APIResourceList, 0, len(resources))
	for _, list := range resources {
		if _, found := preferredGroupVersions[list.GroupVersion]; found {
			preferred = append(preferred, list)
		} else {
			others = append(others, list)
		}
	}

	apiPreferredVersions := map[string]string{}
	apiOtherVersions := map[string]string{}

	for i := range preferred {
		for j := range preferred[i].APIResources {
			apiPreferredVersions[preferred[i].APIResources[j].Kind] = preferred[i].GroupVersion
		}
	}

	for i := range others {
		for j := range others[i].APIResources {
			apiOtherVersions[others[i].APIResources[j].Kind] = others[i].GroupVersion
		}
	}

	return NewPlatformInfoFromVersionMaps(
		versionInfo,
		apiPreferredVersions,
		apiOtherVersions,
	)
}

func NewPlatformInfoFromVersionMaps(versionInfo *version.Info, apiPreferredVersions, apiOtherVersions map[string]string) PlatformInfo {
	return PlatformInfo{
		versionInfo:          versionInfo,
		apiPreferredVersions: apiPreferredVersions,
		apiOtherVersions:     apiOtherVersions,
	}
}

func (platformInfo *PlatformInfo) UseV1Beta1PDB() bool {
	preferredVersion := platformInfo.apiPreferredVersions["PodDisruptionBudget"]

	// If policy isn't v1beta1 version, we default to v1.
	if preferredVersion == "policy/v1beta1" {
		return true
	} else {
		return false
	}
}

func (platformInfo *PlatformInfo) CreatePDBObject() client.Object {
	if platformInfo.UseV1Beta1PDB() {
		return &policyv1beta1.PodDisruptionBudget{}
	} else {
		return &policyv1.PodDisruptionBudget{}
	}
}

func (platformInfo *PlatformInfo) CreatePDBObjectList() client.ObjectList {
	if platformInfo.UseV1Beta1PDB() {
		return &policyv1beta1.PodDisruptionBudgetList{}
	} else {
		return &policyv1.PodDisruptionBudgetList{}
	}
}

func (platformInfo *PlatformInfo) GetAgentResourcesKind(withCiliumResources bool) []ObjectKind {
	return getResourcesKind(withCiliumResources)
}

// IsResourceSupported returns true if a Kubernetes resource is supported by the server
func (platformInfo *PlatformInfo) IsResourceSupported(resource string) bool {
	if platformInfo == nil {
		return false
	}
	if _, ok := platformInfo.apiPreferredVersions[resource]; ok {
		return true
	}
	if _, ok := platformInfo.apiOtherVersions[resource]; ok {
		return true
	}
	return false
}

func (platformInfo *PlatformInfo) GetApiVersions(name string) (preferred string, other string) {
	preferred = platformInfo.apiPreferredVersions[name]
	other = platformInfo.apiOtherVersions[name]
	return preferred, other
}

func (platformInfo *PlatformInfo) GetVersionInfo() *version.Info {
	return platformInfo.versionInfo
}
