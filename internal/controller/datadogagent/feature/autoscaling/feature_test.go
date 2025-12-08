// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autoscaling

import (
	"fmt"
	"sort"
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

const (
	ddaName      = "foo"
	ddaNamespace = "bar"
)

func TestAutoscalingFeature(t *testing.T) {
	tests := test.FeatureTestSuite{
		{
			Name:          "autoscaling disabled",
			DDA:           newAgent(false, false, true),
			ClusterAgent:  testDCAResources(false, false),
			Agent:         testAgentResources(false),
			WantConfigure: false,
		},
		{
			Name:                 "workload autoscaling enabled",
			DDA:                  newAgent(true, false, true),
			WantConfigure:        true,
			ClusterAgent:         testDCAResources(true, false),
			Agent:                testAgentResources(true),
			WantDependenciesFunc: testRBACResources,
		},
		{
			Name:                 "cluster autoscaling enabled",
			DDA:                  newAgent(false, true, false),
			WantConfigure:        true,
			ClusterAgent:         testDCAResources(false, true),
			Agent:                testAgentResources(false),
			WantDependenciesFunc: testRBACResources,
		},
		{
			Name:                 "workload and cluster autoscaling enabled",
			DDA:                  newAgent(true, true, true),
			WantConfigure:        true,
			ClusterAgent:         testDCAResources(true, true),
			Agent:                testAgentResources(true),
			WantDependenciesFunc: testRBACResources,
		},
		{
			Name:                      "autoscaling enabled but admission disabled",
			DDA:                       newAgent(true, true, false),
			ClusterAgent:              testDCAResources(true, true),
			Agent:                     testAgentResources(true),
			WantConfigure:             true,
			WantManageDependenciesErr: true,
		},
	}

	tests.Run(t, buildAutoscalingFeature)
}

func newAgent(workloadEnabled, clusterEnabled, admissionEnabled bool) *v2alpha1.DatadogAgent {
	return &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "bar",
		},
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				Autoscaling: &v2alpha1.AutoscalingFeatureConfig{
					Workload: &v2alpha1.WorkloadAutoscalingFeatureConfig{
						Enabled: apiutils.NewBoolPointer(workloadEnabled),
					},
					Cluster: &v2alpha1.ClusterAutoscalingFeatureConfig{
						Enabled: apiutils.NewBoolPointer(clusterEnabled),
					},
				},
				AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
					Enabled: apiutils.NewBoolPointer(admissionEnabled),
				},
			},
		},
	}
}

