// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package npm

import (
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/api/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
)

func init() {
	err := feature.Register(feature.NPMIDType, buildNPMFeature)
	if err != nil {
		panic(err)
	}
}

func buildNPMFeature(options *feature.Options) feature.Feature {
	npmFeat := &npmFeature{}

	return npmFeat
}

type npmFeature struct {
	collectDNSStats bool
	enableConntrack bool
}

// ID returns the ID of the Feature
func (f *npmFeature) ID() feature.IDType {
	return feature.NPMIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *npmFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	if dda.Spec.Features == nil {
		return
	}

	if dda.Spec.Features.NPM != nil && apiutils.BoolValue(dda.Spec.Features.NPM.Enabled) {
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
		npm := dda.Spec.Features.NPM
		f.collectDNSStats = apiutils.BoolValue(npm.CollectDNSStats)
		f.enableConntrack = apiutils.BoolValue(npm.EnableConntrack)
	}

	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *npmFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *npmFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *npmFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *npmFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	// annotations
	managers.Annotation().AddAnnotation(apicommon.SystemProbeAppArmorAnnotationKey, apicommon.SystemProbeAppArmorAnnotationValue)

	// security context capabilities
	managers.SecurityContext().AddCapabilitiesToContainer(agent.DefaultCapabilitiesForSystemProbe(), apicommonv1.SystemProbeContainerName)

	// procdir volume mount
	procdirVol, procdirVolMount := volume.GetVolumes(apicommon.ProcdirVolumeName, apicommon.ProcdirHostPath, apicommon.ProcdirMountPath, true)
	managers.Volume().AddVolume(&procdirVol)
	managers.VolumeMount().AddVolumeMountToContainers(&procdirVolMount, []apicommonv1.AgentContainerName{apicommonv1.ProcessAgentContainerName, apicommonv1.SystemProbeContainerName})

	// cgroups volume mount
	cgroupsVol, cgroupsVolMount := volume.GetVolumes(apicommon.CgroupsVolumeName, apicommon.CgroupsHostPath, apicommon.CgroupsMountPath, true)
	managers.Volume().AddVolume(&cgroupsVol)
	managers.VolumeMount().AddVolumeMountToContainers(&cgroupsVolMount, []apicommonv1.AgentContainerName{apicommonv1.ProcessAgentContainerName, apicommonv1.SystemProbeContainerName})

	// debugfs volume mount
	debugfsVol, debugfsVolMount := volume.GetVolumes(apicommon.DebugfsVolumeName, apicommon.DebugfsPath, apicommon.DebugfsPath, false)
	managers.Volume().AddVolume(&debugfsVol)
	managers.VolumeMount().AddVolumeMountToContainers(&debugfsVolMount, []apicommonv1.AgentContainerName{apicommonv1.ProcessAgentContainerName, apicommonv1.SystemProbeContainerName})

	// socket volume mount (needs write perms for the system probe container but not the others)
	socketVol, socketVolMount := volume.GetVolumesEmptyDir(apicommon.SystemProbeSocketVolumeName, apicommon.SystemProbeSocketVolumePath, false)
	managers.Volume().AddVolume(&socketVol)
	managers.VolumeMount().AddVolumeMountToContainer(&socketVolMount, apicommonv1.SystemProbeContainerName)

	_, socketVolMountReadOnly := volume.GetVolumesEmptyDir(apicommon.SystemProbeSocketVolumeName, apicommon.SystemProbeSocketVolumePath, true)
	managers.VolumeMount().AddVolumeMountToContainers(
		&socketVolMountReadOnly,
		[]apicommonv1.AgentContainerName{
			apicommonv1.CoreAgentContainerName,
			apicommonv1.ProcessAgentContainerName,
		},
	)

	// env vars

	// env vars for Core Agent, Process Agent and System Probe
	containersForEnvVars := []apicommonv1.AgentContainerName{
		apicommonv1.CoreAgentContainerName,
		apicommonv1.ProcessAgentContainerName,
		apicommonv1.SystemProbeContainerName,
	}

	enableEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDSystemProbeNPMEnabled,
		Value: "true",
	}
	managers.EnvVar().AddEnvVarToContainers(containersForEnvVars, enableEnvVar)

	sysProbeEnableEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDSystemProbeEnabled,
		Value: "true",
	}
	managers.EnvVar().AddEnvVarToContainers(containersForEnvVars, sysProbeEnableEnvVar)

	socketEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDSystemProbeSocket,
		Value: apicommon.DefaultSystemProbeSocketPath,
	}
	managers.EnvVar().AddEnvVarToContainers(containersForEnvVars, socketEnvVar)

	collectDNSStatsEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDSystemProbeCollectDNSStatsEnabled,
		Value: apiutils.BoolToString(&f.collectDNSStats),
	}
	managers.EnvVar().AddEnvVarToContainers([]apicommonv1.AgentContainerName{apicommonv1.CoreAgentContainerName, apicommonv1.SystemProbeContainerName}, collectDNSStatsEnvVar)

	connTrackEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDSystemProbeConntrackEnabled,
		Value: apiutils.BoolToString(&f.enableConntrack),
	}
	managers.EnvVar().AddEnvVarToContainers([]apicommonv1.AgentContainerName{apicommonv1.CoreAgentContainerName, apicommonv1.SystemProbeContainerName}, connTrackEnvVar)

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
func (f *npmFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
