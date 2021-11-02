// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"fmt"
	"strconv"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	kubeStateMetricsRBACPrefix = "ksm-core"
	ksmCoreCheckName           = "kubernetes_state_core.yaml.default"
	ksmCoreCheckFolderName     = "kubernetes_state_core.d"
)

// getKubeStateMetricsConfName get the name of the Configmap for the KSM Core check.
func getKubeStateMetricsConfName(dda *datadoghqv1alpha1.DatadogAgent) string {
	return GetConfName(dda, dda.Spec.Features.KubeStateMetricsCore.Conf, datadoghqv1alpha1.DefaultKubeStateMetricsCoreConf)
}

func ksmCheckConfig(clusteCheck bool) string {
	stringVal := strconv.FormatBool(clusteCheck)
	return fmt.Sprintf(`---
cluster_check: %s
init_config:
instances:
  - collectors:
    - pods
    - replicationcontrollers
    - statefulsets
    - nodes
    - cronjobs
    - jobs
    - replicasets
    - deployments
    - configmaps
    - services
    - endpoints
    - daemonsets
    - horizontalpodautoscalers
    - limitranges
    - resourcequotas
    - secrets
    - namespaces
    - persistentvolumeclaims
    - persistentvolumes
    telemetry: true
    skip_leader_election: %s
`, stringVal, stringVal)
}

func (r *Reconciler) manageKubeStateMetricsCore(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	if !isKSMCoreEnabled(dda) {
		return reconcile.Result{}, nil
	}
	// Only create the default ConfigMap if the conf is not overridden
	return r.manageConfigMap(logger, dda, getKubeStateMetricsConfName(dda), buildKSMCoreConfigMap)
}

func buildKSMCoreConfigMap(dda *datadoghqv1alpha1.DatadogAgent) (*corev1.ConfigMap, error) {
	// Only called if KSMCore is enabled
	if dda.Spec.Features.KubeStateMetricsCore.Conf != nil {
		return buildConfigurationConfigMap(dda, dda.Spec.Features.KubeStateMetricsCore.Conf, getKubeStateMetricsConfName(dda), ksmCoreCheckName)
	}
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        getKubeStateMetricsConfName(dda),
			Namespace:   dda.Namespace,
			Labels:      getDefaultLabels(dda, NewPartOfLabelValue(dda).String(), getAgentVersion(dda)),
			Annotations: getDefaultAnnotations(dda),
		},
		Data: map[string]string{
			ksmCoreCheckName: ksmCheckConfig(*dda.Spec.Features.KubeStateMetricsCore.ClusterCheck),
		},
	}
	return configMap, nil
}

func (r *Reconciler) createKubeStateMetricsClusterRole(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name, version string) (reconcile.Result, error) {
	clusterRole := buildKubeStateMetricsCoreRBAC(dda, name, version)
	logger.V(1).Info("createKubeStateMetricsClusterRole", "clusterRole.name", clusterRole.Name)
	event := buildEventInfo(clusterRole.Name, clusterRole.Namespace, clusterRoleKind, datadog.CreationEvent)
	r.recordEvent(dda, event)
	return reconcile.Result{Requeue: true}, r.client.Create(context.TODO(), clusterRole)
}

func (r *Reconciler) updateIfNeededKubeStateMetricsClusterRole(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name, version string, clusterRole *rbacv1.ClusterRole) (reconcile.Result, error) {
	newClusterRole := buildKubeStateMetricsCoreRBAC(dda, name, version)
	if !isClusterRolesEqual(newClusterRole, clusterRole) {
		logger.V(1).Info("updateKubeStateMetricsClusterRole", "clusterRole.name", clusterRole.Name)
		if err := kubernetes.UpdateFromObject(context.TODO(), r.client, newClusterRole, clusterRole.ObjectMeta); err != nil {
			return reconcile.Result{}, err
		}
		event := buildEventInfo(newClusterRole.Name, newClusterRole.Namespace, clusterRoleKind, datadog.UpdateEvent)
		r.recordEvent(dda, event)
	}
	return reconcile.Result{}, nil
}

func getKubeStateMetricsRBACResourceName(dda *datadoghqv1alpha1.DatadogAgent, suffix string) string {
	return fmt.Sprintf("%s-%s-%s-%s", dda.Namespace, dda.Name, kubeStateMetricsRBACPrefix, suffix)
}

func (r *Reconciler) createOrUpdateKubeStateMetricsCoreRBAC(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, serviceAccountName, componentVersion, nameSuffix string) (reconcile.Result, error) {
	kubeStateMetricsRBACName := getKubeStateMetricsRBACResourceName(dda, nameSuffix)
	kubeStateMetricsClusterRole := &rbacv1.ClusterRole{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: kubeStateMetricsRBACName}, kubeStateMetricsClusterRole); err != nil {
		if errors.IsNotFound(err) {
			return r.createKubeStateMetricsClusterRole(logger, dda, kubeStateMetricsRBACName, componentVersion)
		}
		return reconcile.Result{}, err
	}

	if result, err := r.updateIfNeededKubeStateMetricsClusterRole(logger, dda, kubeStateMetricsRBACName, componentVersion, kubeStateMetricsClusterRole); err != nil {
		return result, err
	}

	kubeStateMetricsClusterRoleBinding := &rbacv1.ClusterRoleBinding{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: kubeStateMetricsRBACName}, kubeStateMetricsClusterRoleBinding); err != nil {
		if errors.IsNotFound(err) {
			return r.createClusterRoleBindingFromInfo(logger, dda, roleBindingInfo{
				name:               kubeStateMetricsRBACName,
				roleName:           kubeStateMetricsRBACName,
				serviceAccountName: serviceAccountName,
			}, componentVersion)
		}
		return reconcile.Result{}, err
	}

	return r.updateIfNeededClusterRoleBinding(logger, dda, kubeStateMetricsRBACName, kubeStateMetricsRBACName, serviceAccountName, componentVersion, kubeStateMetricsClusterRoleBinding)
}

