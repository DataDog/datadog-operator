// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"fmt"
	"strconv"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	orchestratorExplorerRBACPrefix      = "orchestrator-explorer"
	orchestratorExplorerCheckName       = "orchestrator.yaml"
	orchestratorExplorerCheckFolderName = "orchestrator.d"
)

func (r *Reconciler) manageOrchestratorExplorer(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	if !isOrchestratorExplorerEnabled(dda) {
		return reconcile.Result{}, nil
	}
	// Only create the default ConfigMap if the conf is not overridden
	return r.manageConfigMap(logger, dda, getOrchestratorExplorerConfName(dda), buildOrchestratorExplorerConfigMap)
}

func orchestratorExplorerCheckConfig(clusteCheck bool) string {
	stringClusterCheck := strconv.FormatBool(clusteCheck)
	return fmt.Sprintf(`---
cluster_check: %s
ad_identifiers:
  - _kube_orchestrator
init_config:

instances:
  - skip_leader_election: %s
`, stringClusterCheck, stringClusterCheck)
}

func buildOrchestratorExplorerConfigMap(dda *datadoghqv1alpha1.DatadogAgent) (*corev1.ConfigMap, error) {
	// Only called if OrchestratorExplorer or OrchestratorExplorer.ClusterCheck is enabled.
	if dda.Spec.Features.OrchestratorExplorer.Conf != nil {
		return buildConfigurationConfigMap(dda, dda.Spec.Features.OrchestratorExplorer.Conf, getOrchestratorExplorerConfName(dda), orchestratorExplorerCheckName)
	}
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        getOrchestratorExplorerConfName(dda),
			Namespace:   dda.Namespace,
			Labels:      getDefaultLabels(dda, dda.Name, getAgentVersion(dda)),
			Annotations: getDefaultAnnotations(dda),
		},
		Data: map[string]string{
			orchestratorExplorerCheckName: orchestratorExplorerCheckConfig(*dda.Spec.Features.OrchestratorExplorer.ClusterCheck),
		},
	}
	return configMap, nil
}

func (r *Reconciler) createOrchestratorExplorerClusterRole(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name, version string) (reconcile.Result, error) {
	clusterRole := buildOrchestratorExplorerRBAC(dda, name, version)
	logger.V(1).Info("createOrchestratorClusterRole", "clusterRole.name", clusterRole.Name)
	event := buildEventInfo(clusterRole.Name, clusterRole.Namespace, clusterRoleKind, datadog.CreationEvent)
	r.recordEvent(dda, event)
	return reconcile.Result{Requeue: true}, r.client.Create(context.TODO(), clusterRole)
}

func (r *Reconciler) updateIfNeededOrchestratorExplorerClusterRole(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name, version string, clusterRole *rbacv1.ClusterRole) (reconcile.Result, error) {
	newClusterRole := buildOrchestratorExplorerRBAC(dda, name, version)
	if !isClusterRolesEqual(newClusterRole, clusterRole) {
		logger.V(1).Info("updateOrchestratorClusterRole", "clusterRole.name", clusterRole.Name)
		if err := r.client.Update(context.TODO(), newClusterRole); err != nil {
			return reconcile.Result{}, err
		}
		event := buildEventInfo(newClusterRole.Name, newClusterRole.Namespace, clusterRoleKind, datadog.UpdateEvent)
		r.recordEvent(dda, event)
	}
	return reconcile.Result{}, nil
}

func getOrchestratorRBACResourceName(dda *datadoghqv1alpha1.DatadogAgent, suffix string) string {
	return fmt.Sprintf("%s-%s-%s-%s", dda.Namespace, dda.Name, orchestratorExplorerRBACPrefix, suffix)
}

func (r *Reconciler) createOrUpdateOrchestratorCoreRBAC(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, serviceAccountName, componentVersion, nameSuffix string) (reconcile.Result, error) {
	orchestratorRBACName := getOrchestratorRBACResourceName(dda, nameSuffix)
	orchestratorClusterRole := &rbacv1.ClusterRole{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: orchestratorRBACName}, orchestratorClusterRole); err != nil {
		if errors.IsNotFound(err) {
			return r.createOrchestratorExplorerClusterRole(logger, dda, orchestratorRBACName, componentVersion)
		}
		return reconcile.Result{}, err
	}

	if result, err := r.updateIfNeededOrchestratorExplorerClusterRole(logger, dda, orchestratorRBACName, componentVersion, orchestratorClusterRole); err != nil {
		return result, err
	}

	orchestratorClusterRoleBinding := &rbacv1.ClusterRoleBinding{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: orchestratorRBACName}, orchestratorClusterRoleBinding); err != nil {
		if errors.IsNotFound(err) {
			return r.createClusterRoleBindingFromInfo(logger, dda, roleBindingInfo{
				name:               orchestratorRBACName,
				roleName:           orchestratorRBACName,
				serviceAccountName: serviceAccountName,
			}, componentVersion)
		}
		return reconcile.Result{}, err
	}

	return r.updateIfNeededClusterRoleBinding(logger, dda, orchestratorRBACName, orchestratorRBACName, serviceAccountName, componentVersion, orchestratorClusterRoleBinding)
}

