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
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/providercaps"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
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
	// The passwd mount path depends on the node Agent version, but ManageNodeAgent
	// only receives the pod managers. Keep the DatadogAgent from Configure so
	// ProcessPasswdMountPath can read its node Agent image override.
	owner          metav1.Object
	runInCoreAgent bool
}

func (p *processDiscoveryFeature) ID() feature.IDType {
	return feature.ProcessDiscoveryIDType
}

// NodeAgentProviderCapabilities returns provider-conditional pod-template
// mutations for the node agent. Talos has no host user database, so the
// passwd volume (passwd or host-passwd, plus os-release with host-passwd) this feature adds is stripped.
func (p *processDiscoveryFeature) NodeAgentProviderCapabilities() providercaps.ProviderCapabilityMap {
	return providercaps.ProviderCapabilityMap{
		kubernetes.TalosProvider: {
			RemoveVolumes: []string{common.PasswdVolumeName, common.HostPasswdVolumeName, common.OSReleaseFileVolumeName},
		},
	}
}

func (p *processDiscoveryFeature) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) feature.RequiredComponents {
	p.owner = dda
	var reqComp feature.RequiredComponents
	if ddaSpec.Features.ProcessDiscovery == nil || apiutils.BoolValue(ddaSpec.Features.ProcessDiscovery.Enabled) {
		reqContainers := []apicommon.AgentContainerName{
			apicommon.CoreAgentContainerName,
		}

		p.runInCoreAgent = featutils.ShouldRunProcessChecksInCoreAgent(dda, ddaSpec)

		if !p.runInCoreAgent {
			reqContainers = append(reqContainers, apicommon.ProcessAgentContainerName)
		}

		reqComp = feature.RequiredComponents{
			Agent: feature.RequiredComponent{
				IsRequired: new(true),
				Containers: reqContainers,
			},
		}
	}
	return reqComp
}

func (p *processDiscoveryFeature) ManageDependencies(managers feature.ResourceManagers) error {
	return nil
}

func (p *processDiscoveryFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	return nil
}

func (p *processDiscoveryFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
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
	p.manageNodeAgent(containerName, managers)
	return nil
}

func (p *processDiscoveryFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers) error {
	runInCoreAgentEnvVar := &corev1.EnvVar{
		Name:  common.DDProcessConfigRunInCoreAgent,
		Value: apiutils.BoolToString(&p.runInCoreAgent),
	}
	managers.EnvVar().AddEnvVarToContainer(apicommon.UnprivilegedSingleAgentContainerName, runInCoreAgentEnvVar)
	p.manageNodeAgent(apicommon.UnprivilegedSingleAgentContainerName, managers)
	return nil
}

func (p *processDiscoveryFeature) manageNodeAgent(agentContainerName apicommon.AgentContainerName, managers feature.PodTemplateManagers) error {
	// passwd volume mount
	mountPath := featutils.ProcessPasswdMountPath(p.owner)
	passwdVol, passwdMount := featutils.ProcessPasswdVolume(mountPath)
	managers.VolumeMount().AddVolumeMountToContainer(&passwdMount, agentContainerName)
	managers.Volume().AddVolume(&passwdVol)

	// Mounting passwd under /host/etc makes the Agent read host files from there,
	// so host os-release must be mounted there too.
	if mountPath == common.HostPasswdMountPath {
		osReleaseVol, osReleaseVolMount := volume.GetVolumes(common.OSReleaseFileVolumeName, common.OSReleaseFileHostPath, common.OSReleaseFileMountPath, true)
		managers.VolumeMount().AddVolumeMountToContainer(&osReleaseVolMount, agentContainerName)
		managers.Volume().AddVolume(&osReleaseVol)
	}

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

func (p *processDiscoveryFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}

func (p *processDiscoveryFeature) ManageOtelAgentGateway(managers feature.PodTemplateManagers) error {
	return nil
}
