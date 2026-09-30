// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusterchecks

import (
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/testutils"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

func TestClusterChecksFeature(t *testing.T) {
	tests := test.FeatureTestSuite{
		{
			Name: "cluster checks empty, checksum set",
			DDA: &v2alpha1.DatadogAgent{
				Spec: v2alpha1.DatadogAgentSpec{
					Features: &v2alpha1.DatadogFeatures{
						ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{},
					},
				},
			},
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(wantClusterAgentHasNonEmptyChecksumAnnotation),
			WantConfigure: false,
		},
		{
			Name: "cluster checks not enabled and runners not enabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithClusterChecksEnabled(false).
				WithClusterChecksUseCLCEnabled(false).
				Build(),
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(wantClusterAgentHasNonEmptyChecksumAnnotation),
			WantConfigure: false,
		},
		{
			Name: "cluster checks not enabled and runners enabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithClusterChecksEnabled(false).
				WithClusterChecksUseCLCEnabled(true).
				Build(),
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(wantClusterAgentHasNonEmptyChecksumAnnotation),
			WantConfigure: false,
		},
		{
			Name: "cluster checks enabled and runners not enabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithClusterChecksEnabled(true).
				WithClusterChecksUseCLCEnabled(false).
				Build(),
			WantConfigure: true,
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(wantClusterAgentHasExpectedEnvsAndChecksum),
			Agent:         testAgentHasExpectedEnvsWithNoRunners(apicommon.CoreAgentContainerName),
		},
		{
			Name: "cluster checks enabled and runners not enabled with single container strategy",
			DDA: testutils.NewDatadogAgentBuilder().
				WithClusterChecksEnabled(true).
				WithClusterChecksUseCLCEnabled(false).
				WithSingleContainerStrategy(true).
				Build(),
			WantConfigure: true,
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(wantClusterAgentHasExpectedEnvsAndChecksum),
			Agent:         testAgentHasExpectedEnvsWithNoRunners(apicommon.UnprivilegedSingleAgentContainerName),
		},
		{
			Name: "cluster checks enabled and runners enabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithClusterChecksEnabled(true).
				WithClusterChecksUseCLCEnabled(true).
				Build(),
			WantConfigure:       true,
			ClusterAgent:        test.NewDefaultComponentTest().WithWantFunc(wantClusterAgentHasExpectedEnvsAndChecksum),
			ClusterChecksRunner: testClusterChecksRunnerHasExpectedEnvs(),
			Agent:               testAgentHasExpectedEnvsWithRunners(apicommon.CoreAgentContainerName),
		},
		{
			Name: "cluster checks enabled and runners enabled with single container strategy",
			DDA: testutils.NewDatadogAgentBuilder().
				WithClusterChecksEnabled(true).
				WithClusterChecksUseCLCEnabled(true).
				WithSingleContainerStrategy(true).
				Build(),
			WantConfigure:       true,
			ClusterAgent:        test.NewDefaultComponentTest().WithWantFunc(wantClusterAgentHasExpectedEnvsAndChecksum),
			ClusterChecksRunner: testClusterChecksRunnerHasExpectedEnvs(),
			Agent:               testAgentHasExpectedEnvsWithRunners(apicommon.UnprivilegedSingleAgentContainerName),
		},
		{
			Name: "cluster checks enabled, runners enabled, and a dedicated runner group",
			DDA: testutils.NewDatadogAgentBuilder().
				WithClusterChecksEnabled(true).
				WithClusterChecksUseCLCEnabled(true).
				WithClusterChecksRunnerGroups([]v2alpha1.ClusterChecksRunnerGroup{
					{Name: "ksm", ChecksInclude: []string{"kubernetes_state_core"}},
				}).
				Build(),
			WantConfigure:       true,
			ClusterAgent:        test.NewDefaultComponentTest().WithWantFunc(wantClusterAgentWithRunnerGroups(`{"ksm":["kubernetes_state_core"]}`)),
			ClusterChecksRunner: testClusterChecksRunnerHasExpectedEnvs(),
			Agent:               testAgentHasExpectedEnvsWithRunners(apicommon.CoreAgentContainerName),
		},
		{
			Name: "mixed mode: knob on, runners off, node agents keep cluster checks",
			DDA: testutils.NewDatadogAgentBuilder().
				WithClusterChecksEnabled(true).
				WithClusterChecksUseCLCEnabled(false).
				WithKubeChecksRunnerDefault().
				Build(),
			WantConfigure: true,
			// The Cluster Agent learns the kube group's claims; node agents keep
			// the clusterchecks provider and need no change.
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(wantClusterAgentWithRunnerGroups(`{"kube":["kubernetes_state_core","orchestrator"]}`)),
			Agent:        testAgentHasExpectedEnvsWithNoRunners(apicommon.CoreAgentContainerName),
			// Runner pods exist (the kube group): base runner envs apply.
			ClusterChecksRunner: testClusterChecksRunnerHasExpectedEnvs(),
		},
		{
			Name: "knob on and runners on: the Cluster Agent learns both groups",
			DDA: testutils.NewDatadogAgentBuilder().
				WithClusterChecksEnabled(true).
				WithClusterChecksUseCLCEnabled(true).
				WithKubeChecksRunnerDefault().
				WithClusterChecksRunnerGroups([]v2alpha1.ClusterChecksRunnerGroup{
					{Name: "kafka", ChecksInclude: []string{"kafka_consumer"}},
				}).
				Build(),
			WantConfigure:       true,
			ClusterAgent:        test.NewDefaultComponentTest().WithWantFunc(wantClusterAgentWithRunnerGroups(`{"kafka":["kafka_consumer"],"kube":["kubernetes_state_core","orchestrator"]}`)),
			ClusterChecksRunner: testClusterChecksRunnerHasExpectedEnvs(),
			Agent:               testAgentHasExpectedEnvsWithRunners(apicommon.CoreAgentContainerName),
		},
		{
			Name: "runners off and knob off: user-declared groups are still materialized",
			DDA: testutils.NewDatadogAgentBuilder().
				WithClusterChecksEnabled(true).
				WithClusterChecksUseCLCEnabled(false).
				WithClusterChecksRunnerGroups([]v2alpha1.ClusterChecksRunnerGroup{
					{Name: "ksm", ChecksInclude: []string{"kubernetes_state_core"}},
				}).
				Build(),
			WantConfigure:       true,
			ClusterAgent:        test.NewDefaultComponentTest().WithWantFunc(wantClusterAgentWithRunnerGroups(`{"ksm":["kubernetes_state_core"]}`)),
			ClusterChecksRunner: testClusterChecksRunnerHasExpectedEnvs(),
			Agent:               testAgentHasExpectedEnvsWithNoRunners(apicommon.CoreAgentContainerName),
		},
		{
			Name: "runners off, invalid (overlapping) groups: groups are ignored",
			DDA: testutils.NewDatadogAgentBuilder().
				WithClusterChecksEnabled(true).
				WithClusterChecksUseCLCEnabled(false).
				WithClusterChecksRunnerGroups([]v2alpha1.ClusterChecksRunnerGroup{
					{Name: "a", ChecksInclude: []string{"kubernetes_state_core"}},
					{Name: "b", ChecksInclude: []string{"kubernetes_state_core"}},
				}).
				Build(),
			WantConfigure:       true,
			ClusterAgent:        test.NewDefaultComponentTest().WithWantFunc(wantClusterAgentHasExpectedEnvsAndChecksum),
			ClusterChecksRunner: testClusterChecksRunnerHasNoEnvs(),
			Agent:               testAgentHasExpectedEnvsWithNoRunners(apicommon.CoreAgentContainerName),
		},
	}

	tests.Run(t, buildClusterChecksFeature)
}

func TestRunnerGroupsJSON(t *testing.T) {
	assert.Empty(t, runnerGroupsJSON(nil))
	// Keys are sorted, so the Cluster Agent env var is stable across reconciles.
	assert.Equal(t, `{"a":["x"],"b":["y","z"]}`, runnerGroupsJSON([]v2alpha1.ClusterChecksRunnerGroup{
		{Name: "b", ChecksInclude: []string{"y", "z"}},
		{Name: "a", ChecksInclude: []string{"x"}},
	}))
}

func TestClusterAgentChecksumsDifferentForDifferentConfig(t *testing.T) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	logger := logf.Log.WithName("checksum unique")

	annotationKey := fmt.Sprintf(constants.MD5ChecksumAnnotationKey, feature.ClusterChecksIDType)
	feature := buildClusterChecksFeature(&feature.Options{
		Logger: logger,
	})

	podTemplateManager := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})
	md5Values := map[string]string{}

	datadogAgents := []*v2alpha1.DatadogAgent{
		{
			Spec: v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{},
				},
			},
		},
		testutils.NewDatadogAgentBuilder().
			WithClusterChecksEnabled(false).
			WithClusterChecksUseCLCEnabled(false).
			Build(),
		testutils.NewDatadogAgentBuilder().
			WithClusterChecksEnabled(false).
			WithClusterChecksUseCLCEnabled(true).
			Build(),
		testutils.NewDatadogAgentBuilder().
			WithClusterChecksEnabled(true).
			WithClusterChecksUseCLCEnabled(false).
			Build(),
		testutils.NewDatadogAgentBuilder().
			WithClusterChecksEnabled(true).
			WithClusterChecksUseCLCEnabled(true).
			Build(),
	}

	for _, datadogAgent := range datadogAgents {
		feature.Configure(datadogAgent, &datadogAgent.Spec, nil)
		feature.ManageClusterAgent(podTemplateManager)
		md5 := podTemplateManager.AnnotationMgr.Annotations[annotationKey]
		md5Values[md5] = ""
	}

	// First three cases, when cluster checks is disabled md5 is empty string
	assert.Equal(t, 3, len(md5Values))
}

