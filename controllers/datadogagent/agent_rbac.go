// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// manageAgentRBACs creates deletes and updates the RBACs for the Agent
func (r *Reconciler) manageAgentRBACs(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	if !datadoghqv1alpha1.BoolValue(dda.Spec.Agent.Enabled) {
		return r.cleanupAgentRbacResources(logger, dda)
	}

	if !isCreateRBACEnabled(dda.Spec.Agent.Rbac) {
		return reconcile.Result{}, nil
	}

	rbacResourcesName := getAgentRbacResourcesName(dda)
	agentVersion := getAgentVersion(dda)

	// Create or update ClusterRole
	if result, err := r.manageClusterRole(logger, dda, rbacResourcesName, agentVersion, r.createAgentClusterRole, r.updateIfNeededAgentClusterRole, false); err != nil {
		return result, err
	}

	// Create ServiceAccount
	serviceAccountName := getAgentServiceAccount(dda)
	serviceAccount := &corev1.ServiceAccount{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: serviceAccountName, Namespace: dda.Namespace}, serviceAccount); err != nil {
		if errors.IsNotFound(err) {
			return r.createServiceAccount(logger, dda, serviceAccountName, agentVersion)
		}
		return reconcile.Result{}, err
	}

	// Create ClusterRoleBinding
	return r.manageClusterRoleBinding(logger, dda, rbacResourcesName, agentVersion, r.createAgentClusterRoleBinding, r.updateIfNeedAgentClusterRoleBinding, false)
}

func (r *Reconciler) createAgentClusterRoleBinding(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string) (reconcile.Result, error) {
	newClusterRoleBinding := buildAgentClusterRoleBinding(dda, name, agentVersion)

	return r.createClusterRoleBinding(logger, dda, newClusterRoleBinding)
}

func (r *Reconciler) updateIfNeedAgentClusterRoleBinding(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string, clusterRoleBinding *rbacv1.ClusterRoleBinding) (reconcile.Result, error) {
	serviceAccountName := getAgentServiceAccount(dda)
	bindingInfo := roleBindingInfo{
		name:               name,
		roleName:           name,
		serviceAccountName: serviceAccountName,
	}
	newClusterRoleBinding := buildClusterRoleBinding(dda, bindingInfo, agentVersion)

	return r.updateIfNeededClusterRoleBindingRaw(logger, dda, clusterRoleBinding, newClusterRoleBinding)
}

// cleanupAgentRbacResources deletes ClusterRole, ClusterRoleBindings, and ServiceAccount of the Agent
func (r *Reconciler) cleanupAgentRbacResources(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	rbacResourcesName := getAgentRbacResourcesName(dda)

	// Delete ClusterRole
	if result, err := r.cleanupClusterRole(logger, dda, rbacResourcesName); err != nil {
		return result, err
	}

	// Delete Cluster Role Binding
	if result, err := r.cleanupClusterRoleBinding(logger, dda, rbacResourcesName); err != nil {
		return result, err
	}

	// Delete Service Account
	if result, err := r.cleanupServiceAccount(logger, r.client, dda, rbacResourcesName); err != nil {
		return result, err
	}

	return reconcile.Result{}, nil
}

func buildAgentClusterRoleBinding(dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string) *rbacv1.ClusterRoleBinding {
	serviceAccountName := getAgentServiceAccount(dda)
	bindingInfo := roleBindingInfo{
		name:               name,
		roleName:           name,
		serviceAccountName: serviceAccountName,
	}

	return buildClusterRoleBinding(dda, bindingInfo, agentVersion)
}
