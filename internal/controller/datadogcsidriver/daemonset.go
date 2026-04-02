// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogcsidriver

import (
	"fmt"
	"maps"
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/images"
)

func buildDaemonSet(instance *datadoghqv1alpha1.DatadogCSIDriver) *appsv1.DaemonSet {
	driverName := csiDriverName
	apmSocketPath := getAPMSocketPath(instance)
	dsdSocketPath := getDSDSocketPath(instance)
	apmSocketDir := filepath.Dir(apmSocketPath)
	dsdSocketDir := filepath.Dir(dsdSocketPath)

	labels := map[string]string{
		appLabelKey: csiDsName,
	}
	podLabels := map[string]string{
		appLabelKey:                     csiDsName,
		admissionControllerEnabledLabel: "false",
	}

	volumes := buildVolumes(driverName, apmSocketDir, dsdSocketDir)
	csiDriverContainer := buildCSIDriverContainer(instance, driverName, apmSocketPath, dsdSocketPath, apmSocketDir, dsdSocketDir)
	registrarContainer := buildRegistrarContainer(instance, driverName)

	revisionHistoryLimit := int32(10)
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      csiDsName,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					appLabelKey: csiDsName,
				},
			},
			RevisionHistoryLimit: &revisionHistoryLimit,
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type: appsv1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDaemonSet{
					MaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "10%"},
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: podLabels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						csiDriverContainer,
						registrarContainer,
					},
					Volumes: volumes,
				},
			},
		},
	}

	applyOverrides(ds, instance.Spec.Override)
	return ds
}

func buildCSIDriverContainer(instance *datadoghqv1alpha1.DatadogCSIDriver, driverName, apmSocketPath, dsdSocketPath, apmSocketDir, dsdSocketDir string) corev1.Container {
	privileged := true
	readOnlyRootFS := true
	bidirectional := corev1.MountPropagationBidirectional

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      pluginDirVolumeName,
			MountPath: pluginDirMountPath,
		},
		{
			Name:      storageDirVolumeName,
			MountPath: storageDirMountPath,
		},
		{
			Name:      apmSocketVolumeName,
			MountPath: apmSocketDir,
			ReadOnly:  true,
		},
		{
			Name:             mountpointDirVolumeName,
			MountPath:        mountpointDirPath,
			MountPropagation: &bidirectional,
		},
	}

	if dsdSocketDir != apmSocketDir {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      dsdSocketVolumeName,
			MountPath: dsdSocketDir,
			ReadOnly:  true,
		})
	}

	return corev1.Container{
		Name:  datadoghqv1alpha1.CSINodeDriverContainerName,
		Image: resolveCSIDriverImage(instance),
		Args: []string{
			fmt.Sprintf("--apm-host-socket-path=%s", apmSocketPath),
			fmt.Sprintf("--dsd-host-socket-path=%s", dsdSocketPath),
		},
		Env: []corev1.EnvVar{
			{
				Name: envNodeID,
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "spec.nodeName",
					},
				},
			},
			{
				Name:  envDDAPMEnabled,
				Value: "true",
			},
		},
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: csiDriverPort,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		SecurityContext: &corev1.SecurityContext{
			Privileged:             &privileged,
			ReadOnlyRootFilesystem: &readOnlyRootFS,
		},
		VolumeMounts: volumeMounts,
	}
}

func buildRegistrarContainer(instance *datadoghqv1alpha1.DatadogCSIDriver, driverName string) corev1.Container {
	return corev1.Container{
		Name:  datadoghqv1alpha1.CSINodeDriverRegistrarContainerName,
		Image: resolveRegistrarImage(instance),
		Args: []string{
			fmt.Sprintf("--csi-address=$(%s)", envAddress),
			fmt.Sprintf("--kubelet-registration-path=$(%s)", envDriverRegSock),
		},
		Env: []corev1.EnvVar{
			{
				Name:  envAddress,
				Value: csiSocketAddress,
			},
			{
				Name:  envDriverRegSock,
				Value: fmt.Sprintf(csiSocketPathFmt, driverName),
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      pluginDirVolumeName,
				MountPath: pluginDirMountPath,
				ReadOnly:  true,
			},
			{
				Name:      registrationDirVolumeName,
				MountPath: registrarMountPath,
			},
		},
	}
}

func buildVolumes(driverName, apmSocketDir, dsdSocketDir string) []corev1.Volume {
	dirOrCreate := corev1.HostPathDirectoryOrCreate
	dir := corev1.HostPathDirectory

	volumes := []corev1.Volume{
		{
			Name: pluginDirVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: fmt.Sprintf(kubeletPluginsDirFmt, driverName),
					Type: &dirOrCreate,
				},
			},
		},
		{
			Name: storageDirVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: fmt.Sprintf(kubeletStorageDirFmt, driverName),
					Type: &dirOrCreate,
				},
			},
		},
		{
			Name: registrationDirVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: registrationDirPath,
					Type: &dir,
				},
			},
		},
		{
			Name: mountpointDirVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: mountpointDirPath,
					Type: &dirOrCreate,
				},
			},
		},
		{
			Name: apmSocketVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: apmSocketDir,
					Type: &dirOrCreate,
				},
			},
		},
	}

	if dsdSocketDir != apmSocketDir {
		volumes = append(volumes, corev1.Volume{
			Name: dsdSocketVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: dsdSocketDir,
					Type: &dirOrCreate,
				},
			},
		})
	}

	return volumes
}

