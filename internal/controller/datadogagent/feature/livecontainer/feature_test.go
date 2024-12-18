// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package livecontainer

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
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

func TestLiveContainerFeature(t *testing.T) {
	tests := test.FeatureTestSuite{
		{
			Name: "live container collection enabled",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithLiveContainerCollectionEnabled(true).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommon.ProcessAgentContainerName, false),
		},
		{
			Name: "live container collection enabled with single container",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithLiveContainerCollectionEnabled(true).
				WithSingleContainerStrategy(true).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommon.UnprivilegedSingleAgentContainerName, false),
		},
		{
			Name: "live container collection enabled on core agent via env var",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithLiveContainerCollectionEnabled(true).
				WithComponentOverride(
					v2alpha1.NodeAgentComponentName,
					v2alpha1.DatadogAgentComponentOverride{
						Image: &v2alpha1.AgentImageConfig{Tag: "7.57.0"},
						Env:   []corev1.EnvVar{{Name: "DD_PROCESS_CONFIG_RUN_IN_CORE_AGENT_ENABLED", Value: "true"}},
					},
				).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommon.CoreAgentContainerName, true),
		},
		{
			Name: "live container collection enabled on core agent via spec",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithLiveContainerCollectionEnabled(true).
				WithComponentOverride(
					v2alpha1.NodeAgentComponentName,
					v2alpha1.DatadogAgentComponentOverride{
						Image: &v2alpha1.AgentImageConfig{Tag: "7.57.0"},
					},
				).
				WithProcessChecksInCoreAgent(true).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommon.CoreAgentContainerName, true),
		},
		{
			Name: "live container collection enabled in core agent via spec without min version",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithLiveContainerCollectionEnabled(true).
				WithComponentOverride(
					v2alpha1.NodeAgentComponentName,
					v2alpha1.DatadogAgentComponentOverride{
						Image: &v2alpha1.AgentImageConfig{Tag: "7.52.0"},
					},
				).
				WithProcessChecksInCoreAgent(true).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommon.ProcessAgentContainerName, false),
		},
		{
			Name: "live container collection disabled on core agent via env var override",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithLiveContainerCollectionEnabled(true).
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
			Agent:         testExpectedAgent(apicommon.ProcessAgentContainerName, false),
		},
	}

	tests.Run(t, buildLiveContainerFeature)
}

func testExpectedAgent(agentContainerName apicommon.AgentContainerName, runInCoreAgent bool) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[agentContainerName]
			expectedAgentEnvs := []*corev1.EnvVar{
				{
					Name:  v2alpha1.DDProcessConfigRunInCoreAgent,
					Value: utils.BoolToString(&runInCoreAgent),
				},
				{
					Name:  v2alpha1.DDContainerCollectionEnabled,
					Value: "true",
				},
			}

			assert.True(
				t,
				apiutils.IsEqualStruct(agentEnvs, expectedAgentEnvs),
				"%s ENVs \ndiff = %s", agentContainerName, cmp.Diff(agentEnvs, expectedAgentEnvs),
			)

			agentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[agentContainerName]
			expectedVolumeMounts := []corev1.VolumeMount{
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
			assert.True(
				t,
				apiutils.IsEqualStruct(agentVolumeMounts, expectedVolumeMounts),
				"%s VolumeMounts \ndiff = %s", agentContainerName, cmp.Diff(agentVolumeMounts, expectedVolumeMounts),
			)

			agentVolumes := mgr.VolumeMgr.Volumes
			expectedVolumes := []corev1.Volume{
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
			assert.True(
				t,
				apiutils.IsEqualStruct(agentVolumes, expectedVolumes),
				"Agent Volumes \ndiff = %s", cmp.Diff(agentVolumes, expectedVolumes),
			)
		},
	)
}
