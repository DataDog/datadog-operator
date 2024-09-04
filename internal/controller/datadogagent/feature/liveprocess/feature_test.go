// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package liveprocess

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/api/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	v2alpha1test "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1/test"
	"github.com/DataDog/datadog-operator/api/utils"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func Test_liveProcessFeature_Configure(t *testing.T) {
	tests := test.FeatureTestSuite{
		{
			Name: "live process collection not enabled",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithLiveProcessEnabled(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "live process collection enabled",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithLiveProcessEnabled(true).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommonv1.ProcessAgentContainerName, false, false),
		},
		{
			Name: "live process collection enabled with scrub and strip args",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithLiveProcessEnabled(true).
				WithLiveProcessScrubStrip(true, true).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommonv1.ProcessAgentContainerName, false, true),
		},
		{
			Name: "live process collection enabled in core agent via env vars",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithLiveProcessEnabled(true).
				WithComponentOverride(
					v2alpha1.NodeAgentComponentName,
					v2alpha1.DatadogAgentComponentOverride{
						Image: &apicommonv1.AgentImageConfig{Tag: "7.57.0"},
						Env:   []corev1.EnvVar{{Name: "DD_PROCESS_CONFIG_RUN_IN_CORE_AGENT_ENABLED", Value: "true"}},
					},
				).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommonv1.CoreAgentContainerName, true, false),
		},
		{
			Name: "live process collection enabled in core agent via option",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithLiveProcessEnabled(true).
				WithComponentOverride(
					v2alpha1.NodeAgentComponentName,
					v2alpha1.DatadogAgentComponentOverride{
						Image: &apicommonv1.AgentImageConfig{Tag: "7.57.0"},
					},
				).
				Build(),
			WantConfigure:  true,
			FeatureOptions: &feature.Options{ProcessChecksInCoreAgentEnabled: true},
			Agent:          testExpectedAgent(apicommonv1.CoreAgentContainerName, true, false),
		},
		{
			Name: "live process collection enabled in core agent via option without min version",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithLiveProcessEnabled(true).
				WithComponentOverride(
					v2alpha1.NodeAgentComponentName,
					v2alpha1.DatadogAgentComponentOverride{
						Image: &apicommonv1.AgentImageConfig{Tag: "7.52.0"},
					},
				).
				Build(),
			WantConfigure:  true,
			FeatureOptions: &feature.Options{ProcessChecksInCoreAgentEnabled: true},
			Agent:          testExpectedAgent(apicommonv1.ProcessAgentContainerName, false, false),
		},
		{
			Name: "live process collection disabled in core agent via env var override",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithLiveProcessEnabled(true).
				WithComponentOverride(
					v2alpha1.NodeAgentComponentName,
					v2alpha1.DatadogAgentComponentOverride{
						Image: &apicommonv1.AgentImageConfig{Tag: "7.57.0"},
						Env:   []corev1.EnvVar{{Name: "DD_PROCESS_CONFIG_RUN_IN_CORE_AGENT_ENABLED", Value: "false"}},
					},
				).
				Build(),
			WantConfigure:  true,
			FeatureOptions: &feature.Options{ProcessChecksInCoreAgentEnabled: true},
			Agent:          testExpectedAgent(apicommonv1.ProcessAgentContainerName, false, false),
		},
		{
			Name: "live process collection enabled on single container",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithLiveProcessEnabled(true).
				WithSingleContainerStrategy(true).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommonv1.UnprivilegedSingleAgentContainerName, false, false),
		},
	}

	tests.Run(t, buildLiveProcessFeature)
}

func testExpectedAgent(agentContainerName apicommonv1.AgentContainerName, runInCoreAgent bool, ScrubStripArgs bool) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			// check volume mounts
			wantVolumeMounts := []corev1.VolumeMount{
				{
					Name:      apicommon.PasswdVolumeName,
					MountPath: apicommon.PasswdMountPath,
					ReadOnly:  true,
				},
				{
					Name:      apicommon.CgroupsVolumeName,
					MountPath: apicommon.CgroupsMountPath,
					ReadOnly:  true,
				},
				{
					Name:      apicommon.ProcdirVolumeName,
					MountPath: apicommon.ProcdirMountPath,
					ReadOnly:  true,
				},
			}

			agentMounts := mgr.VolumeMountMgr.VolumeMountsByC[agentContainerName]
			assert.True(t, apiutils.IsEqualStruct(agentMounts, wantVolumeMounts), "%s volume mounts \ndiff = %s", agentContainerName, cmp.Diff(agentMounts, wantVolumeMounts))

			// check volumes
			wantVolumes := []corev1.Volume{
				{
					Name: apicommon.PasswdVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: apicommon.PasswdHostPath,
						},
					},
				},
				{
					Name: apicommon.CgroupsVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: apicommon.CgroupsHostPath,
						},
					},
				},
				{
					Name: apicommon.ProcdirVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: apicommon.ProcdirHostPath,
						},
					},
				},
			}

			volumes := mgr.VolumeMgr.Volumes
			assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))

			// check env vars
			wantEnvVars := []*corev1.EnvVar{
				{
					Name:  apicommon.DDProcessConfigRunInCoreAgent,
					Value: utils.BoolToString(&runInCoreAgent),
				},
				{
					Name:  apicommon.DDProcessCollectionEnabled,
					Value: "true",
				},
			}

			if ScrubStripArgs {
				ScrubStripArgsEnvVar := []*corev1.EnvVar{
					{
						Name:  apicommon.DDProcessConfigScrubArgs,
						Value: "true",
					},
					{
						Name:  apicommon.DDProcessConfigStripArgs,
						Value: "true",
					},
				}
				wantEnvVars = append(wantEnvVars, ScrubStripArgsEnvVar...)
			}

			agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[agentContainerName]
			assert.True(t, apiutils.IsEqualStruct(agentEnvVars, wantEnvVars), "%s envvars \ndiff = %s", agentContainerName, cmp.Diff(agentEnvVars, wantEnvVars))
		},
	)
}
