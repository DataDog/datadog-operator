// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controller

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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	byocdefaults "github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/defaults"
	byocimage "github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/image"
	byocresources "github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/resources"
	byocvalidation "github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/validation"
)

const (
	datadogBYOCClusterFinalizer = "finalizer.datadoghq.com/datadogbyoccluster"

	conditionReleaseResolved = "ReleaseResolved"
	conditionReconciled      = "Reconciled"
	conditionAvailable       = "Available"

	datadogBYOCClusterFieldOwner = "datadog-byoccluster-controller"
)

// DatadogBYOCClusterReconciler reconciles a DatadogBYOCCluster object.
type DatadogBYOCClusterReconciler struct {
	Client client.Client
	Scheme *runtime.Scheme

	ImageResolver byocimage.ImageResolver
}

// reconcileFailure is the condition reported as False when reconciliation fails.
type reconcileFailure struct {
	conditionType string
	reason        string
	err           error
}

// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogbyocclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogbyocclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogbyocclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=configmaps;serviceaccounts;services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments;statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete

// Reconcile resolves the requested release and converges all managed resources.
func (r *DatadogBYOCClusterReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
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
	available, failure := r.reconcileResources(ctx, cluster)
	setConditions(cluster, available, failure)

	var err error
	if failure != nil {
		err = failure.err
	}
	if statusErr := r.Client.Status().Update(ctx, cluster); statusErr != nil {
		return ctrl.Result{}, errors.Join(err, fmt.Errorf("update DatadogBYOCCluster status: %w", statusErr))
	}
	return ctrl.Result{}, err
}

// reconcileResources converges the managed resources and reports whether all workloads are available.
func (r *DatadogBYOCClusterReconciler) reconcileResources(ctx context.Context, cluster *datadoghqv1alpha1.DatadogBYOCCluster) (bool, *reconcileFailure) {
	if err := byocvalidation.ValidateClusterSpec(&cluster.Spec).ToAggregate(); err != nil {
		return false, &reconcileFailure{conditionReconciled, "InvalidConfiguration", err}
	}
	images, err := r.ImageResolver.Resolve(ctx, cluster.Spec.Release, cluster.Spec.ImageOverrides)
	if err != nil {
		return false, &reconcileFailure{conditionReleaseResolved, "ResolutionFailed", err}
	}
	setCondition(cluster, conditionReleaseResolved, metav1.ConditionTrue, "Resolved", "Workload images resolved successfully")

	resources, err := byocresources.BuildResources(cluster, images)
	if err != nil {
		return false, &reconcileFailure{conditionReconciled, "InvalidConfiguration", err}
	}
	for _, object := range resources.Objects() {
		if err := r.applyObject(ctx, cluster, object); err != nil {
			return false, &reconcileFailure{conditionReconciled, "ApplyFailed", fmt.Errorf("apply %T %s: %w", object, client.ObjectKeyFromObject(object), err)}
		}
	}
	for _, object := range resources.ObsoleteObjects() {
		if err := r.deleteIfControlled(ctx, cluster, object); err != nil {
			return false, &reconcileFailure{conditionReconciled, "CleanupFailed", err}
		}
	}
	// The applied objects hold the live status returned by the server-side apply.
	return updateComponentStatus(cluster, resources), nil
}

func (r *DatadogBYOCClusterReconciler) finalize(ctx context.Context, cluster *datadoghqv1alpha1.DatadogBYOCCluster) error {
	if !controllerutil.ContainsFinalizer(cluster, datadogBYOCClusterFinalizer) {
		return nil
	}
	indexer := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: byocresources.ComponentResourceName(cluster.Name, byocresources.IndexerComponentName), Namespace: cluster.Namespace}}
	err := r.Client.Delete(ctx, indexer)
	if err == nil {
		// The indexer deletion event requeues the cluster once the StatefulSet is gone.
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete indexer StatefulSet: %w", err)
	}
	controllerutil.RemoveFinalizer(cluster, datadogBYOCClusterFinalizer)
	if err := r.Client.Update(ctx, cluster); err != nil {
		return fmt.Errorf("remove finalizer: %w", err)
	}
	return nil
}

func (r *DatadogBYOCClusterReconciler) applyObject(ctx context.Context, owner *datadoghqv1alpha1.DatadogBYOCCluster, desired client.Object) error {
	if err := controllerutil.SetControllerReference(owner, desired, r.Scheme); err != nil {
		return err
	}
	gvk, err := apiutil.GVKForObject(desired, r.Scheme)
	if err != nil {
		return err
	}
	desired.GetObjectKind().SetGroupVersionKind(gvk)
	return r.Client.Patch(ctx, desired, client.Apply, client.ForceOwnership, client.FieldOwner(datadogBYOCClusterFieldOwner))
}

func (r *DatadogBYOCClusterReconciler) deleteIfControlled(ctx context.Context, owner client.Object, object client.Object) error {
	key := client.ObjectKeyFromObject(object)
	if err := r.Client.Get(ctx, key, object); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("get %T %s: %w", object, key, err)
	}
	if !metav1.IsControlledBy(object, owner) {
		return nil
	}
	if err := client.IgnoreNotFound(r.Client.Delete(ctx, object)); err != nil {
		return fmt.Errorf("delete %T %s: %w", object, key, err)
	}
	return nil
}

func setConditions(cluster *datadoghqv1alpha1.DatadogBYOCCluster, available bool, failure *reconcileFailure) {
	switch {
	case failure != nil:
		message := failure.err.Error()
		setCondition(cluster, failure.conditionType, metav1.ConditionFalse, failure.reason, message)
		if failure.conditionType == conditionReleaseResolved {
			setCondition(cluster, conditionReconciled, metav1.ConditionFalse, failure.reason, message)
		}
		setCondition(cluster, conditionAvailable, metav1.ConditionFalse, failure.reason, message)
	case available:
		setCondition(cluster, conditionReconciled, metav1.ConditionTrue, "Reconciled", "Managed resources match the desired state")
		setCondition(cluster, conditionAvailable, metav1.ConditionTrue, "Available", "All workloads are available")
	default:
		setCondition(cluster, conditionReconciled, metav1.ConditionTrue, "Reconciled", "Managed resources match the desired state")
		setCondition(cluster, conditionAvailable, metav1.ConditionFalse, "WorkloadsUnavailable", "One or more workloads are not yet available")
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

// updateComponentStatus reports the status of each enabled component and clears it for disabled ones.
func updateComponentStatus(cluster *datadoghqv1alpha1.DatadogBYOCCluster, resources *byocresources.Resources) bool {
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
	return allAvailable
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
func (r *DatadogBYOCClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&datadoghqv1alpha1.DatadogBYOCCluster{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&corev1.Service{}).
		Owns(&appsv1.Deployment{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&autoscalingv2.HorizontalPodAutoscaler{}).
		Owns(&policyv1.PodDisruptionBudget{}).
		Complete(r)
}
