// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secretbackend

import (
	"fmt"

	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var secretBackendGlobalRBACPolicyRules = []rbacv1.PolicyRule{
	{
		APIGroups: []string{rbac.CoreAPIGroup},
		Resources: []string{rbac.SecretsResource},
		Verbs:     []string{rbac.GetVerb},
	},
}

// getGlobalPermSecretBackendRBACResourceName return the RBAC resources name associated to the ClusterRole/ClusterRoleBinding to read secrets
func getGlobalPermSecretBackendRBACResourceName(owner metav1.Object) string {
	return fmt.Sprintf("%s-%s-%s", owner.GetNamespace(), owner.GetName(), secretBackendRBACSuffix)
}

// getNamespaceSecretReaderRBACResourceName return the RBAC resources name to the Role/RoleBinding to read secrets from a namespace
func getNamespaceSecretReaderRBACResourceName(owner metav1.Object, namespace string) string {
	return fmt.Sprintf("%s-%s-%s-%s", owner.GetNamespace(), owner.GetName(), secretsReader, namespace)
}

// getSecretsRolesPermissions returns policy rules to allow Datadog agents to get defined secrets
func getSecretsRolesPermissions(role secretBackendRole) []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		{
			APIGroups:     []string{rbac.CoreAPIGroup},
			Resources:     []string{rbac.SecretsResource},
			ResourceNames: role.secretsList,
			Verbs:         []string{rbac.GetVerb},
		},
	}
}
