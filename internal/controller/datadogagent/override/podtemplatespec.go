// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"fmt"
	"sort"
	"strings"

	"github.com/go-logr/logr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/images"
)

func getAgentContainersMap() map[apicommon.AgentContainerName]string {
	return map[apicommon.AgentContainerName]string{
		apicommon.UnprivilegedSingleAgentContainerName: "",
		apicommon.CoreAgentContainerName:               "",
		apicommon.TraceAgentContainerName:              "",
		apicommon.ProcessAgentContainerName:            "",
		apicommon.SecurityAgentContainerName:           "",
		apicommon.SystemProbeContainerName:             "",
		apicommon.OtelAgent:                            "",
		apicommon.AgentDataPlaneContainerName:          "",
		apicommon.ClusterAgentContainerName:            "",
		// apicommon.ClusterChecksRunnersContainerName:    "", // Is the same value as CoreAgentContainerName
	}
}

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
		agentContainersMap := getAgentContainersMap()
		for i, container := range manager.PodTemplateSpec().Spec.Containers {
			if _, ok := agentContainersMap[apicommon.AgentContainerName(container.Name)]; ok {
				manager.PodTemplateSpec().Spec.Containers[i].Image = images.OverrideAgentImage(container.Image, override.Image)
				if override.Image.PullPolicy != nil {
					manager.PodTemplateSpec().Spec.Containers[i].ImagePullPolicy = *override.Image.PullPolicy
				}
			}
		}

		for i, initContainer := range manager.PodTemplateSpec().Spec.InitContainers {
			manager.PodTemplateSpec().Spec.InitContainers[i].Image = images.OverrideAgentImage(initContainer.Image, override.Image)
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

	for _, envFrom := range override.EnvFrom {
		e := envFrom
		manager.EnvFromVar().AddEnvFromVar(&e)
	}

	// Override agent configurations such as datadog.yaml, system-probe.yaml, etc.
	overrideCustomConfigVolumes(logger, manager, override.CustomConfigurations, componentName, ddaName)

	// For ExtraConfd and ExtraChecksd, the ConfigMap contents to an init container. This allows use of
	// the workaround to merge existing config and check files with custom ones. The VolumeMount is already
	// defined in the init container; just overwrite the Volume to mount the ConfigMap instead of an EmptyDir.
	// If both ConfigMap and ConfigData exist, ConfigMap has higher priority.
	if override.ExtraConfd != nil {
		cmName := fmt.Sprintf(extraConfdConfigMapName, strings.ToLower((string(componentName))))
		vol := volume.GetVolumeFromMultiCustomConfig(override.ExtraConfd, common.ConfdVolumeName, cmName)
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
		cmName := fmt.Sprintf(extraChecksdConfigMapName, strings.ToLower((string(componentName))))
		vol := volume.GetVolumeFromMultiCustomConfig(override.ExtraChecksd, common.ChecksdVolumeName, cmName)
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
		Container(agentContainerName, manager, containerOverride, ddaName)
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

	if override.RuntimeClassName != nil {
		manager.PodTemplateSpec().Spec.RuntimeClassName = override.RuntimeClassName
	}

	if override.Affinity != nil {
		manager.PodTemplateSpec().Spec.Affinity = common.MergeAffinities(manager.PodTemplateSpec().Spec.Affinity, override.Affinity)
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

	if override.DNSPolicy != nil {
		manager.PodTemplateSpec().Spec.DNSPolicy = *override.DNSPolicy
	}

	if override.DNSConfig != nil {
		manager.PodTemplateSpec().Spec.DNSConfig = override.DNSConfig
	}

	manager.PodTemplateSpec().Spec.TopologySpreadConstraints = append(manager.PodTemplateSpec().Spec.TopologySpreadConstraints, override.TopologySpreadConstraints...)
}

func overrideCustomConfigVolumes(logger logr.Logger, manager feature.PodTemplateManagers, customConfs map[v2alpha1.AgentConfigFileName]v2alpha1.CustomConfig, componentName v2alpha1.ComponentName, ddaName string) {
	sortedKeys := sortKeys(customConfs)
	for _, fileName := range sortedKeys {
		customConfig := customConfs[fileName]
		defaultConfigMapName := fmt.Sprintf("%s-%s", getDefaultConfigMapName(ddaName, string(fileName)), strings.ToLower(string(componentName)))
		switch componentName {
		case v2alpha1.NodeAgentComponentName, v2alpha1.ClusterChecksRunnerComponentName:
			// For the NodeAgent, there are a few possible config files and each need their own volume.
			// Use a volumeName that matches the defaultConfigMapName.
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
			volumeName := clusterAgentCustomConfigVolumeName
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