func wantClusterAgentHasExpectedEnvsAndChecksum(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	wantClusterAgentHasExpectedEnvs(t, mgrInterface)
	wantClusterAgentHasNonEmptyChecksumAnnotation(t, mgrInterface)
}

// wantClusterAgentWithRunnerGroups asserts the Cluster Agent envs, including
// the runner groups env var, and the checksum annotation.
func wantClusterAgentWithRunnerGroups(runnerGroups string) func(testing.TB, feature.PodTemplateManagers) {
	return func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		wantClusterAgentHasExpectedEnvs(t, mgrInterface, &corev1.EnvVar{Name: DDCLCRunnerGroups, Value: runnerGroups})
		wantClusterAgentHasNonEmptyChecksumAnnotation(t, mgrInterface)
	}
}

func wantClusterAgentHasExpectedEnvs(t testing.TB, mgrInterface feature.PodTemplateManagers, extraEnvs ...*corev1.EnvVar) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)

	clusterAgentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName]
	expectedClusterAgentEnvs := []*corev1.EnvVar{
		{
			Name:  DDClusterChecksEnabled,
			Value: "true",
		},
		{
			Name:  DDExtraConfigProviders,
			Value: kubeServicesAndEndpointsConfigProviders,
		},
		{
			Name:  DDExtraListeners,
			Value: kubeServicesAndEndpointsListeners,
		},
	}
	expectedClusterAgentEnvs = append(expectedClusterAgentEnvs, extraEnvs...)

	assert.True(
		t,
		apiutils.IsEqualStruct(clusterAgentEnvs, expectedClusterAgentEnvs),
		"Cluster Agent ENVs \ndiff = %s", cmp.Diff(clusterAgentEnvs, expectedClusterAgentEnvs),
	)
}

