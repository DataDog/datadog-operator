package gpu

import (
	"errors"
	"path"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
	podRuntimeClassName     string
	podResourcesSocketPath  string
	isPrivilegedModeEnabled bool
	patchCgroupPermissions  bool
}

// ID returns the ID of the Feature
func (f *gpuFeature) ID() feature.IDType {
	return feature.GPUIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *gpuFeature) Configure(_ metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) (reqComp feature.RequiredComponents) {
	if ddaSpec.Features == nil || ddaSpec.Features.GPU == nil || !apiutils.BoolValue(ddaSpec.Features.GPU.Enabled) {
		return reqComp
	}

	f.isPrivilegedModeEnabled = apiutils.BoolValue(ddaSpec.Features.GPU.PrivilegedMode)
	f.patchCgroupPermissions = apiutils.BoolValue(ddaSpec.Features.GPU.PatchCgroupPermissions)

	requiredContainers := []apicommon.AgentContainerName{apicommon.CoreAgentContainerName}
	if f.isPrivilegedModeEnabled {
		requiredContainers = append(requiredContainers, apicommon.SystemProbeContainerName)
	}

	reqComp.Agent = feature.RequiredComponent{
		IsRequired: apiutils.NewBoolPointer(true),
		Containers: requiredContainers,
	}

	if ddaSpec.Features.GPU.PodRuntimeClassName == nil {
		// Configuration option not set, so revert to the default
		f.podRuntimeClassName = defaultGPURuntimeClass
	} else {
		// Configuration option set, use the value. Note that here the value might be an empty
		// string, which tells us to not change the runtime class.
		f.podRuntimeClassName = *ddaSpec.Features.GPU.PodRuntimeClassName
	}

	f.podResourcesSocketPath = ddaSpec.Global.Kubelet.PodResourcesSocketPath

	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *gpuFeature) ManageDependencies(managers feature.ResourceManagers, provider string) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *gpuFeature) ManageClusterAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

func configureSystemProbe(managers feature.PodTemplateManagers) {
	// env var to enable the GPU probe module in system-probe
	enableSPEnvVar := &corev1.EnvVar{
		Name:  DDEnableGPUProbeEnvVar,
		Value: "true",
	}

	// enable gpu_monitoring module
	managers.EnvVar().AddEnvVarToContainer(apicommon.SystemProbeContainerName, enableSPEnvVar)

	// add the env var to the core agent as well, to prevent config mismatches in runtime
	managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, enableSPEnvVar)

	// annotations
	managers.Annotation().AddAnnotation(common.SystemProbeAppArmorAnnotationKey, common.SystemProbeAppArmorAnnotationValue)

	// security context capabilities
	managers.SecurityContext().AddCapabilitiesToContainer(agent.DefaultCapabilitiesForSystemProbe(), apicommon.SystemProbeContainerName)

	// We need CAP_MKNOD in both containers to be able to create the nvidia devices nodes if the driver is not loaded when the container starts
	managers.SecurityContext().AddCapabilitiesToContainer([]corev1.Capability{"MKNOD"}, apicommon.SystemProbeContainerName)
	managers.SecurityContext().AddCapabilitiesToContainer([]corev1.Capability{"MKNOD"}, apicommon.CoreAgentContainerName)

	// Some nvidia-container-runtime setups ignore the NVIDIA_VISIBLE_DEVICES
	// env variable. This is usually configured with the options
	//   accept-nvidia-visible-devices-envvar-when-unprivileged = true
	//   accept-nvidia-visible-devices-as-volume-mounts = true
	// in the NVIDIA container runtime config. In this case, we need to mount the
	// /var/run/nvidia-container-devices/all directory into the container, so that
	// the nvidia-container-runtime can see that we want to use all GPUs.
	nvidiaDevicesMount := &corev1.VolumeMount{
		Name:      nvidiaDevicesVolumeName,
		MountPath: nvidiaDevicesMountPath,
		ReadOnly:  true,
	}

	managers.VolumeMount().AddVolumeMountToContainer(nvidiaDevicesMount, apicommon.SystemProbeContainerName)

	// socket volume mount (needs write perms for the system probe container but not the others)
	procdirVol, procdirMount := volume.GetVolumes(common.ProcdirVolumeName, common.ProcdirHostPath, common.ProcdirMountPath, true)
	managers.VolumeMount().AddVolumeMountToContainer(&procdirMount, apicommon.SystemProbeContainerName)
	managers.Volume().AddVolume(&procdirVol)

	cgroupsVol, cgroupsMount := volume.GetVolumes(common.CgroupsVolumeName, common.CgroupsHostPath, common.CgroupsMountPath, true)
	managers.VolumeMount().AddVolumeMountToContainer(&cgroupsMount, apicommon.SystemProbeContainerName)
	managers.Volume().AddVolume(&cgroupsVol)

	debugfsVol, debugfsMount := volume.GetVolumes(common.DebugfsVolumeName, common.DebugfsPath, common.DebugfsPath, false)
	managers.VolumeMount().AddVolumeMountToContainer(&debugfsMount, apicommon.SystemProbeContainerName)
	managers.Volume().AddVolume(&debugfsVol)

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

	// Now we need to add the NVIDIA_VISIBLE_DEVICES env var to both agents again so
	// that the nvidia runtime can expose the GPU devices in the container
	nvidiaVisibleDevicesEnvVar := &corev1.EnvVar{
		Name:  NVIDIAVisibleDevicesEnvVar,
		Value: "all",
	}

	managers.EnvVar().AddEnvVarToContainer(apicommon.SystemProbeContainerName, nvidiaVisibleDevicesEnvVar)
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

