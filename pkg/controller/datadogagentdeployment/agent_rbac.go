// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadogagentdeployment

import (
	"context"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// manageAgentRBACs creates deletes and updates the RBACs for the Agent
func (r *ReconcileDatadogAgentDeployment) manageAgentRBACs(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment) (reconcile.Result, error) {
	if dad.Spec.Agent == nil {
		return r.cleanupAgentRbacResources(logger, dad)
	}

	if !isCreateRBACEnabled(dad.Spec.Agent.Rbac) {
		return reconcile.Result{}, nil
	}

	rbacResourcesName := getAgentRbacResourcesName(dad)
	agentVersion := getAgentVersion(dad)

	// Create or update ClusterRole
	clusterRole := &rbacv1.ClusterRole{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: rbacResourcesName}, clusterRole); err != nil {
		if errors.IsNotFound(err) {
			return r.createAgentClusterRole(logger, dad, rbacResourcesName, agentVersion)
		}
		return reconcile.Result{}, err
	}
	if result, err := r.updateIfNeededAgentClusterRole(logger, dad, rbacResourcesName, agentVersion, clusterRole); err != nil {
		return result, err
	}

	// Create ServiceAccount
	serviceAccountName := getAgentServiceAccount(dad)
	serviceAccount := &corev1.ServiceAccount{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: serviceAccountName, Namespace: dad.Namespace}, serviceAccount); err != nil {
		if errors.IsNotFound(err) {
			return r.createServiceAccount(logger, dad, serviceAccountName, agentVersion)
		}
		return reconcile.Result{}, err
	}

	// Create ClusterRoleBindig
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: rbacResourcesName}, clusterRoleBinding); err != nil {
		if errors.IsNotFound(err) {
			return r.createClusterRoleBinding(logger, dad, roleBindingInfo{
				name:               rbacResourcesName,
				roleName:           rbacResourcesName,
				serviceAccountName: serviceAccountName,
			}, agentVersion)
		}
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

// cleanupAgentRbacResources deletes ClusterRole, ClusterRoleBindings, and ServiceAccount of the Agent
func (r *ReconcileDatadogAgentDeployment) cleanupAgentRbacResources(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment) (reconcile.Result, error) {
	rbacResourcesName := getAgentRbacResourcesName(dad)

	// Delete ClusterRole
	if result, err := r.cleanupClusterRole(logger, r.client, dad, rbacResourcesName); err != nil {
		return result, err
	}

	// Delete Cluster Role Binding
	if result, err := r.cleanupClusterRoleBinding(logger, r.client, dad, rbacResourcesName); err != nil {
		return result, err
	}

	// Delete Service Account
	if result, err := r.cleanupServiceAccount(logger, r.client, dad, rbacResourcesName); err != nil {
		return result, err
	}

	return reconcile.Result{}, nil
}
