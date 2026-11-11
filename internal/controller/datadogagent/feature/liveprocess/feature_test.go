// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package liveprocess

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/api/utils"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/providercaps"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/testutils"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func Test_liveProcessFeature_Configure(t *testing.T) {
	windowsProfile := testutils.NewDatadogAgentBuilder().
		WithAnnotations(map[string]string{kubernetes.ProviderAnnotationKey: kubernetes.WindowsProvider}).
		WithLiveProcessEnabled(true).
		Build()
	windowsProfile.Labels = map[string]string{constants.ProfileLabelKey: "windows"}

	tests := test.FeatureTestSuite{
		{
			Name: "live process collection not enabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithLiveProcessEnabled(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "live process collection enabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithLiveProcessEnabled(true).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommon.CoreAgentContainerName, true, false, common.PasswdMountPath),
		},
		{
			Name: "live process collection enabled with scrub and strip args",
			DDA: testutils.NewDatadogAgentBuilder().
				WithLiveProcessEnabled(true).
				WithLiveProcessScrubStrip(true, true).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommon.CoreAgentContainerName, true, true, common.PasswdMountPath),
		},
		{
			Name: "live process collection without min version to run in core agent",
			DDA: testutils.NewDatadogAgentBuilder().
				WithLiveProcessEnabled(true).
				WithComponentOverride(
					v2alpha1.NodeAgentComponentName,
					v2alpha1.DatadogAgentComponentOverride{
						Image: &v2alpha1.AgentImageConfig{Tag: "7.52.0"},
					},
				).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommon.ProcessAgentContainerName, false, false, common.PasswdMountPath),
		},
		{
			Name: "live process collection enabled on single container",
			DDA: testutils.NewDatadogAgentBuilder().
				WithLiveProcessEnabled(true).
				WithSingleContainerStrategy(true).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommon.UnprivilegedSingleAgentContainerName, true, false, common.PasswdMountPath),
		},
		{
			Name: "host passwd on supported core agent",
			DDA: testutils.NewDatadogAgentBuilder().WithLiveProcessEnabled(true).
				WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{Image: &v2alpha1.AgentImageConfig{Tag: "7.86.0"}}).Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommon.CoreAgentContainerName, true, false, common.HostPasswdMountPath),
		},
		{
			Name: "host passwd on supported unprivileged single agent",
			DDA: testutils.NewDatadogAgentBuilder().WithLiveProcessEnabled(true).WithSingleContainerStrategy(true).
				WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{Image: &v2alpha1.AgentImageConfig{Tag: "7.86.0"}}).Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommon.UnprivilegedSingleAgentContainerName, true, false, common.HostPasswdMountPath),
		},
		{
			Name:          "live process collection on Windows",
			DDA:           windowsProfile,
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommon.ProcessAgentContainerName, false, false, common.PasswdMountPath),
		},
	}

	tests.Run(t, buildLiveProcessFeature)
}

