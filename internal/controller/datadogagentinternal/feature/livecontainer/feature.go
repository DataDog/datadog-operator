// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package livecontainer

import (
	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature"
	featutils "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/utils"
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

type liveContainerFeature struct {
	runInCoreAgent bool
}

// ID returns the ID of the Feature
func (f *liveContainerFeature) ID() feature.IDType {
	return feature.LiveContainerIDType
}

// Configure is used to configure the feature from a v1alpha1.DatadogAgentInternal instance.
func (f *liveContainerFeature) Configure(ddai *v1alpha1.DatadogAgentInternal) (reqComp feature.RequiredComponents) {
	if ddai.Spec.Features.LiveContainerCollection != nil && apiutils.BoolValue(ddai.Spec.Features.LiveContainerCollection.Enabled) {
		reqContainers := []apicommon.AgentContainerName{
			apicommon.CoreAgentContainerName,
		}

		f.runInCoreAgent = featutils.OverrideProcessConfigRunInCoreAgent(ddai, apiutils.BoolValue(ddai.Spec.Global.RunProcessChecksInCoreAgent))

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
func (f *liveContainerFeature) ManageDependencies(managers feature.ResourceManagers) error {
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
func (f *liveContainerFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers) error {
	runInCoreAgentEnvVar := &corev1.EnvVar{
		Name:  common.DDProcessConfigRunInCoreAgent,
		Value: apiutils.BoolToString(&f.runInCoreAgent),
	}
	managers.EnvVar().AddEnvVarToContainer(apicommon.UnprivilegedSingleAgentContainerName, runInCoreAgentEnvVar)
	f.manageNodeAgent(apicommon.UnprivilegedSingleAgentContainerName, managers)
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *liveContainerFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	// Always add this envvar to Core and Process containers
	runInCoreAgentEnvVar := &corev1.EnvVar{
		Name:  common.DDProcessConfigRunInCoreAgent,
		Value: apiutils.BoolToString(&f.runInCoreAgent),
	}
	managers.EnvVar().AddEnvVarToContainer(apicommon.ProcessAgentContainerName, runInCoreAgentEnvVar)
	managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, runInCoreAgentEnvVar)

	containerName := apicommon.CoreAgentContainerName
	if !f.runInCoreAgent {
		containerName = apicommon.ProcessAgentContainerName
	}
	f.manageNodeAgent(containerName, managers)
	return nil
}

func (f *liveContainerFeature) manageNodeAgent(agentContainerName apicommon.AgentContainerName, managers feature.PodTemplateManagers) error {

	// cgroups volume mount
	cgroupsVol, cgroupsVolMount := volume.GetVolumes(common.CgroupsVolumeName, common.CgroupsHostPath, common.CgroupsMountPath, true)
	managers.VolumeMount().AddVolumeMountToContainer(&cgroupsVolMount, agentContainerName)
	managers.Volume().AddVolume(&cgroupsVol)

	// procdir volume mount
	procdirVol, procdirVolMount := volume.GetVolumes(common.ProcdirVolumeName, common.ProcdirHostPath, common.ProcdirMountPath, true)
	managers.VolumeMount().AddVolumeMountToContainer(&procdirVolMount, agentContainerName)
	managers.Volume().AddVolume(&procdirVol)

	managers.EnvVar().AddEnvVarToContainer(agentContainerName, &corev1.EnvVar{
		Name:  common.DDContainerCollectionEnabled,
		Value: "true",
	})

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *liveContainerFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
