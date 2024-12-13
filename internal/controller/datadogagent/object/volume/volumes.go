// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package volume

import (
	"fmt"
	"sort"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// GetVolumes creates a corev1.Volume and corev1.VolumeMount corresponding to a host path.
func GetVolumes(volumeName, hostPath, mountPath string, readOnly bool) (corev1.Volume, corev1.VolumeMount) {
	var volume corev1.Volume
	var volumeMount corev1.VolumeMount

	volume = corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: hostPath,
			},
		},
	}
	volumeMount = corev1.VolumeMount{
		Name:      volumeName,
		MountPath: mountPath,
		ReadOnly:  readOnly,
	}

	return volume, volumeMount
}

// GetVolumesEmptyDir creates a corev1.Volume (with an empty dir) and corev1.VolumeMount.
func GetVolumesEmptyDir(volumeName, mountPath string, readOnly bool) (corev1.Volume, corev1.VolumeMount) {
	var volume corev1.Volume
	var volumeMount corev1.VolumeMount

	volume = corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
	volumeMount = corev1.VolumeMount{
		Name:      volumeName,
		MountPath: mountPath,
		ReadOnly:  readOnly,
	}

	return volume, volumeMount
}

// common ConfigMapConfig

// GetVolumesFromConfigMap returns a Volume and VolumeMount from a ConfigMapConfig. It is only used in the features that are within the conf.d/ file path
func GetVolumesFromConfigMap(configMap *v2alpha1.ConfigMapConfig, volumeName, defaultCMName, configFolder string) (corev1.Volume, corev1.VolumeMount) {
	volume := GetVolumeFromConfigMap(
		configMap,
		defaultCMName,
		volumeName,
	)

	volumeMount := corev1.VolumeMount{
		Name:      volumeName,
		MountPath: fmt.Sprintf("%s%s/%s", v2alpha1.ConfigVolumePath, v2alpha1.ConfdVolumePath, configFolder),
		ReadOnly:  true,
	}
	return volume, volumeMount
}

// GetVolumeFromConfigMap returns a Volume from a common ConfigMapConfig.
func GetVolumeFromConfigMap(configMap *v2alpha1.ConfigMapConfig, defaultConfigMapName, volumeName string) corev1.Volume {
	cmName := defaultConfigMapName
	if configMap != nil && len(configMap.Name) > 0 {
		cmName = configMap.Name
	}

	cmSource := &corev1.ConfigMapVolumeSource{
		LocalObjectReference: corev1.LocalObjectReference{
			Name: cmName,
		},
	}

	if len(configMap.Items) > 0 {
		cmSource.Items = configMap.Items
	}

	return corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: cmSource,
		},
	}
}

// GetVolumeFromCustomConfig returns a Volume from a v2alpha1 CustomConfig. It is used for agent-level configuration overrides in v2alpha1.
func GetVolumeFromCustomConfig(customConfig v2alpha1.CustomConfig, defaultConfigMapName, volumeName string) corev1.Volume {
	var vol corev1.Volume
	if customConfig.ConfigMap != nil {
		vol = GetVolumeFromConfigMap(
			customConfig.ConfigMap,
			defaultConfigMapName,
			volumeName,
		)
	} else if customConfig.ConfigData != nil {
		vol = GetBasicVolume(defaultConfigMapName, volumeName)
	}
	return vol
}

// GetVolumeFromMultiCustomConfig returns a Volume from a v2alpha1 MultiCustomConfig. It is used for Extra Checksd and Extra Confd.
func GetVolumeFromMultiCustomConfig(multiCustomConfig *v2alpha1.MultiCustomConfig, volumeName, configMapName string) corev1.Volume {
	var vol corev1.Volume
	if multiCustomConfig.ConfigMap != nil {
		vol = corev1.Volume{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: multiCustomConfig.ConfigMap.Name,
					},
					Items: multiCustomConfig.ConfigMap.Items,
				},
			},
		}
	} else if multiCustomConfig.ConfigDataMap != nil {
		// Sort map so that order is consistent between reconcile loops
		sortedKeys := sortKeys(multiCustomConfig.ConfigDataMap)
		keysToPaths := []corev1.KeyToPath{}
		for _, filename := range sortedKeys {
			configData := multiCustomConfig.ConfigDataMap[filename]
			// Validate that user input is valid YAML
			m := make(map[interface{}]interface{})
			if yaml.Unmarshal([]byte(configData), m) != nil {
				continue
			}
			keysToPaths = append(keysToPaths, corev1.KeyToPath{Key: filename, Path: filename})
		}
		vol = corev1.Volume{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: configMapName,
					},
					Items: keysToPaths,
				},
			},
		}
	}
	return vol
}

// GetBasicVolume returns a basic Volume from a config map name and volume name. It is used in features and overrides.
func GetBasicVolume(configMapName, volumeName string) corev1.Volume {
	return corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: configMapName,
				},
			},
		},
	}
}

// GetVolumeMountWithSubPath return a corev1.VolumeMount with a subPath. The subPath is needed
// in situations when a specific file needs to be mounted without affecting the rest of the directory.
// This is used in features and overrides.
func GetVolumeMountWithSubPath(volumeName, volumePath, subPath string) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      volumeName,
		MountPath: volumePath,
		SubPath:   subPath,
		ReadOnly:  true,
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
