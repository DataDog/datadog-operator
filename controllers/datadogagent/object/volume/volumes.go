// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package volume

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
)

// GetCustomConfigSpecVolumes use to generate the corev1.Volume and corev1.VolumeMount corresponding to a CustomConfig.
func GetCustomConfigSpecVolumes(customConfig *apicommon.CustomConfig, volumeName, defaultCMName, configFolder string) (corev1.Volume, corev1.VolumeMount) {
	var volume corev1.Volume
	var volumeMount corev1.VolumeMount
	if customConfig != nil {
		volume = GetVolumeFromCustomConfigSpec(
			customConfig,
			defaultCMName,
			volumeName,
		)
		// subpath only updated to Filekey if config uses configMap, default to ksmCoreCheckName for configData.
		volumeMount = GetVolumeMountFromCustomConfigSpec(
			customConfig,
			volumeName,
			fmt.Sprintf("%s%s/%s", apicommon.ConfigVolumePath, apicommon.ConfdVolumePath, configFolder),
			"",
		)
	} else {
		volume = corev1.Volume{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: defaultCMName,
					},
				},
			},
		}
		volumeMount = corev1.VolumeMount{
			Name:      volumeName,
			MountPath: fmt.Sprintf("%s%s/%s", apicommon.ConfigVolumePath, apicommon.ConfdVolumePath, configFolder),
			ReadOnly:  true,
		}
	}
	return volume, volumeMount
}

// GetVolumeFromCustomConfigSpec return a corev1.Volume corresponding to a CustomConfig.
func GetVolumeFromCustomConfigSpec(cfcm *apicommon.CustomConfig, defaultConfigMapName, volumeName string) corev1.Volume {
	confdVolumeSource := *buildVolumeSourceFromCustomConfigSpec(cfcm, defaultConfigMapName)

	return corev1.Volume{
		Name:         volumeName,
		VolumeSource: confdVolumeSource,
	}
}

// GetVolumeMountFromCustomConfigSpec return a corev1.Volume corresponding to a CustomConfig.
func GetVolumeMountFromCustomConfigSpec(cfcm *apicommon.CustomConfig, volumeName, volumePath, defaultSubPath string) corev1.VolumeMount {
	subPath := defaultSubPath
	if cfcm.ConfigMap != nil && len(cfcm.ConfigMap.Items) > 0 {
		subPath = cfcm.ConfigMap.Items[0].Path
	}

	return corev1.VolumeMount{
		Name:      volumeName,
		MountPath: volumePath,
		SubPath:   subPath,
		ReadOnly:  true,
	}
}

func buildVolumeSourceFromCustomConfigSpec(configDir *apicommon.CustomConfig, defaultConfigMapName string) *corev1.VolumeSource {
	if configDir == nil {
		return nil
	}

	cmName := defaultConfigMapName
	if configDir.ConfigMap != nil && len(configDir.ConfigMap.Name) > 0 {
		cmName = configDir.ConfigMap.Name
	}

	cmSource := &corev1.ConfigMapVolumeSource{
		LocalObjectReference: corev1.LocalObjectReference{
			Name: cmName,
		},
	}

	return &corev1.VolumeSource{
		ConfigMap: cmSource,
	}
}
