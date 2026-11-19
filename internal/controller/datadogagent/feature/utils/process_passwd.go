// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/experimental"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
	pkgutils "github.com/DataDog/datadog-operator/pkg/utils"
)

const (
	// processHostPasswdMinVersion is the first Agent version that reads the host's
	// /etc/passwd from /host/etc/passwd. -0 includes prereleases (RC/devel), matching other Agent
	// feature minimum versions.
	processHostPasswdMinVersion = "7.86.0-0"
)

// ProcessPasswdMountPath returns where process containers mount the host's /etc/passwd:
// /host/etc/passwd for node Agent 7.86+, /etc/passwd otherwise. Like other version-gated
// features, the version comes from common.GetComponentVersion, which ignores experimental
// per-container image annotations and container renames. A mismatch only affects process
// usernames. Windows needs no special case because stripLinuxMounts drops this mount.
func ProcessPasswdMountPath(dda metav1.Object) string {
	// TODO: GKE Autopilot keeps /etc/passwd until a published WorkloadAllowlist permits /host/etc/passwd.
	if experimental.IsAutopilotEnabled(dda) {
		return common.PasswdMountPath
	}
	// Unparseable versions, such as digest-only images or custom tags, keep the legacy path.
	unparseableIsSupported := false
	if pkgutils.IsAboveMinVersion(common.GetComponentVersion(dda, v2alpha1.NodeAgentComponentName), processHostPasswdMinVersion, &unparseableIsSupported) {
		return common.HostPasswdMountPath
	}
	return common.PasswdMountPath
}

// ProcessPasswdVolume returns the volume and mount of the host's /etc/passwd for a process
// container. Both paths use the passwd volume, matching Helm. GKE Autopilot will
// need a separate host-passwd volume for /host/etc/passwd because Warden matches
// allowlisted volumeMounts by name.
func ProcessPasswdVolume(mountPath string) (corev1.Volume, corev1.VolumeMount) {
	return volume.GetVolumes(common.PasswdVolumeName, common.PasswdHostPath, mountPath, true)
}