func (r *Reconciler) cleanupKubeStateMetricsCoreRBAC(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, nameSuffix string) (reconcile.Result, error) {
	kubeStateMetricsRBACName := getKubeStateMetricsRBACResourceName(dda, nameSuffix)

	result, err := r.cleanupClusterRoleBinding(logger, dda, kubeStateMetricsRBACName)
	if err != nil {
		return result, err
	}

	return r.cleanupClusterRole(logger, dda, kubeStateMetricsRBACName)
}

// buildKubeStateMetricsCoreRBAC generates the cluster role required for the KSM informers to query
// what is exposed as of the v2.0 https://github.com/kubernetes/kube-state-metrics/blob/release-2.0/examples/standard/cluster-role.yaml
func buildKubeStateMetricsCoreRBAC(dda *datadoghqv1alpha1.DatadogAgent, name, version string) *rbacv1.ClusterRole {
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      getDefaultLabels(dda, NewPartOfLabelValue(dda).String(), version),
			Annotations: getDefaultAnnotations(dda),
			Name:        name,
		},
	}

	rbacRules := []rbacv1.PolicyRule{
		{
			APIGroups: []string{datadoghqv1alpha1.CoreAPIGroup},
			Resources: []string{
				datadoghqv1alpha1.ConfigMapsResource,
				datadoghqv1alpha1.EndpointsResource,
				datadoghqv1alpha1.EventsResource,
				datadoghqv1alpha1.LimitRangesResource,
				datadoghqv1alpha1.NamespaceResource,
				datadoghqv1alpha1.NodesResource,
				datadoghqv1alpha1.PersistentVolumeClaimsResource,
				datadoghqv1alpha1.PersistentVolumesResource,
				datadoghqv1alpha1.PodsResource,
				datadoghqv1alpha1.ReplicationControllersResource,
				datadoghqv1alpha1.ResourceQuotasResource,
				datadoghqv1alpha1.SecretsResource,
				datadoghqv1alpha1.ServicesResource,
			},
		},
		{
			APIGroups: []string{datadoghqv1alpha1.ExtensionsAPIGroup},
			Resources: []string{
				datadoghqv1alpha1.DaemonsetsResource,
				datadoghqv1alpha1.DeploymentsResource,
				datadoghqv1alpha1.ReplicasetsResource,
			},
		},
		{
			APIGroups: []string{datadoghqv1alpha1.AppsAPIGroup},
			Resources: []string{
				datadoghqv1alpha1.DaemonsetsResource,
				datadoghqv1alpha1.DeploymentsResource,
				datadoghqv1alpha1.ReplicasetsResource,
				datadoghqv1alpha1.StatefulsetsResource,
			},
		},
		{
			APIGroups: []string{datadoghqv1alpha1.BatchAPIGroup},
			Resources: []string{
				datadoghqv1alpha1.CronjobsResource,
				datadoghqv1alpha1.JobsResource,
			},
		},
		{
			APIGroups: []string{datadoghqv1alpha1.AutoscalingAPIGroup},
			Resources: []string{
				datadoghqv1alpha1.HorizontalPodAutoscalersRecource,
			},
		},
		{
			APIGroups: []string{datadoghqv1alpha1.PolicyAPIGroup},
			Resources: []string{
				datadoghqv1alpha1.PodDisruptionBudgetsResource,
			},
		},
		{
			APIGroups: []string{datadoghqv1alpha1.CertificatesAPIGroup},
			Resources: []string{
				datadoghqv1alpha1.CertificatesSigningRequestsResource,
			},
		},
		{
			APIGroups: []string{datadoghqv1alpha1.StorageAPIGroup},
			Resources: []string{
				datadoghqv1alpha1.StorageClassesResource,
				datadoghqv1alpha1.VolumeAttachments,
			},
		},
		{
			APIGroups: []string{datadoghqv1alpha1.AdmissionAPIGroup},
			Resources: []string{
				datadoghqv1alpha1.MutatingConfigResource,
				datadoghqv1alpha1.ValidatingConfigResource,
			},
		},
		{
			APIGroups: []string{datadoghqv1alpha1.NetworkingAPIGroup},
			Resources: []string{
				datadoghqv1alpha1.IngressesResource,
				datadoghqv1alpha1.NetworkPolicyResource,
			},
		},
		{
			APIGroups: []string{datadoghqv1alpha1.CoordinationAPIGroup},
			Resources: []string{
				datadoghqv1alpha1.LeasesResource,
			},
		},
	}

	clusterRole.Rules = rbacRules
	commonVerbs := []string{
		datadoghqv1alpha1.ListVerb,
		datadoghqv1alpha1.WatchVerb,
	}

	for i := range clusterRole.Rules {
		clusterRole.Rules[i].Verbs = commonVerbs
	}

	return clusterRole
}
