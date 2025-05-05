// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package usm

import (
	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/component/agent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/object/volume"
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

// ID returns the ID of the Feature
func (f *usmFeature) ID() feature.IDType {
	return feature.USMIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *usmFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	// Merge configuration from Status.RemoteConfigConfiguration into the Spec
	mergeConfigs(&dda.Spec, &dda.Status)

	usmConfig := dda.Spec.Features.USM

	if usmConfig != nil && apiutils.BoolValue(usmConfig.Enabled) {
		reqComp = feature.RequiredComponents{
			Agent: feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
				Containers: []apicommon.AgentContainerName{
					apicommon.CoreAgentContainerName,
					apicommon.ProcessAgentContainerName,
					apicommon.SystemProbeContainerName,
				},
			},
		}
	}

	return reqComp
}

func mergeConfigs(ddaSpec *v2alpha1.DatadogAgentSpec, ddaStatus *v2alpha1.DatadogAgentStatus) {
	if ddaStatus.RemoteConfigConfiguration == nil || ddaStatus.RemoteConfigConfiguration.Features == nil || ddaStatus.RemoteConfigConfiguration.Features.USM == nil || ddaStatus.RemoteConfigConfiguration.Features.USM.Enabled == nil {
		return
	}

	if ddaSpec.Features == nil {
		ddaSpec.Features = &v2alpha1.DatadogFeatures{}
	}

	if ddaSpec.Features.USM == nil {
		ddaSpec.Features.USM = &v2alpha1.USMFeatureConfig{}
	}

	if ddaStatus.RemoteConfigConfiguration.Features.USM.Enabled != nil {
		ddaSpec.Features.USM.Enabled = ddaStatus.RemoteConfigConfiguration.Features.USM.Enabled
	}
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *usmFeature) ManageDependencies(managers feature.ResourceManagers) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *usmFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *usmFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *usmFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	// annotations
	managers.Annotation().AddAnnotation(common.SystemProbeAppArmorAnnotationKey, common.SystemProbeAppArmorAnnotationValue)

	// security context capabilities
	managers.SecurityContext().AddCapabilitiesToContainer(agent.DefaultCapabilitiesForSystemProbe(), apicommon.SystemProbeContainerName)

	// volume mounts
	procdirVol, procdirMount := volume.GetVolumes(common.ProcdirVolumeName, common.ProcdirHostPath, common.ProcdirMountPath, true)
	managers.VolumeMount().AddVolumeMountToContainer(&procdirMount, apicommon.SystemProbeContainerName)
	managers.Volume().AddVolume(&procdirVol)

	cgroupsVol, cgroupsMount := volume.GetVolumes(common.CgroupsVolumeName, common.CgroupsHostPath, common.CgroupsMountPath, true)
	managers.VolumeMount().AddVolumeMountToContainer(&cgroupsMount, apicommon.SystemProbeContainerName)
	managers.Volume().AddVolume(&cgroupsVol)

	debugfsVol, debugfsMount := volume.GetVolumes(common.DebugfsVolumeName, common.DebugfsPath, common.DebugfsPath, false)
	managers.VolumeMount().AddVolumeMountToContainer(&debugfsMount, apicommon.SystemProbeContainerName)
	managers.Volume().AddVolume(&debugfsVol)

	// socket volume mount (needs write perms for the system probe container but not the others)
	socketDirVol, socketDirMount := volume.GetVolumesEmptyDir(common.SystemProbeSocketVolumeName, common.SystemProbeSocketVolumePath, false)
	managers.VolumeMount().AddVolumeMountToContainers(
		&socketDirMount,
		[]apicommon.AgentContainerName{
			apicommon.SystemProbeContainerName,
		},
	)
	managers.Volume().AddVolume(&socketDirVol)

	_, socketVolMountReadOnly := volume.GetVolumesEmptyDir(common.SystemProbeSocketVolumeName, common.SystemProbeSocketVolumePath, true)
	managers.VolumeMount().AddVolumeMountToContainers(
		&socketVolMountReadOnly,
		[]apicommon.AgentContainerName{
			apicommon.CoreAgentContainerName,
			apicommon.ProcessAgentContainerName,
		},
	)

	// env vars for Core Agent, Process Agent and System Probe
	containersForEnvVars := []apicommon.AgentContainerName{
		apicommon.CoreAgentContainerName,
		apicommon.ProcessAgentContainerName,
		apicommon.SystemProbeContainerName,
	}

	enabledEnvVar := &corev1.EnvVar{
		Name:  DDSystemProbeServiceMonitoringEnabled,
		Value: "true",
	}
	managers.EnvVar().AddEnvVarToContainers(containersForEnvVars, enabledEnvVar)

	sysProbeEnableEnvVar := &corev1.EnvVar{
		Name:  common.DDSystemProbeEnabled,
		Value: "true",
	}
	managers.EnvVar().AddEnvVarToContainers(
		[]apicommon.AgentContainerName{apicommon.CoreAgentContainerName, apicommon.SystemProbeContainerName},
		sysProbeEnableEnvVar,
	)

	sysProbeSocketEnvVar := &corev1.EnvVar{
		Name:  common.DDSystemProbeSocket,
		Value: common.DefaultSystemProbeSocketPath,
	}
	managers.EnvVar().AddEnvVarToContainers(containersForEnvVars, sysProbeSocketEnvVar)

	// env vars for Process Agent only
	sysProbeExternalEnvVar := &corev1.EnvVar{
		Name:  common.DDSystemProbeExternal,
		Value: "true",
	}
	managers.EnvVar().AddEnvVarToContainer(apicommon.ProcessAgentContainerName, sysProbeExternalEnvVar)

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *usmFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
