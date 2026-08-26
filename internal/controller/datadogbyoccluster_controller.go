// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	byocrelease "github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/release"
	byocresources "github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/resources"
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
	Client   client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	ReleaseResolver byocrelease.ReleaseResolver
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
		return r.finalize(ctx, cluster)
	}
	if !controllerutil.ContainsFinalizer(cluster, datadogBYOCClusterFinalizer) {
		controllerutil.AddFinalizer(cluster, datadogBYOCClusterFinalizer)
		if err := r.Client.Update(ctx, cluster); err != nil {
			return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
		}
		return ctrl.Result{Requeue: true}, nil
	}

	if r.ReleaseResolver == nil {
		err := errors.New("release resolver is not configured")
		return ctrl.Result{}, r.fail(ctx, cluster, conditionReleaseResolved, "ResolverNotConfigured", err)
	}
	resolvedRelease, err := r.ReleaseResolver.Resolve(ctx, cluster.Spec.Release)
	if err != nil {
		return ctrl.Result{}, r.fail(ctx, cluster, conditionReleaseResolved, "ResolutionFailed", err)
	}
	r.setCondition(cluster, conditionReleaseResolved, metav1.ConditionTrue, "Resolved", "Release artifact resolved successfully")

	resources, err := byocresources.BuildResources(cluster, resolvedRelease)
	if err != nil {
		return ctrl.Result{}, r.fail(ctx, cluster, conditionReconciled, "InvalidConfiguration", err)
	}
	if err := r.applyResources(ctx, cluster, resources); err != nil {
		return ctrl.Result{}, r.fail(ctx, cluster, conditionReconciled, "ApplyFailed", err)
	}
	if err := r.deleteObsoleteResources(ctx, cluster, resources); err != nil {
		return ctrl.Result{}, r.fail(ctx, cluster, conditionReconciled, "CleanupFailed", err)
	}
	r.setCondition(cluster, conditionReconciled, metav1.ConditionTrue, "Reconciled", "Managed resources match the desired state")

	available := updateComponentStatus(cluster, resources)
	if available {
		r.setCondition(cluster, conditionAvailable, metav1.ConditionTrue, "Available", "All workloads are available")
	} else {
		r.setCondition(cluster, conditionAvailable, metav1.ConditionFalse, "WorkloadsUnavailable", "One or more workloads are not yet available")
	}
	if err := r.updateStatus(ctx, cluster); err != nil {
		return ctrl.Result{}, err
	}
	if !available {
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}
	return ctrl.Result{}, nil
}

func (r *DatadogBYOCClusterReconciler) finalize(ctx context.Context, cluster *datadoghqv1alpha1.DatadogBYOCCluster) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(cluster, datadogBYOCClusterFinalizer) {
		return ctrl.Result{}, nil
	}
	indexer := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: cluster.Name + "-indexer", Namespace: cluster.Namespace}}
	err := r.Client.Delete(ctx, indexer)
	if err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("delete indexer StatefulSet: %w", err)
	}
	if err == nil {
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}
	controllerutil.RemoveFinalizer(cluster, datadogBYOCClusterFinalizer)
	if err := r.Client.Update(ctx, cluster); err != nil {
		return ctrl.Result{}, fmt.Errorf("remove finalizer: %w", err)
	}
	return ctrl.Result{}, nil
}

func (r *DatadogBYOCClusterReconciler) applyResources(ctx context.Context, cluster *datadoghqv1alpha1.DatadogBYOCCluster, resources *byocresources.Resources) error {
	if err := r.applyObjects(ctx, cluster, resources.Shared()); err != nil {
		return err
	}
	if err := r.applyObjects(ctx, cluster, resources.Metastore().Objects()); err != nil {
		return err
	}
	if err := r.applyObjects(ctx, cluster, resources.Indexer().Objects()); err != nil {
		return err
	}
	if err := r.applyObjects(ctx, cluster, resources.Searcher().Objects()); err != nil {
		return err
	}
	if err := r.applyObjects(ctx, cluster, resources.ControlPlane().Objects()); err != nil {
		return err
	}
	if err := r.applyObjects(ctx, cluster, resources.Janitor().Objects()); err != nil {
		return err
	}
	if err := r.applyObjects(ctx, cluster, resources.ReadOnlyMetastore().Objects()); err != nil {
		return err
	}
	if err := r.applyObjects(ctx, cluster, resources.Compactor().Objects()); err != nil {
		return err
	}
	return nil
}

func (r *DatadogBYOCClusterReconciler) applyObjects(ctx context.Context, owner *datadoghqv1alpha1.DatadogBYOCCluster, objects []client.Object) error {
	for _, object := range objects {
		if err := r.applyObject(ctx, owner, object); err != nil {
			return err
		}
	}
	return nil
}

