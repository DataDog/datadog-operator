package gpu

import (
	"path"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
)

const alternativeRuntimeClass = "nvidia-like"

func Test_GPUMonitoringFeature_Configure(t *testing.T) {
	podResourcesSocketPath := "/var/lib/kubelet/pod-resources/"

	ddaGPUMonitoringDisabled := v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				GPU: &v2alpha1.GPUFeatureConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
			},
			Global: &v2alpha1.GlobalConfig{
				Kubelet: &v2alpha1.KubeletConfig{
					PodResourcesSocketPath: podResourcesSocketPath,
				},
			},
		},
	}
	ddaGPUMonitoringEnabled := ddaGPUMonitoringDisabled.DeepCopy()
	ddaGPUMonitoringEnabled.Spec.Features.GPU.Enabled = apiutils.NewBoolPointer(true)
	ddaGPUMonitoringEnabled.Spec.Features.GPU.PrivilegedMode = apiutils.NewBoolPointer(true)

	ddaGPUMonitoringEnabledAlternativeRuntimeClass := ddaGPUMonitoringEnabled.DeepCopy()
	ddaGPUMonitoringEnabledAlternativeRuntimeClass.Spec.Features.GPU.PodRuntimeClassName = apiutils.NewStringPointer(alternativeRuntimeClass)

	ddaGPUMonitoringEnabledANoRuntimeClass := ddaGPUMonitoringEnabled.DeepCopy()
	ddaGPUMonitoringEnabledANoRuntimeClass.Spec.Features.GPU.PodRuntimeClassName = apiutils.NewStringPointer("")

	ddaGPUCoreCheckOnly := ddaGPUMonitoringDisabled.DeepCopy()
	ddaGPUCoreCheckOnly.Spec.Features.GPU.Enabled = apiutils.NewBoolPointer(true)
	ddaGPUCoreCheckOnly.Spec.Features.GPU.PrivilegedMode = apiutils.NewBoolPointer(false)

	ddaGPUInvalidConfig := ddaGPUMonitoringDisabled.DeepCopy()
	ddaGPUInvalidConfig.Spec.Features.GPU.Enabled = apiutils.NewBoolPointer(false)
	ddaGPUInvalidConfig.Spec.Features.GPU.PrivilegedMode = apiutils.NewBoolPointer(true)

	ddaGPUCgroupPermissionsEnabled := ddaGPUMonitoringEnabled.DeepCopy()
	ddaGPUCgroupPermissionsEnabled.Spec.Features.GPU.PatchCgroupPermissions = apiutils.NewBoolPointer(true)

	GPUMonitoringAgentNodeWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers, expectedRuntimeClass string) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)

		// check security context capabilities
		sysProbeCapabilities := mgr.SecurityContextMgr.CapabilitiesByC[apicommon.SystemProbeContainerName]
		assert.True(
			t,
			apiutils.IsEqualStruct(sysProbeCapabilities, agent.DefaultCapabilitiesForSystemProbe()),
			"System Probe security context capabilities \ndiff = %s",
			cmp.Diff(sysProbeCapabilities, agent.DefaultCapabilitiesForSystemProbe()),
		)

		// check volume mounts
		wantCoreAgentVolMounts := []*corev1.VolumeMount{
			{
				Name:      common.SystemProbeSocketVolumeName,
				MountPath: common.SystemProbeSocketVolumePath,
				ReadOnly:  true,
			},
			{
				Name:      nvidiaDevicesVolumeName,
				MountPath: nvidiaDevicesMountPath,
				ReadOnly:  true,
			},
			{
				Name:      common.KubeletPodResourcesVolumeName,
				MountPath: podResourcesSocketPath,
				ReadOnly:  false,
			},
		}

		wantSystemProbeVolMounts := []*corev1.VolumeMount{
			{
				Name:      common.ProcdirVolumeName,
				MountPath: common.ProcdirMountPath,
				ReadOnly:  true,
			},
			{
				Name:      common.CgroupsVolumeName,
				MountPath: common.CgroupsMountPath,
				ReadOnly:  true,
			},
			{
				Name:      common.DebugfsVolumeName,
				MountPath: common.DebugfsPath,
				ReadOnly:  false,
			},
			{
				Name:      common.SystemProbeSocketVolumeName,
				MountPath: common.SystemProbeSocketVolumePath,
				ReadOnly:  false,
			},
			{
				Name:      nvidiaDevicesVolumeName,
				MountPath: nvidiaDevicesMountPath,
				ReadOnly:  true,
			},
		}

		coreAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.CoreAgentContainerName]
		assert.ElementsMatch(t, coreAgentVolumeMounts, wantCoreAgentVolMounts)

		systemProbeVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.SystemProbeContainerName]
		assert.ElementsMatch(t, systemProbeVolumeMounts, wantSystemProbeVolMounts)

		// check volumes
		wantVolumes := []*corev1.Volume{
			{
				Name: common.ProcdirVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: common.ProcdirHostPath,
					},
				},
			},
			{
				Name: common.CgroupsVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: common.CgroupsHostPath,
					},
				},
			},
			{
				Name: common.DebugfsVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: common.DebugfsPath,
					},
				},
			},
			{
				Name: common.SystemProbeSocketVolumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
			{
				Name: nvidiaDevicesVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: devNullPath,
					},
				},
			},
			{
				Name: common.KubeletPodResourcesVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{Path: podResourcesSocketPath},
				},
			},
		}

		volumes := mgr.VolumeMgr.Volumes
		assert.ElementsMatch(t, volumes, wantVolumes)

		// check env vars
		wantSystemProbeEnvVars := []*corev1.EnvVar{
			{
				Name:  common.DDSystemProbeSocket,
				Value: common.DefaultSystemProbeSocketPath,
			},
			{
				Name:  DDEnableGPUProbeEnvVar,
				Value: "true",
			},
			{
				Name:  NVIDIAVisibleDevicesEnvVar,
				Value: "all",
			},
		}

		wantAgentEnvVars := []*corev1.EnvVar{
			{
				Name:  common.DDKubernetesPodResourcesSocket,
				Value: path.Join(podResourcesSocketPath, "kubelet.sock"),
			},
			{
				Name:  DDEnableNVMLDetectionEnvVar,
				Value: "true",
			},
			{
				Name:  DDEnableGPUMonitoringCheckEnvVar,
				Value: "true",
			},
			{
				Name:  DDEnableGPUProbeEnvVar,
				Value: "true",
			},
			{
				Name:  common.DDSystemProbeSocket,
				Value: common.DefaultSystemProbeSocketPath,
			},
			{
				Name:  NVIDIAVisibleDevicesEnvVar,
				Value: "all",
			},
		}

		agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
		assert.ElementsMatch(t, agentEnvVars, wantAgentEnvVars)

		systemProbeEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.SystemProbeContainerName]
		assert.ElementsMatch(t, systemProbeEnvVars, wantSystemProbeEnvVars)

		// Check runtime class
		if expectedRuntimeClass == "" {
			assert.Nil(t, mgr.PodTemplateSpec().Spec.RuntimeClassName)
		} else {
			assert.Equal(t, expectedRuntimeClass, *mgr.PodTemplateSpec().Spec.RuntimeClassName)
		}
	}

	GPUCoreCheckOnlyWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers, expectedRuntimeClass string) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)

		// check that system probe capabilities are NOT set for core-check only
		sysProbeCapabilities := mgr.SecurityContextMgr.CapabilitiesByC[apicommon.SystemProbeContainerName]
		assert.Nil(t, sysProbeCapabilities, "System Probe should not have capabilities when privileged mode is disabled")

		// check volume mounts - core agent should only have nvidia devices and kubelet socket
		wantCoreAgentVolMounts := []*corev1.VolumeMount{
			{
				Name:      nvidiaDevicesVolumeName,
				MountPath: nvidiaDevicesMountPath,
				ReadOnly:  true,
			},
			{
				Name:      common.KubeletPodResourcesVolumeName,
				MountPath: podResourcesSocketPath,
				ReadOnly:  false,
			},
		}

		coreAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.CoreAgentContainerName]
		assert.ElementsMatch(t, coreAgentVolumeMounts, wantCoreAgentVolMounts)

		// check that system probe has NO volume mounts for core-check only
		systemProbeVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.SystemProbeContainerName]
		assert.Empty(t, systemProbeVolumeMounts, "System Probe should not have volume mounts when privileged mode is disabled")

		// check volumes - should only have nvidia devices and kubelet socket
		wantVolumes := []*corev1.Volume{
			{
				Name: nvidiaDevicesVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: devNullPath,
					},
				},
			},
			{
				Name: common.KubeletPodResourcesVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{Path: podResourcesSocketPath},
				},
			},
		}

		volumes := mgr.VolumeMgr.Volumes
		assert.ElementsMatch(t, volumes, wantVolumes)

		// check env vars - core agent should have GPU monitoring check enabled
		wantAgentEnvVars := []*corev1.EnvVar{
			{
				Name:  common.DDKubernetesPodResourcesSocket,
				Value: path.Join(podResourcesSocketPath, "kubelet.sock"),
			},
			{
				Name:  DDEnableNVMLDetectionEnvVar,
				Value: "true",
			},
			{
				Name:  DDEnableGPUMonitoringCheckEnvVar,
				Value: "true",
			},
			{
				Name:  NVIDIAVisibleDevicesEnvVar,
				Value: "all",
			},
		}

		agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
		assert.ElementsMatch(t, agentEnvVars, wantAgentEnvVars)

		// check that system probe has NO env vars for core-check only
		systemProbeEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.SystemProbeContainerName]
		assert.Empty(t, systemProbeEnvVars, "System Probe should not have env vars when privileged mode is disabled")

		// Check runtime class
		if expectedRuntimeClass == "" {
			assert.Nil(t, mgr.PodTemplateSpec().Spec.RuntimeClassName)
		} else {
			assert.Equal(t, expectedRuntimeClass, *mgr.PodTemplateSpec().Spec.RuntimeClassName)
		}
	}

	GPUMonitoringWithCgroupPermissionsWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers, expectedRuntimeClass string) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)

		// Check that cgroup permission patching environment variable is set
		systemProbeEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.SystemProbeContainerName]
		cgroupPermsEnvVar := &corev1.EnvVar{
			Name:  "DD_GPU_MONITORING_CONFIGURE_CGROUP_PERMS",
			Value: "true",
		}
		assert.Contains(t, systemProbeEnvVars, cgroupPermsEnvVar, "System Probe should have cgroup permissions environment variable")

		// Check that host run volume is mounted
		hostRunVolume := &corev1.Volume{
			Name: common.HostRunVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: common.HostRunPath,
				},
			},
		}
		assert.Contains(t, mgr.VolumeMgr.Volumes, hostRunVolume, "Should have host run volume")

		// Check that host run volume is mounted in system probe
		hostRunVolumeMount := &corev1.VolumeMount{
			Name:      common.HostRunVolumeName,
			MountPath: common.HostRunMountPath,
			ReadOnly:  false,
		}
		systemProbeVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.SystemProbeContainerName]
		assert.Contains(t, systemProbeVolumeMounts, hostRunVolumeMount, "System Probe should have host run volume mount")
	}

	tests := test.FeatureTestSuite{
		{
			Name:          "gpu monitoring not enabled",
			DDA:           ddaGPUMonitoringDisabled.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "gpu core-check only (no privileged mode)",
			DDA:           ddaGPUCoreCheckOnly,
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
				GPUCoreCheckOnlyWantFunc(t, mgrInterface, defaultGPURuntimeClass)
			}),
		},
		{
			Name:          "gpu monitoring enabled with privileged mode",
			DDA:           ddaGPUMonitoringEnabled,
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
				GPUMonitoringAgentNodeWantFunc(t, mgrInterface, defaultGPURuntimeClass)
			}),
		},
		{
			Name:          "gpu monitoring enabled, alternative runtime class",
			DDA:           ddaGPUMonitoringEnabledAlternativeRuntimeClass,
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
				GPUMonitoringAgentNodeWantFunc(t, mgrInterface, alternativeRuntimeClass)
			}),
		},

		{
			Name:          "gpu monitoring enabled, no runtime class",
			DDA:           ddaGPUMonitoringEnabledANoRuntimeClass,
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
				GPUMonitoringAgentNodeWantFunc(t, mgrInterface, "")
			}),
		},
		{
			Name:          "gpu invalid config (privileged mode without core-check)",
			DDA:           ddaGPUInvalidConfig,
			WantConfigure: false,
		},
		{
			Name:          "gpu cgroup permissions enabled",
			DDA:           ddaGPUCgroupPermissionsEnabled,
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
				GPUMonitoringWithCgroupPermissionsWantFunc(t, mgrInterface, defaultGPURuntimeClass)
			}),
		},
	}

	tests.Run(t, buildFeature)
}
