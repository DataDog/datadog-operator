// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package servicediscovery

import (
	"testing"

	"k8s.io/utils/ptr"

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

	// Blank imports trigger the init() of these packages, registering them in the
	// feature factory. Tests that check SPL is suppressed when these features are
	// active require them to be registered.
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/cws"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/npm"
)

func Test_serviceDiscoveryFeature_Configure(t *testing.T) {
	ddaServiceDiscoveryDisabled := v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
					Enabled: ptr.To(false),
				},
			},
		},
	}
	ddaServiceDiscoveryEnabled := ddaServiceDiscoveryDisabled.DeepCopy()
	ddaServiceDiscoveryEnabled.Spec.Features.ServiceDiscovery.Enabled = ptr.To(true)

	ddaWithNPM := v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
					Enabled: ptr.To(true),
				},
				NPM: &v2alpha1.NPMFeatureConfig{
					Enabled: ptr.To(true),
				},
			},
		},
	}

	ddaWithCWS := v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
					Enabled: ptr.To(true),
				},
				CWS: &v2alpha1.CWSFeatureConfig{
					Enabled: ptr.To(true),
				},
			},
		},
	}

	ddaEnabledByDefault := v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
					EnabledByDefault: ptr.To(true),
				},
			},
		},
	}

	tests := test.FeatureTestSuite{
		{
			Name:          "service discovery not enabled",
			DDA:           ddaServiceDiscoveryDisabled.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "service discovery enabled",
			DDA:           ddaServiceDiscoveryEnabled,
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().
				WithCreateFunc(createFuncWithSystemProbeContainer()).
				WithWantFunc(getWantFunc(true, true)),
		},
		{
			Name:          "system-probe-lite enabled by default - no system-probe fallback",
			DDA:           &ddaEnabledByDefault,
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().
				WithCreateFunc(createFuncWithSystemProbeContainer()).
				WithWantFunc(getWantFunc(true, false)),
		},
	}

	tests.Run(t, buildFeature)

	// These cases involve multiple registered features (npm, cws are imported via blank imports
	// so they register in the factory). FeatureTestSuite is designed for one feature at a time,
	// so we test service discovery's ManageNodeAgent output directly here.
	for _, tc := range []struct {
		name string
		dda  *v2alpha1.DatadogAgent
	}{
		{"system-probe-lite not used when NPM also enabled", &ddaWithNPM},
		{"system-probe-lite not used when CWS also enabled", &ddaWithCWS},
	} {
		t.Run(tc.name, func(t *testing.T) {
			feat := buildFeature(nil)
			reqComp := feat.Configure(tc.dda, &tc.dda.Spec, tc.dda.Status.RemoteConfigConfiguration)
			assert.True(t, reqComp.IsEnabled())

			tplManager, provider := createFuncWithSystemProbeContainer()(t)
			assert.NoError(t, feat.ManageNodeAgent(tplManager, provider))

			getWantFunc(false, true)(t, tplManager)
		})
	}
}

func createFuncWithSystemProbeContainer() func(testing.TB) (feature.PodTemplateManagers, string) {
	return func(t testing.TB) (feature.PodTemplateManagers, string) {
		newPTS := corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: string(apicommon.CoreAgentContainerName),
					},
					{
						Name: string(apicommon.SystemProbeContainerName),
					},
				},
			},
		}
		return fake.NewPodTemplateManagers(t, newPTS), ""
	}
}

func getWantFunc(useSPL bool, userOptedIn bool) func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	return func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
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
		wantCoreAgentVolMounts := []corev1.VolumeMount{
			{
				Name:      common.SystemProbeSocketVolumeName,
				MountPath: common.SystemProbeSocketVolumePath,
				ReadOnly:  true,
			},
		}

		wantSystemProbeVolMounts := []corev1.VolumeMount{
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
		}

		coreAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.CoreAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, wantCoreAgentVolMounts), "Core agent volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, wantCoreAgentVolMounts))

		systemProbeVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(systemProbeVolumeMounts, wantSystemProbeVolMounts), "System Probe volume mounts \ndiff = %s", cmp.Diff(systemProbeVolumeMounts, wantSystemProbeVolMounts))

		// check volumes
		wantVolumes := []corev1.Volume{
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
		}

		volumes := mgr.VolumeMgr.Volumes
		assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))

		// check env vars
		wantAgentEnvVars := []*corev1.EnvVar{
			{
				Name:  DDServiceDiscoveryEnabled,
				Value: "true",
			},
			{
				Name:  common.DDSystemProbeSocket,
				Value: common.DefaultSystemProbeSocketPath,
			},
		}

		wantSPEnvVars := []*corev1.EnvVar{
			{
				Name:  DDServiceDiscoveryEnabled,
				Value: "true",
			},
			{
				Name:  common.DDSystemProbeSocket,
				Value: common.DefaultSystemProbeSocketPath,
			},
		}

		agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(agentEnvVars, wantAgentEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, wantAgentEnvVars))

		systemProbeEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(systemProbeEnvVars, wantSPEnvVars), "System Probe envvars \ndiff = %s", cmp.Diff(systemProbeEnvVars, wantSPEnvVars))

		// check system-probe container command override
		for _, c := range mgr.PodTemplateSpec().Spec.Containers {
			if c.Name == string(apicommon.SystemProbeContainerName) {
				if useSPL {
					assert.Equal(t, []string{"/bin/sh", "-c"}, c.Command, "System Probe command should be overridden for system-probe-lite")
					assert.Equal(t, []string{systemProbeLiteCommand(common.DefaultSystemProbeSocketPath, userOptedIn)}, c.Args, "System Probe args mismatch")
				} else {
					assert.Empty(t, c.Command, "System Probe command should not be overridden")
					assert.Empty(t, c.Args, "System Probe args should not be overridden")
				}
				break
			}
		}
	}
}