func testExpectedAgent(agentContainerName apicommon.AgentContainerName, runInCoreAgent bool, ScrubStripArgs bool, expectedPath string) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			// check volume mounts
			expectedVolumeMounts := []corev1.VolumeMount{
				{
					Name:      common.PasswdVolumeName,
					MountPath: expectedPath,
					ReadOnly:  true,
				},
			}
			// The relocated passwd also brings host os-release under /host/etc.
			if expectedPath == common.HostPasswdMountPath {
				expectedVolumeMounts = append(expectedVolumeMounts, corev1.VolumeMount{
					Name:      common.OSReleaseFileVolumeName,
					MountPath: common.OSReleaseFileMountPath,
					ReadOnly:  true,
				})
			}
			expectedVolumeMounts = append(expectedVolumeMounts,
				corev1.VolumeMount{
					Name:      common.CgroupsVolumeName,
					MountPath: common.CgroupsMountPath,
					ReadOnly:  true,
				},
				corev1.VolumeMount{
					Name:      common.ProcdirVolumeName,
					MountPath: common.ProcdirMountPath,
					ReadOnly:  true,
				},
			)

			agentMounts := mgr.VolumeMountMgr.VolumeMountsByC[agentContainerName]
			assert.True(t, apiutils.IsEqualStruct(agentMounts, expectedVolumeMounts), "%s volume mounts \ndiff = %s", agentContainerName, cmp.Diff(agentMounts, expectedVolumeMounts))

			// check volumes
			expectedVolumes := []corev1.Volume{
				{
					Name: common.PasswdVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: common.PasswdHostPath,
						},
					},
				},
			}
			if expectedPath == common.HostPasswdMountPath {
				expectedVolumes = append(expectedVolumes, corev1.Volume{
					Name: common.OSReleaseFileVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: common.OSReleaseFileHostPath,
						},
					},
				})
			}
			expectedVolumes = append(expectedVolumes,
				corev1.Volume{
					Name: common.CgroupsVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: common.CgroupsHostPath,
						},
					},
				},
				corev1.Volume{
					Name: common.ProcdirVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: common.ProcdirHostPath,
						},
					},
				},
			)

			volumes := mgr.VolumeMgr.Volumes
			assert.True(t, apiutils.IsEqualStruct(volumes, expectedVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, expectedVolumes))

			// check env vars
			expectedEnvVars := []*corev1.EnvVar{
				{
					Name:  common.DDProcessConfigRunInCoreAgent,
					Value: utils.BoolToString(&runInCoreAgent),
				},
				{
					Name:  common.DDProcessCollectionEnabled,
					Value: "true",
				},
			}

			if ScrubStripArgs {
				ScrubStripArgsEnvVar := []*corev1.EnvVar{
					{
						Name:  DDProcessConfigScrubArgs,
						Value: "true",
					},
					{
						Name:  DDProcessConfigStripArgs,
						Value: "true",
					},
				}
				expectedEnvVars = append(expectedEnvVars, ScrubStripArgsEnvVar...)
			}

			agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[agentContainerName]
			assert.True(t, apiutils.IsEqualStruct(agentEnvVars, expectedEnvVars), "%s envvars \ndiff = %s", agentContainerName, cmp.Diff(agentEnvVars, expectedEnvVars))
		},
	)
}

func Test_liveProcessFeature_NodeAgentProviderCapabilities(t *testing.T) {
	newPodTemplate := func() *corev1.PodTemplateSpec {
		return &corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: string(apicommon.CoreAgentContainerName)},
					{Name: string(apicommon.ProcessAgentContainerName)},
				},
			},
		}
	}

	volumeNames := func(tmpl *corev1.PodTemplateSpec) []string {
		names := make([]string, 0, len(tmpl.Spec.Volumes))
		for _, v := range tmpl.Spec.Volumes {
			names = append(names, v.Name)
		}
		return names
	}

	t.Run("talos strips passwd volume", func(t *testing.T) {
		f := &liveProcessFeature{}
		tmpl := newPodTemplate()
		mgr := feature.NewPodTemplateManagers(tmpl)
		require.NoError(t, f.ManageNodeAgent(mgr))

		providercaps.ApplyProviderCapabilities(mgr, kubernetes.TalosProvider, f.NodeAgentProviderCapabilities())

		assert.NotContains(t, volumeNames(tmpl), common.PasswdVolumeName)
	})

	t.Run("default provider keeps passwd volume", func(t *testing.T) {
		f := &liveProcessFeature{}
		tmpl := newPodTemplate()
		mgr := feature.NewPodTemplateManagers(tmpl)
		require.NoError(t, f.ManageNodeAgent(mgr))

		providercaps.ApplyProviderCapabilities(mgr, kubernetes.DefaultProvider, f.NodeAgentProviderCapabilities())

		assert.Contains(t, volumeNames(tmpl), common.PasswdVolumeName)
	})
}
