// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package datadogbyoccluster implements the DatadogBYOCCluster controller. It lives in its own package so its RBAC
// markers are generated into a dedicated ClusterRole (config/rbac/byoc) instead of manager-role, letting deployments
// grant BYOC permissions only when the controller is enabled.
package datadogbyoccluster

import (
	"context"
	"errors"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	byocdefaults "github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/defaults"
	byocimage "github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/image"
	byocresources "github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/resources"
	byocutils "github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/utils"
	byocvalidation "github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/validation"
)

const (
	datadogBYOCClusterFinalizer = "finalizer.datadoghq.com/datadogbyoccluster"

	conditionReleaseResolved = "ReleaseResolved"
	conditionReconciled      = "Reconciled"
	conditionAvailable       = "Available"

	reasonResolved             = "Resolved"
	reasonResolutionFailed     = "ResolutionFailed"
	reasonInvalidConfiguration = "InvalidConfiguration"
	reasonConflict             = "Conflict"
	reasonApplyFailed          = "ApplyFailed"
	reasonCleanupFailed        = "CleanupFailed"
	reasonCleanupInProgress    = "CleanupInProgress"
	reasonReconciled           = "Reconciled"
	reasonAvailable            = "Available"
	reasonWorkloadsUnavailable = "WorkloadsUnavailable"

	datadogBYOCClusterFieldOwner = "datadog-byoccluster-controller"
)

// Reconciler reconciles a DatadogBYOCCluster object.
type Reconciler struct {
	Client client.Client
	// APIReader reads directly from the API server for decisions that a stale cache must not drive.
	APIReader client.Reader
	Scheme    *runtime.Scheme

	ImageResolver byocimage.ImageResolver
}

// errWorkerDeleting reports a desired worker that is still being deleted.
var errWorkerDeleting = errors.New("worker is being deleted")

// reconcileFailure is the condition reported as False when reconciliation fails.
type reconcileFailure struct {
	conditionType string
	reason        string
	err           error
	// terminal is set when retrying cannot succeed until the spec changes.
	terminal bool
}

// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogbyocclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogbyocclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogbyocclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogobservabilitypipelinesworkers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps;serviceaccounts;services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments;statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete

// Reconcile resolves the requested release and converges all managed resources.
func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	cluster := &datadoghqv1alpha1.DatadogBYOCCluster{}
	if err := r.Client.Get(ctx, request.NamespacedName, cluster); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !cluster.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.finalize(ctx, cluster)
	}
	if controllerutil.AddFinalizer(cluster, datadogBYOCClusterFinalizer) {
		if err := r.Client.Update(ctx, cluster); err != nil {
			return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
		}
	}

	cluster = byocdefaults.Apply(cluster)
	available, deletingWorkers, failure := r.reconcileResources(ctx, cluster)
	setConditions(cluster, available, deletingWorkers, failure)

	var err error
	if failure != nil {
		err = failure.err
	}
	if statusErr := r.Client.Status().Update(ctx, cluster); statusErr != nil {
		return ctrl.Result{}, errors.Join(err, fmt.Errorf("update DatadogBYOCCluster status: %w", statusErr))
	}
	if failure != nil && failure.terminal {
		return ctrl.Result{}, reconcile.TerminalError(err)
	}
	return ctrl.Result{}, err
}

