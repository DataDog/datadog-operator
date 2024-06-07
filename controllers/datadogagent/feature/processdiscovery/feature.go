package processdiscovery

import (
	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	featutils "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object/volume"
)

func init() {
	err := feature.Register(feature.ProcessDiscoveryIDType, buildProcessDiscoveryFeature)
	if err != nil {
		panic(err)
	}
}

func buildProcessDiscoveryFeature(options *feature.Options) feature.Feature {
	pdFeat := &processDiscoveryFeature{}

	if options != nil {
		pdFeat.runInCoreAgent = options.ProcessChecksInCoreAgentEnabled
	}

	return pdFeat
}

type processDiscoveryFeature struct {
	runInCoreAgent bool
}

func (p processDiscoveryFeature) ID() feature.IDType {
	return feature.ProcessDiscoveryIDType
}

func (p *processDiscoveryFeature) Configure(dda *v2alpha1.DatadogAgent) feature.RequiredComponents {
	var reqComp feature.RequiredComponents
	if dda.Spec.Features.ProcessDiscovery == nil || apiutils.BoolValue(dda.Spec.Features.ProcessDiscovery.Enabled) {
		reqContainers := []apicommonv1.AgentContainerName{
			apicommonv1.CoreAgentContainerName,
		}

		p.runInCoreAgent = featutils.OverrideRunInCoreAgent(dda, p.runInCoreAgent)

		if !p.runInCoreAgent {
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

func (p processDiscoveryFeature) ConfigureV1(dda *v1alpha1.DatadogAgent) feature.RequiredComponents {
	return feature.RequiredComponents{}
}

func (p processDiscoveryFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
	return nil
}

func (p processDiscoveryFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	return nil
}

func (p processDiscoveryFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	// Always add this envvar to Core and Process containers
	runInCoreAgentEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDProcessConfigRunInCoreAgent,
		Value: apiutils.BoolToString(&p.runInCoreAgent),
	}
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.ProcessAgentContainerName, runInCoreAgentEnvVar)
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, runInCoreAgentEnvVar)

	containerName := apicommonv1.CoreAgentContainerName
	if !p.runInCoreAgent {
		containerName = apicommonv1.ProcessAgentContainerName
	}
	p.manageNodeAgent(containerName, managers, provider)
	return nil
}

func (p processDiscoveryFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	runInCoreAgentEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDProcessConfigRunInCoreAgent,
		Value: apiutils.BoolToString(&p.runInCoreAgent),
	}
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.UnprivilegedSingleAgentContainerName, runInCoreAgentEnvVar)
	p.manageNodeAgent(apicommonv1.UnprivilegedSingleAgentContainerName, managers, provider)
	return nil
}

func (p processDiscoveryFeature) manageNodeAgent(agentContainerName apicommonv1.AgentContainerName, managers feature.PodTemplateManagers, provider string) error {
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
		Name:  apicommon.DDProcessDiscoveryEnabled,
		Value: "true",
	}

	managers.EnvVar().AddEnvVarToContainer(agentContainerName, enableEnvVar)

	return nil
}

func (p processDiscoveryFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
