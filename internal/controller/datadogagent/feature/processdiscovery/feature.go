// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processdiscovery

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	featutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
)

func init() {
	err := feature.Register(feature.ProcessDiscoveryIDType, buildProcessDiscoveryFeature)
	if err != nil {
		panic(err)
	}
}

func buildProcessDiscoveryFeature(options *feature.Options) feature.Feature {
	return &processDiscoveryFeature{}
}

type processDiscoveryFeature struct {
	runInCoreAgent bool
}

func (p *processDiscoveryFeature) ID() feature.IDType {
	return feature.ProcessDiscoveryIDType
}

func (p *processDiscoveryFeature) Configure(_ metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) feature.RequiredComponents {
	var reqComp feature.RequiredComponents
	if ddaSpec.Features.ProcessDiscovery == nil || apiutils.BoolValue(ddaSpec.Features.ProcessDiscovery.Enabled) {
		reqContainers := []apicommon.AgentContainerName{
			apicommon.CoreAgentContainerName,
		}

		p.runInCoreAgent = featutils.OverrideProcessConfigRunInCoreAgent(ddaSpec, apiutils.BoolValue(ddaSpec.Global.RunProcessChecksInCoreAgent))

		if !p.runInCoreAgent {
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

func (p processDiscoveryFeature) ManageDependencies(managers feature.ResourceManagers, provider string) error {
	return nil
}

func (p *processDiscoveryFeature) ManageClusterAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

func (p *processDiscoveryFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	// Always add this envvar to Core and Process containers
	runInCoreAgentEnvVar := &corev1.EnvVar{
		Name:  common.DDProcessConfigRunInCoreAgent,
		Value: apiutils.BoolToString(&p.runInCoreAgent),
	}
	managers.EnvVar().AddEnvVarToContainer(apicommon.ProcessAgentContainerName, runInCoreAgentEnvVar)
	managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, runInCoreAgentEnvVar)

	containerName := apicommon.CoreAgentContainerName
	if !p.runInCoreAgent {
		containerName = apicommon.ProcessAgentContainerName
	}
	p.manageNodeAgent(containerName, managers, provider)
	return nil
}

func (p *processDiscoveryFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	runInCoreAgentEnvVar := &corev1.EnvVar{
		Name:  common.DDProcessConfigRunInCoreAgent,
		Value: apiutils.BoolToString(&p.runInCoreAgent),
	}
	managers.EnvVar().AddEnvVarToContainer(apicommon.UnprivilegedSingleAgentContainerName, runInCoreAgentEnvVar)
	p.manageNodeAgent(apicommon.UnprivilegedSingleAgentContainerName, managers, provider)
	return nil
}

func (p *processDiscoveryFeature) manageNodeAgent(agentContainerName apicommon.AgentContainerName, managers feature.PodTemplateManagers, provider string) error {
	// passwd volume mount
	passwdVol, passwdVolMount := volume.GetVolumes(common.PasswdVolumeName, common.PasswdHostPath, common.PasswdMountPath, true)
	managers.VolumeMount().AddVolumeMountToContainer(&passwdVolMount, agentContainerName)
	managers.Volume().AddVolume(&passwdVol)

	// cgroups volume mount
	cgroupsVol, cgroupsVolMount := volume.GetVolumes(common.CgroupsVolumeName, common.CgroupsHostPath, common.CgroupsMountPath, true)
	managers.VolumeMount().AddVolumeMountToContainer(&cgroupsVolMount, agentContainerName)
	managers.Volume().AddVolume(&cgroupsVol)

	// procdir volume mount
	procdirVol, procdirVolMount := volume.GetVolumes(common.ProcdirVolumeName, common.ProcdirHostPath, common.ProcdirMountPath, true)
	managers.VolumeMount().AddVolumeMountToContainer(&procdirVolMount, agentContainerName)
	managers.Volume().AddVolume(&procdirVol)

	enableEnvVar := &corev1.EnvVar{
		Name:  DDProcessDiscoveryEnabled,
		Value: "true",
	}

	managers.EnvVar().AddEnvVarToContainer(agentContainerName, enableEnvVar)

	return nil
}

func (p processDiscoveryFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers, provider string) error {
	return nil
}
