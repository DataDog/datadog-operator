// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"fmt"
	"sort"
	"strings"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	"github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object/volume"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/defaulting"

	"github.com/go-logr/logr"
)

// PodTemplateSpec use to override a corev1.PodTemplateSpec with a 2alpha1.DatadogAgentPodTemplateOverride.
func PodTemplateSpec(logger logr.Logger, manager feature.PodTemplateManagers, override *v2alpha1.DatadogAgentComponentOverride, componentName v2alpha1.ComponentName, ddaName string) {
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
			if override.Image.PullPolicy != nil {
				manager.PodTemplateSpec().Spec.Containers[i].ImagePullPolicy = *override.Image.PullPolicy
			}
		}

		for i, initContainer := range manager.PodTemplateSpec().Spec.InitContainers {
			manager.PodTemplateSpec().Spec.InitContainers[i].Image = overrideImage(initContainer.Image, override.Image)
			if override.Image.PullPolicy != nil {
				manager.PodTemplateSpec().Spec.InitContainers[i].ImagePullPolicy = *override.Image.PullPolicy
			}
		}

		if override.Image.PullSecrets != nil {
			manager.PodTemplateSpec().Spec.ImagePullSecrets = *override.Image.PullSecrets
		}
	}

	for _, env := range override.Env {
		e := env
		manager.EnvVar().AddEnvVar(&e)
	}

	// Override agent configurations such as datadog.yaml, system-probe.yaml, etc.
	overrideCustomConfigVolumes(logger, manager, override.CustomConfigurations, componentName, ddaName)

	// For ExtraConfd and ExtraChecksd, the ConfigMap contents to an init container. This allows use of
	// the workaround to merge existing config and check files with custom ones. The VolumeMount is already
	// defined in the init container; just overwrite the Volume to mount the ConfigMap instead of an EmptyDir.
	// If both ConfigMap and ConfigData exist, ConfigMap has higher priority.
	if override.ExtraConfd != nil {
		cmName := fmt.Sprintf(v2alpha1.ExtraConfdConfigMapName, strings.ToLower((string(componentName))))
		vol := volume.GetVolumeFromMultiCustomConfig(override.ExtraConfd, apicommon.ConfdVolumeName, cmName)
		manager.Volume().AddVolume(&vol)

		// Add md5 hash annotation for custom config
		hash, err := comparison.GenerateMD5ForSpec(override.ExtraConfd)
		if err != nil {
			logger.Error(err, "couldn't generate hash for extra confd custom config")
		} else {
			logger.V(2).Info("built extra confd from custom config", "hash", hash)
		}
		annotationKey := object.GetChecksumAnnotationKey(cmName)
		manager.Annotation().AddAnnotation(annotationKey, hash)
	}

	// If both ConfigMap and ConfigData exist, ConfigMap has higher priority.
	if override.ExtraChecksd != nil {
		cmName := fmt.Sprintf(v2alpha1.ExtraChecksdConfigMapName, strings.ToLower((string(componentName))))
		vol := volume.GetVolumeFromMultiCustomConfig(override.ExtraChecksd, apicommon.ChecksdVolumeName, cmName)
		manager.Volume().AddVolume(&vol)

		// Add md5 hash annotation for custom config
		hash, err := comparison.GenerateMD5ForSpec(override.ExtraChecksd)
		if err != nil {
			logger.Error(err, "couldn't generate hash for extra checksd custom config")
		} else {
			logger.V(2).Info("built extra checksd from custom config", "hash", hash)
		}
		annotationKey := object.GetChecksumAnnotationKey(cmName)
		manager.Annotation().AddAnnotation(annotationKey, hash)
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

func overrideCustomConfigVolumes(logger logr.Logger, manager feature.PodTemplateManagers, customConfs map[v2alpha1.AgentConfigFileName]v2alpha1.CustomConfig, componentName v2alpha1.ComponentName, ddaName string) {
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

		// Add md5 hash annotation for custom config
		hash, err := comparison.GenerateMD5ForSpec(customConfig)
		if err != nil {
			logger.Error(err, "couldn't generate hash for custom config", "filename", fileName)
		} else {
			logger.V(2).Info("built file from custom config", "filename", fileName, "hash", hash)
		}
		annotationKey := object.GetChecksumAnnotationKey(string(fileName))
		manager.Annotation().AddAnnotation(annotationKey, hash)
	}
}

func overrideImage(currentImg string, overrideImg *common.AgentImageConfig) string {
	splitImg := strings.Split(currentImg, "/")
	registry := strings.Join(splitImg[:len(splitImg)-1], "/")

	splitName := strings.Split(splitImg[len(splitImg)-1], ":")

	// This deep copies primitives of the struct, we don't care about other fields
	overrideImgCopy := *overrideImg
	if overrideImgCopy.Name == "" {
		overrideImgCopy.Name = splitName[0]
	}

	if overrideImgCopy.Tag == "" {
		// If present need to drop JMX tag suffix
		overrideImgCopy.Tag = strings.TrimSuffix(splitName[1], defaulting.JMXTagSuffix)
	}

	return apicommon.GetImage(&overrideImgCopy, &registry)
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
