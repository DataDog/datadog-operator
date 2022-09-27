// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v2"

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

	// For ExtraConfd and ExtraChecksd, the ConfigMap contents to an init container. This allows use of
	// the workaround to merge existing config and check files with custom ones. The VolumeMount is already
	// defined in the init container; just overwrite the Volume to mount the ConfigMap instead of an EmptyDir.
	// If both ConfigMap and ConfigData exist, ConfigMap has higher priority.
	if override.ExtraConfd != nil {
		if override.ExtraConfd.ConfigMap != nil {
			vol := corev1.Volume{
				Name: apicommon.ConfdVolumeName,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: override.ExtraConfd.ConfigMap.Name,
						},
						Items: override.ExtraConfd.ConfigMap.Items,
					},
				},
			}
			manager.Volume().AddVolume(&vol)
		} else if override.ExtraConfd.ConfigDataMap != nil {
			// Sort map so that order is consistent between reconcile loops
			sortedKeys := sortKeys(override.ExtraConfd.ConfigDataMap)
			keysToPaths := []corev1.KeyToPath{}
			for _, filename := range sortedKeys {
				configData := override.ExtraConfd.ConfigDataMap[filename]
				// Validate that user input is valid YAML
				m := make(map[interface{}]interface{})
				if yaml.Unmarshal([]byte(configData), m) != nil {
					continue
				}
				keysToPaths = append(keysToPaths, corev1.KeyToPath{Key: filename, Path: filename})
			}
			vol := corev1.Volume{
				Name: apicommon.ConfdVolumeName,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: v2alpha1.ExtraConfdConfigMapName,
						},
						Items: keysToPaths,
					},
				},
			}
			manager.Volume().AddVolume(&vol)
		}
	}

	// If both ConfigMap and ConfigData exist, ConfigMap has higher priority.
	if override.ExtraChecksd != nil {
		if override.ExtraChecksd.ConfigMap != nil {
			vol := corev1.Volume{
				Name: apicommon.ChecksdVolumeName,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: override.ExtraChecksd.ConfigMap.Name,
						},
						Items: override.ExtraChecksd.ConfigMap.Items,
					},
				},
			}
			manager.Volume().AddVolume(&vol)
		} else if override.ExtraChecksd.ConfigDataMap != nil {
			// Sort map so that order is consistent between reconcile loops
			sortedKeys := sortKeys(override.ExtraChecksd.ConfigDataMap)
			keysToPaths := []corev1.KeyToPath{}
			for _, filename := range sortedKeys {
				keysToPaths = append(keysToPaths, corev1.KeyToPath{Key: filename, Path: filename})
			}
			vol := corev1.Volume{
				Name: apicommon.ChecksdVolumeName,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: v2alpha1.ExtraChecksdConfigMapName,
						},
						Items: keysToPaths,
					},
				},
			}
			manager.Volume().AddVolume(&vol)
		}
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

	if override.SystemProbeSeccompRootPath != nil {
		vol := corev1.Volume{
			Name: apicommon.SeccompRootVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: *override.SystemProbeSeccompRootPath,
				},
			},
		}
		manager.Volume().AddVolume(&vol)
	}

	if override.SystemProbeSeccompLocalhostProfile != nil {
		for i, container := range manager.PodTemplateSpec().Spec.Containers {
			if container.Name == string(common.SystemProbeContainerName) {
				manager.PodTemplateSpec().Spec.Containers[i].SecurityContext.SeccompProfile.Type = corev1.SeccompProfileTypeLocalhost
				manager.PodTemplateSpec().Spec.Containers[i].SecurityContext.SeccompProfile.LocalhostProfile = override.SystemProbeSeccompLocalhostProfile
			}
		}
	}

	if override.SystemProbeSeccompCustomProfile != nil {
		vol := corev1.Volume{
			Name: apicommon.SeccompSecurityVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: *override.SystemProbeSeccompCustomProfile,
					},
				},
			},
		}
		manager.Volume().AddVolume(&vol)
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

func sortKeys(keysMap map[string]string) []string {
	sortedKeys := make([]string, 0, len(keysMap))
	for key := range keysMap {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Slice(sortedKeys, func(i, j int) bool {
		return sortedKeys[i] < sortedKeys[j]
	})
	return sortedKeys
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
