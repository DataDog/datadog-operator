// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"fmt"
	"strconv"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/configmap"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/defaulting"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
)

const defaultFIPSImageName string = "fips-proxy"

func fipsImage(registry string) string {
	return fmt.Sprintf("%s/%s:%s", registry, defaultFIPSImageName, defaulting.FIPSProxyLatestVersion)
}

// applyFIPSConfig applies FIPS related configs to a pod template spec
func applyFIPSConfig(logger logr.Logger, manager feature.PodTemplateManagers, dda *v2alpha1.DatadogAgent,
	resourcesManager feature.ResourceManagers) {
	globalConfig := dda.Spec.Global
	fipsConfig := globalConfig.FIPS

	// Add FIPS env vars to all containers except System Probe
	for _, cont := range manager.PodTemplateSpec().Spec.Containers {
		if cont.Name != string(apicommon.SystemProbeContainerName) {
			manager.EnvVar().AddEnvVarToContainer(apicommon.AgentContainerName(cont.Name), &corev1.EnvVar{
				Name:  apicommon.DDFIPSEnabled,
				Value: "true",
			})
			manager.EnvVar().AddEnvVarToContainer(apicommon.AgentContainerName(cont.Name), &corev1.EnvVar{
				Name:  apicommon.DDFIPSPortRangeStart,
				Value: strconv.Itoa(int(*fipsConfig.Port)),
			})
			manager.EnvVar().AddEnvVarToContainer(apicommon.AgentContainerName(cont.Name), &corev1.EnvVar{
				Name:  apicommon.DDFIPSUseHTTPS,
				Value: apiutils.BoolToString(fipsConfig.UseHTTPS),
			})
			manager.EnvVar().AddEnvVarToContainer(apicommon.AgentContainerName(cont.Name), &corev1.EnvVar{
				Name:  apicommon.DDFIPSLocalAddress,
				Value: *fipsConfig.LocalAddress,
			})
		}
	}

	// Configure FIPS container
	fipsContainer := getFIPSProxyContainer(dda)

	// If FIPS image is configured, add this image to fips-proxy container override for each component
	if dda.Spec.Override == nil {
		dda.Spec.Override = make(map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride)
	}

	for _, componentName := range []v2alpha1.ComponentName{v2alpha1.NodeAgentComponentName, v2alpha1.ClusterAgentComponentName, v2alpha1.ClusterChecksRunnerComponentName} {

		componentOverride, ok := dda.Spec.Override[componentName]
		if !ok {
			componentOverride = &v2alpha1.DatadogAgentComponentOverride{}
		}

		if componentOverride.Containers == nil {
			componentOverride.Containers = make(map[apicommon.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer)
		}

		if _, ok = componentOverride.Containers[apicommon.FIPSProxyContainerName]; !ok {
			componentOverride.Containers[apicommon.FIPSProxyContainerName] = &v2alpha1.DatadogAgentGenericContainer{}
		}

		if componentOverride.Containers[apicommon.FIPSProxyContainerName].Image == nil {
			componentOverride.Containers[apicommon.FIPSProxyContainerName].Image = &v2alpha1.AgentImageConfig{}
		}

		if componentOverride.Containers[apicommon.FIPSProxyContainerName].Image.Name == "" {
			if fipsConfig.Image != nil {
				componentOverride.Containers[apicommon.FIPSProxyContainerName].Image.Name = overrideImage(fipsContainer.Image, fipsConfig.Image)
			} else {
				componentOverride.Containers[apicommon.FIPSProxyContainerName].Image.Name = fipsContainer.Image
			}
		}

		if fipsConfig.Image.PullPolicy != nil {
			componentOverride.Containers[apicommon.FIPSProxyContainerName].Image.PullPolicy = fipsConfig.Image.PullPolicy
		}

		dda.Spec.Override[componentName] = componentOverride
	}

	manager.PodTemplateSpec().Spec.Containers = append(manager.PodTemplateSpec().Spec.Containers, fipsContainer)

	vol := getFIPSDefaultVolume(dda.Name)
	if fipsConfig.CustomFIPSConfig != nil {
		volMount := corev1.VolumeMount{
			Name:      apicommon.FIPSProxyCustomConfigVolumeName,
			MountPath: apicommon.FIPSProxyCustomConfigMountPath,
			SubPath:   apicommon.FIPSProxyCustomConfigFileName,
			ReadOnly:  true,
		}

		// Add md5 hash annotation to component for custom config
		hash, err := comparison.GenerateMD5ForSpec(fipsConfig.CustomFIPSConfig)
		if err != nil {
			logger.Error(err, "couldn't generate hash for custom config", "filename", apicommon.FIPSProxyCustomConfigFileName)
		}
		annotationKey := object.GetChecksumAnnotationKey(string(apicommon.FIPSProxyCustomConfigFileName))
		if annotationKey != "" && hash != "" {
			manager.Annotation().AddAnnotation(annotationKey, hash)
		}

		// configMap takes priority over configData
		if fipsConfig.CustomFIPSConfig.ConfigMap != nil {
			vol = volume.GetVolumeFromConfigMap(
				fipsConfig.CustomFIPSConfig.ConfigMap,
				fmt.Sprintf(apicommon.FIPSProxyCustomConfigMapName, dda.Name),
				apicommon.FIPSProxyCustomConfigVolumeName,
			)
			// configData
		} else if fipsConfig.CustomFIPSConfig.ConfigData != nil {
			cm, err := configmap.BuildConfigMapMulti(
				dda.Namespace,
				map[string]string{apicommon.FIPSProxyCustomConfigFileName: *fipsConfig.CustomFIPSConfig.ConfigData},
				fmt.Sprintf(apicommon.FIPSProxyCustomConfigMapName, dda.Name),
				false,
			)
			if err != nil {
				logger.Error(err, "couldn't generate config map data for fips custom config")
			}

			if cm != nil {
				// Add custom config hash annotation to configMap
				annotations := object.MergeAnnotationsLabels(logger, cm.GetAnnotations(), map[string]string{annotationKey: hash}, "*")
				cm.SetAnnotations(annotations)

				resourcesManager.Store().AddOrUpdate(kubernetes.ConfigMapKind, cm)
			}
		}
		manager.VolumeMount().AddVolumeMountToContainer(&volMount, apicommon.CoreAgentContainerName)
		manager.VolumeMount().AddVolumeMountToContainer(&volMount, apicommon.FIPSProxyContainerName)
	}
	manager.Volume().AddVolume(&vol)

	if fipsConfig.Resources != nil {
		fipsContainer.Resources = *fipsConfig.Resources
	}
}

func getFIPSProxyContainer(dda *v2alpha1.DatadogAgent) corev1.Container {
	fipsConfig := dda.Spec.Global.FIPS
	registry := *dda.Spec.Global.Registry

	fipsContainer := corev1.Container{
		Name:            string(apicommon.FIPSProxyContainerName),
		Image:           fipsImage(registry),
		ImagePullPolicy: corev1.PullIfNotPresent,
		Ports:           getFIPSPorts(fipsConfig),
		Env: []corev1.EnvVar{
			{
				Name:  apicommon.DDFIPSLocalAddress,
				Value: *fipsConfig.LocalAddress,
			},
		},
	}

	return fipsContainer
}

func getFIPSPorts(fipsConfig *v2alpha1.FIPSConfig) []corev1.ContainerPort {
	ports := []corev1.ContainerPort{}
	for i := 0; i < int(*fipsConfig.PortRange); i++ {
		portNumber := *fipsConfig.Port + int32(i)
		p := corev1.ContainerPort{
			Name:          fmt.Sprintf("port-%d", i),
			ContainerPort: portNumber,
			Protocol:      corev1.ProtocolTCP,
		}
		ports = append(ports, p)
	}

	return ports
}

func getFIPSDefaultVolume(name string) corev1.Volume {
	return corev1.Volume{
		Name: apicommon.FIPSProxyCustomConfigVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: fmt.Sprintf(apicommon.FIPSProxyCustomConfigMapName, name),
				},
				Items: []corev1.KeyToPath{
					{
						Key:  apicommon.FIPSProxyCustomConfigFileName,
						Path: apicommon.FIPSProxyCustomConfigFileName,
					},
				},
			},
		},
	}
}
