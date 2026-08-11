// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"maps"
	"time"

	edsv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

const workloadMigrationRequeuePeriod = time.Second

// migrateDaemonSetToExtendedDaemonSet retires a native DaemonSet before its
// replacement ExtendedDaemonSet is created. The old pods are labelled so the
// EDS controller sees them as the previous revision and replaces them according
// to its rolling-update policy instead of scheduling a second pod on every node.
//
// A DaemonSet cannot be paused. Switching it to OnDelete before orphaning it
// prevents the template-label update below from restarting healthy pods, while
// also ensuring that pods created for nodes joining during the migration carry
// the EDS discovery label.
func (r *Reconciler) migrateDaemonSetToExtendedDaemonSet(
	ctx context.Context,
	ddai *datadoghqv1alpha1.DatadogAgentInternal,
	eds *edsv1alpha1.ExtendedDaemonSet,
	newStatus *datadoghqv1alpha1.DatadogAgentInternalStatus,
) (bool, reconcile.Result, error) {
	logger := ctrl.LoggerFrom(ctx).WithValues(
		"migration", "DaemonSetToExtendedDaemonSet",
		"object.namespace", eds.Namespace,
		"object.name", eds.Name,
	)
	nsName := types.NamespacedName{Namespace: eds.Namespace, Name: eds.Name}
	currentDaemonSet := &appsv1.DaemonSet{}
	if err := r.client.Get(ctx, nsName, currentDaemonSet); err != nil {
		if !apierrors.IsNotFound(err) {
			return true, reconcile.Result{}, err
		}

		// If the target EDS already exists, its ReplicaSets are active owners and
		// must not be touched. This is the converged state (or the EDS portion of
		// a migration that has already advanced past the DaemonSet deletion).
		currentEDS := &edsv1alpha1.ExtendedDaemonSet{}
		if err := r.client.Get(ctx, nsName, currentEDS); err == nil {
			if currentEDS.DeletionTimestamp != nil {
				return true, workloadMigrationResult(), nil
			}
			return false, reconcile.Result{}, nil
		} else if !resourceIsAbsent(err) {
			return true, reconcile.Result{}, err
		}

		// A rollback can leave orphaned ERSs after the EDS itself has gone.
		// Remove that stale ownership layer before recreating the EDS; its pods
		// remain available and are discovered below by their EDS label.
		replicaSets := &edsv1alpha1.ExtendedDaemonSetReplicaSetList{}
		if err := r.client.List(
			ctx,
			replicaSets,
			client.InNamespace(eds.Namespace),
			client.MatchingLabels{edsv1alpha1.ExtendedDaemonSetNameLabelKey: eds.Name},
		); err != nil {
			if !resourceIsAbsent(err) {
				return true, reconcile.Result{}, err
			}
			replicaSets.Items = nil
		}
		if len(replicaSets.Items) > 0 {
			orphan := metav1.DeletePropagationOrphan
			for i := range replicaSets.Items {
				replicaSet := &replicaSets.Items[i]
				if replicaSet.DeletionTimestamp != nil {
					continue
				}
				if err := r.client.Delete(ctx, replicaSet, &client.DeleteOptions{PropagationPolicy: &orphan}); err != nil && !apierrors.IsNotFound(err) {
					return true, reconcile.Result{}, err
				}
				logger.Info("Removed stale ExtendedDaemonSetReplicaSet during rollback", "replicaset.name", replicaSet.Name)
			}
			return true, workloadMigrationResult(), nil
		}

		// Complete interrupted migrations where the DaemonSet was orphaned
		// after creating a pod from the old template but before labelling it.
		if err := r.labelOrphanedDaemonSetPodsForEDS(ctx, eds); err != nil {
			return true, reconcile.Result{}, err
		}
		return false, reconcile.Result{}, nil
	}

	if currentDaemonSet.DeletionTimestamp != nil {
		return true, workloadMigrationResult(), nil
	}

	if prepareDaemonSetForEDSMigration(currentDaemonSet, eds.Name) {
		logger.Info("Preparing DaemonSet for migration to ExtendedDaemonSet")
		if err := r.client.Update(ctx, currentDaemonSet); err != nil {
			return true, reconcile.Result{}, err
		}
		return true, workloadMigrationResult(), nil
	}

	if err := r.labelDaemonSetPodsForEDS(ctx, currentDaemonSet, eds.Name); err != nil {
		return true, reconcile.Result{}, err
	}

	orphan := metav1.DeletePropagationOrphan
	if err := r.client.Delete(ctx, currentDaemonSet, &client.DeleteOptions{PropagationPolicy: &orphan}); err != nil && !apierrors.IsNotFound(err) {
		return true, reconcile.Result{}, err
	}

	logger.Info("Orphaned DaemonSet pods for migration to ExtendedDaemonSet")
	event := buildEventInfo(currentDaemonSet.Name, currentDaemonSet.Namespace, kubernetes.DaemonSetKind, datadog.DeletionEvent)
	r.recordEvent(ddai, event)
	newStatus.Agent = nil

	return true, workloadMigrationResult(), nil
}

