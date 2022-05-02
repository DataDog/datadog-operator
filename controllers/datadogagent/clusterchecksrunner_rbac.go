// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// manageClusterChecksRunnerRBACs creates deletes and updates the RBACs for the Cluster Checks runner
func (r *Reconciler) manageClusterChecksRunnerRBACs(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	if !apiutils.BoolValue(dda.Spec.ClusterChecksRunner.Enabled) {
		return r.cleanupClusterChecksRunnerRbacResources(logger, dda)
	}

	if !isCreateRBACEnabled(dda.Spec.ClusterChecksRunner.Rbac) {
		return reconcile.Result{}, nil
	}

	rbacResourcesName := getClusterChecksRunnerRbacResourcesName(dda)
	clusterChecksRunnerVersion := getClusterChecksRunnerVersion(dda)
	agentVersion := getAgentVersion(dda)

	// Create or update ClusterRole
	clusterRole := &rbacv1.ClusterRole{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: rbacResourcesName}, clusterRole); err != nil {
		if errors.IsNotFound(err) {
			return r.createClusterCheckRunnerClusterRole(logger, dda, rbacResourcesName, agentVersion)
		}
		return reconcile.Result{}, err
	}
	if result, err := r.updateIfNeededClusterCheckRunnerClusterRole(logger, dda, rbacResourcesName, agentVersion, clusterRole); err != nil {
		return result, err
	}

	// Create ServiceAccount
	serviceAccountName := getClusterChecksRunnerServiceAccount(dda)
	serviceAccount := &corev1.ServiceAccount{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: serviceAccountName, Namespace: dda.Namespace}, serviceAccount); err != nil {
		if errors.IsNotFound(err) {
			return r.createServiceAccount(logger, dda, serviceAccountName, agentVersion)
		}
		return reconcile.Result{}, err
	}

	// Create ClusterRoleBinding
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: rbacResourcesName}, clusterRoleBinding); err != nil {
		if errors.IsNotFound(err) {
			return r.createClusterRoleBindingFromInfo(logger, dda, roleBindingInfo{
				name:               rbacResourcesName,
				roleName:           rbacResourcesName,
				serviceAccountName: serviceAccountName,
			}, clusterChecksRunnerVersion)
		}
		return reconcile.Result{}, err
	}

	if result, err := r.updateIfNeededClusterRoleBinding(logger, dda, rbacResourcesName, rbacResourcesName, serviceAccountName, agentVersion, clusterRoleBinding); err != nil {
		return result, err
	}

	if isOrchestratorExplorerEnabled(dda) {
		if result, err := r.createOrUpdateOrchestratorCoreRBAC(logger, dda, serviceAccountName, clusterChecksRunnerVersion, checkRunnersSuffix); err != nil {
			return result, err
		}
	} else {
		if result, err := r.cleanupOrchestratorCoreRBAC(logger, dda, checkRunnersSuffix); err != nil {
			return result, err
		}
	}

	return reconcile.Result{}, nil
}

// cleanupAgentRbacResources deletes ClusterRoleBindings and ServiceAccount of the Cluster Checks Runner
func (r *Reconciler) cleanupClusterChecksRunnerRbacResources(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	rbacResourcesName := getClusterChecksRunnerRbacResourcesName(dda)

	// Delete Cluster Role
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
