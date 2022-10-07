// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"sort"
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
	// Note that there are several attributes in v2alpha1.DatadogAgentComponentOverride, like "Replicas" or "Disabled",
	// that are not related to the pod template spec. The overrides for those attributes are not applied in this function.

	if override == nil {
		return
	}

	if override.ServiceAccountName != nil {
		manager.PodTemplateSpec().Spec.ServiceAccountName = *override.ServiceAccountName
	}

	if override.Image != nil {
		for i, container := range manager.PodTemplateSpec().Spec.Containers {
			manager.PodTemplateSpec().Spec.Containers[i].Image = overrideImage(container.Image, override.Image)
		}

		for i, initContainer := range manager.PodTemplateSpec().Spec.InitContainers {
			manager.PodTemplateSpec().Spec.InitContainers[i].Image = overrideImage(initContainer.Image, override.Image)
		}
	}

	for _, env := range override.Env {
		manager.EnvVar().AddEnvVar(&corev1.EnvVar{
			Name:  env.Name,
			Value: env.Value,
		})
	}

	// Override agent configurations such as datadog.yaml, system-probe.yaml, etc.
	overrideCustomConfigVolumes(manager, override.CustomConfigurations, componentName, ddaName)

	// For ExtraConfd and ExtraChecksd, the ConfigMap contents to an init container. This allows use of
	// the workaround to merge existing config and check files with custom ones. The VolumeMount is already
	// defined in the init container; just overwrite the Volume to mount the ConfigMap instead of an EmptyDir.
	// If both ConfigMap and ConfigData exist, ConfigMap has higher priority.
	if override.ExtraConfd != nil {
		vol := volume.GetVolumeFromMultiCustomConfig(override.ExtraConfd, apicommon.ConfdVolumeName, v2alpha1.ExtraConfdConfigMapName)
		manager.Volume().AddVolume(&vol)
	}

	// If both ConfigMap and ConfigData exist, ConfigMap has higher priority.
	if override.ExtraChecksd != nil {
		vol := volume.GetVolumeFromMultiCustomConfig(override.ExtraChecksd, apicommon.ChecksdVolumeName, v2alpha1.ExtraChecksdConfigMapName)
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

func overrideCustomConfigVolumes(manager feature.PodTemplateManagers, customConfs map[v2alpha1.AgentConfigFileName]v2alpha1.CustomConfig, componentName v2alpha1.ComponentName, ddaName string) {
	sortedKeys := sortKeys(customConfs)
	for _, fileName := range sortedKeys {
		customConfig := customConfs[fileName]
		switch componentName {
		case v2alpha1.NodeAgentComponentName, v2alpha1.ClusterChecksRunnerComponentName:
			// For the NodeAgent, there are a few possible config files and each need their own volume.
			// Use a volumeName that matches the defaultConfigMapName.
			defaultConfigMapName := getDefaultConfigMapName(ddaName, string(fileName))
			volumeName := defaultConfigMapName
			vol := volume.GetVolumeFromCustomConfig(customConfig, defaultConfigMapName, volumeName)
			manager.Volume().AddVolume(&vol)

			volumeMount := volume.GetVolumeMountWithSubPath(
				volumeName,
				"/etc/datadog-agent/"+string(fileName),
				string(fileName),
			)
			manager.VolumeMount().AddVolumeMount(&volumeMount)
		case v2alpha1.ClusterAgentComponentName:
			// For the Cluster Agent, there is only one possible config file so can use a simple volume name.
			volumeName := apicommon.ClusterAgentCustomConfigVolumeName
			defaultConfigMapName := getDefaultConfigMapName(ddaName, string(fileName))
			vol := volume.GetVolumeFromCustomConfig(customConfig, defaultConfigMapName, volumeName)
			manager.Volume().AddVolume(&vol)

			volumeMount := volume.GetVolumeMountWithSubPath(
				volumeName,
				"/etc/datadog-agent/"+string(fileName),
				string(fileName),
			)
			manager.VolumeMount().AddVolumeMount(&volumeMount)
		}
	}
}

func overrideImage(currentImg string, overrideImg *common.AgentImageConfig) string {
	splitImg := strings.Split(currentImg, "/")
	registry := ""
	if len(splitImg) > 2 {
		registry = splitImg[0] + "/" + splitImg[1]
	}

	return apicommon.GetImage(overrideImg, &registry)
}

func sortKeys(keysMap map[v2alpha1.AgentConfigFileName]v2alpha1.CustomConfig) []v2alpha1.AgentConfigFileName {
	sortedKeys := make([]v2alpha1.AgentConfigFileName, 0, len(keysMap))
	for key := range keysMap {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Slice(sortedKeys, func(i, j int) bool {
		return sortedKeys[i] < sortedKeys[j]
	})
	return sortedKeys
}
