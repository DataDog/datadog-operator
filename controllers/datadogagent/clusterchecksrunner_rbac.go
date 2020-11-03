// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

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

// manageClusterChecksRunnerRBACs creates deletes and updates the RBACs for the Cluster Checks runner
func (r *Reconciler) manageClusterChecksRunnerRBACs(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	if dda.Spec.ClusterChecksRunner == nil {
		return r.cleanupClusterChecksRunnerRbacResources(logger, dda)
	}

	if !isCreateRBACEnabled(dda.Spec.ClusterChecksRunner.Rbac) {
		return reconcile.Result{}, nil
	}

	rbacResourcesName := getClusterChecksRunnerRbacResourcesName(dda)
	clusterChecksRunnerVersion := getClusterChecksRunnerVersion(dda)

	// Create ClusterRoleBinding
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: rbacResourcesName}, clusterRoleBinding); err != nil {
		if errors.IsNotFound(err) {
			return r.createClusterRoleBinding(logger, dda, roleBindingInfo{
				name:               rbacResourcesName,
				roleName:           getAgentRbacResourcesName(dda),
				serviceAccountName: getClusterChecksRunnerServiceAccount(dda),
			}, clusterChecksRunnerVersion)
		}
		return reconcile.Result{}, err
	}

	// Create ServiceAccount
	serviceAccount := &corev1.ServiceAccount{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: rbacResourcesName, Namespace: dda.Namespace}, serviceAccount); err != nil {
		if errors.IsNotFound(err) {
			return r.createServiceAccount(logger, dda, rbacResourcesName, clusterChecksRunnerVersion)
		}
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

// cleanupAgentRbacResources deletes ClusterRoleBindings and ServiceAccount of the Cluster Checks Runner
func (r *Reconciler) cleanupClusterChecksRunnerRbacResources(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	rbacResourcesName := getClusterChecksRunnerRbacResourcesName(dda)

	// Delete Cluster Role Binding
	if result, err := r.cleanupClusterRoleBinding(logger, r.client, dda, rbacResourcesName); err != nil {
		return result, err
	}

	// Delete Service Account
	if result, err := r.cleanupServiceAccount(logger, r.client, dda, rbacResourcesName); err != nil {
		return result, err
	}

	return reconcile.Result{}, nil
}
