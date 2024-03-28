package processdiscovery

import (
	"strconv"

	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/liveprocess"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object/volume"
	"github.com/DataDog/datadog-operator/pkg/utils"
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
		pdFeat.runInCoreAgent = options.RunProcessChecksOnCoreAgent
	}

	return pdFeat
}

type processDiscoveryFeature struct {
	runInCoreAgent bool
}

func (p processDiscoveryFeature) ID() feature.IDType {
	return feature.ProcessDiscoveryIDType
}

func (p *processDiscoveryFeature) overrideRunInCoreAgent(dda *v2alpha1.DatadogAgent) {
	if nodeAgent, ok := dda.Spec.Override[v2alpha1.NodeAgentComponentName]; ok {
		// Agent version must >= 7.53.0 to run feature in core agent
		if nodeAgent.Image != nil && !utils.IsAboveMinVersion(component.GetAgentVersionFromImage(*nodeAgent.Image), liveprocess.RunInCoreAgentMinVersion) {
			p.runInCoreAgent = false
		} else {
			for _, env := range nodeAgent.Env {
				if env.Name == apicommon.DDProcessConfigRunInCoreAgent {
					val, err := strconv.ParseBool(env.Value)
					if err == nil {
						p.runInCoreAgent = val
					}
				}
			}
		}
	}
}

func (p *processDiscoveryFeature) Configure(dda *v2alpha1.DatadogAgent) feature.RequiredComponents {
	var reqComp feature.RequiredComponents
	if dda.Spec.Features.ProcessDiscovery == nil || apiutils.BoolValue(dda.Spec.Features.ProcessDiscovery.Enabled) {
		reqContainers := []apicommonv1.AgentContainerName{
			apicommonv1.CoreAgentContainerName,
		}

		p.overrideRunInCoreAgent(dda)

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
	containerName := apicommonv1.CoreAgentContainerName
	if !p.runInCoreAgent {
		containerName = apicommonv1.ProcessAgentContainerName
	}
	runInCoreAgentEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDProcessConfigRunInCoreAgent,
		Value: apiutils.BoolToString(&p.runInCoreAgent),
	}
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.ProcessAgentContainerName, runInCoreAgentEnvVar)
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, runInCoreAgentEnvVar)
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
