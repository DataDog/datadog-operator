// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package livecontainer

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
	err := feature.Register(feature.LiveContainerIDType, buildLiveContainerFeature)
	if err != nil {
		panic(err)
	}
}

func buildLiveContainerFeature(options *feature.Options) feature.Feature {
	liveContainerFeat := &liveContainerFeature{}

	return liveContainerFeat
}

type runInCoreAgentConfig struct {
	enabled bool
}

type liveContainerFeature struct {
	runInCoreAgentCfg *runInCoreAgentConfig
}

// ID returns the ID of the Feature
func (f *liveContainerFeature) ID() feature.IDType {
	return feature.LiveContainerIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *liveContainerFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	// Live Container Collection's functionality is a subset of Live Process Collection
	liveProcessesEnabled := dda.Spec.Features.LiveProcessCollection != nil && apiutils.BoolValue(dda.Spec.Features.LiveProcessCollection.Enabled)

	if !liveProcessesEnabled && (dda.Spec.Features.LiveContainerCollection != nil && apiutils.BoolValue(dda.Spec.Features.LiveContainerCollection.Enabled)) {
		requiredContainers := []apicommonv1.AgentContainerName{
			apicommonv1.CoreAgentContainerName,
			apicommonv1.ProcessAgentContainerName,
		}

		if dda.Spec.Features.LiveContainerCollection.RunInCoreAgent != nil {
			f.runInCoreAgentCfg = &runInCoreAgentConfig{}
			f.runInCoreAgentCfg.enabled = apiutils.BoolValue(dda.Spec.Features.LiveContainerCollection.RunInCoreAgent.Enabled)
			if f.runInCoreAgentCfg.enabled {
				requiredContainers = []apicommonv1.AgentContainerName{
					apicommonv1.CoreAgentContainerName,
				}
			}
		}

		reqComp = feature.RequiredComponents{
			Agent: feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
				Containers: requiredContainers,
			},
		}
	}

	return reqComp
}

// ConfigureV1 use to configure the feature from a v1alpha1.DatadogAgent instance.
func (f *liveContainerFeature) ConfigureV1(dda *v1alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	if dda.Spec.Agent.Process != nil && *dda.Spec.Agent.Process.Enabled {
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
func (f *liveContainerFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *liveContainerFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *liveContainerFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	runInCoreAgent := f.runInCoreAgentCfg != nil && f.runInCoreAgentCfg.enabled
	runInCoreAgentEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDProcessConfigRunInCoreAgent,
		Value: apiutils.BoolToString(&runInCoreAgent),
	}
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.UnprivilegedSingleAgentContainerName, runInCoreAgentEnvVar)
	f.manageNodeAgent(apicommonv1.UnprivilegedSingleAgentContainerName, managers, provider)
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *liveContainerFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	containerName := apicommonv1.ProcessAgentContainerName
	runInCoreAgent := f.runInCoreAgentCfg != nil && f.runInCoreAgentCfg.enabled
	if runInCoreAgent {
		containerName = apicommonv1.CoreAgentContainerName
	}
	runInCoreAgentEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDProcessConfigRunInCoreAgent,
		Value: apiutils.BoolToString(&runInCoreAgent),
	}
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.ProcessAgentContainerName, runInCoreAgentEnvVar)
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, runInCoreAgentEnvVar)

	f.manageNodeAgent(containerName, managers, provider)
	return nil
}

func (f *liveContainerFeature) manageNodeAgent(agentContainerName apicommonv1.AgentContainerName, managers feature.PodTemplateManagers, provider string) error {

	// cgroups volume mount
	cgroupsVol, cgroupsVolMount := volume.GetVolumes(apicommon.CgroupsVolumeName, apicommon.CgroupsHostPath, apicommon.CgroupsMountPath, true)
	managers.VolumeMount().AddVolumeMountToContainer(&cgroupsVolMount, agentContainerName)
	managers.Volume().AddVolume(&cgroupsVol)

	// procdir volume mount
	procdirVol, procdirVolMount := volume.GetVolumes(apicommon.ProcdirVolumeName, apicommon.ProcdirHostPath, apicommon.ProcdirMountPath, true)
	managers.VolumeMount().AddVolumeMountToContainer(&procdirVolMount, agentContainerName)
	managers.Volume().AddVolume(&procdirVol)

	managers.EnvVar().AddEnvVarToContainer(agentContainerName, &corev1.EnvVar{
		Name:  apicommon.DDContainerCollectionEnabled,
		Value: "true",
	})

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *liveContainerFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