func (f *gpuFeature) configureCgroupPermissions(managers feature.PodTemplateManagers) error {
	if !f.isPrivilegedModeEnabled {
		return errors.New("patchCgroupPermissions is only supported in privileged mode")
	}

	managers.EnvVar().AddEnvVarToContainer(apicommon.SystemProbeContainerName, &corev1.EnvVar{
		Name:  DDPatchCgroupPermissionsEnvVar,
		Value: "true",
	})

	hostRunVol, hostRunMount := volume.GetVolumes(common.HostRunVolumeName, common.HostRunPath, common.HostRunMountPath, false)
	managers.VolumeMount().AddVolumeMountToContainer(&hostRunMount, apicommon.SystemProbeContainerName)
	managers.Volume().AddVolume(&hostRunVol)

	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *gpuFeature) ManageNodeAgent(managers feature.PodTemplateManagers, _ string) error {
	// env var to enable the GPU core check
	enableCoreCheckEnvVar := &corev1.EnvVar{
		Name:  DDEnableGPUMonitoringCheckEnvVar,
		Value: "true",
	}

	// DEPRECATED: env var to enable NVML detection, so that workloadmeta features depending on it
	// can be enabled
	nvmlDetectionEnvVar := &corev1.EnvVar{
		Name:  DDEnableNVMLDetectionEnvVar,
		Value: "true",
	}
	managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, nvmlDetectionEnvVar)

	// in the core agent
	managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, enableCoreCheckEnvVar)

	f.configurePodResourcesSocket(managers)

	if f.isPrivilegedModeEnabled {
		configureSystemProbe(managers)
	}

	if f.patchCgroupPermissions {
		if err := f.configureCgroupPermissions(managers); err != nil {
			return err
		}
	}

	// The agent check does not need to be manually enabled, the init config container will
	// check if GPU monitoring is enabled and will enable the check automatically (see
	// Dockerfiles/agent/cont-init.d/60-sysprobe-check.sh in the datadog-agent repo).
	managers.EnvVar().AddEnvVarToInitContainer(apicommon.InitConfigContainerName, enableCoreCheckEnvVar)

	// Now we need to add the NVIDIA_VISIBLE_DEVICES env var to both agents again so
	// that the nvidia runtime can expose the GPU devices in the container
	nvidiaVisibleDevicesEnvVar := &corev1.EnvVar{
		Name:  NVIDIAVisibleDevicesEnvVar,
		Value: "all",
	}

	managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, nvidiaVisibleDevicesEnvVar)

	// Some nvidia-container-runtime setups ignore the NVIDIA_VISIBLE_DEVICES
	// env variable. This is usually configured with the options
	//   accept-nvidia-visible-devices-envvar-when-unprivileged = true
	//   accept-nvidia-visible-devices-as-volume-mounts = true
	// in the NVIDIA container runtime config. In this case, we need to mount the
	// /var/run/nvidia-container-devices/all directory into the container, so that
	// the nvidia-container-runtime can see that we want to use all GPUs.
	devicesVol, devicesMount := volume.GetVolumes(nvidiaDevicesVolumeName, devNullPath, nvidiaDevicesMountPath, true)
	managers.Volume().AddVolume(&devicesVol)
	managers.VolumeMount().AddVolumeMountToContainer(&devicesMount, apicommon.CoreAgentContainerName)

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
func (f *gpuFeature) ManageClusterChecksRunner(feature.PodTemplateManagers, string) error {
	return nil
}

func (f *gpuFeature) ManageOtelAgentGateway(managers feature.PodTemplateManagers, provider string) error {
	return nil
}
