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
	"github.com/DataDog/datadog-operator/pkg/images"
	pkgutils "github.com/DataDog/datadog-operator/pkg/utils"
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
			Agent:         test.NewDefaultComponentTest().WithWantFunc(getWantFunc()),
		},
	}

	tests.Run(t, buildFeature)
}

func Test_serviceDiscoveryFeature_Configure_DefaultingByVersion(t *testing.T) {
	tests := []struct {
		name    string
		ddaSpec *v2alpha1.DatadogAgentSpec
		want    bool
	}{
		{
			name:    "features nil stays disabled without mutation",
			ddaSpec: &v2alpha1.DatadogAgentSpec{},
			want:    false,
		},
		{
			name: "omitted on inherited default agent version follows current default image policy",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{},
			},
			want: serviceDiscoveryEnabledForVersion(images.AgentLatestVersion),
		},
		{
			name: "omitted on supported node agent version is auto-enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{},
				Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
					v2alpha1.NodeAgentComponentName: {
						Image: &v2alpha1.AgentImageConfig{Tag: "7.78.0"},
					},
				},
			},
			want: true,
		},
		{
			name: "omitted on supported full image ref is auto-enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{},
				Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
					v2alpha1.NodeAgentComponentName: {
						Image: &v2alpha1.AgentImageConfig{Name: "docker.io/datadog/agent:7.78.0"},
					},
				},
			},
			want: true,
		},
		{
			name: "omitted on unsupported node agent version stays disabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{},
				Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
					v2alpha1.NodeAgentComponentName: {
						Image: &v2alpha1.AgentImageConfig{Tag: "7.77.2"},
					},
				},
			},
			want: false,
		},
		{
			name: "omitted on partial image override without version follows inherited default image policy",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{},
				Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
					v2alpha1.NodeAgentComponentName: {
						Image: &v2alpha1.AgentImageConfig{PullPolicy: ptr.To(corev1.PullAlways)},
					},
				},
			},
			want: serviceDiscoveryEnabledForVersion(images.AgentLatestVersion),
		},
		{
			name: "omitted on unparseable node agent version is auto-enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{},
				Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
					v2alpha1.NodeAgentComponentName: {
						Image: &v2alpha1.AgentImageConfig{Tag: "latest-dev"},
					},
				},
			},
			want: true,
		},
		{
			name: "explicit false is preserved on supported node agent version",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: ptr.To(false),
					},
				},
				Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
					v2alpha1.NodeAgentComponentName: {
						Image: &v2alpha1.AgentImageConfig{Tag: "7.78.0"},
					},
				},
			},
			want: false,
		},
		{
			name: "explicit true is preserved on unsupported node agent version",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: ptr.To(true),
					},
				},
				Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
					v2alpha1.NodeAgentComponentName: {
						Image: &v2alpha1.AgentImageConfig{Tag: "7.77.2"},
					},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := buildFeature(nil)
			reqComp := f.Configure(nil, tt.ddaSpec, nil)

			if tt.ddaSpec.Features == nil {
				assert.Nil(t, tt.ddaSpec.Features)
			} else {
				assert.NotNil(t, tt.ddaSpec.Features.ServiceDiscovery)
				assert.NotNil(t, tt.ddaSpec.Features.ServiceDiscovery.Enabled)
				assert.Equal(t, tt.want, *tt.ddaSpec.Features.ServiceDiscovery.Enabled)
			}
			assert.Equal(t, tt.want, reqComp.Agent.IsEnabled())
		})
	}
}

func Test_serviceDiscoveryFeature_resolveEnabled_InheritsDefaultVersionWhenImageVersionIsOmitted(t *testing.T) {
	expected := serviceDiscoveryEnabledForVersion(images.AgentLatestVersion)

	tests := []struct {
		name    string
		ddaSpec *v2alpha1.DatadogAgentSpec
	}{
		{
			name: "no node agent override inherits default agent version",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{},
			},
		},
		{
			name: "partial node agent image override inherits default agent version",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{},
				Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
					v2alpha1.NodeAgentComponentName: {
						Image: &v2alpha1.AgentImageConfig{
							PullPolicy: ptr.To(corev1.PullAlways),
						},
					},
				},
			},
		},
		{
			name: "explicit default agent version matches inherited policy",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{},
				Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
					v2alpha1.NodeAgentComponentName: {
						Image: &v2alpha1.AgentImageConfig{
							Tag: images.AgentLatestVersion,
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveEnabled(tt.ddaSpec)

			assert.Equal(t, expected, got)
			assert.NotNil(t, tt.ddaSpec.Features.ServiceDiscovery)
			assert.NotNil(t, tt.ddaSpec.Features.ServiceDiscovery.Enabled)
			assert.Equal(t, expected, *tt.ddaSpec.Features.ServiceDiscovery.Enabled)
		})
	}
}

func serviceDiscoveryEnabledForVersion(version string) bool {
	return pkgutils.IsAboveMinVersion(version, serviceDiscoveryAutoEnableMinVersion, nil)
}

func getWantFunc() func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
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
	}
}