func wantClusterAgentHasNonEmptyChecksumAnnotation(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)
	annotationKey := fmt.Sprintf(constants.MD5ChecksumAnnotationKey, feature.ClusterChecksIDType)
	annotations := mgr.AnnotationMgr.Annotations
	assert.NotEmpty(t, annotations[annotationKey])
}

func testClusterChecksRunnerHasExpectedEnvs() *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			clusterRunnerEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterChecksRunnersContainerName]
			expectedClusterRunnerEnvs := []*corev1.EnvVar{
				{
					Name:  DDClusterChecksEnabled,
					Value: "true",
				},
				{
					Name:  DDExtraConfigProviders,
					Value: clusterChecksConfigProvider,
				},
			}

			assert.True(
				t,
				apiutils.IsEqualStruct(clusterRunnerEnvs, expectedClusterRunnerEnvs),
				"Cluster Runner ENVs \ndiff = %s", cmp.Diff(clusterRunnerEnvs, expectedClusterRunnerEnvs),
			)
		},
	)
}

// testClusterChecksRunnerHasNoEnvs asserts the feature configured no env vars
// on runner pods (no CCR-family Deployment exists at all).
func testClusterChecksRunnerHasNoEnvs() *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			clusterRunnerEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterChecksRunnersContainerName]
			assert.Empty(
				t,
				clusterRunnerEnvs,
				"Cluster Runner ENVs should be empty, got diff = %s", cmp.Diff(clusterRunnerEnvs, []*corev1.EnvVar{}),
			)
		},
	)
}

func testAgentHasExpectedEnvsWithRunners(agentContainerName apicommon.AgentContainerName) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[agentContainerName]
			expectedAgentEnvs := []*corev1.EnvVar{
				{
					Name:  DDExtraConfigProviders,
					Value: endpointsChecksConfigProvider,
				},
			}

			assert.True(
				t,
				apiutils.IsEqualStruct(agentEnvs, expectedAgentEnvs),
				"Cluster Runner ENVs \ndiff = %s", cmp.Diff(agentEnvs, expectedAgentEnvs),
			)
		},
	)
}

func testAgentHasExpectedEnvsWithNoRunners(agentContainerName apicommon.AgentContainerName) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[agentContainerName]
			expectedAgentEnvs := []*corev1.EnvVar{
				{
					Name:  DDExtraConfigProviders,
					Value: clusterAndEndpointsConfigProviders,
				},
			}

			assert.True(
				t,
				apiutils.IsEqualStruct(agentEnvs, expectedAgentEnvs),
				"Cluster Runner ENVs \ndiff = %s", cmp.Diff(agentEnvs, expectedAgentEnvs),
			)
		},
	)
}
