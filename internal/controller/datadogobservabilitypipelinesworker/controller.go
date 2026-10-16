// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package datadogobservabilitypipelinesworker implements the DatadogObservabilityPipelinesWorker controller. Like the
// DatadogBYOCCluster controller, it lives in its own package so its RBAC markers are generated into the BYOC
// ClusterRole (config/rbac/byoc) instead of manager-role.
package datadogobservabilitypipelinesworker

import (
	"context"
	"errors"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	byocutils "github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/utils"
	workerdefaults "github.com/DataDog/datadog-operator/internal/controller/datadogobservabilitypipelinesworker/defaults"
	workerresources "github.com/DataDog/datadog-operator/internal/controller/datadogobservabilitypipelinesworker/resources"
	workervalidation "github.com/DataDog/datadog-operator/internal/controller/datadogobservabilitypipelinesworker/validation"
)

const (
	conditionReconciled = "Reconciled"
	conditionAvailable  = "Available"

	reasonInvalidConfiguration = "InvalidConfiguration"
	reasonConflict             = "Conflict"
	reasonApplyFailed          = "ApplyFailed"
	reasonCleanupFailed        = "CleanupFailed"
	reasonReconciled           = "Reconciled"
	reasonAvailable            = "Available"
	reasonWorkloadUnavailable  = "WorkloadUnavailable"

	datadogObservabilityPipelinesWorkerFieldOwner = "datadog-observability-pipelines-worker-controller"
)

// Reconciler reconciles a DatadogObservabilityPipelinesWorker object.
type Reconciler struct {
	Client client.Client
	// APIReader reads directly from the API server for decisions that a stale cache must not drive.
	APIReader client.Reader
	Scheme    *runtime.Scheme
}

// reconcileFailure is the reason and error reported in the Reconciled and Available conditions when reconciliation fails.
type reconcileFailure struct {
	reason string
	err    error
	// terminal is set when retrying cannot succeed until the spec changes.
	terminal bool
}

// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogobservabilitypipelinesworkers,verbs=get;list;watch
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogobservabilitypipelinesworkers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogobservabilitypipelinesworkers/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=serviceaccounts;services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete

// Reconcile converges the Kubernetes resources managed by a Worker.
func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	worker := &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{}
	if err := r.Client.Get(ctx, request.NamespacedName, worker); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !worker.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	worker = workerdefaults.Apply(worker)
	available, failure := r.reconcileResources(ctx, worker)
	worker.Status.ObservedGeneration = new(worker.Generation)
	setConditions(worker, available, failure)

	var err error
	if failure != nil {
		err = failure.err
	}
	if statusErr := r.Client.Status().Update(ctx, worker); statusErr != nil {
		return ctrl.Result{}, errors.Join(err, fmt.Errorf("update DatadogObservabilityPipelinesWorker status: %w", statusErr))
	}
	if failure != nil && failure.terminal {
		return ctrl.Result{}, reconcile.TerminalError(err)
	}
	return ctrl.Result{}, err
}

// reconcileResources converges the managed resources and reports whether the Worker workload is available.
func (r *Reconciler) reconcileResources(ctx context.Context, worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) (bool, *reconcileFailure) {
	if err := workervalidation.ValidateWorkerSpec(&worker.Spec).ToAggregate(); err != nil {
		return false, &reconcileFailure{reason: reasonInvalidConfiguration, err: err, terminal: true}
	}
	resources, err := workerresources.BuildResources(worker)
	if err != nil {
		return false, &reconcileFailure{reason: reasonInvalidConfiguration, err: err, terminal: true}
	}
	for _, object := range resources.Objects() {
		if err := byocutils.ApplyControlled(ctx, r.Client, r.APIReader, r.Scheme, worker, object, datadogObservabilityPipelinesWorkerFieldOwner); err != nil {
			reason := reasonApplyFailed
			if errors.Is(err, byocutils.ErrObjectConflict) {
				reason = reasonConflict
			}
			return false, &reconcileFailure{reason: reason, err: fmt.Errorf("apply %T %s: %w", object, client.ObjectKeyFromObject(object), err)}
		}
	}
	for _, object := range resources.ObsoleteObjects() {
		if _, err := byocutils.DeleteIfControlled(ctx, r.Client, r.Client, worker, object); err != nil {
			return false, &reconcileFailure{reason: reasonCleanupFailed, err: err}
		}
	}

	// The applied StatefulSet holds the live status, so availability reflects the current generation and HPA replica target.
	statefulSet := resources.StatefulSet
	worker.Status.Replicas = new(statefulSet.Status.Replicas)
	worker.Status.ReadyReplicas = new(statefulSet.Status.ReadyReplicas)
	return statefulSet.Status.ObservedGeneration >= statefulSet.Generation && statefulSet.Status.ReadyReplicas >= ptr.Deref(statefulSet.Spec.Replicas, 1), nil
}

func setConditions(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker, available bool, failure *reconcileFailure) {
	switch {
	case failure != nil:
		message := failure.err.Error()
		setCondition(worker, conditionReconciled, metav1.ConditionFalse, failure.reason, message)
		setCondition(worker, conditionAvailable, metav1.ConditionFalse, failure.reason, message)
	case available:
		setCondition(worker, conditionReconciled, metav1.ConditionTrue, reasonReconciled, "Managed resources match the desired state")
		setCondition(worker, conditionAvailable, metav1.ConditionTrue, reasonAvailable, "Worker workload is available")
	default:
		setCondition(worker, conditionReconciled, metav1.ConditionTrue, reasonReconciled, "Managed resources match the desired state")
		setCondition(worker, conditionAvailable, metav1.ConditionFalse, reasonWorkloadUnavailable, "Worker workload is not yet available")
	}
}

func setCondition(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker, conditionType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&worker.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: worker.Generation,
	})
}

// SetupWithManager creates the DatadogObservabilityPipelinesWorker controller and its owned-resource watches.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&corev1.Service{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&autoscalingv2.HorizontalPodAutoscaler{}).
		Owns(&policyv1.PodDisruptionBudget{}).
		Complete(r)
}
