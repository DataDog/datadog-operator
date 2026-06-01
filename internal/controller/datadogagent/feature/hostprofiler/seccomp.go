// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hostprofiler

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
)

const (
	// seccompSourcePath is the path to the seccomp profile baked into the collector image.
	seccompSourcePath  = "/etc/dd-host-profiler/seccomp.json"
	seccompProfileName = "host-profiler"
)

func defaultCapabilities() []corev1.Capability {
	return []corev1.Capability{
		"BPF",
		"PERFMON",
		"SYS_PTRACE",
		"SYS_RESOURCE",
		"DAC_READ_SEARCH",
		"SYSLOG",
		"CHECKPOINT_RESTORE",
	}
}

func buildSeccompSetupInitContainer(image string) corev1.Container {
	return corev1.Container{
		Name:  "host-profiler-seccomp-setup",
		Image: image,
		Command: []string{
			"cp",
			seccompSourcePath,
			fmt.Sprintf("%s/%s", common.SeccompRootVolumePath, seccompProfileName),
		},
		VolumeMounts: []corev1.VolumeMount{
			common.GetVolumeMountForSeccomp(),
		},
	}
}
