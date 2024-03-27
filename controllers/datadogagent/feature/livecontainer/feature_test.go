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
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
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
			Agent:         testExpectedAgent(apicommonv1.ProcessAgentContainerName, false),
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
			Name: "v1alpha1 live container collection not enabled with single container",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithLiveContainerCollectionEnabled(false).
				WithSingleContainerStrategy(true).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "v2alpha1 live container collection enabled",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithLiveContainerCollectionEnabled(true).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommonv1.ProcessAgentContainerName, false),
		},
		{
			Name: "v2alpha1 live container collection enabled with single container",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithLiveContainerCollectionEnabled(true).
				WithSingleContainerStrategy(true).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommonv1.UnprivilegedSingleAgentContainerName, false),
		},
		{
			Name: "v2alpha1 live container collection enabled on core agent via env var",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithLiveContainerCollectionEnabled(true).
				WithComponentOverride(
					v2alpha1.NodeAgentComponentName,
					v2alpha1.DatadogAgentComponentOverride{
						Image: &apicommonv1.AgentImageConfig{Tag: "7.53.0"},
						Env:   []corev1.EnvVar{{Name: "DD_PROCESS_CONFIG_RUN_IN_CORE_AGENT_ENABLED", Value: "true"}},
					},
				).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommonv1.CoreAgentContainerName, true),
		},
		{
			Name: "v2alpha1 live container collection enabled on core agent via option",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithLiveContainerCollectionEnabled(true).
				WithComponentOverride(
					v2alpha1.NodeAgentComponentName,
					v2alpha1.DatadogAgentComponentOverride{
						Image: &apicommonv1.AgentImageConfig{Tag: "7.53.0"},
					},
				).
				Build(),
			FeatureOptions: &feature.Options{RunProcessChecksOnCoreAgent: true},
			WantConfigure:  true,
			Agent:          testExpectedAgent(apicommonv1.CoreAgentContainerName, true),
		},
		{
			Name: "v2alpha1 live container collection enabled in core agent via option without min version",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithLiveContainerCollectionEnabled(true).
				WithComponentOverride(
					v2alpha1.NodeAgentComponentName,
					v2alpha1.DatadogAgentComponentOverride{
						Image: &apicommonv1.AgentImageConfig{Tag: "7.52.0"},
					},
				).
				Build(),
			WantConfigure:  true,
			FeatureOptions: &feature.Options{RunProcessChecksOnCoreAgent: true},
			Agent:          testExpectedAgent(apicommonv1.ProcessAgentContainerName, false),
		},
		{
			Name: "v2alpha1 live container collection disabled on core agent via env var override",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithLiveContainerCollectionEnabled(true).
				WithComponentOverride(
					v2alpha1.NodeAgentComponentName,
					v2alpha1.DatadogAgentComponentOverride{
						Image: &apicommonv1.AgentImageConfig{Tag: "7.53.0"},
						Env:   []corev1.EnvVar{{Name: "DD_PROCESS_CONFIG_RUN_IN_CORE_AGENT_ENABLED", Value: "false"}},
					},
				).
				Build(),
			FeatureOptions: &feature.Options{RunProcessChecksOnCoreAgent: true},
			WantConfigure:  true,
			Agent:          testExpectedAgent(apicommonv1.ProcessAgentContainerName, false),
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

func testExpectedAgent(agentContainerName apicommonv1.AgentContainerName, runInCoreAgent bool) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[agentContainerName]
			expectedAgentEnvs := []*corev1.EnvVar{
				{
					Name:  apicommon.DDProcessConfigRunInCoreAgent,
					Value: utils.BoolToString(&runInCoreAgent),
				},
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
