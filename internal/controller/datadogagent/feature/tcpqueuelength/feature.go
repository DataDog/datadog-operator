// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tcpqueuelength

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
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
func (f *tcpQueueLengthFeature) Configure(_ metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) (reqComp feature.RequiredComponents) {
	if ddaSpec.Features == nil {
		return
	}
	if ddaSpec.Features.TCPQueueLength != nil && apiutils.BoolValue(ddaSpec.Features.TCPQueueLength.Enabled) {
		reqComp.Agent = feature.RequiredComponent{
			IsRequired: apiutils.NewBoolPointer(true),
			Containers: []apicommon.AgentContainerName{apicommon.CoreAgentContainerName, apicommon.SystemProbeContainerName},
		}
	}

	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *tcpQueueLengthFeature) ManageDependencies(managers feature.ResourceManagers, provider string) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *tcpQueueLengthFeature) ManageClusterAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *tcpQueueLengthFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *tcpQueueLengthFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	// security context capabilities
	managers.SecurityContext().AddCapabilitiesToContainer(agent.DefaultCapabilitiesForSystemProbe(), apicommon.SystemProbeContainerName)

	// modules volume mount
	modulesVol, modulesVolMount := volume.GetVolumes(common.ModulesVolumeName, common.ModulesVolumePath, common.ModulesVolumePath, true)
	managers.VolumeMount().AddVolumeMountToContainer(&modulesVolMount, apicommon.SystemProbeContainerName)
	managers.Volume().AddVolume(&modulesVol)

	// src volume mount
	_, providerValue := kubernetes.GetProviderLabelKeyValue(provider)
	if providerValue != kubernetes.GKECosType {
		srcVol, srcVolMount := volume.GetVolumes(common.SrcVolumeName, common.SrcVolumePath, common.SrcVolumePath, true)
		managers.VolumeMount().AddVolumeMountToContainer(&srcVolMount, apicommon.SystemProbeContainerName)
		managers.Volume().AddVolume(&srcVol)
	}

	// debugfs volume mount
	debugfsVol, debugfsVolMount := volume.GetVolumes(common.DebugfsVolumeName, common.DebugfsPath, common.DebugfsPath, false)
	managers.Volume().AddVolume(&debugfsVol)
	managers.VolumeMount().AddVolumeMountToContainers(&debugfsVolMount, []apicommon.AgentContainerName{apicommon.SystemProbeContainerName})

	// socket volume mount (needs write perms for the system probe container but not the others)
	socketVol, socketVolMount := volume.GetVolumesEmptyDir(common.SystemProbeSocketVolumeName, common.SystemProbeSocketVolumePath, false)
	managers.Volume().AddVolume(&socketVol)
	managers.VolumeMount().AddVolumeMountToContainer(&socketVolMount, apicommon.SystemProbeContainerName)

	_, socketVolMountReadOnly := volume.GetVolumesEmptyDir(common.SystemProbeSocketVolumeName, common.SystemProbeSocketVolumePath, true)
	managers.VolumeMount().AddVolumeMountToContainer(&socketVolMountReadOnly, apicommon.CoreAgentContainerName)

	// env vars
	enableEnvVar := &corev1.EnvVar{
		Name:  DDEnableTCPQueueLengthEnvVar,
		Value: "true",
	}
	managers.EnvVar().AddEnvVarToContainers(
		[]apicommon.AgentContainerName{apicommon.CoreAgentContainerName, apicommon.SystemProbeContainerName},
		enableEnvVar,
	)
	managers.EnvVar().AddEnvVarToInitContainer(apicommon.InitConfigContainerName, enableEnvVar)

	socketEnvVar := &corev1.EnvVar{
		Name:  common.DDSystemProbeSocket,
		Value: common.DefaultSystemProbeSocketPath,
	}
	managers.EnvVar().AddEnvVarToContainers(
		[]apicommon.AgentContainerName{apicommon.CoreAgentContainerName, apicommon.SystemProbeContainerName},
		socketEnvVar,
	)

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *tcpQueueLengthFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

func (f *tcpQueueLengthFeature) ManageOtelAgentGateway(managers feature.PodTemplateManagers, provider string) error {
	return nil
}
