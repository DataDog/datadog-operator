// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sbom

import (
	"testing"

	"k8s.io/utils/ptr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	"github.com/DataDog/datadog-operator/pkg/images"
	"github.com/DataDog/datadog-operator/pkg/utils"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func Test_sbomFeature_Configure(t *testing.T) {

	sbomDisabled := v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				SBOM: &v2alpha1.SBOMFeatureConfig{
					Enabled: ptr.To(false),
				},
			},
		},
	}
	sbomEnabled := sbomDisabled.DeepCopy()
	{
		sbomEnabled.Spec.Features.SBOM.Enabled = ptr.To(true)
	}

	sbomEnabledContainerImageEnabled := sbomEnabled.DeepCopy()
	{
		sbomEnabledContainerImageEnabled.Spec.Features.SBOM.ContainerImage = &v2alpha1.SBOMContainerImageConfig{Enabled: ptr.To(true)}
	}

	sbomEnabledContainerImageOverlayFSEnabled := sbomEnabled.DeepCopy()
	{
		sbomEnabledContainerImageOverlayFSEnabled.Spec.Features.SBOM.ContainerImage = &v2alpha1.SBOMContainerImageConfig{Enabled: ptr.To(true), UncompressedLayersSupport: true, OverlayFSDirectScan: true}
	}

	sbomEnabledContainerImageUncompressedLayers := sbomEnabled.DeepCopy()
	{
		sbomEnabledContainerImageUncompressedLayers.Spec.Features.SBOM.ContainerImage = &v2alpha1.SBOMContainerImageConfig{Enabled: ptr.To(true), UncompressedLayersSupport: true}
	}
	withNodeAgentImage := func(dda *v2alpha1.DatadogAgent, image v2alpha1.AgentImageConfig) *v2alpha1.DatadogAgent {
		dda = dda.DeepCopy()
		dda.Spec.Override = map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
			v2alpha1.NodeAgentComponentName: {Image: &image},
		}
		return dda
	}
	defaultAgentUsesMount := !utils.IsAboveMinVersion(images.AgentLatestVersion, inPlaceScanMinVersion, nil)

	sbomEnabledHostEnabled := sbomEnabled.DeepCopy()
	{
		sbomEnabledHostEnabled.Spec.Features.SBOM.Host = &v2alpha1.SBOMHostConfig{Enabled: ptr.To(true)}
	}

	sbomEnabledEnrichmentUsageEnabled := sbomEnabled.DeepCopy()
	{
		sbomEnabledEnrichmentUsageEnabled.Spec.Features.SBOM.Enrichment = &v2alpha1.SBOMEnrichmentConfig{
			Usage: &v2alpha1.SBOMEnrichmentUsageConfig{Enabled: ptr.To(true)},
		}
	}

	sbomNodeAgentWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)

		wantEnvVars := []*corev1.EnvVar{
			{
				Name:  DDSBOMEnabled,
				Value: "true",
			},
			{
				Name:  DDSBOMContainerImageEnabled,
				Value: "false",
			},
			{
				Name:  DDSBOMHostEnabled,
				Value: "false",
			},
		}

		nodeCoreAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(nodeCoreAgentEnvVars, wantEnvVars), "Core agent envvars \ndiff = %s", cmp.Diff(nodeCoreAgentEnvVars, wantEnvVars))
	}

	sbomWithContainerImageWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)

		wantEnvVars := []*corev1.EnvVar{
			{
				Name:  DDSBOMEnabled,
				Value: "true",
			},
			{
				Name:  DDSBOMContainerImageEnabled,
				Value: "true",
			},
			{
				Name:  DDSBOMHostEnabled,
				Value: "false",
			},
		}

		nodeCoreAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(nodeCoreAgentEnvVars, wantEnvVars), "Core agent envvars \ndiff = %s", cmp.Diff(nodeCoreAgentEnvVars, wantEnvVars))
	}

	sbomWithContainerImageOverlayFSWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)

		wantEnvVars := []*corev1.EnvVar{
			{
				Name:  DDSBOMEnabled,
				Value: "true",
			},
			{
				Name:  DDSBOMContainerImageEnabled,
				Value: "true",
			},
			{

				Name:  DDSBOMContainerOverlayFSDirectScan,
				Value: "true",
			},
			{
				Name:  DDSBOMHostEnabled,
				Value: "false",
			},
		}

		nodeCoreAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(nodeCoreAgentEnvVars, wantEnvVars), "Core agent envvars \ndiff = %s", cmp.Diff(nodeCoreAgentEnvVars, wantEnvVars))
	}

	sbomWithContainerImageUncompressedLayersWantFunc := func(useMount bool) func(testing.TB, feature.PodTemplateManagers) {
		return func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			wantEnvVars := []*corev1.EnvVar{
				{
					Name:  DDSBOMEnabled,
					Value: "true",
				},
				{
					Name:  DDSBOMContainerImageEnabled,
					Value: "true",
				},
			}
			if useMount {
				wantEnvVars = append(wantEnvVars, &corev1.EnvVar{
					Name:  DDSBOMContainerUseMount,
					Value: "true",
				})
			}
			wantEnvVars = append(wantEnvVars, &corev1.EnvVar{
				Name:  DDSBOMHostEnabled,
				Value: "false",
			})

			nodeCoreAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
			assert.True(t, apiutils.IsEqualStruct(nodeCoreAgentEnvVars, wantEnvVars), "Core agent envvars \ndiff = %s", cmp.Diff(nodeCoreAgentEnvVars, wantEnvVars))
			assert.Contains(t, mgr.SecurityContextMgr.CapabilitiesByC[apicommon.CoreAgentContainerName], corev1.Capability("SYS_ADMIN"))
			assert.Equal(t, agentAppArmorAnnotationValue, mgr.AnnotationMgr.Annotations[agentAppArmorAnnotationKey])
		}
	}

	sbomWithHostWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)

		wantCoreAgentsEnvVars := []*corev1.EnvVar{
			{
				Name:  DDSBOMEnabled,
				Value: "true",
			},
			{
				Name:  DDSBOMContainerImageEnabled,
				Value: "false",
			},
			{
				Name:  DDSBOMHostEnabled,
				Value: "true",
			},
			{
				Name:  common.DDHostRootEnvVar,
				Value: common.HostRootMountPath,
			},
		}

		nodeCoreAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(nodeCoreAgentEnvVars, wantCoreAgentsEnvVars), "Core agent envvars \ndiff = %s", cmp.Diff(nodeCoreAgentEnvVars, wantCoreAgentsEnvVars))

		wantVolumeMounts := []corev1.VolumeMount{
			{
				Name:      common.HostRootVolumeName,
				MountPath: common.HostRootMountPath,
				ReadOnly:  true,
			},
		}

		agentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.CoreAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(agentVolumeMounts, wantVolumeMounts), "Agent volume mounts \ndiff = %s", cmp.Diff(agentVolumeMounts, wantVolumeMounts))

		wantVolumes := []corev1.Volume{
			{
				Name: common.HostRootVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: common.HostRootHostPath,
					},
				},
			},
		}

		volumes := mgr.VolumeMgr.Volumes
		assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))
	}

	sbomWithEnrichmentUsageWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)

		wantCoreAgentEnvVars := []*corev1.EnvVar{
			{
				Name:  DDSBOMEnabled,
				Value: "true",
			},
			{
				Name:  DDSBOMContainerImageEnabled,
				Value: "false",
			},
			{
				Name:  DDSBOMHostEnabled,
				Value: "false",
			},
			{
				Name:  DDSBOMEnrichmentUsageEnabled,
				Value: "true",
			},
		}

		nodeCoreAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(nodeCoreAgentEnvVars, wantCoreAgentEnvVars), "Core agent envvars \ndiff = %s", cmp.Diff(nodeCoreAgentEnvVars, wantCoreAgentEnvVars))

		wantSystemProbeEnvVars := []*corev1.EnvVar{
			{
				Name:  DDSBOMEnrichmentUsageEnabled,
				Value: "true",
			},
		}

		nodeSystemProbeEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.SystemProbeContainerName]
		assert.True(t, apiutils.IsEqualStruct(nodeSystemProbeEnvVars, wantSystemProbeEnvVars), "System probe envvars \ndiff = %s", cmp.Diff(nodeSystemProbeEnvVars, wantSystemProbeEnvVars))
	}

	tests := test.FeatureTestSuite{
		{
			Name:          "SBOM not enabled",
			DDA:           sbomDisabled.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "SBOM enabled",
			DDA:           sbomEnabled,
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(sbomNodeAgentWantFunc),
		},
		{
			Name:          "SBOM enabled, ContainerImage enabled",
			DDA:           sbomEnabledContainerImageEnabled,
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(sbomWithContainerImageWantFunc),
		},
		{
			Name:          "SBOM enabled, ContainerImage enabled, overlayFS direct scan",
			DDA:           sbomEnabledContainerImageOverlayFSEnabled,
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(sbomWithContainerImageOverlayFSWantFunc),
		},
		{
			Name:          "SBOM enabled, ContainerImage enabled, uncompressed layers, Agent 7.86",
			DDA:           withNodeAgentImage(sbomEnabledContainerImageUncompressedLayers, v2alpha1.AgentImageConfig{Tag: "7.86.0"}),
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(sbomWithContainerImageUncompressedLayersWantFunc(true)),
		},
		{
			Name:          "SBOM enabled, ContainerImage enabled, uncompressed layers, Agent 7.87 release candidate",
			DDA:           withNodeAgentImage(sbomEnabledContainerImageUncompressedLayers, v2alpha1.AgentImageConfig{Tag: "7.87.0-rc.1"}),
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(sbomWithContainerImageUncompressedLayersWantFunc(false)),
		},
		{
			Name:          "SBOM enabled, ContainerImage enabled, uncompressed layers, image override without a tag",
			DDA:           withNodeAgentImage(sbomEnabledContainerImageUncompressedLayers, v2alpha1.AgentImageConfig{JMXEnabled: true}),
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(sbomWithContainerImageUncompressedLayersWantFunc(defaultAgentUsesMount)),
		},
		{
			Name:          "SBOM enabled, ContainerImage enabled, uncompressed layers, Agent 7.87",
			DDA:           withNodeAgentImage(sbomEnabledContainerImageUncompressedLayers, v2alpha1.AgentImageConfig{Tag: "7.87.0"}),
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(sbomWithContainerImageUncompressedLayersWantFunc(false)),
		},
		{
			Name:          "SBOM enabled, Host enabled",
			DDA:           sbomEnabledHostEnabled,
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(sbomWithHostWantFunc),
		},
		{
			Name:          "SBOM enabled, Enrichment Usage enabled",
			DDA:           sbomEnabledEnrichmentUsageEnabled,
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(sbomWithEnrichmentUsageWantFunc),
		},
	}

	tests.Run(t, buildSBOMFeature)
}
