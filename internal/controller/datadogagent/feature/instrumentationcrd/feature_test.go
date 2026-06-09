// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package instrumentationcrd

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	featureutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/testutils"
)

const resourcesName = "foo"
const resourcesNamespace = "bar"

func Test_instrumentationCRDFeature_Configure(t *testing.T) {
	tests := test.FeatureTestSuite{
		{
			Name: "InstrumentationCRD disabled via annotation",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAnnotations(map[string]string{featureutils.EnableInstrumentationCRDAnnotation: "false"}).
				WithClusterAgentImage("cluster-agent:7.81.0").
				Build(),
			WantConfigure: false,
		},
		{
			Name: "InstrumentationCRD enabled by default when CA version meets minimum",
			DDA: testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
				WithClusterAgentImage("cluster-agent:7.81.0").
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: instrumentationCRDWantDepsFunc(),
			ClusterAgent:         instrumentationCRDWantClusterAgentFunc(),
		},
		{
			Name: "InstrumentationCRD enabled via annotation with CA version meets minimum",
			DDA: testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
				WithAnnotations(map[string]string{featureutils.EnableInstrumentationCRDAnnotation: "true"}).
				WithClusterAgentImage("cluster-agent:7.81.0").
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: instrumentationCRDWantDepsFunc(),
			ClusterAgent:         instrumentationCRDWantClusterAgentFunc(),
		},
		{
			Name: "InstrumentationCRD disabled when version below minimum",
			DDA: testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
				WithClusterAgentImage("cluster-agent:7.79.0").
				Build(),
			WantConfigure: false,
		},
	}

	tests.Run(t, buildInstrumentationCRDFeature)
}

func instrumentationCRDWantDepsFunc() func(t testing.TB, store store.StoreClient) {
	return func(t testing.TB, store store.StoreClient) {

		rbacName := fmt.Sprintf("%s-%s-%s-%s", resourcesNamespace, resourcesName, instrumentationCRDRBACPrefix, common.ClusterAgentSuffix)

		// validate clusterRole policy rules
		crObj, found := store.Get(kubernetes.ClusterRolesKind, "", rbacName)
		if !found {
			t.Error("Should have created ClusterRole")
		} else {
			cr := crObj.(*rbacv1.ClusterRole)
			assert.True(
				t,
				apiutils.IsEqualStruct(cr.Rules, instrumentationCRDRBACPolicyRules),
				"ClusterRole Policy Rules \ndiff = %s", cmp.Diff(cr.Rules, instrumentationCRDRBACPolicyRules),
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
}

func instrumentationCRDWantClusterAgentFunc() *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			// validate env vars
			clusterAgentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName]

			expectedEnvVars := []*corev1.EnvVar{
				{
					Name:  DDInstrumentationCRDControllerEnabled,
					Value: "true",
				},
			}

			assert.True(
				t,
				apiutils.IsEqualStruct(clusterAgentEnvs, expectedEnvVars),
				"Cluster Agent EnvVars \ndiff = %s", cmp.Diff(clusterAgentEnvs, expectedEnvVars),
			)
		})
}
