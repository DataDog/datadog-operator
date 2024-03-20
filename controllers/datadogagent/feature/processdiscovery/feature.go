package processdiscovery

import (
	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object/volume"
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

// RespectCurrentEnvVarMergeFunction to only add envvar when not already set.
func RespectCurrentEnvVarMergeFunction(current, newEnv *corev1.EnvVar) (*corev1.EnvVar, error) {
	if current.Value != "" {
		return current.DeepCopy(), nil
	}
	return newEnv, nil
}

type runInCoreAgentConfig struct {
	enabled bool
}

type processDiscoveryFeature struct {
	runInCoreAgentCfg *runInCoreAgentConfig
}

func (p processDiscoveryFeature) ID() feature.IDType {
	return feature.ProcessDiscoveryIDType
}

func (p *processDiscoveryFeature) Configure(dda *v2alpha1.DatadogAgent) feature.RequiredComponents {
	var reqComp feature.RequiredComponents
	liveProcessesEnabled := dda.Spec.Features.LiveProcessCollection != nil && apiutils.BoolValue(dda.Spec.Features.LiveProcessCollection.Enabled)
	if !liveProcessesEnabled && (dda.Spec.Features.ProcessDiscovery == nil || apiutils.BoolValue(dda.Spec.Features.ProcessDiscovery.Enabled)) {
		requiredContainers := []apicommonv1.AgentContainerName{
			apicommonv1.CoreAgentContainerName,
			apicommonv1.ProcessAgentContainerName,
		}

		if dda.Spec.Features.ProcessDiscovery != nil && dda.Spec.Features.ProcessDiscovery.RunInCoreAgent != nil {
			p.runInCoreAgentCfg = &runInCoreAgentConfig{}
			p.runInCoreAgentCfg.enabled = apiutils.BoolValue(dda.Spec.Features.ProcessDiscovery.RunInCoreAgent.Enabled)
			if p.runInCoreAgentCfg.enabled {
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
	containerName := apicommonv1.ProcessAgentContainerName
	runInCoreAgent := p.runInCoreAgentCfg != nil && p.runInCoreAgentCfg.enabled
	if runInCoreAgent {
		containerName = apicommonv1.CoreAgentContainerName
	}

	runInCoreAgentEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDProcessConfigRunInCoreAgent,
		Value: apiutils.BoolToString(&runInCoreAgent),
	}
	managers.EnvVar().AddEnvVarToContainerWithMergeFunc(apicommonv1.ProcessAgentContainerName, runInCoreAgentEnvVar, RespectCurrentEnvVarMergeFunction)
	managers.EnvVar().AddEnvVarToContainerWithMergeFunc(apicommonv1.CoreAgentContainerName, runInCoreAgentEnvVar, RespectCurrentEnvVarMergeFunction)

	p.manageNodeAgent(containerName, managers, provider)
	return nil
}

func (p processDiscoveryFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	runInCoreAgent := p.runInCoreAgentCfg != nil && p.runInCoreAgentCfg.enabled
	runInCoreAgentEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDProcessConfigRunInCoreAgent,
		Value: apiutils.BoolToString(&runInCoreAgent),
	}
	managers.EnvVar().AddEnvVarToContainerWithMergeFunc(apicommonv1.UnprivilegedSingleAgentContainerName, runInCoreAgentEnvVar, RespectCurrentEnvVarMergeFunction)
	p.manageNodeAgent(apicommonv1.UnprivilegedSingleAgentContainerName, managers, provider)
	return nil
}

func (p processDiscoveryFeature) manageNodeAgent(agentContainerName apicommonv1.AgentContainerName, managers feature.PodTemplateManagers, provider string) error {
	passwdVol, passwdVolMount := volume.GetVolumes(apicommon.PasswdVolumeName, apicommon.PasswdHostPath, apicommon.PasswdMountPath, true)
	managers.VolumeMount().AddVolumeMountToContainer(&passwdVolMount, agentContainerName)
	managers.Volume().AddVolume(&passwdVol)

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
