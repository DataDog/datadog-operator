// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"fmt"

	"github.com/DataDog/datadog-operator/controllers/datadogagent/dependencies"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
)

// RBACManager use to manage RBAC resources.
type RBACManager interface {
	AddServiceAccount(namespace string, name string) error
	AddPolicyRules(namespace string, roleName string, saName string, policies []rbacv1.PolicyRule) error
	AddClusterPolicyRules(namespace string, roleName string, saName string, policies []rbacv1.PolicyRule) error
}

// NewRBACManager return new RBACManager instance
func NewRBACManager(store dependencies.StoreClient) RBACManager {
	manager := &rbacManagerImpl{
		store: store,
	}
	return manager
}

// rbacManagerImpl use to manage RBAC resources.
type rbacManagerImpl struct {
	store dependencies.StoreClient
}

// AddServiceAccount use to create a ServiceAccount
func (m *rbacManagerImpl) AddServiceAccount(namespace string, name string) error {
	obj, _ := m.store.GetOrCreate(kubernetes.ServiceAccountsKind, namespace, name)
	sa, ok := obj.(*corev1.ServiceAccount)
	if !ok {
		return fmt.Errorf("unable to get from the store the ServiceAccount %s/%s", namespace, name)
	}

	m.store.AddOrUpdate(kubernetes.ServiceAccountsKind, sa)
	return nil
}

// AddPolicyRules use to add PolicyRules to a Role. It also create the RoleBinding.
func (m *rbacManagerImpl) AddPolicyRules(namespace string, roleName string, saName string, policies []rbacv1.PolicyRule) error {
	obj, _ := m.store.GetOrCreate(kubernetes.RolesKind, namespace, roleName)
	role, ok := obj.(*rbacv1.Role)
	if !ok {
		return fmt.Errorf("unable to get from the store the ClusterRole %s", roleName)
	}

	// TODO: can be improve by checking if the policies don't already existe.
	role.Rules = append(role.Rules, policies...)
	m.store.AddOrUpdate(kubernetes.RolesKind, role)

	bindingObj, _ := m.store.GetOrCreate(kubernetes.RoleBindingKind, namespace, roleName)
	roleBinding, ok := bindingObj.(*rbacv1.RoleBinding)
	if !ok {
		return fmt.Errorf("unable to get from the store the RoleBinding %s/%s", namespace, roleName)
	}

	roleBinding.RoleRef = rbacv1.RoleRef{
		APIGroup: rbac.RbacAPIGroup,
		Kind:     rbac.RoleKind,
		Name:     roleName,
	}
	found := false
	for _, sub := range roleBinding.Subjects {
		if sub.Namespace == namespace && sub.Name == saName && sub.Kind == rbac.ServiceAccountKind {
			found = true
			break
		}
	}
	if !found {
		roleBinding.Subjects = append(roleBinding.Subjects, rbacv1.Subject{
			Kind:      rbac.ServiceAccountKind,
			Name:      saName,
			Namespace: namespace,
		})
	}
	m.store.AddOrUpdate(kubernetes.RoleBindingKind, roleBinding)

	return nil
}

// AddClusterPolicyRules use to add PolicyRules to a ClusterRole. It also create the ClusterRoleBinding.
func (m *rbacManagerImpl) AddClusterPolicyRules(namespace string, roleName string, saName string, policies []rbacv1.PolicyRule) error {
	obj, _ := m.store.GetOrCreate(kubernetes.ClusterRolesKind, "", roleName)
	clusterRole, ok := obj.(*rbacv1.ClusterRole)
	if !ok {
		return fmt.Errorf("unable to get from the store the ClusterRole %s", roleName)
	}

	// TODO: can be improve by checking if the policies don't already existe.
	clusterRole.Rules = append(clusterRole.Rules, policies...)
	m.store.AddOrUpdate(kubernetes.ClusterRolesKind, clusterRole)

	bindingObj, _ := m.store.GetOrCreate(kubernetes.ClusterRoleBindingKind, "", roleName)
	clusterRoleBinding, ok := bindingObj.(*rbacv1.ClusterRoleBinding)
	if !ok {
		return fmt.Errorf("unable to get from the store the ClusterRoleBinding %s/%s", namespace, roleName)
	}

	clusterRoleBinding.RoleRef = rbacv1.RoleRef{
		APIGroup: rbac.RbacAPIGroup,
		Kind:     rbac.ClusterRoleKind,
		Name:     roleName,
	}
	found := false
	for _, sub := range clusterRoleBinding.Subjects {
		if sub.Namespace == namespace && sub.Name == saName && sub.Kind == rbac.ServiceAccountKind {
			found = true
			break
		}
	}
	if !found {
		clusterRoleBinding.Subjects = append(clusterRoleBinding.Subjects, rbacv1.Subject{
			Kind:      rbac.ServiceAccountKind,
			Name:      saName,
			Namespace: namespace,
		})
	}
	m.store.AddOrUpdate(kubernetes.ClusterRoleBindingKind, clusterRoleBinding)

	return nil
}