// reconcileResources converges the managed resources and workers. It reports whether all workloads are available
// and whether obsolete workers are still being deleted.
func (r *Reconciler) reconcileResources(ctx context.Context, cluster *datadoghqv1alpha1.DatadogBYOCCluster) (available, deletingWorkers bool, failure *reconcileFailure) {
	if err := byocvalidation.ValidateClusterSpec(&cluster.Spec).ToAggregate(); err != nil {
		return false, false, &reconcileFailure{conditionType: conditionReconciled, reason: reasonInvalidConfiguration, err: err, terminal: true}
	}
	images, err := r.ImageResolver.Resolve(ctx, cluster.Spec.Release, cluster.Spec.ImageOverrides)
	if err != nil {
		return false, false, &reconcileFailure{conditionType: conditionReleaseResolved, reason: reasonResolutionFailed, err: err}
	}
	setCondition(cluster, conditionReleaseResolved, metav1.ConditionTrue, reasonResolved, "Workload images resolved successfully")

	resources, err := byocresources.BuildResources(cluster, images)
	if err != nil {
		return false, false, &reconcileFailure{conditionType: conditionReconciled, reason: reasonInvalidConfiguration, err: err, terminal: true}
	}
	workers := make([]*datadoghqv1alpha1.DatadogObservabilityPipelinesWorker, 0, len(cluster.Spec.Components.Pipelines))
	for _, pipeline := range cluster.Spec.Components.Pipelines {
		worker, err := byocresources.BuildObservabilityPipelinesWorker(cluster, &pipeline, images.ObservabilityPipelinesWorker)
		if err != nil {
			return false, false, &reconcileFailure{conditionType: conditionReconciled, reason: reasonInvalidConfiguration, err: fmt.Errorf("pipeline %s: %w", pipeline.Name, err), terminal: true}
		}
		workers = append(workers, worker)
	}

	for _, object := range resources.Objects() {
		if err := byocutils.ApplyControlled(ctx, r.Client, r.APIReader, r.Scheme, cluster, object, datadogBYOCClusterFieldOwner); err != nil {
			return false, false, applyFailure(object, err)
		}
	}
	for _, worker := range workers {
		if err := r.applyWorker(ctx, cluster, worker); err != nil {
			return false, false, applyFailure(worker, err)
		}
	}
	for _, object := range resources.ObsoleteObjects() {
		if _, err := byocutils.DeleteIfControlled(ctx, r.Client, r.Client, cluster, object); err != nil {
			return false, false, &reconcileFailure{conditionType: conditionReconciled, reason: reasonCleanupFailed, err: err}
		}
	}
	deleting, cleanupErr := r.deleteObsoleteWorkers(ctx, r.Client, cluster, workers)
	if cleanupErr != nil {
		return false, false, &reconcileFailure{conditionType: conditionReconciled, reason: reasonCleanupFailed, err: cleanupErr}
	}
	// The applied objects hold the live status returned by the server-side apply.
	return updateComponentStatus(cluster, resources, workers), deleting, nil
}

func (r *Reconciler) finalize(ctx context.Context, cluster *datadoghqv1alpha1.DatadogBYOCCluster) error {
	if !controllerutil.ContainsFinalizer(cluster, datadogBYOCClusterFinalizer) {
		return nil
	}
	// The cache may not have observed recently created workers or indexer yet.
	deletingWorkers, err := r.deleteObsoleteWorkers(ctx, r.APIReader, cluster, nil)
	if err != nil {
		return err
	}
	if deletingWorkers {
		// The worker deletion events requeue the cluster once the workers are gone.
		return nil
	}
	indexer := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: byocresources.ComponentResourceName(cluster.Name, byocresources.IndexerComponentName), Namespace: cluster.Namespace}}
	// Foreground deletion keeps the StatefulSet until its pods terminate, so the resources they use outlive them.
	deleted, err := byocutils.DeleteIfControlled(ctx, r.Client, r.APIReader, cluster, indexer, client.PropagationPolicy(metav1.DeletePropagationForeground))
	if err != nil {
		return err
	}
	if deleted {
		// The indexer deletion event requeues the cluster once the StatefulSet is gone.
		return nil
	}
	controllerutil.RemoveFinalizer(cluster, datadogBYOCClusterFinalizer)
	if err := r.Client.Update(ctx, cluster); err != nil {
		return fmt.Errorf("remove finalizer: %w", err)
	}
	return nil
}

// applyWorker applies the desired worker unless the existing one is not controlled by the cluster or is being deleted.
func (r *Reconciler) applyWorker(ctx context.Context, cluster *datadoghqv1alpha1.DatadogBYOCCluster, desired *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) error {
	current := &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{ObjectMeta: metav1.ObjectMeta{Name: desired.Name, Namespace: desired.Namespace}}
	switch err := byocutils.GetFresh(ctx, r.Client, r.APIReader, current); {
	case apierrors.IsNotFound(err):
	case err != nil:
		return err
	case metav1.IsControlledBy(current, cluster) && !current.DeletionTimestamp.IsZero():
		return errWorkerDeleting
	}
	return byocutils.ApplyControlled(ctx, r.Client, r.APIReader, r.Scheme, cluster, desired, datadogBYOCClusterFieldOwner)
}

