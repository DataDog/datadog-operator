// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package livecontainer

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
	"github.com/DataDog/datadog-operator/pkg/testutils"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestLiveContainerFeature(t *testing.T) {
	tests := test.FeatureTestSuite{
		{
			Name: "live container collection enabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithLiveContainerCollectionEnabled(true).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommon.CoreAgentContainerName, true),
		},
		{
			Name: "live container collection enabled with single container",
			DDA: testutils.NewDatadogAgentBuilder().
				WithLiveContainerCollectionEnabled(true).
				WithSingleContainerStrategy(true).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommon.UnprivilegedSingleAgentContainerName, true),
		},
		{
			Name: "live container collection without min version to run in core agent",
			DDA: testutils.NewDatadogAgentBuilder().
				WithLiveContainerCollectionEnabled(true).
				WithComponentOverride(
					v2alpha1.NodeAgentComponentName,
					v2alpha1.DatadogAgentComponentOverride{
						Image: &v2alpha1.AgentImageConfig{Tag: "7.52.0"},
					},
				).
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
					Name:  common.DDProcessConfigRunInCoreAgent,
					Value: utils.BoolToString(&runInCoreAgent),
				},
				{
					Name:  common.DDContainerCollectionEnabled,
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
					Name:      common.CgroupsVolumeName,
					MountPath: common.CgroupsMountPath,
					ReadOnly:  true,
				},
				{
					Name:      common.ProcdirVolumeName,
					MountPath: common.ProcdirMountPath,
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
					Name: common.CgroupsVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: common.CgroupsHostPath,
						},
					},
				},
				{
					Name: common.ProcdirVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: common.ProcdirHostPath,
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
