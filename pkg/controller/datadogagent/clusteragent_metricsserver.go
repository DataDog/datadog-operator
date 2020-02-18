// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadogagent

import (
	"context"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/go-logr/logr"
	rbacv1 "k8s.io/api/rbac/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// buildMetricsServerClusterRoleBinding creates a ClusterRoleBinding for the Cluster Agent HPA metrics server
func buildMetricsServerClusterRoleBinding(dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string) *rbacv1.ClusterRoleBinding {
	if isMetricsProviderEnabled(dda.Spec.ClusterAgent) {
		return &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Labels: getDefaultLabels(dda, name, agentVersion),
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
					Name:      getClusterAgentServiceAccount(dda),
					Namespace: dda.Namespace,
				},
			},
		}
	}
	return nil
}

func (r *ReconcileDatadogAgent) deleteIfNeededHpaClusterRoleBinding(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string, clusterRoleBinding *rbacv1.ClusterRoleBinding) (reconcile.Result, error) {
	newClusterRoleBinding := buildMetricsServerClusterRoleBinding(dda, name, agentVersion)
	if newClusterRoleBinding != nil && clusterRoleBinding != nil && !apiequality.Semantic.DeepEqual(&rbacv1.ClusterRoleBinding{}, clusterRoleBinding.Subjects) {
		// External Metrics Server used for HPA has been disabled
		// Delete its ClusterRoleBinding
		logger.V(1).Info("deleteClusterAgentHPARoleBinding", "clusterRoleBinding.name", clusterRoleBinding.Name)
		event := buildEventInfo(clusterRoleBinding.Name, clusterRoleBinding.Namespace, clusterRoleBindingKind, datadog.DeletionEvent)
		r.recordEvent(dda, event)
		if err := r.client.Delete(context.TODO(), clusterRoleBinding); err != nil {
			if errors.IsNotFound(err) {
				return reconcile.Result{}, nil
			}
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileDatadogAgent) createHPAClusterRoleBinding(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string) (reconcile.Result, error) {
	clusterRoleBinding := buildMetricsServerClusterRoleBinding(dda, name, agentVersion)
	if clusterRoleBinding == nil {
		return reconcile.Result{}, nil
	}
	if err := SetOwnerReference(dda, clusterRoleBinding, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	logger.V(1).Info("createClusterAgentHPARoleBinding", "clusterRoleBinding.name", clusterRoleBinding.Name)
	event := buildEventInfo(clusterRoleBinding.Name, clusterRoleBinding.Namespace, clusterRoleBindingKind, datadog.UpdateEvent)
	r.recordEvent(dda, event)
	return reconcile.Result{Requeue: true}, r.client.Create(context.TODO(), clusterRoleBinding)
}
