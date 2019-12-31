// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadogagentdeployment

import (
	"context"
	"fmt"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// buildMetricsServerClusterRoleBinding creates a ClusterRoleBinding for the Cluster Agent HPA metrics server
func buildMetricsServerClusterRoleBinding(dad *datadoghqv1alpha1.DatadogAgentDeployment, name, agentVersion string) *rbacv1.ClusterRoleBinding {
	if isMetricsProviderEnabled(dad.Spec.ClusterAgent) {
		return &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Labels: getDefaultLabels(dad, name, agentVersion),
				Name:   name,
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: datadoghqv1alpha1.RbacAPIGroup,
				Kind:     datadoghqv1alpha1.ClusterRoleKind,
				Name:     "system:auth-delegator",
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      datadoghqv1alpha1.ServiceAccountKind,
					Name:      getClusterAgentServiceAccount(dad),
					Namespace: dad.Namespace,
				},
			},
		}
	}
	return nil
}

func (r *ReconcileDatadogAgentDeployment) deleteIfNeededHpaClusterRoleBinding(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, name, agentVersion string, clusterRoleBinding *rbacv1.ClusterRoleBinding) (reconcile.Result, error) {
	newClusterRoleBinding := buildMetricsServerClusterRoleBinding(dad, name, agentVersion)
	if newClusterRoleBinding != nil && clusterRoleBinding != nil && !apiequality.Semantic.DeepEqual(&rbacv1.ClusterRoleBinding{}, clusterRoleBinding.Subjects) {
		// External Metrics Server used for HPA has been disabled
		// Delete its ClusterRoleBinding
		logger.V(1).Info("deleteClusterAgentHPARoleBinding", "clusterRoleBinding.name", clusterRoleBinding.Name)
		r.recorder.Event(dad, corev1.EventTypeNormal, "Delete ClusterRoleBinding", fmt.Sprintf("%s/%s", clusterRoleBinding.Namespace, clusterRoleBinding.Name))
		if err := r.client.Delete(context.TODO(), clusterRoleBinding); err != nil {
			if errors.IsNotFound(err) {
				return reconcile.Result{}, nil
			}
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileDatadogAgentDeployment) createHPAClusterRoleBinding(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, name, agentVersion string) (reconcile.Result, error) {
	clusterRoleBinding := buildMetricsServerClusterRoleBinding(dad, name, agentVersion)
	if clusterRoleBinding == nil {
		return reconcile.Result{}, nil
	}
	if err := SetOwnerReference(dad, clusterRoleBinding, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	logger.V(1).Info("createClusterAgentHPARoleBinding", "clusterRoleBinding.name", clusterRoleBinding.Name)
	r.recorder.Event(dad, corev1.EventTypeNormal, "Update ClusterRoleBinding", fmt.Sprintf("%s/%s", clusterRoleBinding.Namespace, clusterRoleBinding.Name))
	return reconcile.Result{Requeue: true}, r.client.Create(context.TODO(), clusterRoleBinding)
}
