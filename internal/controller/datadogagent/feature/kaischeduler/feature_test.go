// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kaischeduler

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/testutils"
)

const (
	resourcesName      = "foo"
	resourcesNamespace = "bar"
	clusterAgentImage  = "gcr.io/datadoghq/cluster-agent:7.86.0"
)

var dcaRBACName = fmt.Sprintf("%s-%s-%s-dca", resourcesNamespace, resourcesName, kaiSchedulerRBACPrefix)

func Test_kaiSchedulerFeature_Configure(t *testing.T) {
	tests := test.FeatureTestSuite{
		{
			Name: "KAI Scheduler disabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithKAISchedulerEnabled(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "KAI Scheduler enabled with Cluster Agent too old",
			DDA: testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
				WithKAISchedulerEnabled(true).
				WithClusterAgentImage("gcr.io/datadoghq/cluster-agent:7.85.0").
				Build(),
			WantConfigure: false,
		},
		{
			Name: "KAI Scheduler enabled with prerelease Cluster Agent",
			DDA: testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
				WithKAISchedulerEnabled(true).
				WithClusterAgentImage("gcr.io/datadoghq/cluster-agent:7.86.0-rc.1").
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: assertClusterRole(true),
			ClusterAgent:         clusterAgentWantFunc(true),
		},
		{
			Name: "KAI Scheduler enabled without GPU monitoring",
			DDA: testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
				WithKAISchedulerEnabled(true).
				WithClusterAgentImage(clusterAgentImage).
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: assertClusterRole(true),
			ClusterAgent:         clusterAgentWantFunc(true),
			Agent:                nodeAgentWantFunc(false),
		},
		{
			Name: "KAI Scheduler enabled with GPU monitoring",
			DDA: testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
				WithKAISchedulerEnabled(true).
				WithGPUMonitoringEnabled(true).
				WithClusterAgentImage(clusterAgentImage).
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: assertClusterRole(true),
			ClusterAgent:         clusterAgentWantFunc(true),
			Agent:                nodeAgentWantFunc(true),
		},
		{
			Name: "KAI Scheduler enabled with GPU monitoring and Cluster Agent too old",
			DDA: testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
				WithKAISchedulerEnabled(true).
				WithGPUMonitoringEnabled(true).
				WithClusterAgentImage("gcr.io/datadoghq/cluster-agent:7.85.0").
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: assertClusterRole(false),
			ClusterAgent:         clusterAgentWantFunc(false),
			Agent:                nodeAgentWantFunc(true),
		},
	}

	tests.Run(t, buildKAISchedulerFeature)
}

func assertClusterRole(want bool) func(testing.TB, store.StoreClient) {
	return func(t testing.TB, s store.StoreClient) {
		_, foundRole := s.Get(kubernetes.ClusterRolesKind, "", dcaRBACName)
		_, foundBinding := s.Get(kubernetes.ClusterRoleBindingKind, "", dcaRBACName)
		assert.Equal(t, want, foundRole, "ClusterRole %s", dcaRBACName)
		assert.Equal(t, want, foundBinding, "ClusterRoleBinding %s", dcaRBACName)
	}
}

func clusterAgentWantFunc(wantEnv bool) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)
			envVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName]
			if !wantEnv {
				assert.Empty(t, envVars)
				return
			}
			assert.Equal(t, []*corev1.EnvVar{{Name: DDClusterAgentKAISchedulerEnabled, Value: "true"}}, envVars)
		})
}

func nodeAgentWantFunc(wantEnv bool) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)
			envVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
			if !wantEnv {
				assert.Empty(t, envVars)
				return
			}
			assert.Equal(t, []*corev1.EnvVar{{Name: DDGPUKAISchedulerEnabled, Value: "true"}}, envVars)
		})
}
