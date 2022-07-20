// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package usm

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object/volume"
)

func init() {
	err := feature.Register(feature.USMIDType, buildUSMFeature)
	if err != nil {
		panic(err)
	}
}

func buildUSMFeature(options *feature.Options) feature.Feature {
	usmFeat := &usmFeature{}

	return usmFeat
}

type usmFeature struct{}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *usmFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	if dda.Spec.Features != nil && dda.Spec.Features.USM != nil && apiutils.BoolValue(dda.Spec.Features.USM.Enabled) {
		reqComp = feature.RequiredComponents{
			Agent: feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
				Containers: []apicommonv1.AgentContainerName{
					apicommonv1.CoreAgentContainerName,
					apicommonv1.ProcessAgentContainerName,
					apicommonv1.SystemProbeContainerName,
				},
			},
		}
	}

	return reqComp
}

// ConfigureV1 use to configure the feature from a v1alpha1.DatadogAgent instance.
func (f *usmFeature) ConfigureV1(dda *v1alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	if dda.Spec.Agent.SystemProbe == nil {
		return reqComp
	}

	enabledEnvVarIsSet := false
	for _, envVar := range dda.Spec.Agent.SystemProbe.Env {
		if envVar.Name == apicommon.DDSystemProbeServiceMonitoringEnabled && envVar.Value == "true" {
			enabledEnvVarIsSet = true
		}
	}

	if dda.Spec.Agent.SystemProbe != nil && *dda.Spec.Agent.SystemProbe.Enabled && enabledEnvVarIsSet {
		reqComp = feature.RequiredComponents{
			Agent: feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
				Containers: []apicommonv1.AgentContainerName{
					apicommonv1.CoreAgentContainerName,
					apicommonv1.ProcessAgentContainerName,
					apicommonv1.SystemProbeContainerName,
				},
			},
		}
	}

	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *usmFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *usmFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *usmFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	// annotations
	managers.Annotation().AddAnnotation(apicommon.SystemProbeAppArmorAnnotationKey, apicommon.SystemProbeAppArmorAnnotationValue)

	// security context capabilities
	capabilities := []corev1.Capability{
		"SYS_ADMIN",
		"SYS_RESOURCE",
		"SYS_PTRACE",
		"NET_ADMIN",
		"NET_BROADCAST",
		"NET_RAW",
		"IPC_LOCK",
		"CHOWN",
	}
	managers.SecurityContext().AddCapabilitiesToContainer(capabilities, apicommonv1.SystemProbeContainerName)

	// volume mounts
	procdirVol, procdirMount := volume.GetVolumes(apicommon.ProcdirVolumeName, apicommon.ProcdirHostPath, apicommon.ProcdirMountPath, true)
	managers.VolumeMount().AddVolumeMountToContainer(&procdirMount, apicommonv1.SystemProbeContainerName)
	managers.Volume().AddVolume(&procdirVol)

	cgroupsVol, cgroupsMount := volume.GetVolumes(apicommon.CgroupsVolumeName, apicommon.CgroupsHostPath, apicommon.CgroupsMountPath, true)
	managers.VolumeMount().AddVolumeMountToContainer(&cgroupsMount, apicommonv1.SystemProbeContainerName)
	managers.Volume().AddVolume(&cgroupsVol)

	debugfsVol, debugfsMount := volume.GetVolumes(apicommon.DebugfsVolumeName, apicommon.DebugfsPath, apicommon.DebugfsPath, false)
	managers.VolumeMount().AddVolumeMountToContainer(&debugfsMount, apicommonv1.SystemProbeContainerName)
	managers.Volume().AddVolume(&debugfsVol)

	// socket volume mount (needs write perms for the system probe container but not the others)
	socketDirVol, socketDirMount := volume.GetVolumesEmptyDir(apicommon.SystemProbeSocketVolumeName, apicommon.SystemProbeSocketVolumePath, false)
	managers.VolumeMount().AddVolumeMountToContainers(
		&socketDirMount,
		[]apicommonv1.AgentContainerName{
			apicommonv1.SystemProbeContainerName,
		},
	)
	managers.Volume().AddVolume(&socketDirVol)

	_, socketVolMountReadOnly := volume.GetVolumesEmptyDir(apicommon.SystemProbeSocketVolumeName, apicommon.SystemProbeSocketVolumePath, true)
	managers.VolumeMount().AddVolumeMountToContainers(
		&socketVolMountReadOnly,
		[]apicommonv1.AgentContainerName{
			apicommonv1.CoreAgentContainerName,
			apicommonv1.ProcessAgentContainerName,
		},
	)

	// env vars for System Probe and Process Agent
	enabledEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDSystemProbeServiceMonitoringEnabled,
		Value: "true",
	}
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.SystemProbeContainerName, enabledEnvVar)
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.ProcessAgentContainerName, enabledEnvVar)

	sysProbeSocketEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDSystemProbeSocket,
		Value: apicommon.DefaultSystemProbeSocketPath,
	}
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.ProcessAgentContainerName, sysProbeSocketEnvVar)
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.SystemProbeContainerName, sysProbeSocketEnvVar)

	// env vars for Process Agent only
	sysProbeExternalEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDSystemProbeExternal,
		Value: "true",
	}
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.ProcessAgentContainerName, sysProbeExternalEnvVar)

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *usmFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
