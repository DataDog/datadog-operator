package ebpfcheck

import (
	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object/volume"
)

func init() {
	err := feature.Register(feature.EBPFCheckIDType, buildEBPFCheckFeature)
	if err != nil {
		panic(err)
	}
}

func buildEBPFCheckFeature(options *feature.Options) feature.Feature {
	ebpfCheckFeat := &ebpfCheckFeature{}

	return ebpfCheckFeat
}

type ebpfCheckFeature struct{}

// ID returns the ID of the Feature
func (f *ebpfCheckFeature) ID() feature.IDType {
	return feature.EBPFCheckIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *ebpfCheckFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	if dda.Spec.Features != nil && dda.Spec.Features.EBPFCheck != nil && apiutils.BoolValue(dda.Spec.Features.EBPFCheck.Enabled) {
		reqComp.Agent = feature.RequiredComponent{
			IsRequired: apiutils.NewBoolPointer(true),
			Containers: []apicommonv1.AgentContainerName{apicommonv1.CoreAgentContainerName, apicommonv1.SystemProbeContainerName},
		}
	}

	return reqComp
}

// ConfigureV1 use to configure the feature from a v1alpha1.DatadogAgent instance.
func (f *ebpfCheckFeature) ConfigureV1(dda *v1alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *ebpfCheckFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *ebpfCheckFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *ebpfCheckFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	// security context capabilities
	managers.SecurityContext().AddCapabilitiesToContainer(agent.DefaultCapabilitiesForSystemProbe(), apicommonv1.SystemProbeContainerName)

	// debugfs volume mount
	debugfsVol, debugfsVolMount := volume.GetVolumes(apicommon.DebugfsVolumeName, apicommon.DebugfsPath, apicommon.DebugfsPath, false)
	managers.Volume().AddVolume(&debugfsVol)
	managers.VolumeMount().AddVolumeMountToContainers(&debugfsVolMount, []apicommonv1.AgentContainerName{apicommonv1.SystemProbeContainerName})

	// socket volume mount (needs write perms for the system probe container but not the others)
	socketVol, socketVolMount := volume.GetVolumesEmptyDir(apicommon.SystemProbeSocketVolumeName, apicommon.SystemProbeSocketVolumePath, false)
	managers.Volume().AddVolume(&socketVol)
	managers.VolumeMount().AddVolumeMountToContainer(&socketVolMount, apicommonv1.SystemProbeContainerName)

	_, socketVolMountReadOnly := volume.GetVolumesEmptyDir(apicommon.SystemProbeSocketVolumeName, apicommon.SystemProbeSocketVolumePath, true)
	managers.VolumeMount().AddVolumeMountToContainer(&socketVolMountReadOnly, apicommonv1.CoreAgentContainerName)

	enableEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDEnableEBPFCheckEnvVar,
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

// ManageMonoContainerNodeAgent allows a feature to configure the mono-container Node Agent's corev1.PodTemplateSpec
// if mono-container usage is enabled and can be used with the current feature set
// It should do nothing if the feature doesn't need to configure it.
func (f *ebpfCheckFeature) ManageMonoContainerNodeAgent(managers feature.PodTemplateManagers) error {
	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *ebpfCheckFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
