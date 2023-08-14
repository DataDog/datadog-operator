package ebpfcheck

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/test"
)

func Test_ebpfCheckFeature_Configure(t *testing.T) {
	ddav1EBPFCheckDisabled := v1alpha1.DatadogAgent{
		Spec: v1alpha1.DatadogAgentSpec{
			Agent: v1alpha1.DatadogAgentSpecAgentSpec{
				SystemProbe: &v1alpha1.SystemProbeSpec{
					Enabled: apiutils.NewBoolPointer(false),
				},
			},
		},
	}

	ddav1EBPFCheckEnabled := ddav1EBPFCheckDisabled.DeepCopy()
	{
		ddav1EBPFCheckEnabled.Spec.Agent.SystemProbe.Enabled = apiutils.NewBoolPointer(true)
		ddav1EBPFCheckEnabled.Spec.Agent.SystemProbe.Env = append(
			ddav1EBPFCheckEnabled.Spec.Agent.SystemProbe.Env,
			corev1.EnvVar{
				Name:  apicommon.DDEnableEBPFCheckEnvVar,
				Value: "true",
			},
		)
	}

	ddav2EBPFCheckDisabled := v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
			},
		},
	}
	ddav2EBPFCheckEnabled := ddav2EBPFCheckDisabled.DeepCopy()
	{
		ddav2EBPFCheckEnabled.Spec.Features.EBPFCheck.Enabled = apiutils.NewBoolPointer(true)
	}

	ebpfCheckAgentNodeWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)

		// check security context capabilities
		sysProbeCapabilities := mgr.SecurityContextMgr.CapabilitiesByC[apicommonv1.SystemProbeContainerName]
		assert.True(
			t,
			apiutils.IsEqualStruct(sysProbeCapabilities, agent.DefaultCapabilitiesForSystemProbe()),
			"System Probe security context capabilities \ndiff = %s",
			cmp.Diff(sysProbeCapabilities, agent.DefaultCapabilitiesForSystemProbe()),
		)

		// check volume mounts
		wantCoreAgentVolMounts := []corev1.VolumeMount{
			{
				Name:      apicommon.SystemProbeSocketVolumeName,
				MountPath: apicommon.SystemProbeSocketVolumePath,
				ReadOnly:  true,
			},
		}

		wantSystemProbeVolMounts := []corev1.VolumeMount{
			{
				Name:      apicommon.DebugfsVolumeName,
				MountPath: apicommon.DebugfsPath,
				ReadOnly:  false,
			},
			{
				Name:      apicommon.SystemProbeSocketVolumeName,
				MountPath: apicommon.SystemProbeSocketVolumePath,
				ReadOnly:  false,
			},
		}

		coreAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.CoreAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantCoreAgentVolMounts), "Core agent volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantCoreAgentVolMounts))

		systemProbeVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(systemProbeVolumeMounts, wantSystemProbeVolMounts), "System Probe volume mounts \ndiff = %s", cmp.Diff(systemProbeVolumeMounts, wantSystemProbeVolMounts))

		// check volumes
		wantVolumes := []corev1.Volume{
			{
				Name: apicommon.DebugfsVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: apicommon.DebugfsPath,
					},
				},
			},
			{
				Name: apicommon.SystemProbeSocketVolumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
		}

		volumes := mgr.VolumeMgr.Volumes
		assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))

		// check env vars
		wantEnvVars := []*corev1.EnvVar{
			{
				Name:  apicommon.DDEnableEBPFCheckEnvVar,
				Value: "true",
			},
			{
				Name:  apicommon.DDSystemProbeSocket,
				Value: apicommon.DefaultSystemProbeSocketPath,
			},
		}
		agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.CoreAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(agentEnvVars, wantEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, wantEnvVars))

		systemProbeEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(systemProbeEnvVars, wantEnvVars), "System Probe envvars \ndiff = %s", cmp.Diff(systemProbeEnvVars, wantEnvVars))
	}

	tests := test.FeatureTestSuite{
		///////////////////////////
		// v1alpha1.DatadogAgent //
		///////////////////////////
		{
			Name:          "v1alpha1 ebpf check not enabled",
			DDAv1:         ddav1EBPFCheckDisabled.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "v1alpha1 ebpf check enabled",
			DDAv1:         ddav1EBPFCheckEnabled,
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(ebpfCheckAgentNodeWantFunc),
		},
		///////////////////////////
		// v2alpha1.DatadogAgent //
		///////////////////////////
		{
			Name:          "v2alpha1 ebpf check not enabled",
			DDAv2:         ddav2EBPFCheckDisabled.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "v2alpha1 ebpf check enabled",
			DDAv2:         ddav2EBPFCheckEnabled,
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(ebpfCheckAgentNodeWantFunc),
		},
	}

	tests.Run(t, buildEBPFCheckFeature)
}
