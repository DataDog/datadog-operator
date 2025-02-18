// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/util/errors"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

// RBACManager use to manage RBAC resources.
type RBACManager interface {
	AddServiceAccount(namespace string, name string) error
	AddServiceAccountByComponent(namespace, name, component string) error
	AddServiceAccountAnnotations(namespace string, name string, annotations map[string]string) error
	AddServiceAccountAnnotationsByComponent(namespace string, name string, annotations map[string]string, component string) error
	AddPolicyRules(namespace string, roleName string, saName string, policies []rbacv1.PolicyRule, saNamespace ...string) error
	AddPolicyRulesByComponent(namespace string, roleName string, saName string, policies []rbacv1.PolicyRule, component string) error
	AddRoleBinding(roleNamespace, roleName, saNamespace, saName string, roleRef rbacv1.RoleRef) error
	AddClusterPolicyRules(namespace string, roleName string, saName string, policies []rbacv1.PolicyRule) error
	AddClusterPolicyRulesByComponent(namespace string, roleName string, saName string, policies []rbacv1.PolicyRule, component string) error
	AddClusterRoleBinding(namespace string, name string, saName string, roleRef rbacv1.RoleRef) error
	DeleteServiceAccountByComponent(component, namespace string) error
	DeleteRoleByComponent(component, namespace string) error
	DeleteClusterRoleByComponent(component string) error
}

// NewRBACManager return new RBACManager instance
func NewRBACManager(store store.StoreClient) RBACManager {
	manager := &rbacManagerImpl{
		store:                     store,
		serviceAccountByComponent: make(map[string][]string),
		roleByComponent:           make(map[string][]string),
		clusterRoleByComponent:    make(map[string][]string),
	}
	return manager
}

// rbacManagerImpl use to manage RBAC resources.
type rbacManagerImpl struct {
	store store.StoreClient

	serviceAccountByComponent map[string][]string
	roleByComponent           map[string][]string
	clusterRoleByComponent    map[string][]string
}

// AddServiceAccount use to create a ServiceAccount
func (m *rbacManagerImpl) AddServiceAccount(namespace string, name string) error {
	obj, _ := m.store.GetOrCreate(kubernetes.ServiceAccountsKind, namespace, name)
	sa, ok := obj.(*corev1.ServiceAccount)
	if !ok {
		return fmt.Errorf("unable to get from the store the ServiceAccount %s/%s", namespace, name)
	}

	return m.store.AddOrUpdate(kubernetes.ServiceAccountsKind, sa)
}

// AddServiceAccountByComponent is used to create a ServiceAccount and associate it with a component
func (m *rbacManagerImpl) AddServiceAccountByComponent(namespace, name, component string) error {
	m.serviceAccountByComponent[component] = append(m.serviceAccountByComponent[component], name)
	return m.AddServiceAccount(namespace, name)
}

// DeleteServiceAccount use to remove a ServiceAccount from the store
func (m *rbacManagerImpl) DeleteServiceAccount(namespace string, name string) error {
	found := m.store.Delete(kubernetes.ServiceAccountsKind, namespace, name)
	if !found {
		return fmt.Errorf("unable to delete ServiceAccount from the store because it was not found: %s/%s", namespace, name)
	}
	return nil
}

func (m *rbacManagerImpl) DeleteServiceAccountByComponent(component, namespace string) error {
	errs := make([]error, 0, len(m.serviceAccountByComponent[component]))
	for _, name := range m.serviceAccountByComponent[component] {
		errs = append(errs, m.DeleteServiceAccount(namespace, name))
	}
	return errors.NewAggregate(errs)
}

// AddServiceAccountAnnotations updates the annotations for an existing ServiceAccount.
func (m *rbacManagerImpl) AddServiceAccountAnnotations(namespace, saName string, annotations map[string]string) error {
	obj, _ := m.store.Get(kubernetes.ServiceAccountsKind, namespace, saName)
	sa, ok := obj.(*corev1.ServiceAccount)
	if !ok {
		return fmt.Errorf("unable to get from the store the ServiceAccount %s/%s", namespace, saName)
	}
	if sa.Annotations == nil {
		sa.Annotations = make(map[string]string)
	}
	for key, value := range annotations {
		sa.Annotations[key] = value
	}
	return m.store.AddOrUpdate(kubernetes.ServiceAccountsKind, sa)
}

// AddServiceAccountAnnotationsByComponent updates the annotations for a ServiceAccount and associates it with a component.
func (m *rbacManagerImpl) AddServiceAccountAnnotationsByComponent(namespace, saName string, annotations map[string]string, component string) error {
	m.serviceAccountByComponent[component] = append(m.serviceAccountByComponent[component], saName)
	return m.AddServiceAccountAnnotations(namespace, saName, annotations)
}

// AddPolicyRules is used to add PolicyRules to a Role. It also creates the RoleBinding.
func (m *rbacManagerImpl) AddPolicyRules(namespace string, roleName string, saName string, policies []rbacv1.PolicyRule, saNamespace ...string) error {
	obj, _ := m.store.GetOrCreate(kubernetes.RolesKind, namespace, roleName)
	role, ok := obj.(*rbacv1.Role)
	if !ok {
		return fmt.Errorf("unable to get from the store the ClusterRole %s", roleName)
	}

	// TODO: can be improve by checking if the policies don't already exist.
	role.Rules = append(role.Rules, policies...)
	if err := m.store.AddOrUpdate(kubernetes.RolesKind, role); err != nil {
		return err
	}

	roleRef := rbacv1.RoleRef{
		APIGroup: rbac.RbacAPIGroup,
		Kind:     rbac.RoleKind,
		Name:     roleName,
	}

	// If saNamespace is not provided, defaults to using role namespace.
	targetSaNamespace := namespace
	if len(saNamespace) > 0 {
		targetSaNamespace = saNamespace[0]
	}

	return m.AddRoleBinding(namespace, roleName, targetSaNamespace, saName, roleRef)
}

