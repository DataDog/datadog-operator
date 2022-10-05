// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"fmt"
	"strconv"

	"github.com/DataDog/datadog-operator/apis/datadoghq/common"
	commonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
)

// Container use to override a corev1.Container with a 2alpha1.DatadogAgentGenericContainer.
func Container(containerName commonv1.AgentContainerName, manager feature.PodTemplateManagers, override *v2alpha1.DatadogAgentGenericContainer) {
	if override == nil {
		return
	}

	if override.LogLevel != nil && *override.LogLevel != "" {
		overrideLogLevel(containerName, manager, *override.LogLevel)
	}

	addEnvs(containerName, manager, override.Env)

	addVolMounts(containerName, manager, override.VolumeMounts)

	if override.HealthPort != nil {
		addHealthPort(containerName, manager, *override.HealthPort)
	}

	for i, container := range manager.PodTemplateSpec().Spec.Containers {
		if container.Name == string(containerName) {
			overrideContainer(&manager.PodTemplateSpec().Spec.Containers[i], override)
		}
	}

	overrideSeccompProfile(containerName, manager, override)

	overrideAppArmorProfile(containerName, manager, override)
}

func overrideLogLevel(containerName commonv1.AgentContainerName, manager feature.PodTemplateManagers, logLevel string) {
	manager.EnvVar().AddEnvVarToContainer(
		containerName,
		&corev1.EnvVar{
			Name:  common.DDLogLevel,
			Value: logLevel,
		},
	)
}

func addEnvs(containerName commonv1.AgentContainerName, manager feature.PodTemplateManagers, envs []corev1.EnvVar) {
	for _, env := range envs {
		e := env
		manager.EnvVar().AddEnvVarToContainer(containerName, &e)
	}
}

func addVolMounts(containerName commonv1.AgentContainerName, manager feature.PodTemplateManagers, mounts []corev1.VolumeMount) {
	for _, mount := range mounts {
		m := mount
		manager.VolumeMount().AddVolumeMountToContainer(&m, containerName)
	}
}

func addHealthPort(containerName commonv1.AgentContainerName, manager feature.PodTemplateManagers, healthPort int32) {
	manager.EnvVar().AddEnvVarToContainer(
		containerName,
		&corev1.EnvVar{
			Name:  common.DDHealthPort,
			Value: strconv.Itoa(int(healthPort)),
		},
	)
}

func overrideContainer(container *corev1.Container, override *v2alpha1.DatadogAgentGenericContainer) {
	if override.Name != nil {
		container.Name = *override.Name
	}

	if override.Resources != nil {
		container.Resources = *override.Resources
	}

	if override.Command != nil {
		container.Command = override.Command
	}

	if override.Args != nil {
		container.Args = override.Args
	}

	if override.ReadinessProbe != nil {
		container.ReadinessProbe = override.ReadinessProbe
	}

	if override.LivenessProbe != nil {
		container.LivenessProbe = override.LivenessProbe
	}

	if override.SecurityContext != nil {
		container.SecurityContext = override.SecurityContext
	}
}

func overrideSeccompProfile(containerName commonv1.AgentContainerName, manager feature.PodTemplateManagers, override *v2alpha1.DatadogAgentGenericContainer) {
	// NOTE: for now, only support custom Seccomp Profiles on the System Probe
	if containerName == commonv1.SystemProbeContainerName {
		seccompRootPath := common.SeccompRootVolumePath
		if override.SeccompConfig != nil && override.SeccompConfig.CustomRootPath != nil {
			vol := corev1.Volume{
				Name: common.SeccompRootVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: *override.SeccompConfig.CustomRootPath,
					},
				},
			}
			manager.Volume().AddVolume(&vol)
			seccompRootPath = *override.SeccompConfig.CustomRootPath
		}

		// TODO support ConfigMap creation when ConfigData is used.
		if override.SeccompConfig != nil && override.SeccompConfig.CustomProfile != nil && override.SeccompConfig.CustomProfile.ConfigMap != nil {
			vol := corev1.Volume{
				Name: common.SeccompSecurityVolumeName,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: override.SeccompConfig.CustomProfile.ConfigMap.Name,
						},
						Items: override.SeccompConfig.CustomProfile.ConfigMap.Items,
					},
				},
			}
			manager.Volume().AddVolume(&vol)

			// Add workaround command to seccomp-setup container
			for id, container := range manager.PodTemplateSpec().Spec.InitContainers {
				if container.Name == string(commonv1.SeccompSetupContainerName) {
					manager.PodTemplateSpec().Spec.InitContainers[id].Args = []string{
						fmt.Sprintf("cp %s/%s-seccomp.json %s/%s",
							common.SeccompSecurityVolumePath,
							string(containerName),
							seccompRootPath,
							string(containerName),
						),
					}
				}
				// TODO: Support for custom Seccomp profiles on other containers will require updating the LocalhostProfile.
				// 	manager.PodTemplateSpec().Spec.InitContainers[id].SecurityContext = &corev1.SecurityContext{
				// 		SeccompProfile: &corev1.SeccompProfile{
				// 			Type:             corev1.SeccompProfileTypeLocalhost,
				// 			LocalhostProfile: apiutils.NewStringPointer(containerName),
				// 		},
				// 	}
			}
		}
	}
}

func overrideAppArmorProfile(containerName commonv1.AgentContainerName, manager feature.PodTemplateManagers, override *v2alpha1.DatadogAgentGenericContainer) {
	if override.AppArmorProfileName != nil {
		var annotation string
		if override.Name != nil {
			annotation = fmt.Sprintf("%s/%s", common.AppArmorAnnotationKey, *override.Name)
		} else {
			annotation = fmt.Sprintf("%s/%s", common.AppArmorAnnotationKey, containerName)
		}

		manager.Annotation().AddAnnotation(annotation, *override.AppArmorProfileName)
	}
}