// migrateExtendedDaemonSetToDaemonSet removes the two-level EDS ownership
// chain (EDS -> ERS -> Pod) with orphan propagation. Once both owners are gone,
// the native DaemonSet controller can adopt the matching pods before applying
// its normal update policy. Pods that cannot match the target selector are
// deleted before the new DaemonSet is created so they cannot compete for node
// capacity.
func (r *Reconciler) migrateExtendedDaemonSetToDaemonSet(
	ctx context.Context,
	ddai *datadoghqv1alpha1.DatadogAgentInternal,
	daemonSet *appsv1.DaemonSet,
	newStatus *datadoghqv1alpha1.DatadogAgentInternalStatus,
) (bool, reconcile.Result, error) {
	logger := ctrl.LoggerFrom(ctx).WithValues(
		"migration", "ExtendedDaemonSetToDaemonSet",
		"object.namespace", daemonSet.Namespace,
		"object.name", daemonSet.Name,
	)
	nsName := types.NamespacedName{Namespace: daemonSet.Namespace, Name: daemonSet.Name}
	currentEDS := &edsv1alpha1.ExtendedDaemonSet{}
	if err := r.client.Get(ctx, nsName, currentEDS); err == nil {
		if currentEDS.DeletionTimestamp != nil {
			return true, workloadMigrationResult(), nil
		}

		orphan := metav1.DeletePropagationOrphan
		if deleteErr := r.client.Delete(ctx, currentEDS, &client.DeleteOptions{PropagationPolicy: &orphan}); deleteErr != nil && !apierrors.IsNotFound(deleteErr) {
			return true, reconcile.Result{}, deleteErr
		}

		logger.Info("Orphaned ExtendedDaemonSet ReplicaSets for migration to DaemonSet")
		event := buildEventInfo(currentEDS.Name, currentEDS.Namespace, kubernetes.ExtendedDaemonSetKind, datadog.DeletionEvent)
		r.recordEvent(ddai, event)
		newStatus.Agent = nil
		return true, workloadMigrationResult(), nil
	} else if !resourceIsAbsent(err) {
		return true, reconcile.Result{}, err
	}

	replicaSets := &edsv1alpha1.ExtendedDaemonSetReplicaSetList{}
	if err := r.client.List(
		ctx,
		replicaSets,
		client.InNamespace(daemonSet.Namespace),
		client.MatchingLabels{edsv1alpha1.ExtendedDaemonSetNameLabelKey: daemonSet.Name},
	); err != nil {
		if !resourceIsAbsent(err) {
			return true, reconcile.Result{}, err
		}
		replicaSets.Items = nil
	}

	if len(replicaSets.Items) > 0 {
		orphan := metav1.DeletePropagationOrphan
		for i := range replicaSets.Items {
			replicaSet := &replicaSets.Items[i]
			if replicaSet.DeletionTimestamp != nil {
				continue
			}
			if err := r.client.Delete(ctx, replicaSet, &client.DeleteOptions{PropagationPolicy: &orphan}); err != nil && !apierrors.IsNotFound(err) {
				return true, reconcile.Result{}, err
			}
			logger.Info("Orphaned ExtendedDaemonSetReplicaSet pods", "replicaset.name", replicaSet.Name)
		}
		return true, workloadMigrationResult(), nil
	}

	handled, err := r.prepareOrphanedEDSPodsForDaemonSet(ctx, daemonSet)
	if err != nil {
		return true, reconcile.Result{}, err
	}
	if handled {
		return true, workloadMigrationResult(), nil
	}

	return false, reconcile.Result{}, nil
}

func prepareDaemonSetForEDSMigration(daemonSet *appsv1.DaemonSet, edsName string) bool {
	needsUpdate := false
	if daemonSet.Spec.UpdateStrategy.Type != appsv1.OnDeleteDaemonSetStrategyType || daemonSet.Spec.UpdateStrategy.RollingUpdate != nil {
		daemonSet.Spec.UpdateStrategy = appsv1.DaemonSetUpdateStrategy{
			Type: appsv1.OnDeleteDaemonSetStrategyType,
		}
		needsUpdate = true
	}
	if daemonSet.Spec.Template.Labels == nil {
		daemonSet.Spec.Template.Labels = map[string]string{}
	}
	if daemonSet.Spec.Template.Labels[edsv1alpha1.ExtendedDaemonSetNameLabelKey] != edsName {
		daemonSet.Spec.Template.Labels[edsv1alpha1.ExtendedDaemonSetNameLabelKey] = edsName
		needsUpdate = true
	}
	return needsUpdate
}