func (r *Reconciler) cleanupOrchestratorCoreRBAC(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, nameSuffix string) (reconcile.Result, error) {
	orchestratorRBACName := getOrchestratorRBACResourceName(dda, nameSuffix)

	result, err := r.cleanupClusterRoleBinding(logger, dda, orchestratorRBACName)
	if err != nil {
		return result, err
	}

	return r.cleanupClusterRole(logger, dda, orchestratorRBACName)
}

// buildOrchestratorExplorerRBAC generates the cluster role required for the KSM informers to query
// what is exposed as of the v2.0 https://github.com/kubernetes/kube-state-metrics/blob/release-2.0/examples/standard/cluster-role.yaml
func buildOrchestratorExplorerRBAC(dda *datadoghqv1alpha1.DatadogAgent, name, version string) *rbacv1.ClusterRole {
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      getDefaultLabels(dda, name, version),
			Annotations: getDefaultAnnotations(dda),
			Name:        name,
		},
	}

	rbacRules := []rbacv1.PolicyRule{
		// To get the kube-system namespace UID and generate a cluster ID
		{
			APIGroups:     []string{datadoghqv1alpha1.CoreAPIGroup},
			Resources:     []string{datadoghqv1alpha1.NamespaceResource},
			ResourceNames: []string{datadoghqv1alpha1.KubeSystemResourceName},
			Verbs:         []string{datadoghqv1alpha1.GetVerb},
		},
		// To create the cluster-id configmap
		{
			APIGroups:     []string{datadoghqv1alpha1.CoreAPIGroup},
			Resources:     []string{datadoghqv1alpha1.ConfigMapsResource},
			ResourceNames: []string{datadoghqv1alpha1.DatadogClusterIDResourceName},
			Verbs: []string{
				datadoghqv1alpha1.GetVerb,
				datadoghqv1alpha1.CreateVerb,
				datadoghqv1alpha1.UpdateVerb,
			},
		},
		{
			APIGroups: []string{datadoghqv1alpha1.CoreAPIGroup},
			Resources: []string{
				datadoghqv1alpha1.PodsResource,
				datadoghqv1alpha1.ServicesResource,
				datadoghqv1alpha1.NodesResource,
			},
		},
		{
			APIGroups: []string{datadoghqv1alpha1.AppsAPIGroup},
			Resources: []string{
				datadoghqv1alpha1.DeploymentsResource,
				datadoghqv1alpha1.ReplicasetsResource,
				datadoghqv1alpha1.DaemonsetsResource,
				datadoghqv1alpha1.StatefulsetsResource,
			},
		},
		{
			APIGroups: []string{datadoghqv1alpha1.BatchAPIGroup},
			Resources: []string{
				datadoghqv1alpha1.JobsResource,
				datadoghqv1alpha1.CronjobsResource,
			},
		},

		{
			APIGroups: []string{datadoghqv1alpha1.CoreAPIGroup},
			Resources: []string{
				datadoghqv1alpha1.PersistentVolumesResource,
				datadoghqv1alpha1.PersistentVolumeClaimsResource,
			},
		},
	}

	clusterRole.Rules = rbacRules
	defaultVerbs := []string{
		datadoghqv1alpha1.ListVerb,
		datadoghqv1alpha1.WatchVerb,
	}

	for i := range clusterRole.Rules {
		if clusterRole.Rules[i].Verbs == nil {
			// Add defaultVerbs only on Rules with no Verbs yet.
			clusterRole.Rules[i].Verbs = defaultVerbs
		}
	}

	return clusterRole
}

// getOrchestratorExplorerConfName get the name of the Configmap for the Orchestrator Core check.
func getOrchestratorExplorerConfName(dda *datadoghqv1alpha1.DatadogAgent) string {
	return GetConfName(dda, dda.Spec.Features.OrchestratorExplorer.Conf, datadoghqv1alpha1.DefaultOrchestratorExplorerConf)
}

func isOrchestratorExplorerClusterCheck(dda *datadoghqv1alpha1.DatadogAgent) bool {
	return isOrchestratorExplorerEnabled(dda) && datadoghqv1alpha1.BoolValue(dda.Spec.Features.OrchestratorExplorer.ClusterCheck)
}

func isOrchestratorExplorerEnabled(dda *datadoghqv1alpha1.DatadogAgent) bool {
	if dda.Spec.Features.OrchestratorExplorer == nil {
		return false
	}
	return datadoghqv1alpha1.BoolValue(dda.Spec.Features.OrchestratorExplorer.Enabled)
}
