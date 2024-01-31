// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"fmt"
	"strconv"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	"github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object/configmap"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object/volume"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
)

// ApplyFIPSConfig applies FIPS related configs to a pod template spec
func ApplyFIPSConfig(logger logr.Logger, manager feature.PodTemplateManagers, dda *v2alpha1.DatadogAgent,
	resourcesManager feature.ResourceManagers) *corev1.PodTemplateSpec {
	globalConfig := dda.Spec.Global
	fipsConfig := globalConfig.FIPS

	// Add FIPS env vars to all containers
	for _, cont := range manager.PodTemplateSpec().Spec.Containers {
		if cont.Name != string(common.SystemProbeContainerName) {
			manager.EnvVar().AddEnvVarToContainer(common.AgentContainerName(cont.Name), &corev1.EnvVar{
				Name:  apicommon.DDFIPSEnabled,
				Value: "true",
			})
			manager.EnvVar().AddEnvVarToContainer(common.AgentContainerName(cont.Name), &corev1.EnvVar{
				Name:  apicommon.DDFIPSPortRangeStart,
				Value: strconv.Itoa(int(*fipsConfig.Port)),
			})
			manager.EnvVar().AddEnvVarToContainer(common.AgentContainerName(cont.Name), &corev1.EnvVar{
				Name:  apicommon.DDFIPSUseHTTPS,
				Value: apiutils.BoolToString(fipsConfig.UseHTTPS),
			})
			manager.EnvVar().AddEnvVarToContainer(common.AgentContainerName(cont.Name), &corev1.EnvVar{
				Name:  apicommon.DDFIPSLocalAddress,
				Value: *fipsConfig.LocalAddress,
			})
		}
	}

	// Configure FIPS container
	fipsContainer := getFIPSProxyContainer(fipsConfig)

	image := apicommon.GetImage(fipsConfig.Image, globalConfig.Registry)
	fipsContainer.Image = image

	// Add FIPS container to pod
	found := false
	for i, cont := range manager.PodTemplateSpec().Spec.Containers {
		if cont.Name == fipsContainer.Name {
			manager.PodTemplateSpec().Spec.Containers[i] = fipsContainer
			found = true
		}
	}
	if !found {
		manager.PodTemplateSpec().Spec.Containers = append(manager.PodTemplateSpec().Spec.Containers, fipsContainer)
	}

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
		manager.VolumeMount().AddVolumeMountToContainer(&volMount, common.CoreAgentContainerName)
		manager.VolumeMount().AddVolumeMountToContainer(&volMount, common.FIPSProxyContainerName)
	}
	manager.Volume().AddVolume(&vol)

	if fipsConfig.Resources != nil {
		fipsContainer.Resources = *fipsConfig.Resources
	}

	return manager.PodTemplateSpec()
}

func getFIPSProxyContainer(fipsConfig *v2alpha1.FIPSConfig) corev1.Container {
	fipsContainer := corev1.Container{
		Name:            string(common.FIPSProxyContainerName),
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
