package gpu

import (
	"path"

	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
)

func init() {
	if err := feature.Register(feature.GPUIDType, buildFeature); err != nil {
		panic(err)
	}
}

func buildFeature(*feature.Options) feature.Feature {
	return &gpuFeature{}
}

type gpuFeature struct {
	// podRuntimeClassName is the value to set in the runtimeClassName
	// configuration of the agent pod. If this is empty, the runtimeClassName
	// will not be changed.
	podRuntimeClassName string
	podResourcesSocketPath string
}

// ID returns the ID of the Feature
func (f *gpuFeature) ID() feature.IDType {
	return feature.GPUIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *gpuFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	if dda.Spec.Features == nil || dda.Spec.Features.GPU == nil || !apiutils.BoolValue(dda.Spec.Features.GPU.Enabled) {
		return reqComp
	}

	reqComp.Agent = feature.RequiredComponent{
		IsRequired: apiutils.NewBoolPointer(true),
		Containers: []apicommon.AgentContainerName{apicommon.CoreAgentContainerName, apicommon.SystemProbeContainerName},
	}

	if dda.Spec.Features.GPU.PodRuntimeClassName == nil {
		// Configuration option not set, so revert to the default
		f.podRuntimeClassName = defaultGPURuntimeClass
	} else {
		// Configuration option set, use the value. Note that here the value might be an empty
		// string, which tells us to not change the runtime class.
		f.podRuntimeClassName = *dda.Spec.Features.GPU.PodRuntimeClassName
	}

	f.podResourcesSocketPath = dda.Spec.Global.Kubelet.PodResourcesSocketPath

	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *gpuFeature) ManageDependencies(feature.ResourceManagers, feature.RequiredComponents) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *gpuFeature) ManageClusterAgent(feature.PodTemplateManagers) error {
	return nil
}

func configureSystemProbe(managers feature.PodTemplateManagers) {
	// annotations
	managers.Annotation().AddAnnotation(common.SystemProbeAppArmorAnnotationKey, common.SystemProbeAppArmorAnnotationValue)

	// security context capabilities
	managers.SecurityContext().AddCapabilitiesToContainer(agent.DefaultCapabilitiesForSystemProbe(), apicommon.SystemProbeContainerName)

	// socket volume mount (needs write perms for the system probe container but not the others)
	procdirVol, procdirMount := volume.GetVolumes(common.ProcdirVolumeName, common.ProcdirHostPath, common.ProcdirMountPath, true)
	managers.VolumeMount().AddVolumeMountToContainer(&procdirMount, apicommon.SystemProbeContainerName)
	managers.Volume().AddVolume(&procdirVol)

	cgroupsVol, cgroupsMount := volume.GetVolumes(common.CgroupsVolumeName, common.CgroupsHostPath, common.CgroupsMountPath, true)
	managers.VolumeMount().AddVolumeMountToContainer(&cgroupsMount, apicommon.SystemProbeContainerName)
	managers.Volume().AddVolume(&cgroupsVol)

	socketVol, socketVolMount := volume.GetVolumesEmptyDir(common.SystemProbeSocketVolumeName, common.SystemProbeSocketVolumePath, false)
	managers.Volume().AddVolume(&socketVol)
	managers.VolumeMount().AddVolumeMountToContainer(&socketVolMount, apicommon.SystemProbeContainerName)

	_, socketVolMountReadOnly := volume.GetVolumesEmptyDir(common.SystemProbeSocketVolumeName, common.SystemProbeSocketVolumePath, true)
	managers.VolumeMount().AddVolumeMountToContainer(&socketVolMountReadOnly, apicommon.CoreAgentContainerName)

	socketEnvVar := &corev1.EnvVar{
		Name:  common.DDSystemProbeSocket,
		Value: common.DefaultSystemProbeSocketPath,
	}

	managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, socketEnvVar)
	managers.EnvVar().AddEnvVarToContainer(apicommon.SystemProbeContainerName, socketEnvVar)
}

