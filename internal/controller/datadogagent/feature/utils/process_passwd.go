// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/experimental"
	"github.com/DataDog/datadog-operator/pkg/allowlistsynchronizer"
	"github.com/DataDog/datadog-operator/pkg/images"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	pkgutils "github.com/DataDog/datadog-operator/pkg/utils"
)

// Keep version selection aligned with Helm's process-passwd-mount-path helper.
// This covers Process Check/Discovery only; Gohai still uses local user lookup.
var (
	processHostPasswdMinVersion          = semver.MustParse("7.86.0")
	processHostPasswdMinAllowlistVersion = semver.MustParse("1.0.8")
	processAllowlistVersionRegexp        = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)
)

// ProcessPasswdMountPath selects a process-specific destination without changing
// shared security mounts or eligibility. HOST_ETC is initialized by the Agent;
// do not override an explicitly configured HOST_ETC or ignore_host_etc here.
func ProcessPasswdMountPath(dda metav1.Object, override *v2alpha1.DatadogAgentComponentOverride, containerName apicommon.AgentContainerName, pod *corev1.PodTemplateSpec) string {
	// Windows does not use the Unix host-passwd resolver. Older Autopilot
	// allowlist pins permit only the legacy destination.
	if dda != nil && dda.GetAnnotations()[kubernetes.ProviderAnnotationKey] == kubernetes.WindowsProvider {
		return common.PasswdMountPath
	}
	if experimental.IsAutopilotEnabled(dda) && !autopilotSupportsProcessHostPasswd(dda) {
		return common.PasswdMountPath
	}

	image, ok := resolveProcessAgentImage(dda, override, containerName, pod)
	if !ok {
		return common.PasswdMountPath
	}
	// Strip build variants, not prereleases such as -devel or -rc.1.
	tag := pkgutils.GetTagFromImageName(image)
	for _, suffix := range []string{images.WindowsServerCoreTagSuffix, images.FullTagSuffix, images.JMXTagSuffix, images.FIPSTagSuffix} {
		tag = strings.TrimSuffix(tag, suffix)
	}
	if tag == "7" || tag == "latest" {
		tag = images.AgentLatestVersion
	}
	// Parse the whole tag: an embedded version in a custom tag is not evidence
	// of compatibility. Exclude prereleases even above the minimum version.
	version, err := semver.NewVersion(tag)
	if err == nil && version.Prerelease() == "" && !version.LessThan(processHostPasswdMinVersion) {
		return "/host/etc/passwd"
	}
	return common.PasswdMountPath
}

// autopilotSupportsProcessHostPasswd follows CreateAllowlistSynchronizer's
// override validation: missing or malformed pins use the shared default.
// Valid older pins must retain /etc/passwd even with a compatible Agent image.
func autopilotSupportsProcessHostPasswd(dda metav1.Object) bool {
	version := dda.GetAnnotations()[experimental.ExperimentalAnnotationPrefix+"/"+experimental.ExperimentalAutopilotAllowlistVersionSubkey]
	if !processAllowlistVersionRegexp.MatchString(version) {
		version = allowlistsynchronizer.DefaultWorkloadAllowlistVersion
	}
	allowlistVersion, err := semver.NewVersion(version)
	return err == nil && !allowlistVersion.LessThan(processHostPasswdMinAllowlistVersion)
}

// resolveProcessAgentImage predicts the image after component and experimental
// overrides, without mutating the pod. A false result means mount selection
// must keep the legacy path.
func resolveProcessAgentImage(dda metav1.Object, override *v2alpha1.DatadogAgentComponentOverride, containerName apicommon.AgentContainerName, pod *corev1.PodTemplateSpec) (string, bool) {
	// Use the pod image when available, otherwise the Operator's bundled default.
	image := images.GetLatestAgentImage()
	if pod != nil {
		for _, container := range pod.Spec.Containers {
			if container.Name == string(containerName) && container.Image != "" {
				image = container.Image
				break
			}
		}
	}
	if override != nil {
		if override.Image != nil {
			// Digests are not version evidence. A registry port without an image
			// tag is also unsafe for OverrideAgentImage's tag-based parser.
			name := override.Image.Name
			if strings.Contains(name, "@") || (strings.Contains(name, ":") && strings.LastIndex(name, ":") < strings.LastIndex(name, "/")) {
				return "", false
			}
			image = images.OverrideAgentImage(image, override.Image)
		}
		// Experimental overrides are keyed by the final container name.
		if container := override.Containers[containerName]; container != nil && container.Name != nil {
			containerName = apicommon.AgentContainerName(*container.Name)
		}
	}
	// Match reconciliation precedence, including per-container experimental
	// overrides. GetAgentVersionFromImage alone incorrectly prioritizes Tag over
	// a tag embedded in Name, and would ignore this final override altogether.
	image, err := experimental.ResolveImageOverride(dda, string(containerName), image)
	return image, err == nil && !strings.Contains(image, "@")
}
