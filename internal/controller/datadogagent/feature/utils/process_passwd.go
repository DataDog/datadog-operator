// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"github.com/Masterminds/semver/v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/experimental"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
	"github.com/DataDog/datadog-operator/pkg/allowlistsynchronizer"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	pkgutils "github.com/DataDog/datadog-operator/pkg/utils"
)

// Keep version selection aligned with Helm's process-passwd-mount-path helper.
// This covers Process Check/Discovery only; Gohai still uses local user lookup.
const (
	// -0 includes 7.86.0+ prereleases (RC/devel), matching other Agent feature minimum versions.
	processHostPasswdMinVersion = "7.86.0-0"
)

var processHostPasswdMinAllowlistVersion = semver.MustParse("1.0.8")

// ProcessPasswdMountPath selects a process-specific destination without changing
// shared security mounts or eligibility. HOST_ETC is initialized by the Agent;
// do not override an explicitly configured HOST_ETC or ignore_host_etc here.
//
// Like other version-gated features, the Agent version comes from the node Agent
// image override (spec.override.nodeAgent.image), or the Operator's default image.
// Known gap, shared with those features: experimental per-container image
// annotations and container renames are not considered, so they can make the
// mount disagree with the running image. The impact is limited to process
// usernames: an older Agent given /host/etc/passwd falls back to image-local
// users, and a 7.86+ Agent given /etc/passwd behaves as before this change.
func ProcessPasswdMountPath(dda metav1.Object) string {
	// Windows does not use the Unix host-passwd resolver. Older Autopilot
	// allowlist pins permit only the legacy destination.
	if dda != nil && dda.GetAnnotations()[kubernetes.ProviderAnnotationKey] == kubernetes.WindowsProvider {
		return common.PasswdMountPath
	}
	if experimental.IsAutopilotEnabled(dda) && !autopilotSupportsProcessHostPasswd(dda) {
		return common.PasswdMountPath
	}
	// Unparseable versions, such as digest-only images or custom tags, keep the legacy path.
	fallback := false
	if pkgutils.IsAboveMinVersion(common.GetComponentVersion(dda, v2alpha1.NodeAgentComponentName), processHostPasswdMinVersion, &fallback) {
		return common.HostPasswdMountPath
	}
	return common.PasswdMountPath
}

// ProcessPasswdVolume returns the host passwd volume and mount for a process
// container. It derives the volume name from the mount path so the two always
// match the WorkloadAllowlist (passwd at /etc/passwd, host-passwd at /host/etc/passwd).
func ProcessPasswdVolume(mountPath string) (corev1.Volume, corev1.VolumeMount) {
	return volume.GetVolumes(ProcessPasswdVolumeName(mountPath), common.PasswdHostPath, mountPath, true)
}

// ProcessPasswdVolumeName returns the volume backing a process passwd mount path.
// GKE Warden matches volumeMounts by name, so the relocated path needs its own volume.
func ProcessPasswdVolumeName(mountPath string) string {
	if mountPath == common.HostPasswdMountPath {
		return common.HostPasswdVolumeName
	}
	return common.PasswdVolumeName
}

// autopilotSupportsProcessHostPasswd follows CreateAllowlistSynchronizer's
// override validation: missing or malformed pins use the shared default.
// Valid older pins must retain /etc/passwd even with a compatible Agent image.
func autopilotSupportsProcessHostPasswd(dda metav1.Object) bool {
	pin := dda.GetAnnotations()[experimental.ExperimentalAnnotationPrefix+"/"+experimental.ExperimentalAutopilotAllowlistVersionSubkey]
	allowlistVersion, err := semver.NewVersion(allowlistsynchronizer.ResolveWorkloadAllowlistVersion(pin))
	return err == nil && !allowlistVersion.LessThan(processHostPasswdMinAllowlistVersion)
}
