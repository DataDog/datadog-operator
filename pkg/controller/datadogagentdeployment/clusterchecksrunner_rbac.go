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

// manageClusterChecksRunnerRBACs creates deletes and updates the RBACs for the Cluster Checks runner
func (r *ReconcileDatadogAgentDeployment) manageClusterChecksRunnerRBACs(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment) (reconcile.Result, error) {
	if dad.Spec.ClusterChecksRunner == nil {
		return r.cleanupClusterChecksRunnerRbacResources(logger, dad)
	}

	if !isCreateRBACEnabled(dad.Spec.ClusterChecksRunner.Rbac) {
		return reconcile.Result{}, nil
	}

	rbacResourcesName := getClusterChecksRunnerRbacResourcesName(dad)
	clusterChecksRunnerVersion := getClusterChecksRunnerVersion(dad)

	// Create ClusterRoleBindig
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: rbacResourcesName}, clusterRoleBinding); err != nil {
		if errors.IsNotFound(err) {
			return r.createClusterRoleBinding(logger, dad, roleBindingInfo{
				name:               rbacResourcesName,
				roleName:           getAgentRbacResourcesName(dad),
				serviceAccountName: getClusterChecksRunnerServiceAccount(dad),
			}, clusterChecksRunnerVersion)
		}
		return reconcile.Result{}, err
	}

	// Create ServiceAccount
	serviceAccount := &corev1.ServiceAccount{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: rbacResourcesName, Namespace: dad.Namespace}, serviceAccount); err != nil {
		if errors.IsNotFound(err) {
			return r.createServiceAccount(logger, dad, rbacResourcesName, clusterChecksRunnerVersion)
		}
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

// cleanupAgentRbacResources deletes ClusterRoleBindings and ServiceAccount of the Cluster Checks Runner
func (r *ReconcileDatadogAgentDeployment) cleanupClusterChecksRunnerRbacResources(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment) (reconcile.Result, error) {
	rbacResourcesName := getClusterChecksRunnerRbacResourcesName(dad)

	// Delete Cluster Role Binding
	if result, err := cleanupClusterRoleBinding(r.client, rbacResourcesName); err != nil {
		return result, err
	}

	// Delete Service Account
	if result, err := cleanupServiceAccount(r.client, rbacResourcesName); err != nil {
		return result, err
	}

	return reconcile.Result{}, nil
}
