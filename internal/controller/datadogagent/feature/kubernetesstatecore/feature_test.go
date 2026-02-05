// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetesstatecore

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	mergerfake "github.com/DataDog/datadog-operator/internal/controller/datadogagent/merger/fake"
	"github.com/DataDog/datadog-operator/pkg/testutils"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

const (
	customData = `cluster_check: true
init_config:
instances:
    collectors:
    - pods`
)

func Test_ksmFeature_Configure(t *testing.T) {
	tests := test.FeatureTestSuite{
		{
			Name: "ksm-core not enabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithKSMEnabled(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "ksm-core not enabled with single agent container",
			DDA: testutils.NewDatadogAgentBuilder().
				WithKSMEnabled(false).
				WithSingleContainerStrategy(true).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "ksm-core enabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithKSMEnabled(true).
				Build(),
			WantConfigure: true,
			ClusterAgent:  ksmClusterAgentWantFunc(false),
			Agent:         test.NewDefaultComponentTest().WithWantFunc(ksmAgentNodeWantFunc),
		},
		{
			Name: "ksm-core enabled with single agent container",
			DDA: testutils.NewDatadogAgentBuilder().
				WithKSMEnabled(true).
				WithSingleContainerStrategy(true).
				Build(),
			WantConfigure: true,
			ClusterAgent:  ksmClusterAgentWantFunc(false),
			Agent:         test.NewDefaultComponentTest().WithWantFunc(ksmAgentSingleAgentWantFunc),
		},
		{
			Name: "ksm-core enabled, custom config",
			DDA: testutils.NewDatadogAgentBuilder().
				WithKSMEnabled(true).
				WithKSMCustomConf(customData).
				Build(),
			WantConfigure: true,
			ClusterAgent:  ksmClusterAgentWantFunc(true),
			Agent:         test.NewDefaultComponentTest().WithWantFunc(ksmAgentNodeWantFunc),
		},
		{
			Name: "ksm-core enabled, custom config with single agent container",
			DDA: testutils.NewDatadogAgentBuilder().
				WithKSMEnabled(true).
				WithKSMCustomConf(customData).
				WithSingleContainerStrategy(true).
				Build(),
			WantConfigure: true,
			ClusterAgent:  ksmClusterAgentWantFunc(true),
			Agent:         test.NewDefaultComponentTest().WithWantFunc(ksmAgentSingleAgentWantFunc),
		},
		{
			Name: "ksm-core enabled, cluster agent with image >= 7.72.0",
			DDA: testutils.NewDatadogAgentBuilder().
				WithKSMEnabled(true).
				WithClusterAgentImage("gcr.io/datadoghq/agent:7.72.0").
				Build(),
			WantConfigure: true,
			ClusterAgent:  ksmClusterAgentWantFunc(false),
			Agent:         test.NewDefaultComponentTest().WithWantFunc(ksmAgentNodeWantFunc),
		},
		{
			Name: "ksm-core enabled, cluster agent with image < 7.72.0",
			DDA: testutils.NewDatadogAgentBuilder().
				WithKSMEnabled(true).
				WithClusterAgentImage("gcr.io/datadoghq/agent:7.71.0").
				Build(),
			WantConfigure: true,
			ClusterAgent:  ksmClusterAgentWantFunc(false),
			Agent:         test.NewDefaultComponentTest().WithWantFunc(ksmAgentNodeWantFunc),
		},
		{
			Name: "ksm-core enabled, cluster checks runner with image >= 7.72.0",
			DDA: testutils.NewDatadogAgentBuilder().
				WithKSMEnabled(true).
				WithClusterChecks(true, true).
				WithClusterChecksRunnerImage("gcr.io/datadoghq/agent:7.72.0").
				Build(),
			WantConfigure:       true,
			Agent:               test.NewDefaultComponentTest().WithWantFunc(ksmAgentNodeWantFunc),
			ClusterAgent:        test.NewDefaultComponentTest().WithWantFunc(func(t testing.TB, mgrInterface feature.PodTemplateManagers) {}),
			ClusterChecksRunner: test.NewDefaultComponentTest().WithWantFunc(func(t testing.TB, mgrInterface feature.PodTemplateManagers) {}),
		},
		{
			Name: "ksm-core enabled, cluster checks runner with image < 7.72.0",
			DDA: testutils.NewDatadogAgentBuilder().
				WithKSMEnabled(true).
				WithClusterChecks(true, true).
				WithClusterChecksRunnerImage("gcr.io/datadoghq/agent:7.71.0").
				Build(),
			WantConfigure:       true,
			Agent:               test.NewDefaultComponentTest().WithWantFunc(ksmAgentNodeWantFunc),
			ClusterAgent:        test.NewDefaultComponentTest().WithWantFunc(func(t testing.TB, mgrInterface feature.PodTemplateManagers) {}),
			ClusterChecksRunner: test.NewDefaultComponentTest().WithWantFunc(func(t testing.TB, mgrInterface feature.PodTemplateManagers) {}),
		},
	}

	tests.Run(t, buildKSMFeature)
}

func ksmClusterAgentWantFunc(useClusterChecksRunner bool) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)
			dcaEnvVars := mgr.EnvVarMgr.EnvVarsByC[mergerfake.AllContainers]

			want := []*corev1.EnvVar{
				{
					Name:  DDKubeStateMetricsCoreEnabled,
					Value: "true",
				},
				{
					Name:  DDKubeStateMetricsCoreConfigMap,
					Value: "-kube-state-metrics-core-config",
				},
			}
			assert.True(t, apiutils.IsEqualStruct(dcaEnvVars, want), "DCA envvars \ndiff = %s", cmp.Diff(dcaEnvVars, want))
		},
	)
}

func ksmAgentNodeWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	ksmAgentWantFunc(t, mgrInterface, apicommon.CoreAgentContainerName)
}

func ksmAgentSingleAgentWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	ksmAgentWantFunc(t, mgrInterface, apicommon.UnprivilegedSingleAgentContainerName)
}

func ksmAgentWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers, agentContainerName apicommon.AgentContainerName) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)
	agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[agentContainerName]

	want := []*corev1.EnvVar{
		{
			Name:  DDIgnoreAutoConf,
			Value: "kubernetes_state",
		},
	}
	assert.True(t, apiutils.IsEqualStruct(agentEnvVars, want), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, want))
}
