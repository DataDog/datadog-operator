// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package volume

import (
	corev1 "k8s.io/api/core/v1"
)

// ApplyMountPropagation sets the MountPropagation mode on all volume mounts in the PodTemplateSpec
// that are backed by HostPath volumes. This mirrors the Helm chart's hostVolumeMountPropagation setting.
func ApplyMountPropagation(podTemplate *corev1.PodTemplateSpec, mode *corev1.MountPropagationMode) {
	if mode == nil {
		return
	}

	// Build a set of volume names that use HostPath
	hostPathVolumes := make(map[string]struct{})
	for _, vol := range podTemplate.Spec.Volumes {
		if vol.VolumeSource.HostPath != nil {
			hostPathVolumes[vol.Name] = struct{}{}
		}
	}

	// Apply mount propagation to all containers (regular and init) for host-path-backed mounts
	applyToContainers(podTemplate.Spec.Containers, hostPathVolumes, mode)
	applyToContainers(podTemplate.Spec.InitContainers, hostPathVolumes, mode)
}

func applyToContainers(containers []corev1.Container, hostPathVolumes map[string]struct{}, mode *corev1.MountPropagationMode) {
	for i := range containers {
		for j := range containers[i].VolumeMounts {
			if _, ok := hostPathVolumes[containers[i].VolumeMounts[j].Name]; ok {
				// Only set if not already explicitly configured (e.g., by a per-mount override)
				if containers[i].VolumeMounts[j].MountPropagation == nil {
					containers[i].VolumeMounts[j].MountPropagation = mode
				}
			}
		}
	}
}
