// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hostprofiler

import (
	"crypto/sha256"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
)

const (
	// seccompSourcePath is the path to the default seccomp profile baked into the collector image.
	seccompSourcePath = "/etc/dd-host-profiler/seccomp.json"
	// loggingSeccompSourcePath is the path to the seccomp profile that also permits logging syscalls.
	loggingSeccompSourcePath = "/etc/dd-host-profiler/logging-seccomp.json"
)

// seccompProfileName returns a name unique to the image and variant, avoiding
// races when multiple host-profiler versions coexist on the same node.
func seccompProfileName(imageRef string, logging bool) string {
	h := sha256.Sum256([]byte(imageRef))
	name := fmt.Sprintf("host-profiler-%x", h[:4])
	if logging {
		name += "-logging"
	}
	return name
}

func defaultCapabilities() []corev1.Capability {
	return []corev1.Capability{
		"BPF",
		"PERFMON",
		"SYS_PTRACE",
		"SYS_RESOURCE",
		"DAC_READ_SEARCH",
		"SYSLOG",
		"CHECKPOINT_RESTORE",
		"IPC_LOCK",
	}
}

func buildSeccompSetupInitContainer(image string, loggingSeccomp bool) corev1.Container {
	dst := fmt.Sprintf("%s/%s", common.SeccompRootVolumePath, seccompProfileName(image, loggingSeccomp))
	var command []string
	if loggingSeccomp {
		// Prefer the logging profile, but fall back to the default if the image predates it
		// so an older image degrades gracefully instead of crash-looping on a missing file.
		command = []string{"sh", "-c", fmt.Sprintf(
			"if [ -f %[1]s ]; then cp %[1]s %[3]s; else echo 'WARNING: logging-seccomp.json not found in image, falling back to default seccomp profile'; cp %[2]s %[3]s; fi",
			loggingSeccompSourcePath, seccompSourcePath, dst,
		)}
	} else {
		command = []string{"cp", seccompSourcePath, dst}
	}
	return corev1.Container{
		Name:    string(apicommon.HostProfilerSeccompSetupContainerName),
		Image:   image,
		Command: command,
		SecurityContext: &corev1.SecurityContext{
			SELinuxOptions: &corev1.SELinuxOptions{Type: defaultSELinuxType},
		},
		VolumeMounts: []corev1.VolumeMount{
			common.GetVolumeMountForSeccomp(),
		},
	}
}