func applyOverrides(ds *appsv1.DaemonSet, override *datadoghqv1alpha1.DatadogCSIDriverOverride) {
	if override == nil {
		return
	}

	// Labels: merge, override values win on key conflicts
	if override.Labels != nil {
		maps.Copy(ds.Spec.Template.Labels, override.Labels)
	}

	// Annotations: merge, override values win on key conflicts
	if override.Annotations != nil {
		if ds.Spec.Template.Annotations == nil {
			ds.Spec.Template.Annotations = map[string]string{}
		}
		maps.Copy(ds.Spec.Template.Annotations, override.Annotations)
	}

	// NodeSelector: merge keys, override values win on key conflicts
	if override.NodeSelector != nil {
		if ds.Spec.Template.Spec.NodeSelector == nil {
			ds.Spec.Template.Spec.NodeSelector = map[string]string{}
		}
		maps.Copy(ds.Spec.Template.Spec.NodeSelector, override.NodeSelector)
	}

	// Tolerations: append (accumulates, following DatadogAgent pattern)
	if override.Tolerations != nil {
		ds.Spec.Template.Spec.Tolerations = append(ds.Spec.Template.Spec.Tolerations, override.Tolerations...)
	}

	// Affinity: override replaces entirely
	if override.Affinity != nil {
		ds.Spec.Template.Spec.Affinity = override.Affinity
	}

	if override.PriorityClassName != nil {
		ds.Spec.Template.Spec.PriorityClassName = *override.PriorityClassName
	}

	if override.SecurityContext != nil {
		ds.Spec.Template.Spec.SecurityContext = override.SecurityContext
	}

	if override.ServiceAccountName != nil {
		ds.Spec.Template.Spec.ServiceAccountName = *override.ServiceAccountName
	}

	if override.UpdateStrategy != nil {
		ds.Spec.UpdateStrategy = appsv1.DaemonSetUpdateStrategy{
			Type: appsv1.DaemonSetUpdateStrategyType(override.UpdateStrategy.Type),
		}
		if override.UpdateStrategy.RollingUpdate != nil {
			ds.Spec.UpdateStrategy.RollingUpdate = &appsv1.RollingUpdateDaemonSet{
				MaxUnavailable: override.UpdateStrategy.RollingUpdate.MaxUnavailable,
			}
		}
	}

	// Volumes: merge by name, override wins on name conflicts
	if override.Volumes != nil {
		ds.Spec.Template.Spec.Volumes = mergeVolumesByName(ds.Spec.Template.Spec.Volumes, override.Volumes)
	}

	// Env vars: merge by name for all containers, override wins on name conflicts
	if override.Env != nil {
		for i := range ds.Spec.Template.Spec.Containers {
			ds.Spec.Template.Spec.Containers[i].Env = mergeEnvVarsByName(ds.Spec.Template.Spec.Containers[i].Env, override.Env)
		}
	}

	// Per-container overrides
	if override.Containers != nil {
		for i := range ds.Spec.Template.Spec.Containers {
			containerName := ds.Spec.Template.Spec.Containers[i].Name
			if containerOverride, ok := override.Containers[containerName]; ok && containerOverride != nil {
				applyContainerOverrides(&ds.Spec.Template.Spec.Containers[i], containerOverride)
			}
		}
	}
}

func applyContainerOverrides(container *corev1.Container, override *v2alpha1.DatadogAgentGenericContainer) {
	if override.Name != nil {
		container.Name = *override.Name
	}

	// Resources: partial merge — only override resources are applied,
	// existing resources not in override are preserved.
	if override.Resources != nil {
		if override.Resources.Requests != nil {
			if container.Resources.Requests == nil {
				container.Resources.Requests = corev1.ResourceList{}
			}
			maps.Copy(container.Resources.Requests, override.Resources.Requests)
		}
		if override.Resources.Limits != nil {
			if container.Resources.Limits == nil {
				container.Resources.Limits = corev1.ResourceList{}
			}
			maps.Copy(container.Resources.Limits, override.Resources.Limits)
		}
	}

	if override.Command != nil {
		container.Command = override.Command
	}
	if override.Args != nil {
		container.Args = override.Args
	}
	if override.ReadinessProbe != nil {
		container.ReadinessProbe = override.ReadinessProbe
	}
	if override.LivenessProbe != nil {
		container.LivenessProbe = override.LivenessProbe
	}
	if override.StartupProbe != nil {
		container.StartupProbe = override.StartupProbe
	}
	if override.SecurityContext != nil {
		container.SecurityContext = override.SecurityContext
	}

	// Env vars: merge by name, override wins on name conflicts
	if override.Env != nil {
		container.Env = mergeEnvVarsByName(container.Env, override.Env)
	}

	// Volume mounts: merge by name, override wins on name conflicts
	if override.VolumeMounts != nil {
		container.VolumeMounts = mergeVolumeMountsByName(container.VolumeMounts, override.VolumeMounts)
	}

	// Ports: merge by containerPort, override wins on conflicts
	if override.Ports != nil {
		container.Ports = mergePortsByContainerPort(container.Ports, override.Ports)
	}
}

