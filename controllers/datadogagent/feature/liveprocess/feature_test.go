// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package liveprocess

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	v2alpha1test "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1/test"
	"github.com/DataDog/datadog-operator/apis/utils"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/test"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func Test_liveProcessFeature_Configure(t *testing.T) {
	tests := test.FeatureTestSuite{
		{
			Name: "v2alpha1 live process collection not enabled",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithLiveProcessEnabled(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "v2alpha1 live process collection enabled",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithLiveProcessEnabled(true).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommonv1.ProcessAgentContainerName, false, false),
		},
		{
			Name: "v2alpha1 live process collection enabled with scrub and strip args",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithLiveProcessEnabled(true).
				WithLiveProcessScrubStrip(true, true).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommonv1.ProcessAgentContainerName, false, true),
		},
		{
			Name: "v2alpha1 live process collection enabled with run in core agent enabled",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithLiveProcessEnabled(true).
				WithLiveProcessRunInCoreAgent(true).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommonv1.CoreAgentContainerName, true, false),
		},
		{
			Name: "v2alpha1 live process collection enabled with scrub and strip args",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithLiveProcessEnabled(true).
				WithLiveProcessScrubStrip(true, true).
				WithLiveProcessRunInCoreAgent(true).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommonv1.CoreAgentContainerName, true, true),
		},
		{
			Name: "v2alpha1 live process collection enabled with run in core agent disabled",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithLiveProcessEnabled(true).
				WithLiveProcessRunInCoreAgent(false).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommonv1.ProcessAgentContainerName, false, false),
		},
		{
			Name: "v2alpha1 live process collection enabled with run in core agent enabled -- single container",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithLiveProcessEnabled(true).
				WithLiveProcessRunInCoreAgent(true).
				WithSingleContainerStrategy(true).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommonv1.UnprivilegedSingleAgentContainerName, true, false),
		},
		{
			Name: "v2alpha1 live process collection enabled with run in core agent disabled -- single container",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithLiveProcessEnabled(true).
				WithLiveProcessRunInCoreAgent(false).
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
				wantEnvVars = []*corev1.EnvVar{
					{
						Name:  apicommon.DDProcessConfigRunInCoreAgent,
						Value: utils.BoolToString(&runInCoreAgent),
					},
					{
						Name:  apicommon.DDProcessCollectionEnabled,
						Value: "true",
					},
					{
						Name:  apicommon.DDProcessConfigScrubArgs,
						Value: "true",
					},
					{
						Name:  apicommon.DDProcessConfigStripArgs,
						Value: "true",
					},
				}
			}

			agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[agentContainerName]
			assert.True(t, apiutils.IsEqualStruct(agentEnvVars, wantEnvVars), "%s envvars \ndiff = %s", agentContainerName, cmp.Diff(agentEnvVars, wantEnvVars))
		},
	)
}
