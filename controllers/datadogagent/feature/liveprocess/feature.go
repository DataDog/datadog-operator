// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package liveprocess

import (
	"strconv"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/pkg/utils"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object/volume"
)

func init() {
	err := feature.Register(feature.LiveProcessIDType, buildLiveProcessFeature)
	if err != nil {
		panic(err)
	}
}

func buildLiveProcessFeature(options *feature.Options) feature.Feature {
	liveProcessFeat := &liveProcessFeature{runInCoreAgent: options.RunProcessChecksOnCoreAgent}

	return liveProcessFeat
}

type liveProcessFeature struct {
	scrubArgs      *bool
	stripArgs      *bool
	runInCoreAgent bool
}

const RunInCoreAgentMinVersion = "7.53.0"

// ID returns the ID of the Feature
func (f *liveProcessFeature) ID() feature.IDType {
	return feature.LiveProcessIDType
}

func (f *liveProcessFeature) overrideRunInCoreAgent(dda *v2alpha1.DatadogAgent) {
	if nodeAgent, ok := dda.Spec.Override[v2alpha1.NodeAgentComponentName]; ok {
		// Agent version must >= 7.53.0 to run feature in core agent
		if nodeAgent.Image != nil && !utils.IsAboveMinVersion(component.GetAgentVersionFromImage(*nodeAgent.Image), RunInCoreAgentMinVersion) {
			f.runInCoreAgent = false
		} else if nodeAgent.Env != nil {
			for _, env := range nodeAgent.Env {
				if env.Name == apicommon.DDProcessConfigRunInCoreAgent {
					val, err := strconv.ParseBool(env.Value)
					if err != nil {
						f.runInCoreAgent = val
					}
				}
			}
		}
	}
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

		reqContainers := []apicommonv1.AgentContainerName{
			apicommonv1.CoreAgentContainerName,
		}

		f.overrideRunInCoreAgent(dda)

		if !f.runInCoreAgent {
			reqContainers = append(reqContainers, apicommonv1.ProcessAgentContainerName)
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

// ConfigureV1 use to configure the feature from a v1alpha1.DatadogAgent instance.
func (f *liveProcessFeature) ConfigureV1(dda *v1alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	if dda.Spec.Agent.Process != nil && *dda.Spec.Agent.Process.ProcessCollectionEnabled {
		reqComp = feature.RequiredComponents{
			Agent: feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
				Containers: []apicommonv1.AgentContainerName{
					apicommonv1.CoreAgentContainerName,
					apicommonv1.ProcessAgentContainerName,
				},
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
	f.manageNodeAgent(apicommonv1.UnprivilegedSingleAgentContainerName, managers, provider)
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *liveProcessFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	containerName := apicommonv1.CoreAgentContainerName
	if !f.runInCoreAgent {
		containerName = apicommonv1.ProcessAgentContainerName
	}
	runInCoreAgentEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDProcessConfigRunInCoreAgent,
		Value: apiutils.BoolToString(&f.runInCoreAgent),
	}
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.ProcessAgentContainerName, runInCoreAgentEnvVar)
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, runInCoreAgentEnvVar)

	f.manageNodeAgent(containerName, managers, provider)
	return nil
}

func (f *liveProcessFeature) manageNodeAgent(agentContainerName apicommonv1.AgentContainerName, managers feature.PodTemplateManagers, provider string) error {

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
