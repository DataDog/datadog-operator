// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secretsbackend

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

var roles = []v2alpha1.SecretsBackendRolesConfig{
	{
		Namespace: apiutils.NewStringPointer("foo"),
		Secrets:   []string{"bar", "baz", "qux"},
	},
	{
		Namespace: apiutils.NewStringPointer("x"),
		Secrets:   []string{"y", "z"},
	},
}

var expectedSecretsBackendCommandEnv = []*corev1.EnvVar{
	{
		Name:  apicommon.DDSecretBackendCommand,
		Value: command,
	},
}

var expectedSecretsBackendArgsEnv = []*corev1.EnvVar{
	{
		Name:  apicommon.DDSecretBackendArguments,
		Value: args,
	},
}

var expectedSecretsBackendTimeoutEnv = []*corev1.EnvVar{
	{
		Name:  apicommon.DDSecretBackendTimeout,
		Value: strconv.FormatInt(int64(timeout), 10),
	},
}

var expectedSecretsBackendEnvs = []*corev1.EnvVar{
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

func Test_secretsBackendFeature_Configure(t *testing.T) {
	tests := test.FeatureTestSuite{
		// Individual env var testing
		{
			Name: "v2alpha1 secrets backend command only - node Agent",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithSecretsBackendCommand(command).
				Build(),
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(secretsBackendNodeAgentCommandWantFunc),
		},
		{
			Name: "v2alpha1 secrets backend empty command - node Agent",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithSecretsBackendCommand("").
				Build(),
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(secretsBackendNodeAgentEmptyCommandWantFunc),
		},
		{
			Name: "v2alpha1 secrets backend args only - node Agent",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithSecretsBackendArgs(args).
				Build(),
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(secretsBackendNodeAgentArgsWantFunc),
		},
		{
			Name: "v2alpha1 secrets backend timeout only - node Agent",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithSecretsBackendTimeout(timeout).
				Build(),
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(secretsBackendNodeAgentTimeoutWantFunc),
		},
		// All env vars and all components
		{
			Name: "v2alpha1 secrets backend command & args & timeout - node Agent & DCA & CCR",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithSecretsBackendCommand(command).
				WithSecretsBackendArgs(args).
				WithSecretsBackendTimeout(timeout).
				WithClusterChecksEnabled(true).
				WithClusterChecksUseCLCEnabled(true).
				Build(),
			WantConfigure:       true,
			Agent:               test.NewDefaultComponentTest().WithWantFunc(secretsBackendNodeAgentCommandArgsTimeoutWantFunc),
			ClusterAgent:        test.NewDefaultComponentTest().WithWantFunc(secretsBackendDCACommandArgsTimeoutWantFunc),
			ClusterChecksRunner: test.NewDefaultComponentTest().WithWantFunc(secretsBackendCCRCommandArgsTimeoutWantFunc),
		},
		// Global RBAC permissions
		{
			Name: "v2alpha1 secrets backend enabled global permissions",
			DDAv2: addNameNamespaceToDDA(
				ddaName,
				ddaNamespace,
				v2alpha1test.NewDatadogAgentBuilder().
					WithSecretsBackendEnabledGlobalPermissions(true).
					Build()),
			WantConfigure:        true,
			WantDependenciesFunc: testGlobalPermissionsRBACResources,
		},
		// Roles permissions
		{
			Name: "v2alpha1 roles permissions",
			DDAv2: addNameNamespaceToDDA(
				ddaName,
				ddaNamespace,
				v2alpha1test.NewDatadogAgentBuilder().
					WithSecretsBackendRoles(roles).
					Build()),
			WantConfigure:        true,
			WantDependenciesFunc: testRolesPermissionsRBACResources,
		},
		// Global RBAC and roles permissions
		{
			Name: "v2alpha1 enabled global permissions & roles permissions",
			DDAv2: addNameNamespaceToDDA(
				ddaName,
				ddaNamespace,
				v2alpha1test.NewDatadogAgentBuilder().
					WithSecretsBackendEnabledGlobalPermissions(true).
					WithSecretsBackendRoles(roles).
					Build()),
			WantConfigure:        true,
			WantDependenciesFunc: testEnabledGlobalAndRolesPermissionsRBACResources,
		},
	}

	tests.Run(t, buildSecretsBackendFeature)
}

func secretsBackendNodeAgentCommandWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)
	agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.AllContainers]
	assert.True(t, apiutils.IsEqualStruct(agentEnvVars, expectedSecretsBackendCommandEnv), "Node Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, expectedSecretsBackendCommandEnv))
}

func secretsBackendNodeAgentEmptyCommandWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)
	agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.AllContainers]
	assert.Nil(t, agentEnvVars, "Node Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, nil))
}

func secretsBackendNodeAgentArgsWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)
	agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.AllContainers]
	assert.True(t, apiutils.IsEqualStruct(agentEnvVars, expectedSecretsBackendArgsEnv), "Node Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, expectedSecretsBackendArgsEnv))
}

func secretsBackendNodeAgentTimeoutWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)
	agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.AllContainers]
	assert.True(t, apiutils.IsEqualStruct(agentEnvVars, expectedSecretsBackendTimeoutEnv), "Node Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, expectedSecretsBackendTimeoutEnv))
}

func secretsBackendNodeAgentCommandArgsTimeoutWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)
	agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.AllContainers]
	assert.True(t, apiutils.IsEqualStruct(agentEnvVars, expectedSecretsBackendEnvs), "Node Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, expectedSecretsBackendEnvs))
}

func secretsBackendDCACommandArgsTimeoutWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)
	agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.ClusterAgentContainerName]
	assert.True(t, apiutils.IsEqualStruct(agentEnvVars, expectedSecretsBackendEnvs), "DCA envvars \ndiff = %s", cmp.Diff(agentEnvVars, expectedSecretsBackendEnvs))
}

func secretsBackendCCRCommandArgsTimeoutWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)
	agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.ClusterChecksRunnersContainerName]
	assert.True(t, apiutils.IsEqualStruct(agentEnvVars, expectedSecretsBackendEnvs), "CCR envvars \ndiff = %s", cmp.Diff(agentEnvVars, expectedSecretsBackendEnvs))
}

func addNameNamespaceToDDA(name string, namespace string, dda *v2alpha1.DatadogAgent) *v2alpha1.DatadogAgent {
	dda.Name = name
	dda.Namespace = namespace
	return dda
}

func testGlobalPermissionsRBACResources(t testing.TB, store dependencies.StoreClient) {
	rbacName := fmt.Sprintf("%s-%s-%s", ddaNamespace, ddaName, secretsBackendRBACSuffix)
	crObj, found := store.Get(kubernetes.ClusterRolesKind, "", rbacName)

	// Validate ClusterRole policy rules
	if !found {
		t.Error("Should have created ClusterRole")
	} else {
		cr := crObj.(*rbacv1.ClusterRole)
		assert.True(
			t,
			apiutils.IsEqualStruct(cr.Rules, secretsBackendGlobalRBACPolicyRules),
			"ClusterRole Policy Rules \ndiff = %s", cmp.Diff(cr.Rules, secretsBackendGlobalRBACPolicyRules),
		)
	}

	expectedRoleRef := rbacv1.RoleRef{
		APIGroup: rbacv1.GroupName,
		Kind:     kubernetes.ClusterRolesKind,
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
			Kind:     kubernetes.RolesKind,
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
	rbacName := fmt.Sprintf("%s-%s-%s", ddaNamespace, ddaName, secretsBackendRBACSuffix)
	crObj, _ := store.Get(kubernetes.ClusterRolesKind, "", rbacName)
	assert.Nil(
		t,
		crObj,
		"Should NOT have created ClusterRole",
	)
	// Assert roles are created
	testRolesPermissionsRBACResources(t, store)
}
