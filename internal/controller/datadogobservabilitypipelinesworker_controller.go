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
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	workerdefaults "github.com/DataDog/datadog-operator/internal/controller/datadogobservabilitypipelinesworker/defaults"
	workerresources "github.com/DataDog/datadog-operator/internal/controller/datadogobservabilitypipelinesworker/resources"
	workervalidation "github.com/DataDog/datadog-operator/internal/controller/datadogobservabilitypipelinesworker/validation"
)

const (
	reasonWorkloadUnavailable = "WorkloadUnavailable"

	datadogObservabilityPipelinesWorkerFieldOwner = "datadog-observability-pipelines-worker-controller"
)

// DatadogObservabilityPipelinesWorkerReconciler reconciles a DatadogObservabilityPipelinesWorker object.
type DatadogObservabilityPipelinesWorkerReconciler struct {
	Client client.Client
	// APIReader reads directly from the API server for decisions that a stale cache must not drive.
	APIReader client.Reader
	Scheme    *runtime.Scheme
}

// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogobservabilitypipelinesworkers,verbs=get;list;watch
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogobservabilitypipelinesworkers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=serviceaccounts;services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete

// Reconcile converges the Kubernetes resources managed by a Worker.
func (r *DatadogObservabilityPipelinesWorkerReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	worker := &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{}
	if err := r.Client.Get(ctx, request.NamespacedName, worker); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !worker.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	statusBase := worker.DeepCopy()
	available, failure := r.reconcileResources(ctx, worker)
	worker.Status.ObservedGeneration = new(worker.Generation)
	setWorkerConditions(worker, available, failure)

	var err error
	if failure != nil {
		err = failure.err
	}
	if statusErr := r.updateStatus(ctx, statusBase, worker); statusErr != nil {
		return ctrl.Result{}, errors.Join(err, statusErr)
	}
	if failure != nil && failure.terminal {
		return ctrl.Result{}, reconcile.TerminalError(err)
	}
	return ctrl.Result{}, err
}

// reconcileResources converges the managed resources and reports whether the Worker workload is available.
func (r *DatadogObservabilityPipelinesWorkerReconciler) reconcileResources(ctx context.Context, worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) (bool, *reconcileFailure) {
	// Keep the defaults out of worker so that the status patch contains only status changes.
	defaulted := workerdefaults.Apply(worker)
	if err := workervalidation.ValidateWorkerSpec(&defaulted.Spec).ToAggregate(); err != nil {
		return false, &reconcileFailure{conditionType: conditionReconciled, reason: reasonInvalidConfiguration, err: err, terminal: true}
	}
	resources, err := workerresources.BuildResources(defaulted)
	if err != nil {
		return false, &reconcileFailure{conditionType: conditionReconciled, reason: reasonInvalidConfiguration, err: err, terminal: true}
	}
	for _, object := range resources.Objects() {
		if err := r.applyObject(ctx, worker, object); err != nil {
			return false, applyFailure(object, err)
		}
	}
	for _, object := range resources.ObsoleteObjects() {
		if _, err := deleteIfControlled(ctx, r.Client, r.Client, worker, object); err != nil {
			return false, &reconcileFailure{conditionType: conditionReconciled, reason: reasonCleanupFailed, err: err}
		}
	}

	// The applied StatefulSet holds the live status, so availability reflects the current generation and HPA replica target.
	statefulSet := resources.StatefulSet
	worker.Status.Replicas = new(statefulSet.Status.Replicas)
	worker.Status.ReadyReplicas = new(statefulSet.Status.ReadyReplicas)
	_, available := statefulSetStatus(statefulSet)
	return available, nil
}

func (r *DatadogObservabilityPipelinesWorkerReconciler) applyObject(ctx context.Context, owner *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker, desired client.Object) error {
	if err := checkOwnership(ctx, r.Client, r.APIReader, owner, desired); err != nil {
		return err
	}
	return forceApply(ctx, r.Client, r.Scheme, owner, desired, datadogObservabilityPipelinesWorkerFieldOwner)
}

func (r *DatadogObservabilityPipelinesWorkerReconciler) updateStatus(ctx context.Context, base, worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) error {
	if apiequality.Semantic.DeepEqual(base.Status, worker.Status) {
		return nil
	}
	if err := r.Client.Status().Patch(ctx, worker, client.MergeFrom(base)); err != nil {
		return fmt.Errorf("update DatadogObservabilityPipelinesWorker status: %w", err)
	}
	return nil
}

func setWorkerConditions(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker, available bool, failure *reconcileFailure) {
	switch {
	case failure != nil:
		message := failure.err.Error()
		setWorkerCondition(worker, conditionReconciled, metav1.ConditionFalse, failure.reason, message)
		setWorkerCondition(worker, conditionAvailable, metav1.ConditionFalse, failure.reason, message)
	case available:
		setWorkerCondition(worker, conditionReconciled, metav1.ConditionTrue, reasonReconciled, "Managed resources match the desired state")
		setWorkerCondition(worker, conditionAvailable, metav1.ConditionTrue, reasonAvailable, "Worker workload is available")
	default:
		setWorkerCondition(worker, conditionReconciled, metav1.ConditionTrue, reasonReconciled, "Managed resources match the desired state")
		setWorkerCondition(worker, conditionAvailable, metav1.ConditionFalse, reasonWorkloadUnavailable, "Worker workload is not yet available")
	}
}

func setWorkerCondition(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker, conditionType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&worker.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: worker.Generation,
	})
}

// SetupWithManager creates the DatadogObservabilityPipelinesWorker controller and its owned-resource watches.
func (r *DatadogObservabilityPipelinesWorkerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&autoscalingv2.HorizontalPodAutoscaler{}).
		Owns(&policyv1.PodDisruptionBudget{}).
		Complete(r)
}
