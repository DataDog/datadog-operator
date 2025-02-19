// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autoscaling

import (
	"fmt"
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
			Name:          "v2alpha1 autoscaling disabled",
			DDA:           newAgent(false, true),
			WantConfigure: false,
		},
		{
			Name:                 "v2alpha1 autoscaling enabeld",
			DDA:                  newAgent(true, true),
			WantConfigure:        true,
			ClusterAgent:         testDCAResources(true),
			WantDependenciesFunc: testRBACResources,
		},
		{
			Name:                      "v2alpha1 autoscaling enabeld but admission disabled",
			DDA:                       newAgent(true, false),
			WantConfigure:             true,
			WantManageDependenciesErr: true,
		},
	}

	tests.Run(t, buildAutoscalingFeature)
}

func newAgent(enabled bool, admissionEnabled bool) *v2alpha1.DatadogAgent {
	return &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "bar",
		},
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				Autoscaling: &v2alpha1.AutoscalingFeatureConfig{
					Workload: &v2alpha1.WorkloadAutoscalingFeatureConfig{
						Enabled: apiutils.NewPointer(enabled),
					},
				},
				AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
					Enabled: apiutils.NewPointer(admissionEnabled),
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

			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName]
			var expectedAgentEnvs []*corev1.EnvVar
			if enabled {
				expectedAgentEnvs = append(expectedAgentEnvs,
					&corev1.EnvVar{
						Name:  DDAutoscalingWorkloadEnabled,
						Value: "true",
					},
				)
			}
			assert.True(
				t,
				apiutils.IsEqualStruct(agentEnvs, expectedAgentEnvs),
				"Cluster Agent ENVs \ndiff = %s", cmp.Diff(agentEnvs, expectedAgentEnvs),
			)
		},
	)
}
