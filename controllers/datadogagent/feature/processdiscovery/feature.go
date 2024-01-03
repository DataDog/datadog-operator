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

type processDiscoveryFeature struct{}

func (p processDiscoveryFeature) ID() feature.IDType {
	return feature.ProcessDiscoveryIDType
}

func (p processDiscoveryFeature) Configure(dda *v2alpha1.DatadogAgent) feature.RequiredComponents {
	var reqComp feature.RequiredComponents
	if dda.Spec.Features.ProcessDiscovery == nil || apiutils.BoolValue(dda.Spec.Features.ProcessDiscovery.Enabled) {
		reqComp = feature.RequiredComponents{
			Agent: feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
				Containers: []apicommonv1.AgentContainerName{
					apicommonv1.CoreAgentContainerName,
					apicommonv1.ProcessAgentContainerName,
				},
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
	p.manageNodeAgent(apicommonv1.ProcessAgentContainerName, managers)
	return nil
}

func (p processDiscoveryFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	p.manageNodeAgent(apicommonv1.ProcessAgentContainerName, managers)
	return nil
}

func (p processDiscoveryFeature) ManageMultiProcessNodeAgent(managers feature.PodTemplateManagers) error {
	p.manageNodeAgent(apicommonv1.UnprivilegedMultiProcessAgentContainerName, managers)
	return nil
}

func (p processDiscoveryFeature) manageNodeAgent(agentContainerName apicommonv1.AgentContainerName, managers feature.PodTemplateManagers) error {
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
