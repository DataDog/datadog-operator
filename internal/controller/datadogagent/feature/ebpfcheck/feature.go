package ebpfcheck

import (
	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
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
			Containers: []apicommon.AgentContainerName{apicommon.CoreAgentContainerName, apicommon.SystemProbeContainerName},
		}
	}

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
func (f *ebpfCheckFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	// security context capabilities
	managers.SecurityContext().AddCapabilitiesToContainer(agent.DefaultCapabilitiesForSystemProbe(), apicommon.SystemProbeContainerName)

	// debugfs volume mount
	debugfsVol, debugfsVolMount := volume.GetVolumes(apicommon.DebugfsVolumeName, apicommon.DebugfsPath, apicommon.DebugfsPath, false)
	managers.Volume().AddVolume(&debugfsVol)
	managers.VolumeMount().AddVolumeMountToContainers(&debugfsVolMount, []apicommon.AgentContainerName{apicommon.SystemProbeContainerName})

	// socket volume mount (needs write perms for the system probe container but not the others)
	socketVol, socketVolMount := volume.GetVolumesEmptyDir(apicommon.SystemProbeSocketVolumeName, apicommon.SystemProbeSocketVolumePath, false)
	managers.Volume().AddVolume(&socketVol)
	managers.VolumeMount().AddVolumeMountToContainer(&socketVolMount, apicommon.SystemProbeContainerName)

	_, socketVolMountReadOnly := volume.GetVolumesEmptyDir(apicommon.SystemProbeSocketVolumeName, apicommon.SystemProbeSocketVolumePath, true)
	managers.VolumeMount().AddVolumeMountToContainer(&socketVolMountReadOnly, apicommon.CoreAgentContainerName)

	// env vars
	enableEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDEnableEBPFCheckEnvVar,
		Value: "true",
	}

	managers.EnvVar().AddEnvVarToContainers([]apicommon.AgentContainerName{apicommon.CoreAgentContainerName, apicommon.SystemProbeContainerName}, enableEnvVar)
	managers.EnvVar().AddEnvVarToInitContainer(apicommon.InitConfigContainerName, enableEnvVar)

	socketEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDSystemProbeSocket,
		Value: apicommon.DefaultSystemProbeSocketPath,
	}

	managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, socketEnvVar)
	managers.EnvVar().AddEnvVarToContainer(apicommon.SystemProbeContainerName, socketEnvVar)

	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *ebpfCheckFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *ebpfCheckFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
