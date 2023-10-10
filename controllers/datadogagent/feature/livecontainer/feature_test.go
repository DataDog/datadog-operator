// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package livecontainer

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	v2alpha1test "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1/test"

	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/test"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestLiveContainerFeature(t *testing.T) {
	tests := test.FeatureTestSuite{
		//////////////////////////
		// v1Alpha1.DatadogAgent
		//////////////////////////
		{
			Name:          "v1alpha1 live container collection not enabled",
			DDAv1:         newV1Agent(false),
			WantConfigure: false,
		},
		{
			Name:          "v1alpha1 live container collection enabled",
			DDAv1:         newV1Agent(true),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommonv1.ProcessAgentContainerName),
		},

		//////////////////////////
		// v2Alpha1.DatadogAgent
		//////////////////////////
		{
			Name: "v1alpha1 live container collection not enabled",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithLiveContainerCollectionEnabled(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "v1alpha1 live container collection not enabled with multi-process container",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithLiveContainerCollectionEnabled(false).
				WithMultiProcessContainer(true).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "v2alpha1 live container collection enabled",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithLiveContainerCollectionEnabled(true).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommonv1.ProcessAgentContainerName),
		},
		{
			Name: "v2alpha1 live container collection enabled with multi-process container",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithLiveContainerCollectionEnabled(true).
				WithMultiProcessContainer(true).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommonv1.NonPrivilegedMultiProcessAgentContainerName),
		},
	}

	tests.Run(t, buildLiveContainerFeature)
}

func newV1Agent(enableLiveContainer bool) *v1alpha1.DatadogAgent {
	return &v1alpha1.DatadogAgent{
		Spec: v1alpha1.DatadogAgentSpec{
			Agent: v1alpha1.DatadogAgentSpecAgentSpec{
				Process: &v1alpha1.ProcessSpec{
					Enabled: apiutils.NewBoolPointer(enableLiveContainer),
				},
			},
		},
	}
}

func testExpectedAgent(agentContainerName apicommonv1.AgentContainerName) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[agentContainerName]
			expectedAgentEnvs := []*corev1.EnvVar{
				{
					Name:  apicommon.DDContainerCollectionEnabled,
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
			assert.True(
				t,
				apiutils.IsEqualStruct(agentVolumeMounts, expectedVolumeMounts),
				"%s VolumeMounts \ndiff = %s", agentContainerName, cmp.Diff(agentVolumeMounts, expectedVolumeMounts),
			)

			agentVolumes := mgr.VolumeMgr.Volumes
			expectedVolumes := []corev1.Volume{
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
			assert.True(
				t,
				apiutils.IsEqualStruct(agentVolumes, expectedVolumes),
				"Agent Volumes \ndiff = %s", cmp.Diff(agentVolumes, expectedVolumes),
			)
		},
	)
}