// deleteObsoleteWorkers deletes owned workers absent from desired and reports whether deletion is still pending.
// An empty desired list deletes all owned workers, including during cluster finalization.
// The workers are listed through reader.
func (r *Reconciler) deleteObsoleteWorkers(ctx context.Context, reader client.Reader, cluster *datadoghqv1alpha1.DatadogBYOCCluster, desired []*datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) (bool, error) {
	wanted := make(map[string]struct{}, len(desired))
	for _, worker := range desired {
		wanted[worker.Name] = struct{}{}
	}
	workers := &datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerList{}
	if err := reader.List(ctx, workers, client.InNamespace(cluster.Namespace)); err != nil {
		return false, fmt.Errorf("list DatadogObservabilityPipelinesWorkers: %w", err)
	}
	deleting := false
	for i := range workers.Items {
		worker := &workers.Items[i]
		if !metav1.IsControlledBy(worker, cluster) {
			continue
		}
		if _, keep := wanted[worker.Name]; keep {
			continue
		}
		deleting = true
		if !worker.DeletionTimestamp.IsZero() {
			continue
		}
		if err := r.Client.Delete(ctx, worker, client.PropagationPolicy(metav1.DeletePropagationForeground), client.Preconditions{
			UID:             &worker.UID,
			ResourceVersion: &worker.ResourceVersion,
		}); err != nil && !apierrors.IsNotFound(err) {
			return deleting, fmt.Errorf("delete DatadogObservabilityPipelinesWorker %s: %w", client.ObjectKeyFromObject(worker), err)
		}
	}
	return deleting, nil
}

// applyFailure reports a failure to apply object, distinguishing objects with a managed name that are not controlled.
func applyFailure(object client.Object, err error) *reconcileFailure {
	reason := reasonApplyFailed
	if errors.Is(err, byocutils.ErrObjectConflict) {
		reason = reasonConflict
	}
	return &reconcileFailure{conditionType: conditionReconciled, reason: reason, err: fmt.Errorf("apply %T %s: %w", object, client.ObjectKeyFromObject(object), err)}
}

func setConditions(cluster *datadoghqv1alpha1.DatadogBYOCCluster, available, deletingWorkers bool, failure *reconcileFailure) {
	if failure != nil {
		message := failure.err.Error()
		setCondition(cluster, failure.conditionType, metav1.ConditionFalse, failure.reason, message)
		if failure.conditionType == conditionReleaseResolved {
			setCondition(cluster, conditionReconciled, metav1.ConditionFalse, failure.reason, message)
		}
		setCondition(cluster, conditionAvailable, metav1.ConditionFalse, failure.reason, message)
		return
	}
	if deletingWorkers {
		setCondition(cluster, conditionReconciled, metav1.ConditionFalse, reasonCleanupInProgress, "Waiting for obsolete workers to be deleted")
	} else {
		setCondition(cluster, conditionReconciled, metav1.ConditionTrue, reasonReconciled, "Managed resources match the desired state")
	}
	if available {
		setCondition(cluster, conditionAvailable, metav1.ConditionTrue, reasonAvailable, "All workloads are available")
	} else {
		setCondition(cluster, conditionAvailable, metav1.ConditionFalse, reasonWorkloadsUnavailable, "One or more workloads are not yet available")
	}
}

func setCondition(cluster *datadoghqv1alpha1.DatadogBYOCCluster, conditionType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: cluster.Generation,
	})
}

