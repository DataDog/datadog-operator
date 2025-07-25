// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package global

import (
	"fmt"
	"strconv"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/configmap"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/images"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// applyFIPSConfig applies FIPS related configs to a pod template spec
func applyFIPSConfig(logger logr.Logger, manager feature.PodTemplateManagers, ddaMeta metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, resourcesManager feature.ResourceManagers) {
	globalConfig := ddaSpec.Global
	fipsConfig := globalConfig.FIPS

	// Add FIPS env vars to all containers except System Probe
	for _, cont := range manager.PodTemplateSpec().Spec.Containers {
		if cont.Name != string(apicommon.SystemProbeContainerName) {
			manager.EnvVar().AddEnvVarToContainer(apicommon.AgentContainerName(cont.Name), &corev1.EnvVar{
				Name:  DDFIPSEnabled,
				Value: "true",
			})
			manager.EnvVar().AddEnvVarToContainer(apicommon.AgentContainerName(cont.Name), &corev1.EnvVar{
				Name:  DDFIPSPortRangeStart,
				Value: strconv.Itoa(int(*fipsConfig.Port)),
			})
			manager.EnvVar().AddEnvVarToContainer(apicommon.AgentContainerName(cont.Name), &corev1.EnvVar{
				Name:  DDFIPSUseHTTPS,
				Value: apiutils.BoolToString(fipsConfig.UseHTTPS),
			})
			manager.EnvVar().AddEnvVarToContainer(apicommon.AgentContainerName(cont.Name), &corev1.EnvVar{
				Name:  DDFIPSLocalAddress,
				Value: *fipsConfig.LocalAddress,
			})
		}
	}

	// Configure FIPS container
	fipsContainer := getFIPSProxyContainer(fipsConfig)

	image := images.AssembleImage(fipsConfig.Image, *globalConfig.Registry)
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

	vol := getFIPSDefaultVolume(ddaMeta.GetName())
	if fipsConfig.CustomFIPSConfig != nil {
		volMount := corev1.VolumeMount{
			Name:      FIPSProxyCustomConfigVolumeName,
			MountPath: FIPSProxyCustomConfigMountPath,
			SubPath:   FIPSProxyCustomConfigFileName,
			ReadOnly:  true,
		}
		// Add md5 hash annotation to component for custom config
		hash, err := comparison.GenerateMD5ForSpec(fipsConfig.CustomFIPSConfig)
		if err != nil {
			logger.Error(err, "couldn't generate hash for custom config", "filename", FIPSProxyCustomConfigFileName)
		}
		annotationKey := object.GetChecksumAnnotationKey(string(FIPSProxyCustomConfigFileName))
		if annotationKey != "" && hash != "" {
			manager.Annotation().AddAnnotation(annotationKey, hash)
		}

		// configMap takes priority over configData
		if fipsConfig.CustomFIPSConfig.ConfigMap != nil {
			vol = volume.GetVolumeFromConfigMap(
				fipsConfig.CustomFIPSConfig.ConfigMap,
				fmt.Sprintf(FIPSProxyCustomConfigMapName, ddaMeta.GetName()),
				FIPSProxyCustomConfigVolumeName,
			)
			// configData
		} else if fipsConfig.CustomFIPSConfig.ConfigData != nil {
			cm, err := configmap.BuildConfigMapMulti(
				ddaMeta.GetNamespace(),
				map[string]string{FIPSProxyCustomConfigFileName: *fipsConfig.CustomFIPSConfig.ConfigData},
				fmt.Sprintf(FIPSProxyCustomConfigMapName, ddaMeta.GetName()),
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
				Name:  DDFIPSLocalAddress,
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
		Name: FIPSProxyCustomConfigVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: fmt.Sprintf(FIPSProxyCustomConfigMapName, name),
				},
				Items: []corev1.KeyToPath{
					{
						Key:  FIPSProxyCustomConfigFileName,
						Path: FIPSProxyCustomConfigFileName,
					},
				},
			},
		},
	}
}
