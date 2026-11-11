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
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	pkgutils "github.com/DataDog/datadog-operator/pkg/utils"
)

const (
	// processHostPasswdMinVersion is the first Agent version that reads host passwd
	// from /host/etc/passwd. -0 includes prereleases (RC/devel), matching other Agent
	// feature minimum versions.
	processHostPasswdMinVersion = "7.86.0-0"
)

// ProcessPasswdMountPath selects a process-specific destination without changing
// shared security mounts or eligibility.
// Like other version-gated features, the Agent version comes from the node Agent
// image override (spec.override.nodeAgent.image), or the Operator's default Agent version.
// Experimental per-container image annotations and container renames are not considered,
// so in rare cases the mount can disagree with the running Agent in either direction.
// The impact is limited to process usernames since an older Agent given /host/etc/passwd
// falls back to image-local users, and a 7.86+ Agent given /etc/passwd behaves as before this change.
func ProcessPasswdMountPath(dda metav1.Object) string {
	// Windows does not use the Unix host passwd resolver.
	if dda != nil && dda.GetAnnotations()[kubernetes.ProviderAnnotationKey] == kubernetes.WindowsProvider {
		return common.PasswdMountPath
	}
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

// ProcessPasswdVolume returns the host passwd volume and mount for a process
// container. Both paths use the passwd volume, matching Helm. GKE Autopilot will
// need a separate host-passwd volume for /host/etc/passwd because Warden matches
// allowlisted volumeMounts by name.
func ProcessPasswdVolume(mountPath string) (corev1.Volume, corev1.VolumeMount) {
	return volume.GetVolumes(common.PasswdVolumeName, common.PasswdHostPath, mountPath, true)
}
