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
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/test"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/testutils"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

func TestClusterChecksFeature(t *testing.T) {
	tests := test.FeatureTestSuite{
		{
			Name: "cluster checks empty, checksum set",
			DDAI: &v1alpha1.DatadogAgentInternal{
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
			DDAI: testutils.NewDatadogAgentInternalBuilder().
				WithClusterChecksEnabled(false).
				WithClusterChecksUseCLCEnabled(false).
				Build(),
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(wantClusterAgentHasNonEmptyChecksumAnnotation),
			WantConfigure: false,
		},
		{
			Name: "cluster checks not enabled and runners enabled",
			DDAI: testutils.NewDatadogAgentInternalBuilder().
				WithClusterChecksEnabled(false).
				WithClusterChecksUseCLCEnabled(true).
				Build(),
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(wantClusterAgentHasNonEmptyChecksumAnnotation),
			WantConfigure: false,
		},
		{
			Name: "cluster checks enabled and runners not enabled",
			DDAI: testutils.NewDatadogAgentInternalBuilder().
				WithClusterChecksEnabled(true).
				WithClusterChecksUseCLCEnabled(false).
				Build(),
			WantConfigure: true,
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(wantClusterAgentHasExpectedEnvsAndChecksum),
			Agent:         testAgentHasExpectedEnvsWithNoRunners(apicommon.CoreAgentContainerName),
		},
		{
			Name: "cluster checks enabled and runners not enabled with single container strategy",
			DDAI: testutils.NewDatadogAgentInternalBuilder().
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
			DDAI: testutils.NewDatadogAgentInternalBuilder().
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
			DDAI: testutils.NewDatadogAgentInternalBuilder().
				WithClusterChecksEnabled(true).
				WithClusterChecksUseCLCEnabled(true).
				WithSingleContainerStrategy(true).
				Build(),
			WantConfigure:       true,
			ClusterAgent:        test.NewDefaultComponentTest().WithWantFunc(wantClusterAgentHasExpectedEnvsAndChecksum),
			ClusterChecksRunner: testClusterChecksRunnerHasExpectedEnvs(),
			Agent:               testAgentHasExpectedEnvsWithRunners(apicommon.UnprivilegedSingleAgentContainerName),
		},
	}

	tests.Run(t, buildClusterChecksFeature)
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

	datadogAgents := []*v1alpha1.DatadogAgentInternal{
		{
			Spec: v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{},
				},
			},
		},
		testutils.NewDatadogAgentInternalBuilder().
			WithClusterChecksEnabled(false).
			WithClusterChecksUseCLCEnabled(false).
			Build(),
		testutils.NewDatadogAgentInternalBuilder().
			WithClusterChecksEnabled(false).
			WithClusterChecksUseCLCEnabled(true).
			Build(),
		testutils.NewDatadogAgentInternalBuilder().
			WithClusterChecksEnabled(true).
			WithClusterChecksUseCLCEnabled(false).
			Build(),
		testutils.NewDatadogAgentInternalBuilder().
			WithClusterChecksEnabled(true).
			WithClusterChecksUseCLCEnabled(true).
			Build(),
	}

	for _, datadogAgent := range datadogAgents {
		feature.Configure(datadogAgent)
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

func wantClusterAgentHasExpectedEnvs(t testing.TB, mgrInterface feature.PodTemplateManagers) {
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
