// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetesstatecore

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	featureutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
	mergerfake "github.com/DataDog/datadog-operator/internal/controller/datadogagent/merger/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
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
			WantConfigure:        true,
			ClusterAgent:         ksmClusterAgentWantFunc(false),
			Agent:                test.NewDefaultComponentTest().WithWantFunc(ksmAgentNodeWantFunc),
			WantDependenciesFunc: ksmWantChecksumRegistered(false, collectorOptions{enableAPIService: true, enableCRD: true, enableControllerRevisions: true}),
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
			WantConfigure:        true,
			ClusterAgent:         ksmClusterAgentWantFunc(true),
			Agent:                test.NewDefaultComponentTest().WithWantFunc(ksmAgentNodeWantFunc),
			WantDependenciesFunc: ksmWantChecksumRegisteredForContent(customData),
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
			Name: "ksm-core enabled, custom config referenced via ConfigMap",
			DDA: func() *v2alpha1.DatadogAgent {
				dda := testutils.NewDatadogAgentBuilder().
					WithKSMEnabled(true).
					Build()
				dda.Spec.Features.KubeStateMetricsCore.Conf = &v2alpha1.CustomConfig{
					ConfigMap: &v2alpha1.ConfigMapConfig{Name: "user-provided-ksm-cm"},
				}
				return dda
			}(),
			WantConfigure:        true,
			Agent:                test.NewDefaultComponentTest().WithWantFunc(ksmAgentNodeWantFunc),
			WantDependenciesFunc: ksmWantNoChecksumRegistered,
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
			WantConfigure:        true,
			Agent:                test.NewDefaultComponentTest().WithWantFunc(ksmAgentNodeWantFunc),
			ClusterAgent:         test.NewDefaultComponentTest().WithWantFunc(func(t testing.TB, mgrInterface feature.PodTemplateManagers) {}),
			ClusterChecksRunner:  test.NewDefaultComponentTest().WithWantFunc(func(t testing.TB, mgrInterface feature.PodTemplateManagers) {}),
			WantDependenciesFunc: ksmWantChecksumRegistered(true, collectorOptions{enableAPIService: true, enableCRD: true, enableControllerRevisions: true}),
		},
		{
			Name: "ksm-core enabled, cluster checks runner with image < 7.72.0",
			DDA: testutils.NewDatadogAgentBuilder().
				WithKSMEnabled(true).
				WithClusterChecks(true, true).
				WithClusterChecksRunnerImage("gcr.io/datadoghq/agent:7.71.0").
				Build(),
			WantConfigure:        true,
			Agent:                test.NewDefaultComponentTest().WithWantFunc(ksmAgentNodeWantFunc),
			ClusterAgent:         test.NewDefaultComponentTest().WithWantFunc(func(t testing.TB, mgrInterface feature.PodTemplateManagers) {}),
			ClusterChecksRunner:  test.NewDefaultComponentTest().WithWantFunc(func(t testing.TB, mgrInterface feature.PodTemplateManagers) {}),
			WantDependenciesFunc: ksmWantChecksumRegistered(true, collectorOptions{enableAPIService: true, enableCRD: true}),
		},
		{
			Name: "ksm-core enabled, useApiServerCache annotation set",
			DDA: testutils.NewDatadogAgentBuilder().
				WithKSMEnabled(true).
				WithAnnotations(map[string]string{
					featureutils.EnableKSMApiServerCacheAnnotation: "true",
				}).
				Build(),
			WantConfigure:        true,
			ClusterAgent:         ksmClusterAgentApiServerCacheWantFunc(),
			Agent:                test.NewDefaultComponentTest().WithWantFunc(ksmAgentNodeWantFunc),
			WantDependenciesFunc: ksmWantChecksumRegistered(false, collectorOptions{enableAPIService: true, enableCRD: true, enableControllerRevisions: true, useApiServerCache: true}),
		},
	}

	tests.Run(t, buildKSMFeature)
}

// hasCustomConfig is unused; kept so call sites don't need to change.
func ksmClusterAgentWantFunc(hasCustomConfig bool) *test.ComponentTest {
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

func ksmClusterAgentApiServerCacheWantFunc() *test.ComponentTest {
	return ksmClusterAgentWantFunc(false)
}

// ksmWantChecksumRegistered asserts ManageDependencies registered the default KSM ConfigMap's checksum against the Cluster Agent.
func ksmWantChecksumRegistered(clusterCheck bool, collectorOpts collectorOptions) func(testing.TB, store.StoreClient) {
	return ksmWantChecksumRegisteredForContent(ksmCheckConfig(clusterCheck, collectorOpts))
}

// ksmWantChecksumRegisteredForContent asserts ManageDependencies registered the given check config content's checksum against the Cluster Agent.
func ksmWantChecksumRegisteredForContent(content string) func(testing.TB, store.StoreClient) {
	return func(t testing.TB, s store.StoreClient) {
		wantHash, err := comparison.GenerateMD5ForSpec(map[string]string{
			ksmCoreCheckName: content,
		})
		assert.NoError(t, err)

		wantKey := "checksum.datadoghq.com/clusterAgent." + string(feature.KubernetesStateCoreIDType)
		annotations := s.GetComponentChecksums(v2alpha1.ClusterAgentComponentName)
		assert.Equal(t, wantHash, annotations[wantKey])
	}
}

// ksmWantNoChecksumRegistered asserts ManageDependencies registered no checksum against the Cluster Agent,
// e.g. when the KSM config is provided via a user-supplied ConfigMap rather than generated by the feature.
func ksmWantNoChecksumRegistered(t testing.TB, s store.StoreClient) {
	assert.Nil(t, s.GetComponentChecksums(v2alpha1.ClusterAgentComponentName))
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