// updateComponentStatus reports the status of each enabled component and worker, and clears it for disabled components.
func updateComponentStatus(cluster *datadoghqv1alpha1.DatadogBYOCCluster, resources *byocresources.Resources, workers []*datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) bool {
	status := &cluster.Status
	statefulSets := []struct {
		name   string
		status **datadoghqv1alpha1.DatadogBYOCClusterStatefulSetStatus
	}{
		{byocresources.IndexerComponentName, &status.Indexer},
		{byocresources.SearcherComponentName, &status.Searcher},
	}
	deployments := []struct {
		name   string
		status **datadoghqv1alpha1.DatadogBYOCClusterDeploymentStatus
	}{
		{byocresources.MetastoreComponentName, &status.Metastore},
		{byocresources.ReadOnlyMetastoreComponentName, &status.ReadOnlyMetastore},
		{byocresources.ControlPlaneComponentName, &status.ControlPlane},
		{byocresources.CompactorComponentName, &status.Compactor},
		{byocresources.JanitorComponentName, &status.Janitor},
	}

	allAvailable := true
	for _, s := range statefulSets {
		*s.status = nil
		if component := resources.Component(s.name); component != nil {
			componentStatus, available := statefulSetStatus(component.StatefulSet)
			*s.status = componentStatus
			allAvailable = allAvailable && available
		}
	}
	for _, d := range deployments {
		*d.status = nil
		if component := resources.Component(d.name); component != nil {
			componentStatus, available := deploymentStatus(component.Deployment)
			*d.status = componentStatus
			allAvailable = allAvailable && available
		}
	}
	status.Pipelines = make([]datadoghqv1alpha1.DatadogBYOCClusterPipelineStatus, len(workers))
	for i, worker := range workers {
		status.Pipelines[i] = datadoghqv1alpha1.DatadogBYOCClusterPipelineStatus{
			Name:       cluster.Spec.Components.Pipelines[i].Name,
			WorkerName: worker.Name,
		}
		allAvailable = allAvailable && isWorkerAvailable(worker)
	}
	return allAvailable
}

func isWorkerAvailable(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) bool {
	condition := meta.FindStatusCondition(worker.Status.Conditions, conditionAvailable)
	return worker.DeletionTimestamp.IsZero() &&
		condition != nil && condition.ObservedGeneration == worker.Generation && condition.Status == metav1.ConditionTrue
}

func statefulSetStatus(statefulSet *appsv1.StatefulSet) (*datadoghqv1alpha1.DatadogBYOCClusterStatefulSetStatus, bool) {
	status := &datadoghqv1alpha1.DatadogBYOCClusterStatefulSetStatus{
		ObservedGeneration: new(statefulSet.Status.ObservedGeneration),
		Replicas:           new(statefulSet.Status.Replicas),
		ReadyReplicas:      new(statefulSet.Status.ReadyReplicas),
	}
	available := statefulSet.Status.ObservedGeneration >= statefulSet.Generation && statefulSet.Status.ReadyReplicas >= ptr.Deref(statefulSet.Spec.Replicas, 1)
	return status, available
}

func deploymentStatus(deployment *appsv1.Deployment) (*datadoghqv1alpha1.DatadogBYOCClusterDeploymentStatus, bool) {
	status := &datadoghqv1alpha1.DatadogBYOCClusterDeploymentStatus{
		Replicas:            new(deployment.Status.Replicas),
		ReadyReplicas:       new(deployment.Status.ReadyReplicas),
		UnavailableReplicas: new(deployment.Status.UnavailableReplicas),
		AvailableReplicas:   new(deployment.Status.AvailableReplicas),
	}
	available := deployment.Status.ObservedGeneration >= deployment.Generation && deployment.Status.AvailableReplicas >= ptr.Deref(deployment.Spec.Replicas, 1)
	return status, available
}

// SetupWithManager creates the DatadogBYOCCluster controller and its owned-resource watches.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// Status updates do not bump the generation, so this skips requeues caused by the controller's own status writes.
		// Owned resources stay unfiltered: workload status changes drive availability and drift must still be detected.
		For(&datadoghqv1alpha1.DatadogBYOCCluster{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Owns(&datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&corev1.Service{}).
		Owns(&appsv1.Deployment{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&autoscalingv2.HorizontalPodAutoscaler{}).
		Owns(&policyv1.PodDisruptionBudget{}).
		Complete(r)
}
