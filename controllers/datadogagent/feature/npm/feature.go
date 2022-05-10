// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package npm

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
	enable bool
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *npmFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	if dda.Spec.Features.NPM != nil && apiutils.BoolValue(dda.Spec.Features.NPM.Enabled) {
		f.enable = true
		reqComp = feature.RequiredComponents{
			Agent: feature.RequiredComponent{
				IsRequired: &f.enable,
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

// ConfigureV1 use to configure the feature from a v1alpha1.DatadogAgent instance.
func (f *npmFeature) ConfigureV1(dda *v1alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	if dda.Spec.Features.NetworkMonitoring != nil && *dda.Spec.Features.NetworkMonitoring.Enabled {
		f.enable = true
		reqComp = feature.RequiredComponents{
			Agent: feature.RequiredComponent{
				IsRequired: &f.enable,
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

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *npmFeature) ManageDependencies(managers feature.ResourceManagers) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *npmFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *npmFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	// procdir volume mount
	procdirVol, procdirVolMount := volume.GetVolumes(apicommon.ProcdirVolumeName, apicommon.ProcdirHostPath, apicommon.ProcdirMountPath)
	managers.Volume().AddVolumeToContainers(&procdirVol, &procdirVolMount, []apicommonv1.AgentContainerName{apicommonv1.ProcessAgentContainerName, apicommonv1.SystemProbeContainerName})

	// cgroups volume mount
	cgroupsVol, cgroupsVolMount := volume.GetVolumes(apicommon.CgroupsVolumeName, apicommon.CgroupsHostPath, apicommon.CgroupsMountPath)
	managers.Volume().AddVolumeToContainers(&cgroupsVol, &cgroupsVolMount, []apicommonv1.AgentContainerName{apicommonv1.ProcessAgentContainerName, apicommonv1.SystemProbeContainerName})

	// debugfs volume mount
	debugfsVol, debugfsVolMount := volume.GetVolumes(apicommon.DebugfsVolumeName, apicommon.DebugfsVolumePath, apicommon.DebugfsVolumePath)
	managers.Volume().AddVolumeToContainers(&debugfsVol, &debugfsVolMount, []apicommonv1.AgentContainerName{apicommonv1.ProcessAgentContainerName, apicommonv1.SystemProbeContainerName})

	// socket volume mount
	socketVol, socketVolMount := volume.GetVolumes(apicommon.SysprobeSocketVolumeName, apicommon.SysprobeSocketVolumePath, apicommon.SysprobeSocketVolumePath)
	managers.Volume().AddVolumeToContainers(&socketVol, &socketVolMount, []apicommonv1.AgentContainerName{apicommonv1.ProcessAgentContainerName, apicommonv1.SystemProbeContainerName})

	// env vars
	enableEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDSystemProbeNPMEnabledEnvVar,
		Value: "true",
	}
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.ProcessAgentContainerName, enableEnvVar)
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.SystemProbeContainerName, enableEnvVar)

	sysProbeEnableEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDSystemProbeEnabledEnvVar,
		Value: "true",
	}
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.ProcessAgentContainerName, sysProbeEnableEnvVar)
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.SystemProbeContainerName, sysProbeEnableEnvVar)

	socketEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDSystemProbeSocketEnvVar,
		Value: apicommon.DefaultSysprobeSocketPath,
	}

	managers.EnvVar().AddEnvVarToContainer(apicommonv1.ProcessAgentContainerName, socketEnvVar)
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.SystemProbeContainerName, socketEnvVar)

	processEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDProcessAgentEnabledEnvVar,
		Value: "true",
	}

	managers.EnvVar().AddEnvVarToContainer(apicommonv1.ProcessAgentContainerName, processEnvVar)
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.SystemProbeContainerName, processEnvVar)

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *npmFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
