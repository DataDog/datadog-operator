// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autoscaling

import (
	"fmt"
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/test"
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
			Name:          "v2alpha1 autoscaling disabled",
			DDAI:          newAgent(false, true),
			ClusterAgent:  testDCAResources(false),
			Agent:         testAgentResources(false),
			WantConfigure: false,
		},
		{
			Name:                 "v2alpha1 autoscaling enabeld",
			DDAI:                 newAgent(true, true),
			WantConfigure:        true,
			ClusterAgent:         testDCAResources(true),
			Agent:                testAgentResources(true),
			WantDependenciesFunc: testRBACResources,
		},
		{
			Name:                      "v2alpha1 autoscaling enabeld but admission disabled",
			DDAI:                      newAgent(true, false),
			ClusterAgent:              testDCAResources(true),
			Agent:                     testAgentResources(true),
			WantConfigure:             true,
			WantManageDependenciesErr: true,
		},
	}

	tests.Run(t, buildAutoscalingFeature)
}

func newAgent(enabled bool, admissionEnabled bool) *v1alpha1.DatadogAgentInternal {
	return &v1alpha1.DatadogAgentInternal{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "bar",
		},
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				Autoscaling: &v2alpha1.AutoscalingFeatureConfig{
					Workload: &v2alpha1.WorkloadAutoscalingFeatureConfig{
						Enabled: apiutils.NewBoolPointer(enabled),
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

	if !found {
		t.Error("Should have created ClusterRole")
	} else {
		cr := crObj.(*rbacv1.ClusterRole)
		assert.True(
			t,
			apiutils.IsEqualStruct(cr.Rules, []rbacv1.PolicyRule{
				{
					Verbs:     []string{"*"},
					APIGroups: []string{"datadoghq.com"},
					Resources: []string{"datadogpodautoscalers", "datadogpodautoscalers/status"},
				},
				{
					Verbs:     []string{"get", "update"},
					APIGroups: []string{"*"},
					Resources: []string{"*/scale"},
				},
				{
					Verbs:     []string{"create", "patch"},
					APIGroups: []string{""},
					Resources: []string{"events"},
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
			}),
			"ClusterRole Policy Rules \ndiff = %s", cmp.Diff(cr.Rules, ""),
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

func testDCAResources(enabled bool) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			clusterAgentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName]
			var expectedClusterAgentEnvVars []*corev1.EnvVar

			if enabled {
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

			assert.True(
				t,
				apiutils.IsEqualStruct(clusterAgentEnvs, expectedClusterAgentEnvVars),
				"Cluster Agent ENVs \ndiff = %s", cmp.Diff(clusterAgentEnvs, expectedClusterAgentEnvVars),
			)
		},
	)
}

func testAgentResources(enabled bool) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			coreAgentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
			var expectedCoreAgentEnvVars []*corev1.EnvVar
			if enabled {
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