func (r *DatadogBYOCClusterReconciler) applyObject(ctx context.Context, owner *datadoghqv1alpha1.DatadogBYOCCluster, desired client.Object) error {
	if err := controllerutil.SetControllerReference(owner, desired, r.Scheme); err != nil {
		return wrapApplyError(desired, err)
	}
	gvk, err := apiutil.GVKForObject(desired, r.Scheme)
	if err != nil {
		return wrapApplyError(desired, err)
	}
	desired.GetObjectKind().SetGroupVersionKind(gvk)
	if err := r.Client.Patch(ctx, desired, client.Apply, client.ForceOwnership, client.FieldOwner(datadogBYOCClusterFieldOwner)); err != nil {
		return wrapApplyError(desired, err)
	}
	return nil
}

func (r *DatadogBYOCClusterReconciler) deleteObsoleteResources(ctx context.Context, cluster *datadoghqv1alpha1.DatadogBYOCCluster, resources *byocresources.Resources) error {
	if resources.ReadOnlyMetastore() == nil {
		if err := r.deleteDeploymentComponent(ctx, cluster, "metastore-ro"); err != nil {
			return err
		}
	}
	if resources.Compactor() == nil {
		if err := r.deleteDeploymentComponent(ctx, cluster, "compactor"); err != nil {
			return err
		}
	}
	if resources.Indexer().HPA == nil {
		if err := r.deleteHPA(ctx, cluster, "indexer"); err != nil {
			return err
		}
	}
	if resources.Searcher().HPA == nil {
		if err := r.deleteHPA(ctx, cluster, "searcher"); err != nil {
			return err
		}
	}
	desiredPodDisruptionBudgets := map[string]*policyv1.PodDisruptionBudget{
		"indexer":       resources.Indexer().PodDisruptionBudget,
		"searcher":      resources.Searcher().PodDisruptionBudget,
		"metastore":     resources.Metastore().PodDisruptionBudget,
		"control-plane": resources.ControlPlane().PodDisruptionBudget,
		"janitor":       resources.Janitor().PodDisruptionBudget,
	}
	if resources.ReadOnlyMetastore() != nil {
		desiredPodDisruptionBudgets["metastore-ro"] = resources.ReadOnlyMetastore().PodDisruptionBudget
	}
	if resources.Compactor() != nil {
		desiredPodDisruptionBudgets["compactor"] = resources.Compactor().PodDisruptionBudget
	}
	for _, component := range []string{"indexer", "searcher", "metastore", "control-plane", "janitor", "metastore-ro", "compactor"} {
		if desiredPodDisruptionBudgets[component] == nil {
			if err := r.deletePodDisruptionBudget(ctx, cluster, component); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *DatadogBYOCClusterReconciler) deleteDeploymentComponent(ctx context.Context, cluster *datadoghqv1alpha1.DatadogBYOCCluster, component string) error {
	name := byocresources.ComponentResourceName(cluster.Name, component)
	if err := deleteOwnedIfExists(ctx, r.Client, cluster, &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: cluster.Namespace}}); err != nil {
		return err
	}
	return deleteOwnedIfExists(ctx, r.Client, cluster, &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: cluster.Namespace}})
}

func (r *DatadogBYOCClusterReconciler) deleteHPA(ctx context.Context, cluster *datadoghqv1alpha1.DatadogBYOCCluster, component string) error {
	name := byocresources.ComponentResourceName(cluster.Name, component)
	return deleteOwnedIfExists(ctx, r.Client, cluster, &autoscalingv2.HorizontalPodAutoscaler{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: cluster.Namespace}})
}

func (r *DatadogBYOCClusterReconciler) deletePodDisruptionBudget(ctx context.Context, cluster *datadoghqv1alpha1.DatadogBYOCCluster, component string) error {
	podDisruptionBudget := &policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{
		Name:      byocresources.ComponentResourceName(cluster.Name, component),
		Namespace: cluster.Namespace,
	}}
	return deleteOwnedIfExists(ctx, r.Client, cluster, podDisruptionBudget)
}

func (r *DatadogBYOCClusterReconciler) fail(ctx context.Context, cluster *datadoghqv1alpha1.DatadogBYOCCluster, conditionType, reason string, reconcileErr error) error {
	r.setCondition(cluster, conditionType, metav1.ConditionFalse, reason, reconcileErr.Error())
	if conditionType == conditionReleaseResolved {
		r.setCondition(cluster, conditionReconciled, metav1.ConditionFalse, reason, reconcileErr.Error())
	}
	if conditionType != conditionAvailable {
		r.setCondition(cluster, conditionAvailable, metav1.ConditionFalse, reason, reconcileErr.Error())
	}
	if statusErr := r.updateStatus(ctx, cluster); statusErr != nil {
		return errors.Join(reconcileErr, statusErr)
	}
	return reconcileErr
}

