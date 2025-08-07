// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package experimental

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/pkg/allowlistsynchronizer"
)

var (
	forbiddenAgentVolumes = map[string]struct{}{
	common.AuthVolumeName:            {},
	common.CriSocketVolumeName:       {},
	common.DogstatsdSocketVolumeName: {},
	common.APMSocketVolumeName:       {},
	}

	forbiddenInitMounts = map[string]struct{}{
	common.AuthVolumeName:            {},
	common.CriSocketVolumeName:       {},
	}

	forbiddenCoreAgentMounts = map[string]struct{}{
	common.AuthVolumeName:            {},
	common.CriSocketVolumeName:       {},
	common.DogstatsdSocketVolumeName: {},
	}

	forbiddenTraceAgentMounts = map[string]struct{}{
	common.AuthVolumeName:            {},
	common.CriSocketVolumeName:       {},
	common.DogstatsdSocketVolumeName: {},
	common.ProcdirVolumeName:         {},
	common.CgroupsVolumeName:         {},
	common.APMSocketVolumeName:       {},
	}

	forbiddenProcessAgentMounts = map[string]struct{}{
	common.AuthVolumeName:            {},
	common.CriSocketVolumeName:       {},
	common.DogstatsdSocketVolumeName: {},
	}
)

func IsAutopilotEnabled(obj metav1.Object) bool {
	if obj == nil {
		return false
	}
	ann := obj.GetAnnotations()
	if ann == nil {
		return false
	}

	return strings.EqualFold(ann[getExperimentalAnnotationKey(ExperimentalAutopilotSubkey)], "true")
}

func applyExperimentalAutopilotOverrides(dda metav1.Object, manager feature.PodTemplateManagers) {
	if IsAutopilotEnabled(dda) {
		allowlistsynchronizer.CreateAllowlistSynchronizer()

		if manager.PodTemplateSpec().Labels == nil {
			manager.PodTemplateSpec().Labels = map[string]string{}
		}
		// Prevent the agent DS from being mutated by the admission controller on Autopilot
		manager.PodTemplateSpec().Labels["admission.datadoghq.com/enabled"] = "false"

		// Change args of init-volume
		for i := range manager.PodTemplateSpec().Spec.InitContainers {
			if manager.PodTemplateSpec().Spec.InitContainers[i].Name == "init-volume" {
				manager.PodTemplateSpec().Spec.InitContainers[i].Args = []string{"cp -r /etc/datadog-agent /opt"}
			}
		}

		// Remove agent volumes
		v := manager.PodTemplateSpec().Spec.Volumes[:0]
		for _, vol := range manager.PodTemplateSpec().Spec.Volumes {
			if _, found := forbiddenAgentVolumes[vol.Name]; !found {
				v = append(v, vol)
			}
		}
		manager.PodTemplateSpec().Spec.Volumes = v

		// Remove init-container volume mounts
		for idx := range manager.PodTemplateSpec().Spec.InitContainers {
			vm := []corev1.VolumeMount{}
			for _, m := range manager.PodTemplateSpec().Spec.InitContainers[idx].VolumeMounts {
				if _, found := forbiddenInitMounts[m.Name]; !found {
					vm = append(vm, m)
				}
			}
			manager.PodTemplateSpec().Spec.InitContainers[idx].VolumeMounts = vm
		}

		// Remove core agent container volume mounts
		for idx := range manager.PodTemplateSpec().Spec.Containers {
			if manager.PodTemplateSpec().Spec.Containers[idx].Name == string(apicommon.CoreAgentContainerName) {
				vm := []corev1.VolumeMount{}
				for _, m := range manager.PodTemplateSpec().Spec.Containers[idx].VolumeMounts {
					if _, found := forbiddenCoreAgentMounts[m.Name]; !found {
						vm = append(vm, m)
					}
				}
				manager.PodTemplateSpec().Spec.Containers[idx].VolumeMounts = vm
			}
		}

		// Remove trace agent container volume mounts and change command
		for idx := range manager.PodTemplateSpec().Spec.Containers {
			if manager.PodTemplateSpec().Spec.Containers[idx].Name == string(apicommon.TraceAgentContainerName) {
				vm := []corev1.VolumeMount{}
				for _, m := range manager.PodTemplateSpec().Spec.Containers[idx].VolumeMounts {
					if _, found := forbiddenTraceAgentMounts[m.Name]; !found {
						vm = append(vm, m)
					}
				}
				manager.PodTemplateSpec().Spec.Containers[idx].VolumeMounts = vm

				manager.PodTemplateSpec().Spec.Containers[idx].Command = []string{
					"trace-agent",
					"-config=/etc/datadog-agent/datadog.yaml",
				}
			}
		}

		// Remove process agent container volume mounts and change command
		for idx := range manager.PodTemplateSpec().Spec.Containers {
			if manager.PodTemplateSpec().Spec.Containers[idx].Name == string(apicommon.ProcessAgentContainerName) {
				vm := []corev1.VolumeMount{}
				for _, m := range manager.PodTemplateSpec().Spec.Containers[idx].VolumeMounts {
					if _, found := forbiddenProcessAgentMounts[m.Name]; !found {
						vm = append(vm, m)
					}
				}
				manager.PodTemplateSpec().Spec.Containers[idx].VolumeMounts = vm

				manager.PodTemplateSpec().Spec.Containers[idx].Command = []string{
					"process-agent",
					"-config=/etc/datadog-agent/datadog.yaml",
				}
			}
		}
	}
}
