package kubernetes

import (
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
)

type PlatformInfo struct {
	versionInfo          *version.Info
	apiPreferredVersions map[string]string
	apiOtherVersions     map[string]string
}

func NewPlatformInfo1(versionInfo *version.Info, groups []*v1.APIGroup, resources []*v1.APIResourceList) PlatformInfo {
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

	return NewPlatformInfo(
		versionInfo,
		apiPreferredVersions,
		apiOtherVersions,
	)
}

func NewPlatformInfo(versionInfo *version.Info, apiPreferredVersions, apiOtherVersions map[string]string) PlatformInfo {
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