func (r *DatadogBYOCClusterReconciler) setCondition(cluster *datadoghqv1alpha1.DatadogBYOCCluster, conditionType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: cluster.Generation,
	})
}

func (r *DatadogBYOCClusterReconciler) updateStatus(ctx context.Context, cluster *datadoghqv1alpha1.DatadogBYOCCluster) error {
	if err := r.Client.Status().Update(ctx, cluster); err != nil {
		return fmt.Errorf("update DatadogBYOCCluster status: %w", err)
	}
	return nil
}

func updateComponentStatus(cluster *datadoghqv1alpha1.DatadogBYOCCluster, resources *byocresources.Resources) bool {
	indexerStatus, indexerAvailable := statefulSetStatus(resources.Indexer().StatefulSet)
	cluster.Status.Indexer = indexerStatus
	searcherStatus, searcherAvailable := statefulSetStatus(resources.Searcher().StatefulSet)
	cluster.Status.Searcher = searcherStatus
	metastoreStatus, metastoreAvailable := deploymentStatus(resources.Metastore().Deployment)
	cluster.Status.Metastore = metastoreStatus
	controlPlaneStatus, controlPlaneAvailable := deploymentStatus(resources.ControlPlane().Deployment)
	cluster.Status.ControlPlane = controlPlaneStatus
	janitorStatus, janitorAvailable := deploymentStatus(resources.Janitor().Deployment)
	cluster.Status.Janitor = janitorStatus

	readOnlyMetastoreAvailable := true
	cluster.Status.ReadOnlyMetastore = nil
	if readOnlyMetastore := resources.ReadOnlyMetastore(); readOnlyMetastore != nil {
		readOnlyMetastoreStatus, available := deploymentStatus(readOnlyMetastore.Deployment)
		cluster.Status.ReadOnlyMetastore = readOnlyMetastoreStatus
		readOnlyMetastoreAvailable = available
	}

	compactorAvailable := true
	cluster.Status.Compactor = nil
	if compactor := resources.Compactor(); compactor != nil {
		compactorStatus, available := deploymentStatus(compactor.Deployment)
		cluster.Status.Compactor = compactorStatus
		compactorAvailable = available
	}

	return indexerAvailable && searcherAvailable && metastoreAvailable && controlPlaneAvailable && janitorAvailable && readOnlyMetastoreAvailable && compactorAvailable
}

func statefulSetStatus(statefulSet *appsv1.StatefulSet) (*datadoghqv1alpha1.DatadogBYOCClusterStatefulSetStatus, bool) {
	status := &datadoghqv1alpha1.DatadogBYOCClusterStatefulSetStatus{
		ObservedGeneration: ptr.To(statefulSet.Status.ObservedGeneration),
		Replicas:           ptr.To(statefulSet.Status.Replicas),
		ReadyReplicas:      ptr.To(statefulSet.Status.ReadyReplicas),
	}
	available := statefulSet.Status.ObservedGeneration >= statefulSet.Generation && statefulSet.Status.ReadyReplicas >= ptr.Deref(statefulSet.Spec.Replicas, 1)
	return status, available
}

func deploymentStatus(deployment *appsv1.Deployment) (*datadoghqv1alpha1.DatadogBYOCClusterDeploymentStatus, bool) {
	status := &datadoghqv1alpha1.DatadogBYOCClusterDeploymentStatus{
		Replicas:            ptr.To(deployment.Status.Replicas),
		ReadyReplicas:       ptr.To(deployment.Status.ReadyReplicas),
		UnavailableReplicas: ptr.To(deployment.Status.UnavailableReplicas),
		AvailableReplicas:   ptr.To(deployment.Status.AvailableReplicas),
	}
	available := deployment.Status.ObservedGeneration >= deployment.Generation && deployment.Status.AvailableReplicas >= ptr.Deref(deployment.Spec.Replicas, 1)
	return status, available
}

func deleteOwnedIfExists(ctx context.Context, kubeClient client.Client, owner client.Object, object client.Object) error {
	if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(object), object); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("get %T %s: %w", object, client.ObjectKeyFromObject(object), err)
	}
	if !metav1.IsControlledBy(object, owner) {
		return nil
	}
	if err := kubeClient.Delete(ctx, object); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete %T %s: %w", object, client.ObjectKeyFromObject(object), err)
	}
	return nil
}

func wrapApplyError(object client.Object, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("apply %T %s: %w", object, client.ObjectKeyFromObject(object), err)
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
