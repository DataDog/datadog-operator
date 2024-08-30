// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secretbackend

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	v2alpha1test "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1/test"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/dependencies"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/test"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

const (
	command      string = "/foo.sh"
	args         string = "bar baz"
	timeout      int32  = 60
	ddaName             = "foo"
	ddaNamespace        = "bar"
)

var roles = []v2alpha1.SecretBackendRolesConfig{
	{
		Namespace: apiutils.NewStringPointer("foo"),
		Secrets:   []string{"bar", "baz", "qux"},
	},
	{
		Namespace: apiutils.NewStringPointer("x"),
		Secrets:   []string{"y", "z"},
	},
}

var expectedSecretBackendCommandEnv = []*corev1.EnvVar{
	{
		Name:  apicommon.DDSecretBackendCommand,
		Value: command,
	},
}

var expectedSecretBackendArgsEnv = []*corev1.EnvVar{
	{
		Name:  apicommon.DDSecretBackendArguments,
		Value: args,
	},
}

var expectedSecretBackendTimeoutEnv = []*corev1.EnvVar{
	{
		Name:  apicommon.DDSecretBackendTimeout,
		Value: strconv.FormatInt(int64(timeout), 10),
	},
}

var expectedSecretBackendEnvs = []*corev1.EnvVar{
	{
		Name:  apicommon.DDSecretBackendCommand,
		Value: command,
	},
	{
		Name:  apicommon.DDSecretBackendArguments,
		Value: args,
	},
	{
		Name:  apicommon.DDSecretBackendTimeout,
		Value: strconv.FormatInt(int64(timeout), 10),
	},
}

var expectedSubjects = []rbacv1.Subject{
	{
		Kind:      "ServiceAccount",
		Name:      ddaName + "-" + apicommon.DefaultAgentResourceSuffix,
		Namespace: ddaNamespace,
	},
	{
		Kind:      "ServiceAccount",
		Name:      ddaName + "-" + apicommon.DefaultClusterAgentResourceSuffix,
		Namespace: ddaNamespace,
	},
}

func Test_secretBackendFeature_Configure(t *testing.T) {
	tests := test.FeatureTestSuite{
		// Individual env var testing
		{
			Name: "v2alpha1 secret backend command only - node Agent",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithSecretBackendCommand(command).
				Build(),
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(secretBackendNodeAgentCommandWantFunc),
		},
		{
			Name: "v2alpha1 secret backend empty command - node Agent",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithSecretBackendCommand("").
				Build(),
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(secretBackendNodeAgentEmptyCommandWantFunc),
		},
		{
			Name: "v2alpha1 secret backend args only - node Agent",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithSecretBackendArgs(args).
				Build(),
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(secretBackendNodeAgentArgsWantFunc),
		},
		{
			Name: "v2alpha1 secret backend timeout only - node Agent",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithSecretBackendTimeout(timeout).
				Build(),
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(secretBackendNodeAgentTimeoutWantFunc),
		},
		// All env vars and all components
		{
			Name: "v2alpha1 secret backend command & args & timeout - node Agent & DCA & CCR",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithSecretBackendCommand(command).
				WithSecretBackendArgs(args).
				WithSecretBackendTimeout(timeout).
				WithClusterChecksEnabled(true).
				WithClusterChecksUseCLCEnabled(true).
				Build(),
			WantConfigure:       true,
			Agent:               test.NewDefaultComponentTest().WithWantFunc(secretBackendNodeAgentCommandArgsTimeoutWantFunc),
			ClusterAgent:        test.NewDefaultComponentTest().WithWantFunc(secretBackendDCACommandArgsTimeoutWantFunc),
			ClusterChecksRunner: test.NewDefaultComponentTest().WithWantFunc(secretBackendCCRCommandArgsTimeoutWantFunc),
		},
		// Global RBAC permissions
		{
			Name: "v2alpha1 secret backend enabled global permissions",
			DDA: addNameNamespaceToDDA(
				ddaName,
				ddaNamespace,
				v2alpha1test.NewDatadogAgentBuilder().
					WithSecretBackendEnabledGlobalPermissions(true).
					Build()),
			WantConfigure:        true,
			WantDependenciesFunc: testGlobalPermissionsRBACResources,
		},
		// Roles permissions
		{
			Name: "v2alpha1 roles permissions",
			DDA: addNameNamespaceToDDA(
				ddaName,
				ddaNamespace,
				v2alpha1test.NewDatadogAgentBuilder().
					WithSecretBackendRoles(roles).
					Build()),
			WantConfigure:        true,
			WantDependenciesFunc: testRolesPermissionsRBACResources,
		},
		// Global RBAC and roles permissions
		{
			Name: "v2alpha1 enabled global permissions & roles permissions",
			DDA: addNameNamespaceToDDA(
				ddaName,
				ddaNamespace,
				v2alpha1test.NewDatadogAgentBuilder().
					WithSecretBackendEnabledGlobalPermissions(true).
					WithSecretBackendRoles(roles).
					Build()),
			WantConfigure:        true,
			WantDependenciesFunc: testEnabledGlobalAndRolesPermissionsRBACResources,
		},
	}

	tests.Run(t, buildSecretBackendFeature)
}

func secretBackendNodeAgentCommandWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)
	agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.AllContainers]
	assert.True(t, apiutils.IsEqualStruct(agentEnvVars, expectedSecretBackendCommandEnv), "Node Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, expectedSecretBackendCommandEnv))
}

func secretBackendNodeAgentEmptyCommandWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)
	agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.AllContainers]
	assert.Nil(t, agentEnvVars, "Node Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, nil))
}

func secretBackendNodeAgentArgsWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)
	agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.AllContainers]
	assert.True(t, apiutils.IsEqualStruct(agentEnvVars, expectedSecretBackendArgsEnv), "Node Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, expectedSecretBackendArgsEnv))
}

func secretBackendNodeAgentTimeoutWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)
	agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.AllContainers]
	assert.True(t, apiutils.IsEqualStruct(agentEnvVars, expectedSecretBackendTimeoutEnv), "Node Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, expectedSecretBackendTimeoutEnv))
}

func secretBackendNodeAgentCommandArgsTimeoutWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)
	agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.AllContainers]
	assert.True(t, apiutils.IsEqualStruct(agentEnvVars, expectedSecretBackendEnvs), "Node Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, expectedSecretBackendEnvs))
}

func secretBackendDCACommandArgsTimeoutWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)
	agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.ClusterAgentContainerName]
	assert.True(t, apiutils.IsEqualStruct(agentEnvVars, expectedSecretBackendEnvs), "DCA envvars \ndiff = %s", cmp.Diff(agentEnvVars, expectedSecretBackendEnvs))
}

func secretBackendCCRCommandArgsTimeoutWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)
	agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.ClusterChecksRunnersContainerName]
	assert.True(t, apiutils.IsEqualStruct(agentEnvVars, expectedSecretBackendEnvs), "CCR envvars \ndiff = %s", cmp.Diff(agentEnvVars, expectedSecretBackendEnvs))
}

func addNameNamespaceToDDA(name string, namespace string, dda *v2alpha1.DatadogAgent) *v2alpha1.DatadogAgent {
	dda.Name = name
	dda.Namespace = namespace
	return dda
}