// AddPolicyRulesByComponent is used to add PolicyRules to a Role, create a RoleBinding, and associate them with a component
func (m *rbacManagerImpl) AddPolicyRulesByComponent(namespace string, roleName string, saName string, policies []rbacv1.PolicyRule, component string) error {
	m.roleByComponent[component] = append(m.roleByComponent[component], roleName)
	return m.AddPolicyRules(namespace, roleName, saName, policies)
}

// DeleteRole is used to delete a Role and RoleBinding.
func (m *rbacManagerImpl) DeleteRole(namespace string, roleName string) error {
	found := m.store.Delete(kubernetes.RolesKind, namespace, roleName)
	if !found {
		return fmt.Errorf("unable to delete Role from the store because it was not found: %s/%s", namespace, roleName)
	}

	found = m.store.Delete(kubernetes.RoleBindingKind, namespace, roleName)
	if !found {
		return fmt.Errorf("unable to delete RoleBinding from the store because it was not found: %s/%s", namespace, roleName)
	}
	return nil
}

func (m *rbacManagerImpl) DeleteRoleByComponent(component, namespace string) error {
	errs := make([]error, 0, len(m.roleByComponent[component]))
	for _, name := range m.roleByComponent[component] {
		errs = append(errs, m.DeleteRole(namespace, name))
	}
	return errors.NewAggregate(errs)
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
	if err := m.store.AddOrUpdate(kubernetes.ClusterRolesKind, clusterRole); err != nil {
		return err
	}

	roleRef := rbacv1.RoleRef{
		APIGroup: rbac.RbacAPIGroup,
		Kind:     rbac.ClusterRoleKind,
		Name:     roleName,
	}
	return m.AddClusterRoleBinding(namespace, roleName, saName, roleRef)
}

// AddClusterPolicyRulesByComponent use to add PolicyRules to a ClusterRole. It also create the ClusterRoleBinding.
func (m *rbacManagerImpl) AddClusterPolicyRulesByComponent(namespace string, roleName string, saName string, policies []rbacv1.PolicyRule, component string) error {
	m.clusterRoleByComponent[component] = append(m.clusterRoleByComponent[component], roleName)
	return m.AddClusterPolicyRules(namespace, roleName, saName, policies)
}

// DeleteClusterRole is used to delete a ClusterRole and ClusterRoleBinding.
func (m *rbacManagerImpl) DeleteClusterRole(roleName string) error {
	found := m.store.Delete(kubernetes.ClusterRolesKind, "", roleName)
	if !found {
		return fmt.Errorf("unable to delete ClusterRole from the store because it was not found: %s/%s", "", roleName)
	}

	found = m.store.Delete(kubernetes.ClusterRoleBindingKind, "", roleName)
	if !found {
		return fmt.Errorf("unable to delete ClusterRoleBinding from the store because it was not found: %s/%s", "", roleName)
	}

	return nil
}

func (m *rbacManagerImpl) DeleteClusterRoleByComponent(component string) error {
	errs := make([]error, 0, len(m.clusterRoleByComponent[component]))
	for _, name := range m.clusterRoleByComponent[component] {
		errs = append(errs, m.DeleteClusterRole(name))
	}
	return errors.NewAggregate(errs)
}

// AddRoleBinding is used to create a standalone RoleBinding.
func (m *rbacManagerImpl) AddRoleBinding(roleNamespace, roleName, saNamespace, saName string, roleRef rbacv1.RoleRef) error {
	bindingObj, _ := m.store.GetOrCreate(kubernetes.RoleBindingKind, roleNamespace, roleName)
	roleBinding, ok := bindingObj.(*rbacv1.RoleBinding)
	if !ok {
		return fmt.Errorf("unable to get from the store the RoleBinding %s/%s", roleNamespace, roleName)
	}

	roleBinding.RoleRef = roleRef
	found := false
	for _, sub := range roleBinding.Subjects {
		if sub.Namespace == saNamespace && sub.Name == saName && sub.Kind == rbac.ServiceAccountKind {
			found = true
			break
		}
	}
	if !found {
		roleBinding.Subjects = append(roleBinding.Subjects, rbacv1.Subject{
			Kind:      rbac.ServiceAccountKind,
			Name:      saName,
			Namespace: saNamespace,
		})
	}
	if err := m.store.AddOrUpdate(kubernetes.RoleBindingKind, roleBinding); err != nil {
		return err
	}

	return nil
}

// AddClusterRoleBinding is used to create a standalone ClusterRoleBinding.
func (m *rbacManagerImpl) AddClusterRoleBinding(namespace string, roleName string, saName string, roleRef rbacv1.RoleRef) error {
	bindingObj, _ := m.store.GetOrCreate(kubernetes.ClusterRoleBindingKind, "", roleName)
	clusterRoleBinding, ok := bindingObj.(*rbacv1.ClusterRoleBinding)
	if !ok {
		return fmt.Errorf("unable to get from the store the ClusterRoleBinding %s/%s", namespace, roleName)
	}

	clusterRoleBinding.RoleRef = roleRef
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
	if err := m.store.AddOrUpdate(kubernetes.ClusterRoleBindingKind, clusterRoleBinding); err != nil {
		return err
	}

	return nil
}
