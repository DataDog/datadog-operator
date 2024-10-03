// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package usm

import (
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component/agent"
	corev1 "k8s.io/api/core/v1"

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
func (f *usmFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
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
	managers.Annotation().AddAnnotation(apicommon.SystemProbeAppArmorAnnotationKey, apicommon.SystemProbeAppArmorAnnotationValue)

	// security context capabilities
	managers.SecurityContext().AddCapabilitiesToContainer(agent.DefaultCapabilitiesForSystemProbe(), apicommonv1.SystemProbeContainerName)

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

	// env vars for Core Agent, Process Agent and System Probe
	containersForEnvVars := []apicommonv1.AgentContainerName{
		apicommonv1.CoreAgentContainerName,
		apicommonv1.ProcessAgentContainerName,
		apicommonv1.SystemProbeContainerName,
	}

	enabledEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDSystemProbeServiceMonitoringEnabled,
		Value: "true",
	}
	managers.EnvVar().AddEnvVarToContainers(containersForEnvVars, enabledEnvVar)

	sysProbeEnableEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDSystemProbeEnabled,
		Value: "true",
	}
	managers.EnvVar().AddEnvVarToContainers(
		[]apicommonv1.AgentContainerName{apicommonv1.CoreAgentContainerName, apicommonv1.SystemProbeContainerName},
		sysProbeEnableEnvVar,
	)

	sysProbeSocketEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDSystemProbeSocket,
		Value: apicommon.DefaultSystemProbeSocketPath,
	}
	managers.EnvVar().AddEnvVarToContainers(containersForEnvVars, sysProbeSocketEnvVar)

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
