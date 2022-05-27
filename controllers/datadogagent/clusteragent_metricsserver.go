// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
	"github.com/go-logr/logr"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// buildMetricsServerClusterRoleBinding creates a ClusterRoleBinding for the Cluster Agent HPA metrics server
func buildMetricsServerClusterRoleBinding(dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string) *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Labels: object.GetDefaultLabels(dda, name, agentVersion),
			Name:   name,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbac.RbacAPIGroup,
			Kind:     rbac.ClusterRoleKind,
			Name:     "system:auth-delegator",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbac.ServiceAccountKind,
				Name:      getClusterAgentServiceAccount(dda),
				Namespace: dda.Namespace,
			},
		},
	}
}

func (r *Reconciler) createHPAClusterRoleBinding(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string) (reconcile.Result, error) {
	clusterRoleBinding := buildMetricsServerClusterRoleBinding(dda, name, agentVersion)
	logger.V(1).Info("createClusterAgentHPARoleBinding", "clusterRoleBinding.name", clusterRoleBinding.Name)
	event := buildEventInfo(clusterRoleBinding.Name, clusterRoleBinding.Namespace, clusterRoleBindingKind, datadog.UpdateEvent)
	r.recordEvent(dda, event)
	return reconcile.Result{Requeue: true}, r.client.Create(context.TODO(), clusterRoleBinding)
}

func (r *Reconciler) updateIfNeededHPAClusterRole(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string, clusterRoleBinding *rbacv1.ClusterRoleBinding) (reconcile.Result, error) {
	newClusterRoleBinding := buildMetricsServerClusterRoleBinding(dda, name, agentVersion)
	return r.updateIfNeededClusterRoleBindingRaw(logger, dda, clusterRoleBinding, newClusterRoleBinding)
}

// buildExternalMetricsReaderClusterRoleBinding creates a ClusterRoleBinding for the HPA controller to be able to read external metrics
func buildExternalMetricsReaderClusterRoleBinding(dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string) *rbacv1.ClusterRoleBinding {
	if isMetricsProviderEnabled(dda.Spec.ClusterAgent) {
		return &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Labels: object.GetDefaultLabels(dda, name, agentVersion),
				Name:   name,
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: rbac.RbacAPIGroup,
				Kind:     rbac.ClusterRoleKind,
				Name:     name, // Cluster role has the same name as its binding
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      rbac.ServiceAccountKind,
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
	logger.V(1).Info("createExternalMetricsClusterRoleBinding", "clusterRoleBinding.name", clusterRoleBinding.Name)
	event := buildEventInfo(clusterRoleBinding.Name, clusterRoleBinding.Namespace, clusterRoleBindingKind, datadog.UpdateEvent)
	r.recordEvent(dda, event)
	return reconcile.Result{Requeue: true}, r.client.Create(context.TODO(), clusterRoleBinding)
}

func (r *Reconciler) updateIfNeededExternalMetricsReaderClusterRoleBinding(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string, clusterRoleBinding *rbacv1.ClusterRoleBinding) (reconcile.Result, error) {
	newClusterRoleBinding := buildExternalMetricsReaderClusterRoleBinding(dda, name, agentVersion)

	if newClusterRoleBinding == nil {
		return reconcile.Result{}, nil
	}

	return r.updateIfNeededClusterRoleBindingRaw(logger, dda, clusterRoleBinding, newClusterRoleBinding)
}

func (r *Reconciler) createExternalMetricsReaderClusterRole(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string) (reconcile.Result, error) {
	clusterRole := buildExternalMetricsReaderClusterRole(dda, name, agentVersion)
	if clusterRole == nil {
		return reconcile.Result{}, nil
	}
	logger.V(1).Info("createExternalMetricsClusterRole", "clusterRole.name", clusterRole.Name)
	event := buildEventInfo(clusterRole.Name, clusterRole.Namespace, clusterRoleKind, datadog.CreationEvent)
	r.recordEvent(dda, event)
	return reconcile.Result{Requeue: true}, r.client.Create(context.TODO(), clusterRole)
}

func (r *Reconciler) updateIfNeededExternalMetricsReaderClusterRole(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string, clusterRole *rbacv1.ClusterRole) (reconcile.Result, error) {
	newClusterRole := buildExternalMetricsReaderClusterRole(dda, name, agentVersion)
	if newClusterRole == nil {
		return reconcile.Result{}, nil
	}
	return r.updateIfNeededClusterRole(logger, dda, clusterRole, newClusterRole)
}

// buildExternalMetricsReaderClusterRole creates a ClusterRole object for access to external metrics resources
func buildExternalMetricsReaderClusterRole(dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string) *rbacv1.ClusterRole {
	if isMetricsProviderEnabled(dda.Spec.ClusterAgent) {
		clusterRole := &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Labels: object.GetDefaultLabels(dda, object.NewPartOfLabelValue(dda).String(), agentVersion),
				Name:   name,
			},
		}

		rbacRules := []rbacv1.PolicyRule{
			{
				APIGroups: []string{
					"external.metrics.k8s.io",
				},
				Resources: []string{"*"},
				Verbs: []string{
					rbac.GetVerb,
					rbac.ListVerb,
					rbac.WatchVerb,
				},
			},
		}

		// If the secret backend uses the provided `/readsecret_multiple_providers.sh` script, then we need to add secrets GET permissions
		if *dda.Spec.Credentials.UseSecretBackend &&
			checkSecretBackendMultipleProvidersUsed(dda.Spec.ClusterAgent.Config.Env) {
			rbacRules = append(rbacRules, rbacv1.PolicyRule{
				APIGroups: []string{rbac.CoreAPIGroup},
				Resources: []string{rbac.SecretsResource},
				Verbs:     []string{rbac.GetVerb},
			})
		}

		clusterRole.Rules = rbacRules
		return clusterRole
	}
	return nil
}
