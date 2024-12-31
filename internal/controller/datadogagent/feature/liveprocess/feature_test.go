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
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	"github.com/DataDog/datadog-operator/pkg/testutils"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func Test_liveProcessFeature_Configure(t *testing.T) {
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
			Agent:         testExpectedAgent(apicommon.ProcessAgentContainerName, false, false),
		},
		{
			Name: "live process collection enabled with scrub and strip args",
			DDA: testutils.NewDatadogAgentBuilder().
				WithLiveProcessEnabled(true).
				WithLiveProcessScrubStrip(true, true).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommon.ProcessAgentContainerName, false, true),
		},
		{
			Name: "live process collection enabled in core agent via env vars",
			DDA: testutils.NewDatadogAgentBuilder().
				WithLiveProcessEnabled(true).
				WithComponentOverride(
					v2alpha1.NodeAgentComponentName,
					v2alpha1.DatadogAgentComponentOverride{
						Image: &v2alpha1.AgentImageConfig{Tag: "7.57.0"},
						Env:   []corev1.EnvVar{{Name: "DD_PROCESS_CONFIG_RUN_IN_CORE_AGENT_ENABLED", Value: "true"}},
					},
				).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommon.CoreAgentContainerName, true, false),
		},
		{
			Name: "live process collection enabled in core agent via spec",
			DDA: testutils.NewDatadogAgentBuilder().
				WithLiveProcessEnabled(true).
				WithComponentOverride(
					v2alpha1.NodeAgentComponentName,
					v2alpha1.DatadogAgentComponentOverride{
						Image: &v2alpha1.AgentImageConfig{Tag: "7.57.0"},
					},
				).
				WithProcessChecksInCoreAgent(true).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommon.CoreAgentContainerName, true, false),
		},
		{
			Name: "live process collection enabled in core agent via spec without min version",
			DDA: testutils.NewDatadogAgentBuilder().
				WithLiveProcessEnabled(true).
				WithComponentOverride(
					v2alpha1.NodeAgentComponentName,
					v2alpha1.DatadogAgentComponentOverride{
						Image: &v2alpha1.AgentImageConfig{Tag: "7.52.0"},
					},
				).
				WithProcessChecksInCoreAgent(true).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommon.ProcessAgentContainerName, false, false),
		},
		{
			Name: "live process collection disabled in core agent via env var override",
			DDA: testutils.NewDatadogAgentBuilder().
				WithLiveProcessEnabled(true).
				WithComponentOverride(
					v2alpha1.NodeAgentComponentName,
					v2alpha1.DatadogAgentComponentOverride{
						Image: &v2alpha1.AgentImageConfig{Tag: "7.57.0"},
						Env:   []corev1.EnvVar{{Name: "DD_PROCESS_CONFIG_RUN_IN_CORE_AGENT_ENABLED", Value: "false"}},
					},
				).
				WithProcessChecksInCoreAgent(true).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommon.ProcessAgentContainerName, false, false),
		},
		{
			Name: "live process collection enabled on single container",
			DDA: testutils.NewDatadogAgentBuilder().
				WithLiveProcessEnabled(true).
				WithSingleContainerStrategy(true).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommon.UnprivilegedSingleAgentContainerName, false, false),
		},
	}

	tests.Run(t, buildLiveProcessFeature)
}

func testExpectedAgent(agentContainerName apicommon.AgentContainerName, runInCoreAgent bool, ScrubStripArgs bool) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			// check volume mounts
			wantVolumeMounts := []corev1.VolumeMount{
				{
					Name:      v2alpha1.PasswdVolumeName,
					MountPath: v2alpha1.PasswdMountPath,
					ReadOnly:  true,
				},
				{
					Name:      v2alpha1.CgroupsVolumeName,
					MountPath: v2alpha1.CgroupsMountPath,
					ReadOnly:  true,
				},
				{
					Name:      v2alpha1.ProcdirVolumeName,
					MountPath: v2alpha1.ProcdirMountPath,
					ReadOnly:  true,
				},
			}

			agentMounts := mgr.VolumeMountMgr.VolumeMountsByC[agentContainerName]
			assert.True(t, apiutils.IsEqualStruct(agentMounts, wantVolumeMounts), "%s volume mounts \ndiff = %s", agentContainerName, cmp.Diff(agentMounts, wantVolumeMounts))

			// check volumes
			wantVolumes := []corev1.Volume{
				{
					Name: v2alpha1.PasswdVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: v2alpha1.PasswdHostPath,
						},
					},
				},
				{
					Name: v2alpha1.CgroupsVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: v2alpha1.CgroupsHostPath,
						},
					},
				},
				{
					Name: v2alpha1.ProcdirVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: v2alpha1.ProcdirHostPath,
						},
					},
				},
			}

			volumes := mgr.VolumeMgr.Volumes
			assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))

			// check env vars
			wantEnvVars := []*corev1.EnvVar{
				{
					Name:  v2alpha1.DDProcessConfigRunInCoreAgent,
					Value: utils.BoolToString(&runInCoreAgent),
				},
				{
					Name:  v2alpha1.DDProcessCollectionEnabled,
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
				wantEnvVars = append(wantEnvVars, ScrubStripArgsEnvVar...)
			}

			agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[agentContainerName]
			assert.True(t, apiutils.IsEqualStruct(agentEnvVars, wantEnvVars), "%s envvars \ndiff = %s", agentContainerName, cmp.Diff(agentEnvVars, wantEnvVars))
		},
	)
}
