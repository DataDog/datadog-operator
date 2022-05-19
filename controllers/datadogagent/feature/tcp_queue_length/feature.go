// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tcpqueuelength

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

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *tcpQueueLengthFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
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
	// modules volume mount
	modulesVol, modulesVolMount := volume.GetVolumes(apicommon.ModulesVolumeName, apicommon.ModulesVolumePath, apicommon.ModulesVolumePath, true)
	managers.Volume().AddVolumeToContainer(&modulesVol, &modulesVolMount, apicommonv1.SystemProbeContainerName)

	// src volume mount
	srcVol, srcVolMount := volume.GetVolumes(apicommon.SrcVolumeName, apicommon.SrcVolumePath, apicommon.SrcVolumePath, true)
	managers.Volume().AddVolumeToContainer(&srcVol, &srcVolMount, apicommonv1.SystemProbeContainerName)

	enableEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDEnableTCPQueueLengthEnvVar,
		Value: "true",
	}

	managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, enableEnvVar)
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.SystemProbeContainerName, enableEnvVar)

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *tcpQueueLengthFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
