// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package liveprocess

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	featutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
)

func init() {
	err := feature.Register(feature.LiveProcessIDType, buildLiveProcessFeature)
	if err != nil {
		panic(err)
	}
}

func buildLiveProcessFeature(options *feature.Options) feature.Feature {
	liveProcessFeat := &liveProcessFeature{}
	return liveProcessFeat
}

type liveProcessFeature struct {
	scrubArgs      *bool
	stripArgs      *bool
	runInCoreAgent bool
}

// ID returns the ID of the Feature
func (f *liveProcessFeature) ID() feature.IDType {
	return feature.LiveProcessIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *liveProcessFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	if dda.Spec.Features.LiveProcessCollection != nil && apiutils.BoolValue(dda.Spec.Features.LiveProcessCollection.Enabled) {
		if dda.Spec.Features.LiveProcessCollection.ScrubProcessArguments != nil {
			f.scrubArgs = apiutils.NewBoolPointer(*dda.Spec.Features.LiveProcessCollection.ScrubProcessArguments)
		}
		if dda.Spec.Features.LiveProcessCollection.StripProcessArguments != nil {
			f.stripArgs = apiutils.NewBoolPointer(*dda.Spec.Features.LiveProcessCollection.StripProcessArguments)
		}

		reqContainers := []apicommon.AgentContainerName{
			apicommon.CoreAgentContainerName,
		}

		f.runInCoreAgent = featutils.OverrideRunInCoreAgent(dda, apiutils.BoolValue(dda.Spec.Global.RunProcessChecksInCoreAgent))

		if !f.runInCoreAgent {
			reqContainers = append(reqContainers, apicommon.ProcessAgentContainerName)
		}

		reqComp = feature.RequiredComponents{
			Agent: feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
				Containers: reqContainers,
			},
		}
	}

	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *liveProcessFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *liveProcessFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *liveProcessFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	runInCoreAgentEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDProcessConfigRunInCoreAgent,
		Value: apiutils.BoolToString(&f.runInCoreAgent),
	}
	managers.EnvVar().AddEnvVarToContainer(apicommon.UnprivilegedSingleAgentContainerName, runInCoreAgentEnvVar)
	f.manageNodeAgent(apicommon.UnprivilegedSingleAgentContainerName, managers, provider)
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *liveProcessFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	// Always add this envvar to Core and Process containers
	runInCoreAgentEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDProcessConfigRunInCoreAgent,
		Value: apiutils.BoolToString(&f.runInCoreAgent),
	}
	managers.EnvVar().AddEnvVarToContainer(apicommon.ProcessAgentContainerName, runInCoreAgentEnvVar)
	managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, runInCoreAgentEnvVar)

	containerName := apicommon.CoreAgentContainerName
	if !f.runInCoreAgent {
		containerName = apicommon.ProcessAgentContainerName
	}
	f.manageNodeAgent(containerName, managers, provider)
	return nil
}

func (f *liveProcessFeature) manageNodeAgent(agentContainerName apicommon.AgentContainerName, managers feature.PodTemplateManagers, provider string) error {

	// passwd volume mount
	passwdVol, passwdVolMount := volume.GetVolumes(apicommon.PasswdVolumeName, apicommon.PasswdHostPath, apicommon.PasswdMountPath, true)
	managers.VolumeMount().AddVolumeMountToContainer(&passwdVolMount, agentContainerName)
	managers.Volume().AddVolume(&passwdVol)

	// cgroups volume mount
	cgroupsVol, cgroupsVolMount := volume.GetVolumes(apicommon.CgroupsVolumeName, apicommon.CgroupsHostPath, apicommon.CgroupsMountPath, true)
	managers.VolumeMount().AddVolumeMountToContainer(&cgroupsVolMount, agentContainerName)
	managers.Volume().AddVolume(&cgroupsVol)

	// procdir volume mount
	procdirVol, procdirVolMount := volume.GetVolumes(apicommon.ProcdirVolumeName, apicommon.ProcdirHostPath, apicommon.ProcdirMountPath, true)
	managers.VolumeMount().AddVolumeMountToContainer(&procdirVolMount, agentContainerName)
	managers.Volume().AddVolume(&procdirVol)

	enableEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDProcessCollectionEnabled,
		Value: "true",
	}

	managers.EnvVar().AddEnvVarToContainer(agentContainerName, enableEnvVar)

	if f.scrubArgs != nil {
		scrubArgsEnvVar := &corev1.EnvVar{
			Name:  apicommon.DDProcessConfigScrubArgs,
			Value: apiutils.BoolToString(f.scrubArgs),
		}
		managers.EnvVar().AddEnvVarToContainer(agentContainerName, scrubArgsEnvVar)
	}

	if f.stripArgs != nil {
		stripArgsEnvVar := &corev1.EnvVar{
			Name:  apicommon.DDProcessConfigStripArgs,
			Value: apiutils.BoolToString(f.stripArgs),
		}
		managers.EnvVar().AddEnvVarToContainer(agentContainerName, stripArgsEnvVar)
	}

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *liveProcessFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
