// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tcpqueuelength

import (
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component/agent"
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
	err := feature.Register(feature.TCPQueueLengthIDType, buildTCPQueueLengthFeature)
	if err != nil {
		panic(err)
	}
}

func buildTCPQueueLengthFeature(options *feature.Options) feature.Feature {
	tcpQueueLengthFeat := &tcpQueueLengthFeature{}

	return tcpQueueLengthFeat
}

type tcpQueueLengthFeature struct{}

// ID returns the ID of the Feature
func (f *tcpQueueLengthFeature) ID() feature.IDType {
	return feature.TCPQueueLengthIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *tcpQueueLengthFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	if dda.Spec.Features == nil {
		return
	}
	if dda.Spec.Features.TCPQueueLength != nil && apiutils.BoolValue(dda.Spec.Features.TCPQueueLength.Enabled) {
		reqComp.Agent = feature.RequiredComponent{
			IsRequired: apiutils.NewBoolPointer(true),
			Containers: []apicommonv1.AgentContainerName{apicommonv1.CoreAgentContainerName, apicommonv1.SystemProbeContainerName},
		}
	}

	return reqComp
}

// ConfigureV1 use to configure the feature from a v1alpha1.DatadogAgent instance.
func (f *tcpQueueLengthFeature) ConfigureV1(dda *v1alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	if dda.Spec.Agent.SystemProbe != nil && apiutils.BoolValue(dda.Spec.Agent.SystemProbe.EnableTCPQueueLength) {
		reqComp.Agent = feature.RequiredComponent{
			IsRequired: apiutils.NewBoolPointer(true),
			Containers: []apicommonv1.AgentContainerName{apicommonv1.CoreAgentContainerName, apicommonv1.SystemProbeContainerName},
		}
	}

	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *tcpQueueLengthFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *tcpQueueLengthFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *tcpQueueLengthFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	// security context capabilities
	managers.SecurityContext().AddCapabilitiesToContainer(agent.DefaultCapabilitiesForSystemProbe(), apicommonv1.SystemProbeContainerName)

	// modules volume mount
	modulesVol, modulesVolMount := volume.GetVolumes(apicommon.ModulesVolumeName, apicommon.ModulesVolumePath, apicommon.ModulesVolumePath, true)
	managers.VolumeMount().AddVolumeMountToContainer(&modulesVolMount, apicommonv1.SystemProbeContainerName)
	managers.Volume().AddVolume(&modulesVol)

	// src volume mount
	srcVol, srcVolMount := volume.GetVolumes(apicommon.SrcVolumeName, apicommon.SrcVolumePath, apicommon.SrcVolumePath, true)
	managers.VolumeMount().AddVolumeMountToContainer(&srcVolMount, apicommonv1.SystemProbeContainerName)
	managers.Volume().AddVolume(&srcVol)

	// debugfs volume mount
	debugfsVol, debugfsVolMount := volume.GetVolumes(apicommon.DebugfsVolumeName, apicommon.DebugfsPath, apicommon.DebugfsPath, false)
	managers.Volume().AddVolume(&debugfsVol)
	managers.VolumeMount().AddVolumeMountToContainers(&debugfsVolMount, []apicommonv1.AgentContainerName{apicommonv1.ProcessAgentContainerName, apicommonv1.SystemProbeContainerName})

	// socket volume mount (needs write perms for the system probe container but not the others)
	socketVol, socketVolMount := volume.GetVolumesEmptyDir(apicommon.SystemProbeSocketVolumeName, apicommon.SystemProbeSocketVolumePath, false)
	managers.Volume().AddVolume(&socketVol)
	managers.VolumeMount().AddVolumeMountToContainer(&socketVolMount, apicommonv1.SystemProbeContainerName)

	_, socketVolMountReadOnly := volume.GetVolumesEmptyDir(apicommon.SystemProbeSocketVolumeName, apicommon.SystemProbeSocketVolumePath, true)
	managers.VolumeMount().AddVolumeMountToContainer(&socketVolMountReadOnly, apicommonv1.CoreAgentContainerName)

	enableEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDEnableTCPQueueLengthEnvVar,
		Value: "true",
	}

	managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, enableEnvVar)
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.SystemProbeContainerName, enableEnvVar)
	managers.EnvVar().AddEnvVarToInitContainer(apicommonv1.InitConfigContainerName, enableEnvVar)

	socketEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDSystemProbeSocket,
		Value: apicommon.DefaultSystemProbeSocketPath,
	}

	managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, socketEnvVar)
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.SystemProbeContainerName, socketEnvVar)

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *tcpQueueLengthFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