// mergeEnvVarsByName merges env vars by name. Override values replace existing
// vars with the same name; new vars are appended.
func mergeEnvVarsByName(base, overrides []corev1.EnvVar) []corev1.EnvVar {
	result := append(make([]corev1.EnvVar, 0, len(base)+len(overrides)), base...)
	for _, override := range overrides {
		found := false
		for i, existing := range result {
			if existing.Name == override.Name {
				result[i] = override
				found = true
				break
			}
		}
		if !found {
			result = append(result, override)
		}
	}
	return result
}

// mergeVolumeMountsByName merges volume mounts by name. Override mounts replace
// existing ones with the same name; new mounts are appended.
func mergeVolumeMountsByName(base, overrides []corev1.VolumeMount) []corev1.VolumeMount {
	result := append(make([]corev1.VolumeMount, 0, len(base)+len(overrides)), base...)
	for _, override := range overrides {
		found := false
		for i, existing := range result {
			if existing.Name == override.Name {
				result[i] = override
				found = true
				break
			}
		}
		if !found {
			result = append(result, override)
		}
	}
	return result
}

// mergeVolumesByName merges volumes by name. Override volumes replace existing
// ones with the same name; new volumes are appended.
func mergeVolumesByName(base, overrides []corev1.Volume) []corev1.Volume {
	result := append(make([]corev1.Volume, 0, len(base)+len(overrides)), base...)
	for _, override := range overrides {
		found := false
		for i, existing := range result {
			if existing.Name == override.Name {
				result[i] = override
				found = true
				break
			}
		}
		if !found {
			result = append(result, override)
		}
	}
	return result
}

// mergePortsByContainerPort merges container ports by port number. Override ports
// replace existing ones with the same containerPort; new ports are appended.
func mergePortsByContainerPort(base, overrides []corev1.ContainerPort) []corev1.ContainerPort {
	result := append(make([]corev1.ContainerPort, 0, len(base)+len(overrides)), base...)
	for _, override := range overrides {
		found := false
		for i, existing := range result {
			if existing.ContainerPort == override.ContainerPort {
				result[i] = override
				found = true
				break
			}
		}
		if !found {
			result = append(result, override)
		}
	}
	return result
}

// Image resolution: uses the same pattern as the DatadogAgent controller via
// pkg/images. Users can specify just a tag (uses default registry/name), a
// name:tag (uses as-is), or a full registry/name:tag.

func resolveCSIDriverImage(instance *datadoghqv1alpha1.DatadogCSIDriver) string {
	defaultImage := &v2alpha1.AgentImageConfig{
		Name: defaultCSIDriverImageName,
		Tag:  defaultCSIDriverImageTag,
	}
	if instance.Spec.CSIDriverImage == nil {
		return images.AssembleImage(defaultImage, defaultCSIDriverImageRegistry)
	}
	return images.OverrideAgentImage(
		images.AssembleImage(defaultImage, defaultCSIDriverImageRegistry),
		instance.Spec.CSIDriverImage,
	)
}

func resolveRegistrarImage(instance *datadoghqv1alpha1.DatadogCSIDriver) string {
	defaultImage := &v2alpha1.AgentImageConfig{
		Name: defaultRegistrarImageName,
		Tag:  defaultRegistrarImageTag,
	}
	if instance.Spec.RegistrarImage == nil {
		return images.AssembleImage(defaultImage, defaultRegistrarImageRegistry)
	}
	return images.OverrideAgentImage(
		images.AssembleImage(defaultImage, defaultRegistrarImageRegistry),
		instance.Spec.RegistrarImage,
	)
}

// Helper functions to get configured or default values

func getAPMSocketPath(instance *datadoghqv1alpha1.DatadogCSIDriver) string {
	if instance.Spec.APMSocketPath != nil && *instance.Spec.APMSocketPath != "" {
		return *instance.Spec.APMSocketPath
	}
	return defaultAPMSocketPath
}

func getDSDSocketPath(instance *datadoghqv1alpha1.DatadogCSIDriver) string {
	if instance.Spec.DSDSocketPath != nil && *instance.Spec.DSDSocketPath != "" {
		return *instance.Spec.DSDSocketPath
	}
	return defaultDSDSocketPath
}
