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

	ddaGPUMonitoringEnabledAlternativeRuntimeClass := ddaGPUMonitoringEnabled.DeepCopy()
	ddaGPUMonitoringEnabledAlternativeRuntimeClass.Spec.Features.GPU.PodRuntimeClassName = apiutils.NewStringPointer(alternativeRuntimeClass)

	ddaGPUMonitoringEnabledANoRuntimeClass := ddaGPUMonitoringEnabled.DeepCopy()
	ddaGPUMonitoringEnabledANoRuntimeClass.Spec.Features.GPU.PodRuntimeClassName = apiutils.NewStringPointer("")

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
				Name:  DDEnableGPUMonitoringEnvVar,
				Value: "true",
			},
			{
				Name:  NVIDIAVisibleDevicesEnvVar,
				Value: "all",
			},
		}

		wantAgentEnvVars := append([]*corev1.EnvVar{
			{
				Name:  common.DDKubernetesPodResourcesSocket,
				Value: path.Join(podResourcesSocketPath, "kubelet.sock"),
			},
		}, wantSystemProbeEnvVars...)

		agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
		assert.ElementsMatch(t, agentEnvVars, wantAgentEnvVars)

		systemProbeEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(systemProbeEnvVars, wantSystemProbeEnvVars), "System Probe envvars \ndiff = %s", cmp.Diff(systemProbeEnvVars, wantSystemProbeEnvVars))

		// Check runtime class
		if expectedRuntimeClass == "" {
			assert.Nil(t, mgr.PodTemplateSpec().Spec.RuntimeClassName)
		} else {
			assert.Equal(t, expectedRuntimeClass, *mgr.PodTemplateSpec().Spec.RuntimeClassName)
		}
	}

	tests := test.FeatureTestSuite{
		{
			Name:          "gpu monitoring not enabled",
			DDA:           ddaGPUMonitoringDisabled.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "gpu monitoring enabled",
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
	}

	tests.Run(t, buildFeature)
}