func (f *gpuFeature) configurePodResourcesSocket(managers feature.PodTemplateManagers) {
	if f.podResourcesSocketPath == "" {
		return
	}
	managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, &corev1.EnvVar{
		Name:  common.DDKubernetesPodResourcesSocket,
		Value: path.Join(f.podResourcesSocketPath, "kubelet.sock"),
	})

	podResourcesVol, podResourcesMount := volume.GetVolumes(common.KubeletPodResourcesVolumeName, f.podResourcesSocketPath, f.podResourcesSocketPath, false)
	managers.VolumeMount().AddVolumeMountToContainer(
		&podResourcesMount,
		apicommon.CoreAgentContainerName,
	)
	managers.Volume().AddVolume(&podResourcesVol)
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *gpuFeature) ManageNodeAgent(managers feature.PodTemplateManagers, _ string) error {
	configureSystemProbe(managers)
	f.configurePodResourcesSocket(managers)

	// env var to enable the GPU module
	enableEnvVar := &corev1.EnvVar{
		Name:  DDEnableGPUMonitoringEnvVar,
		Value: "true",
	}

	// Both in the core agent and the system probe
	managers.EnvVar().AddEnvVarToContainers([]apicommon.AgentContainerName{apicommon.CoreAgentContainerName, apicommon.SystemProbeContainerName}, enableEnvVar)

	// The agent check does not need to be manually enabled, the init config container will
	// check if GPU monitoring is enabled and will enable the check automatically (see
	// Dockerfiles/agent/cont-init.d/60-sysprobe-check.sh in the datadog-agent repo).
	managers.EnvVar().AddEnvVarToInitContainer(apicommon.InitConfigContainerName, enableEnvVar)

	// Now we need to add the NVIDIA_VISIBLE_DEVICES env var to both agents again so
	// that the nvidia runtime can expose the GPU devices in the container
	nvidiaVisibleDevicesEnvVar := &corev1.EnvVar{
		Name:  NVIDIAVisibleDevicesEnvVar,
		Value: "all",
	}

	managers.EnvVar().AddEnvVarToContainers([]apicommon.AgentContainerName{apicommon.CoreAgentContainerName, apicommon.SystemProbeContainerName}, nvidiaVisibleDevicesEnvVar)

	// Some nvidia-container-runtime setups ignore the NVIDIA_VISIBLE_DEVICES
	// env variable. This is usually configured with the options
	//   accept-nvidia-visible-devices-envvar-when-unprivileged = true
	//   accept-nvidia-visible-devices-as-volume-mounts = true
	// in the NVIDIA container runtime config. In this case, we need to mount the
	// /var/run/nvidia-container-devices/all directory into the container, so that
	// the nvidia-container-runtime can see that we want to use all GPUs.
	devicesVol, devicesMount := volume.GetVolumes(nvidiaDevicesVolumeName, devNullPath, nvidiaDevicesMountPath, true)
	managers.Volume().AddVolume(&devicesVol)
	managers.VolumeMount().AddVolumeMountToContainers(&devicesMount, []apicommon.AgentContainerName{apicommon.CoreAgentContainerName, apicommon.SystemProbeContainerName})

	// Configure the runtime class for the pod
	if f.podRuntimeClassName != "" {
		managers.PodTemplateSpec().Spec.RuntimeClassName = &f.podRuntimeClassName
	}

	// Note: we don't need to mount the NVML library, as it's mounted
	// automatically by the nvidia-container-runtime. However, if needed, we
	// could add a config option for that and mount that in the agent and
	// system-probe folders, and then set the correct configuration option so
	// that the binaries can find the library.

	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *gpuFeature) ManageSingleContainerNodeAgent(feature.PodTemplateManagers, string) error {
	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *gpuFeature) ManageClusterChecksRunner(feature.PodTemplateManagers) error {
	return nil
}
