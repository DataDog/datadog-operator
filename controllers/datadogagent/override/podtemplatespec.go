// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"fmt"
	"strings"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	"github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object/volume"
	corev1 "k8s.io/api/core/v1"
)

// PodTemplateSpec use to override a corev1.PodTemplateSpec with a 2alpha1.DatadogAgentPodTemplateOverride.
func PodTemplateSpec(manager feature.PodTemplateManagers, override *v2alpha1.DatadogAgentComponentOverride, componentName v2alpha1.ComponentName, ddaName string) {
	// Notice that there are several attributes in a
	// v2alpha1.DatadogAgentComponentOverride, like "Replicas" or "Disabled",
	// that are not related with a pod template spec. The overrides for those
	// attributes are not applied in this function.

	if override == nil {
		return
	}

	// TODO: seccomprootpath, seccompcustomprofile, seccomprofilename.
	// They should only apply to system-probe, and I think there's some setup
	// that we need to do before being able to override them.

	if override.ServiceAccountName != nil {
		manager.PodTemplateSpec().Spec.ServiceAccountName = *override.ServiceAccountName
	}

	if override.Image != nil {
		for i, container := range manager.PodTemplateSpec().Spec.Containers {
			manager.PodTemplateSpec().Spec.Containers[i].Image = overriddenImage(container.Image, override.Image)
		}

		for i, initContainer := range manager.PodTemplateSpec().Spec.InitContainers {
			manager.PodTemplateSpec().Spec.InitContainers[i].Image = overriddenImage(initContainer.Image, override.Image)
		}
	}

	for _, env := range override.Env {
		manager.EnvVar().AddEnvVar(&corev1.EnvVar{
			Name:  env.Name,
			Value: env.Value,
		})
	}

	overrideCustomConfigs(manager, override.CustomConfigurations, componentName, ddaName)

	// Note: override.ExtraConfd.ConfigData requires creating a configmap, so it cannot be handled here
	if override.ExtraConfd != nil && override.ExtraConfd.ConfigMap != nil {
		vol := volume.GetVolumeFromConfigMapConfig(
			override.ExtraConfd.ConfigMap, apicommon.ConfdVolumeName, apicommon.ConfdVolumeName,
		)
		manager.Volume().AddVolume(&vol)
	}

	// Note: override.ExtraChecksd.ConfigData requires creating a configmap, so it cannot be handled here
	if override.ExtraChecksd != nil && override.ExtraChecksd.ConfigMap != nil {
		vol := volume.GetVolumeFromConfigMapConfig(
			override.ExtraChecksd.ConfigMap, apicommon.ChecksdVolumeName, apicommon.ChecksdVolumeName,
		)
		manager.Volume().AddVolume(&vol)
	}

	for agentContainerName, containerOverride := range override.Containers {
		Container(agentContainerName, manager, containerOverride)
	}

	for _, vol := range override.Volumes {
		v := vol
		manager.Volume().AddVolume(&v)
	}

	if override.SecurityContext != nil {
		manager.PodTemplateSpec().Spec.SecurityContext = override.SecurityContext
	}

	if override.PriorityClassName != nil {
		manager.PodTemplateSpec().Spec.PriorityClassName = *override.PriorityClassName
	}

	if override.Affinity != nil {
		manager.PodTemplateSpec().Spec.Affinity = override.Affinity
	}

	for selectorKey, selectorVal := range override.NodeSelector {
		if manager.PodTemplateSpec().Spec.NodeSelector == nil {
			manager.PodTemplateSpec().Spec.NodeSelector = make(map[string]string)
		}

		manager.PodTemplateSpec().Spec.NodeSelector[selectorKey] = selectorVal
	}

	manager.PodTemplateSpec().Spec.Tolerations = append(manager.PodTemplateSpec().Spec.Tolerations, override.Tolerations...)

	for annotationName, annotationVal := range override.Annotations {
		manager.Annotation().AddAnnotation(annotationName, annotationVal)
	}

	for labelName, labelVal := range override.Labels {
		manager.PodTemplateSpec().Labels[labelName] = labelVal
	}

	if override.HostNetwork != nil {
		manager.PodTemplateSpec().Spec.HostNetwork = *override.HostNetwork
	}

	if override.HostPID != nil {
		manager.PodTemplateSpec().Spec.HostPID = *override.HostPID
	}
}

func overrideCustomConfigs(manager feature.PodTemplateManagers, customConfs map[v2alpha1.AgentConfigFileName]v2alpha1.CustomConfig, componentName v2alpha1.ComponentName, ddaName string) {
	for _, customConfig := range customConfs {
		// Note: customConfig.ConfigData requires creating a configmap, so it cannot be handled here
		if customConfig.ConfigMap != nil {
			switch componentName {
			case v2alpha1.NodeAgentComponentName, v2alpha1.ClusterChecksRunnerComponentName:
				vol := volume.GetVolumeFromCustomConfigSpec(
					v2alpha1.ConvertCustomConfig(&customConfig),
					getAgentCustomConfConfigMapName(ddaName),
					apicommon.AgentCustomConfigVolumeName,
				)
				manager.Volume().AddVolume(&vol)

				volumeMount := volume.GetVolumeMountFromCustomConfigSpec(
					v2alpha1.ConvertCustomConfig(&customConfig),
					apicommon.AgentCustomConfigVolumeName,
					apicommon.AgentCustomConfigVolumePath,
					apicommon.AgentCustomConfigVolumeSubPath,
				)
				manager.VolumeMount().AddVolumeMount(&volumeMount)
			case v2alpha1.ClusterAgentComponentName:
				vol := volume.GetVolumeFromCustomConfigSpec(
					v2alpha1.ConvertCustomConfig(&customConfig),
					getClusterAgentCustomConfConfigMapName(ddaName),
					apicommon.ClusterAgentCustomConfigVolumeName,
				)
				manager.Volume().AddVolume(&vol)

				volumeMount := volume.GetVolumeMountFromCustomConfigSpec(
					v2alpha1.ConvertCustomConfig(&customConfig),
					apicommon.ClusterAgentCustomConfigVolumeName,
					apicommon.ClusterAgentCustomConfigVolumePath,
					apicommon.ClusterAgentCustomConfigVolumeSubPath,
				)
				manager.VolumeMount().AddVolumeMount(&volumeMount)
			}
		}
	}
}

func overriddenImage(currentImg string, overrideImg *common.AgentImageConfig) string {
	splitImg := strings.Split(currentImg, "/")
	registry := ""
	if len(splitImg) > 2 {
		registry = splitImg[0] + "/" + splitImg[1]
	}

	return apicommon.GetImage(overrideImg, &registry)
}

func getAgentCustomConfConfigMapName(ddaName string) string {
	return fmt.Sprintf("%s-datadog-yaml", ddaName)
}

func getClusterAgentCustomConfConfigMapName(ddaName string) string {
	return fmt.Sprintf("%s-cluster-datadog-yaml", ddaName)
}
