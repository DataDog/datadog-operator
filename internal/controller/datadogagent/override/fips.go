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
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
)

// applyFIPSConfig applies FIPS related configs to a pod template spec
func applyFIPSConfig(logger logr.Logger, manager feature.PodTemplateManagers, dda *v2alpha1.DatadogAgent,
	resourcesManager feature.ResourceManagers) {
	globalConfig := dda.Spec.Global
	fipsConfig := globalConfig.FIPS

	// Add FIPS env vars to all containers except System Probe
	for _, cont := range manager.PodTemplateSpec().Spec.Containers {
		if cont.Name != string(apicommon.SystemProbeContainerName) {
			manager.EnvVar().AddEnvVarToContainer(apicommon.AgentContainerName(cont.Name), &corev1.EnvVar{
				Name:  v2alpha1.DDFIPSEnabled,
				Value: "true",
			})
			manager.EnvVar().AddEnvVarToContainer(apicommon.AgentContainerName(cont.Name), &corev1.EnvVar{
				Name:  v2alpha1.DDFIPSPortRangeStart,
				Value: strconv.Itoa(int(*fipsConfig.Port)),
			})
			manager.EnvVar().AddEnvVarToContainer(apicommon.AgentContainerName(cont.Name), &corev1.EnvVar{
				Name:  v2alpha1.DDFIPSUseHTTPS,
				Value: apiutils.BoolToString(fipsConfig.UseHTTPS),
			})
			manager.EnvVar().AddEnvVarToContainer(apicommon.AgentContainerName(cont.Name), &corev1.EnvVar{
				Name:  v2alpha1.DDFIPSLocalAddress,
				Value: *fipsConfig.LocalAddress,
			})
		}
	}

	// Configure FIPS container
	fipsContainer := getFIPSProxyContainer(fipsConfig)

	image := v2alpha1.GetImage(fipsConfig.Image, globalConfig.Registry)
	fipsContainer.Image = image
	if fipsConfig.Image.PullPolicy != nil {
		fipsContainer.ImagePullPolicy = *fipsConfig.Image.PullPolicy
	}

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
			Name:      v2alpha1.FIPSProxyCustomConfigVolumeName,
			MountPath: v2alpha1.FIPSProxyCustomConfigMountPath,
			SubPath:   v2alpha1.FIPSProxyCustomConfigFileName,
			ReadOnly:  true,
		}

		// Add md5 hash annotation to component for custom config
		hash, err := comparison.GenerateMD5ForSpec(fipsConfig.CustomFIPSConfig)
		if err != nil {
			logger.Error(err, "couldn't generate hash for custom config", "filename", v2alpha1.FIPSProxyCustomConfigFileName)
		}
		annotationKey := object.GetChecksumAnnotationKey(string(v2alpha1.FIPSProxyCustomConfigFileName))
		if annotationKey != "" && hash != "" {
			manager.Annotation().AddAnnotation(annotationKey, hash)
		}

		// configMap takes priority over configData
		if fipsConfig.CustomFIPSConfig.ConfigMap != nil {
			vol = volume.GetVolumeFromConfigMap(
				fipsConfig.CustomFIPSConfig.ConfigMap,
				fmt.Sprintf(v2alpha1.FIPSProxyCustomConfigMapName, dda.Name),
				v2alpha1.FIPSProxyCustomConfigVolumeName,
			)
			// configData
		} else if fipsConfig.CustomFIPSConfig.ConfigData != nil {
			cm, err := configmap.BuildConfigMapMulti(
				dda.Namespace,
				map[string]string{v2alpha1.FIPSProxyCustomConfigFileName: *fipsConfig.CustomFIPSConfig.ConfigData},
				fmt.Sprintf(v2alpha1.FIPSProxyCustomConfigMapName, dda.Name),
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

func getFIPSProxyContainer(fipsConfig *v2alpha1.FIPSConfig) corev1.Container {
	fipsContainer := corev1.Container{
		Name:            string(apicommon.FIPSProxyContainerName),
		ImagePullPolicy: corev1.PullIfNotPresent,
		Ports:           getFIPSPorts(fipsConfig),
		Env: []corev1.EnvVar{
			{
				Name:  v2alpha1.DDFIPSLocalAddress,
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
		Name: v2alpha1.FIPSProxyCustomConfigVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: fmt.Sprintf(v2alpha1.FIPSProxyCustomConfigMapName, name),
				},
				Items: []corev1.KeyToPath{
					{
						Key:  v2alpha1.FIPSProxyCustomConfigFileName,
						Path: v2alpha1.FIPSProxyCustomConfigFileName,
					},
				},
			},
		},
	}
}
