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
	"github.com/DataDog/datadog-operator/pkg/images"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	pkgutils "github.com/DataDog/datadog-operator/pkg/utils"
)

const (
	// processHostPasswdMinVersion is the first Agent version that reads host passwd
	// from /host/etc/passwd. -0 includes prereleases (RC/devel), matching other Agent
	// feature minimum versions.
	processHostPasswdMinVersion = "7.86.0-0"
)

// processHostPasswdMinAllowlistVersion is the first GKE Autopilot WorkloadAllowlist
// that permits the host-passwd mount at /host/etc/passwd.
var processHostPasswdMinAllowlistVersion = semver.MustParse("1.0.8")

// ProcessPasswdMountPath selects a process-specific destination without changing
// shared security mounts or eligibility.
// Like other version-gated features, the Agent version comes from the node Agent
// image override (spec.override.nodeAgent.image), or the Operator's default Agent version
// when there is no override or the override sets no image name or tag.
// Experimental per-container image annotations and container renames are not considered,
// so in rare cases the mount can disagree with the running Agent in either direction.
// The impact is limited to process usernames since an older Agent given /host/etc/passwd
// falls back to image-local users, and a 7.86+ Agent given /etc/passwd behaves as before this change.
func ProcessPasswdMountPath(dda metav1.Object) string {
	// Windows does not use the Unix host passwd resolver.
	if dda != nil && dda.GetAnnotations()[kubernetes.ProviderAnnotationKey] == kubernetes.WindowsProvider {
		return common.PasswdMountPath
	}
	// An older GKE Autopilot allowlist pin permits only /etc/passwd.
	if experimental.IsAutopilotEnabled(dda) && !autopilotSupportsProcessHostPasswd(dda) {
		return common.PasswdMountPath
	}
	// Unparseable versions, such as digest-only images or custom tags, keep the legacy path.
	unparseableIsSupported := false
	if pkgutils.IsAboveMinVersion(nodeAgentVersion(dda, images.AgentLatestVersion), processHostPasswdMinVersion, &unparseableIsSupported) {
		return common.HostPasswdMountPath
	}
	return common.PasswdMountPath
}

// nodeAgentVersion returns the node Agent version from the DatadogAgent. An image override
// without a name or tag (for example only pullSecrets) keeps the default image tag, so it
// resolves to defaultVersion; it is a parameter so tests can use a 7.86+ default.
func nodeAgentVersion(dda metav1.Object, defaultVersion string) string {
	if version := common.GetComponentVersion(dda, v2alpha1.NodeAgentComponentName); version != "" {
		return version
	}
	return defaultVersion
}

// ProcessPasswdVolume returns the host passwd volume and mount for a process
// container. Both paths use the passwd volume, matching Helm, except on GKE
// Autopilot: Warden matches allowlisted volumeMounts by name, so /host/etc/passwd
// uses the host-passwd volume there.
func ProcessPasswdVolume(dda metav1.Object, mountPath string) (corev1.Volume, corev1.VolumeMount) {
	name := common.PasswdVolumeName
	if mountPath == common.HostPasswdMountPath && experimental.IsAutopilotEnabled(dda) {
		name = common.HostPasswdVolumeName
	}
	return volume.GetVolumes(name, common.PasswdHostPath, mountPath, true)
}

// autopilotSupportsProcessHostPasswd follows CreateAllowlistSynchronizer's pin
// handling: a missing or malformed pin uses the default allowlist, and a valid
// pin older than v1.0.8 keeps /etc/passwd even with a 7.86+ Agent.
func autopilotSupportsProcessHostPasswd(dda metav1.Object) bool {
	pin := dda.GetAnnotations()[experimental.ExperimentalAnnotationPrefix+"/"+experimental.ExperimentalAutopilotAllowlistVersionSubkey]
	allowlistVersion, err := semver.NewVersion(allowlistsynchronizer.ResolveWorkloadAllowlistVersion(pin))
	return err == nil && !allowlistVersion.LessThan(processHostPasswdMinAllowlistVersion)
}