func (r *Reconciler) labelDaemonSetPodsForEDS(ctx context.Context, daemonSet *appsv1.DaemonSet, edsName string) error {
	selector := labels.Everything()
	if daemonSet.Spec.Selector != nil {
		var err error
		selector, err = metav1.LabelSelectorAsSelector(daemonSet.Spec.Selector)
		if err != nil {
			return err
		}
	}

	pods := &corev1.PodList{}
	if err := r.client.List(
		ctx,
		pods,
		client.InNamespace(daemonSet.Namespace),
		client.MatchingLabelsSelector{Selector: selector},
	); err != nil {
		return err
	}

	for i := range pods.Items {
		pod := &pods.Items[i]
		controller := metav1.GetControllerOf(pod)
		if controller == nil ||
			controller.Kind != "DaemonSet" ||
			controller.Name != daemonSet.Name ||
			(daemonSet.UID != "" && controller.UID != daemonSet.UID) {
			continue
		}
		if err := r.setEDSPodLabel(ctx, pod, edsName); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reconciler) labelOrphanedDaemonSetPodsForEDS(ctx context.Context, eds *edsv1alpha1.ExtendedDaemonSet) error {
	selectorLabels := map[string]string{}
	for _, key := range []string{
		kubernetes.AppKubernetesInstanceLabelKey,
		apicommon.AgentDeploymentComponentLabelKey,
	} {
		if value := eds.Spec.Template.Labels[key]; value != "" {
			selectorLabels[key] = value
		}
	}
	if len(selectorLabels) == 0 {
		return nil
	}

	pods := &corev1.PodList{}
	if err := r.client.List(
		ctx,
		pods,
		client.InNamespace(eds.Namespace),
		client.MatchingLabels(selectorLabels),
	); err != nil {
		return err
	}

	for i := range pods.Items {
		pod := &pods.Items[i]
		controller := metav1.GetControllerOf(pod)
		if controller != nil && !(controller.Kind == "DaemonSet" && controller.Name == eds.Name) {
			continue
		}
		if err := r.setEDSPodLabel(ctx, pod, eds.Name); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reconciler) setEDSPodLabel(ctx context.Context, pod *corev1.Pod, edsName string) error {
	if pod.Labels[edsv1alpha1.ExtendedDaemonSetNameLabelKey] == edsName {
		return nil
	}
	updated := pod.DeepCopy()
	if updated.Labels == nil {
		updated.Labels = map[string]string{}
	}
	updated.Labels[edsv1alpha1.ExtendedDaemonSetNameLabelKey] = edsName
	return r.client.Patch(ctx, updated, client.MergeFrom(pod))
}

// prepareOrphanedEDSPodsForDaemonSet makes EDS pods match the native
// DaemonSet selector before it is created. Kubernetes then adopts them during
// the first DaemonSet reconciliation. Selectors that cannot be satisfied by
// adding MatchLabels fall back to deleting the old pod.
func (r *Reconciler) prepareOrphanedEDSPodsForDaemonSet(ctx context.Context, daemonSet *appsv1.DaemonSet) (bool, error) {
	selector, err := metav1.LabelSelectorAsSelector(daemonSet.Spec.Selector)
	if err != nil {
		return false, err
	}

	pods := &corev1.PodList{}
	if err := r.client.List(
		ctx,
		pods,
		client.InNamespace(daemonSet.Namespace),
		client.MatchingLabels{edsv1alpha1.ExtendedDaemonSetNameLabelKey: daemonSet.Name},
	); err != nil {
		return false, err
	}

	handled := false
	for i := range pods.Items {
		pod := &pods.Items[i]
		controller := metav1.GetControllerOf(pod)
		if controller != nil {
			if controller.Kind == "ExtendedDaemonSetReplicaSet" {
				// Orphan propagation has not removed the controller reference yet.
				handled = true
			}
			continue
		}
		if selector.Matches(labels.Set(pod.Labels)) {
			continue
		}

		updated := pod.DeepCopy()
		if updated.Labels == nil {
			updated.Labels = map[string]string{}
		}
		maps.Copy(updated.Labels, daemonSet.Spec.Selector.MatchLabels)
		if selector.Matches(labels.Set(updated.Labels)) {
			if err := r.client.Patch(ctx, updated, client.MergeFrom(pod)); err != nil {
				return true, err
			}
		} else {
			if err := r.client.Delete(ctx, pod); err != nil && !apierrors.IsNotFound(err) {
				return true, err
			}
		}
		handled = true
	}
	return handled, nil
}

func workloadMigrationResult() reconcile.Result {
	return reconcile.Result{RequeueAfter: workloadMigrationRequeuePeriod}
}

func resourceIsAbsent(err error) bool {
	return apierrors.IsNotFound(err) || apimeta.IsNoMatchError(err) || k8sruntime.IsNotRegisteredError(err)
}
