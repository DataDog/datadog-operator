// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadogagent

import (
	"context"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/go-logr/logr"
	rbacv1 "k8s.io/api/rbac/v1"
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

func (r *Reconciler) createHPAClusterRoleBinding(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string) (reconcile.Result, error) {
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

// buildExternalMetricsReaderClusterRoleBinding creates a ClusterRoleBinding for the HPA controller to be able to read external metrics
func buildExternalMetricsReaderClusterRoleBinding(dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string) *rbacv1.ClusterRoleBinding {
	if isMetricsProviderEnabled(dda.Spec.ClusterAgent) {
		return &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Labels: getDefaultLabels(dda, name, agentVersion),
				Name:   name,
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: datadoghqv1alpha1.RbacAPIGroup,
				Kind:     datadoghqv1alpha1.ClusterRoleKind,
				Name:     name, // Cluster role has the same name as its binding
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      datadoghqv1alpha1.ServiceAccountKind,
					Name:      "horizontal-pod-autoscaler",
					Namespace: "kube-system",
				},
			},
		}
	}
	return nil
}

func (r *Reconciler) createExternalMetricsReaderClusterRoleBinding(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string) (reconcile.Result, error) {
	clusterRoleBinding := buildExternalMetricsReaderClusterRoleBinding(dda, name, agentVersion)
	if clusterRoleBinding == nil {
		return reconcile.Result{}, nil
	}
	if err := SetOwnerReference(dda, clusterRoleBinding, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	logger.V(1).Info("createExternalMetricsClusterRoleBinding", "clusterRoleBinding.name", clusterRoleBinding.Name)
	event := buildEventInfo(clusterRoleBinding.Name, clusterRoleBinding.Namespace, clusterRoleBindingKind, datadog.UpdateEvent)
	r.recordEvent(dda, event)
	return reconcile.Result{Requeue: true}, r.client.Create(context.TODO(), clusterRoleBinding)
}

func (r *Reconciler) createExternalMetricsReaderClusterRole(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string) (reconcile.Result, error) {
	clusterRole := buildExternalMetricsReaderClusterRole(dda, name, agentVersion)
	if clusterRole == nil {
		return reconcile.Result{}, nil
	}
	if err := SetOwnerReference(dda, clusterRole, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	logger.V(1).Info("createExternalMetricsClusterRole", "clusterRole.name", clusterRole.Name)
	event := buildEventInfo(clusterRole.Name, clusterRole.Namespace, clusterRoleKind, datadog.CreationEvent)
	r.recordEvent(dda, event)
	return reconcile.Result{Requeue: true}, r.client.Create(context.TODO(), clusterRole)
}

// buildExternalMetricsReaderClusterRole creates a ClusterRole object for access to external metrics resources
func buildExternalMetricsReaderClusterRole(dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string) *rbacv1.ClusterRole {
	if isMetricsProviderEnabled(dda.Spec.ClusterAgent) {
		return &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Labels: getDefaultLabels(dda, name, agentVersion),
				Name:   name,
			},
			Rules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{
						"external.metrics.k8s.io",
					},
					Resources: []string{"*"},
					Verbs: []string{
						datadoghqv1alpha1.GetVerb,
						datadoghqv1alpha1.ListVerb,
						datadoghqv1alpha1.WatchVerb,
					},
				},
			},
		}
	}
	return nil
}