func testGlobalPermissionsRBACResources(t testing.TB, store dependencies.StoreClient) {
	rbacName := fmt.Sprintf("%s-%s-%s", ddaNamespace, ddaName, secretBackendRBACSuffix)
	crObj, found := store.Get(kubernetes.ClusterRolesKind, "", rbacName)

	// Validate ClusterRole policy rules
	if !found {
		t.Error("Should have created ClusterRole")
	} else {
		cr := crObj.(*rbacv1.ClusterRole)
		assert.True(
			t,
			apiutils.IsEqualStruct(cr.Rules, secretBackendGlobalRBACPolicyRules),
			"ClusterRole Policy Rules \ndiff = %s", cmp.Diff(cr.Rules, secretBackendGlobalRBACPolicyRules),
		)
	}

	expectedRoleRef := rbacv1.RoleRef{
		APIGroup: rbacv1.GroupName,
		Kind:     rbac.ClusterRoleKind,
		Name:     rbacName,
	}

	crbObj, found := store.Get(kubernetes.ClusterRoleBindingKind, "", rbacName)
	if !found {
		t.Error("Should have created ClusterRoleBinding")
	} else {
		crb := crbObj.(*rbacv1.ClusterRoleBinding)
		// Validate ClusterRoleBinding roleRef name
		assert.True(
			t,
			apiutils.IsEqualStruct(crb.RoleRef, expectedRoleRef),
			"ClusterRoleBinding Role Ref \ndiff = %s", cmp.Diff(crb.RoleRef, expectedRoleRef),
		)
		// Validate ClusterRoleBinding subjects
		assert.True(
			t,
			apiutils.IsEqualStruct(crb.Subjects, expectedSubjects),
			"ClusterRoleBinding Subjects \ndiff = %s", cmp.Diff(crb.Subjects, expectedSubjects),
		)
	}
}

func testRolesPermissionsRBACResources(t testing.TB, store dependencies.StoreClient) {
	for _, role := range roles {
		secretNs := *role.Namespace
		rbacName := fmt.Sprintf("%s-%s-%s-%s", ddaNamespace, ddaName, secretsReader, secretNs)
		expectedRules := []rbacv1.PolicyRule{
			{
				APIGroups:     []string{rbac.CoreAPIGroup},
				Resources:     []string{rbac.SecretsResource},
				ResourceNames: role.Secrets,
				Verbs:         []string{rbac.GetVerb},
			},
		}
		expectedRoleRef := rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     rbac.RoleKind,
			Name:     rbacName,
		}

		// Validate Role policy rules
		rObj, found := store.Get(kubernetes.RolesKind, secretNs, rbacName)
		if !found {
			t.Error("Should have created Role")
		} else {
			r := rObj.(*rbacv1.Role)
			assert.True(
				t,
				apiutils.IsEqualStruct(r.Rules, expectedRules),
				"Role Policy Rules \n diff = %s", cmp.Diff(r.Rules, expectedRules),
			)
		}

		rbObj, found := store.Get(kubernetes.RoleBindingKind, secretNs, rbacName)
		if !found {
			t.Error("Should have created RoleBinding")
		} else {
			rb := rbObj.(*rbacv1.RoleBinding)
			// Validate RoleBinding roleRef name
			assert.True(
				t,
				apiutils.IsEqualStruct(rb.RoleRef, expectedRoleRef),
				"RoleBinding Role Ref \ndiff = %s", cmp.Diff(rb.RoleRef, expectedRoleRef),
			)
			// Validate RoleBinding Subjects
			assert.True(
				t,
				apiutils.IsEqualStruct(rb.Subjects, expectedSubjects),
				"RoleBinding Subjects \ndiff = %s", cmp.Diff(rb.Subjects, expectedSubjects),
			)
		}
	}
}

func testEnabledGlobalAndRolesPermissionsRBACResources(t testing.TB, store dependencies.StoreClient) {
	// Assert ClusterRole is not created (if defined, roles take precedence over enabled global permissions)
	rbacName := fmt.Sprintf("%s-%s-%s", ddaNamespace, ddaName, secretBackendRBACSuffix)
	crObj, _ := store.Get(kubernetes.ClusterRolesKind, "", rbacName)
	assert.Nil(
		t,
		crObj,
		"Should NOT have created ClusterRole",
	)
	// Assert roles are created
	testRolesPermissionsRBACResources(t, store)
}
