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
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// Container use to override a corev1.Container with a 2alpha1.DatadogAgentGenericContainer.
func Container(containerName commonv1.AgentContainerName, manager feature.PodTemplateManagers, override *v2alpha1.DatadogAgentGenericContainer) {
	if override == nil {
		return
	}

	if override.LogLevel != nil && *override.LogLevel != "" {
		overrideLogLevel(containerName, manager, *override.LogLevel)
	}

	if override.HealthPort != nil {
		addHealthPort(containerName, manager, *override.HealthPort)
	}

	addEnvsToContainer(containerName, manager, override.Env)
	addVolMountsToContainer(containerName, manager, override.VolumeMounts)

	addEnvsToInitContainer(containerName, manager, override.Env)
	addVolMountsToInitContainer(containerName, manager, override.VolumeMounts)

	overrideSeccompProfile(containerName, manager, override)
	overrideAppArmorProfile(containerName, manager, override)

	for i, container := range manager.PodTemplateSpec().Spec.Containers {
		if container.Name == string(containerName) {
			overrideContainer(&manager.PodTemplateSpec().Spec.Containers[i], override)
		}
	}

	for i, initContainer := range manager.PodTemplateSpec().Spec.InitContainers {
		if initContainer.Name == string(containerName) {
			overrideInitContainer(&manager.PodTemplateSpec().Spec.InitContainers[i], override)
		}
	}
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

func addEnvsToContainer(containerName commonv1.AgentContainerName, manager feature.PodTemplateManagers, envs []corev1.EnvVar) {
	for _, env := range envs {
		e := env
		manager.EnvVar().AddEnvVarToContainer(containerName, &e)
	}

}

func addEnvsToInitContainer(containerName commonv1.AgentContainerName, manager feature.PodTemplateManagers, envs []corev1.EnvVar) {
	for _, env := range envs {
		e := env
		manager.EnvVar().AddEnvVarToInitContainer(containerName, &e)
	}
}

func addVolMountsToContainer(containerName commonv1.AgentContainerName, manager feature.PodTemplateManagers, mounts []corev1.VolumeMount) {
	for _, mount := range mounts {
		m := mount
		manager.VolumeMount().AddVolumeMountToContainer(&m, containerName)
	}
}

func addVolMountsToInitContainer(containerName commonv1.AgentContainerName, manager feature.PodTemplateManagers, mounts []corev1.VolumeMount) {
	for _, mount := range mounts {
		m := mount
		manager.VolumeMount().AddVolumeMountToInitContainer(&m, containerName)

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
		container.ReadinessProbe = overrideReadinessProbe(override.ReadinessProbe)
	}

	if override.LivenessProbe != nil {
		container.LivenessProbe = overrideLivenessProbe(override.LivenessProbe)
	}

	if override.SecurityContext != nil {
		container.SecurityContext = override.SecurityContext
	}
}

func overrideInitContainer(initContainer *corev1.Container, override *v2alpha1.DatadogAgentGenericContainer) {
	if override.Name != nil {
		initContainer.Name = *override.Name
	}

	if override.Resources != nil {
		initContainer.Resources = *override.Resources
	}

	if override.Args != nil {
		initContainer.Args = override.Args
	}

	if override.SecurityContext != nil {
		initContainer.SecurityContext = override.SecurityContext
	}
}

func overrideSeccompProfile(containerName commonv1.AgentContainerName, manager feature.PodTemplateManagers, override *v2alpha1.DatadogAgentGenericContainer) {
	// NOTE: for now, only support custom Seccomp Profiles on the System Probe
	if containerName == commonv1.SystemProbeContainerName {
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

			// TODO: Support for custom Seccomp profiles on other containers will require updating the LocalhostProfile.
			// for id, container := range manager.PodTemplateSpec().Spec.InitContainers {
			// 	manager.PodTemplateSpec().Spec.InitContainers[id].SecurityContext = &corev1.SecurityContext{
			// 		SeccompProfile: &corev1.SeccompProfile{
			// 			Type:             corev1.SeccompProfileTypeLocalhost,
			// 			LocalhostProfile: apiutils.NewStringPointer(containerName),
			// 		},
			// 	}
			// }
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

func overrideReadinessProbe(readinessProbeOverride *corev1.Probe) *corev1.Probe {
	// Add default httpGet.path and httpGet.port if not present in readinessProbe override
	if readinessProbeOverride.HTTPGet == nil {
		readinessProbeOverride.HTTPGet = &corev1.HTTPGetAction{
			Path: common.DefaultReadinessProbeHTTPPath,
			Port: intstr.IntOrString{IntVal: common.DefaultAgentHealthPort}}
	}
	return readinessProbeOverride
}

func overrideLivenessProbe(livenessProbeOverride *corev1.Probe) *corev1.Probe {
	// Add default httpGet.path and httpGet.port if not present in livenessProbe override
	if livenessProbeOverride.HTTPGet == nil {
		livenessProbeOverride.HTTPGet = &corev1.HTTPGetAction{
			Path: common.DefaultLivenessProbeHTTPPath,
			Port: intstr.IntOrString{IntVal: common.DefaultAgentHealthPort}}
	}
	return livenessProbeOverride
}
