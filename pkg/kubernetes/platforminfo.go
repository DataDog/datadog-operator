package kubernetes

import (
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/version"
)

type PlatformInfo struct {
	VersionInfo          *version.Info
	ApiPreferredVersions map[string]string
	ApiOtherVersions     map[string]string
}

func (platformInfo *PlatformInfo) UseV1Beta1PDB(logger logr.Logger) bool {
	otherVersion, ok := platformInfo.ApiOtherVersions["PodDisruptionBudget"]
	preferredVersion := platformInfo.ApiPreferredVersions["PodDisruptionBudget"]

	if ok && otherVersion != "" {
		preferredVersion = otherVersion
	}

	if preferredVersion == "policy/v1beta1" {
		return true
	} else if preferredVersion == "policy/v1" {
		return false
	} else {
		logger.Info("Unrecognized PodDisruptionBudget version, defaulting to policy/v1", "current version", preferredVersion)
		return false
	}
}