func testRBACResources(t testing.TB, store store.StoreClient) {
	// RBAC
	rbacName := fmt.Sprintf("%s-%s", ddaName, "cluster-agent-autoscaling")

	// validate clusterRole policy rules
	crObj, found := store.Get(kubernetes.ClusterRolesKind, "", rbacName)

	policyRules := []rbacv1.PolicyRule{
		{
			Verbs:     []string{"*"},
			APIGroups: []string{"datadoghq.com"},
			Resources: []string{"datadogpodautoscalers", "datadogpodautoscalers/status"},
		},
		{
			Verbs:     []string{"create", "patch"},
			APIGroups: []string{""},
			Resources: []string{"events"},
		},
		{
			Verbs:     []string{"get", "update"},
			APIGroups: []string{"*"},
			Resources: []string{"*/scale"},
		},
		{
			Verbs:     []string{"patch"},
			APIGroups: []string{""},
			Resources: []string{"pods"},
		},
		{
			Verbs:     []string{"patch"},
			APIGroups: []string{"apps"},
			Resources: []string{"deployments"},
		},
		{
			Verbs:     []string{"patch"},
			APIGroups: []string{"argoproj.io"},
			Resources: []string{"rollouts"},
		},
	}

	if t.Name() == "TestAutoscalingFeature/cluster_autoscaling_enabled" {
		policyRules = []rbacv1.PolicyRule{
			{
				Verbs:     []string{"create", "delete", "get", "list", "patch"},
				APIGroups: []string{"karpenter.sh"},
				Resources: []string{"*"},
			},
			{
				Verbs:     []string{"create", "patch"},
				APIGroups: []string{""},
				Resources: []string{"events"},
			},
			{
				Verbs:     []string{"get", "list"},
				APIGroups: []string{"karpenter.k8s.aws"},
				Resources: []string{"*"},
			},
		}
	}

	if t.Name() == "TestAutoscalingFeature/workload_and_cluster_autoscaling_enabled" {
		policyRules = append(policyRules, []rbacv1.PolicyRule{
			{
				Verbs:     []string{"create", "delete", "get", "list", "patch"},
				APIGroups: []string{"karpenter.sh"},
				Resources: []string{"*"},
			},
			{
				Verbs:     []string{"get", "list"},
				APIGroups: []string{"karpenter.k8s.aws"},
				Resources: []string{"*"},
			},
		}...,
		)
	}

	if !found {
		t.Error("Should have created ClusterRole")
	} else {
		cr := crObj.(*rbacv1.ClusterRole)
		fmt.Println(cr.Rules)
		assert.True(
			t,
			rulesEqual(cr.Rules, policyRules),
			"ClusterRole Policy Rules \ndiff = %s", cmp.Diff(cr.Rules, policyRules),
		)
	}

	// validate clusterRoleBinding roleRef name
	expectedRoleRef := rbacv1.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "ClusterRole",
		Name:     rbacName,
	}

	crbObj, found := store.Get(kubernetes.ClusterRoleBindingKind, "", rbacName)

	if !found {
		t.Error("Should have created ClusterRoleBinding")
	} else {
		crb := crbObj.(*rbacv1.ClusterRoleBinding)
		assert.True(
			t,
			apiutils.IsEqualStruct(crb.RoleRef, expectedRoleRef),
			"ClusterRoleBinding Role Ref \ndiff = %s", cmp.Diff(crb.RoleRef, expectedRoleRef),
		)
	}
}

func testDCAResources(workloadEnabled, clusterEnabled bool) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			clusterAgentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName]

			var expectedClusterAgentEnvVars []*corev1.EnvVar
			if workloadEnabled {
				expectedClusterAgentEnvVars = append(expectedClusterAgentEnvVars,
					&corev1.EnvVar{
						Name:  DDAutoscalingWorkloadEnabled,
						Value: "true",
					},
					&corev1.EnvVar{
						Name:  DDAutoscalingFailoverEnabled,
						Value: "true",
					},
				)
			}

			if clusterEnabled {
				expectedClusterAgentEnvVars = append(expectedClusterAgentEnvVars,
					&corev1.EnvVar{
						Name:  DDAutoscalingClusterEnabled,
						Value: "true",
					},
				)
			}

			assert.True(
				t,
				apiutils.IsEqualStruct(clusterAgentEnvs, expectedClusterAgentEnvVars),
				"Cluster Agent ENVs \ndiff = %s", cmp.Diff(clusterAgentEnvs, expectedClusterAgentEnvVars),
			)
		},
	)
}

func testAgentResources(workloadEnabled bool) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			coreAgentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
			var expectedCoreAgentEnvVars []*corev1.EnvVar
			if workloadEnabled {
				expectedCoreAgentEnvVars = append(expectedCoreAgentEnvVars,
					&corev1.EnvVar{
						Name:  DDAutoscalingFailoverEnabled,
						Value: "true",
					},
					&corev1.EnvVar{
						Name:  DDAutoscalingFailoverMetrics,
						Value: defaultFailoverMetrics,
					},
				)
			}

			assert.True(
				t,
				apiutils.IsEqualStruct(coreAgentEnvs, expectedCoreAgentEnvVars),
				"Core Agent ENVs \ndiff = %s", cmp.Diff(coreAgentEnvs, expectedCoreAgentEnvVars),
			)
		},
	)
}

func rulesEqual(in, out []rbacv1.PolicyRule) bool {
	sort.Slice(in, func(i, j int) bool {
		return in[i].APIGroups[0] < in[j].APIGroups[0]
	})

	sort.Slice(out, func(i, j int) bool {
		return out[i].APIGroups[0] < out[j].APIGroups[0]
	})

	return apiutils.IsEqualStruct(in, out)
}
