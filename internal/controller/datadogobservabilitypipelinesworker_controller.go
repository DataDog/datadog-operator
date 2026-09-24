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
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	workerdefaults "github.com/DataDog/datadog-operator/internal/controller/datadogobservabilitypipelinesworker/defaults"
	workerresources "github.com/DataDog/datadog-operator/internal/controller/datadogobservabilitypipelinesworker/resources"
)

const datadogObservabilityPipelinesWorkerFieldOwner = "datadog-observability-pipelines-worker-controller"

// DatadogObservabilityPipelinesWorkerReconciler reconciles a DatadogObservabilityPipelinesWorker object.
type DatadogObservabilityPipelinesWorkerReconciler struct {
	Client   client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
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
	defaulted := workerdefaults.Apply(worker)

	resources, err := workerresources.BuildResources(defaulted)
	if err != nil {
		return ctrl.Result{}, r.fail(ctx, statusBase, worker, "InvalidConfiguration", err)
	}
	for _, object := range resources.Objects() {
		if err := r.applyObject(ctx, worker, object); err != nil {
			return ctrl.Result{}, r.fail(ctx, statusBase, worker, "ApplyFailed", err)
		}
	}
	if err := r.deleteObsoleteResources(ctx, worker, resources); err != nil {
		return ctrl.Result{}, r.fail(ctx, statusBase, worker, "CleanupFailed", err)
	}

	// Use the apply response so availability reflects the current generation and HPA replica target.
	statefulSet := resources.StatefulSet
	worker.Status.ObservedGeneration = new(worker.Generation)
	worker.Status.Replicas = new(statefulSet.Status.Replicas)
	worker.Status.ReadyReplicas = new(statefulSet.Status.ReadyReplicas)
	r.setCondition(worker, conditionReconciled, metav1.ConditionTrue, "Reconciled", "Managed resources match the desired state")

	_, available := statefulSetStatus(statefulSet)
	if available {
		r.setCondition(worker, conditionAvailable, metav1.ConditionTrue, "Available", "Worker workload is available")
	} else {
		r.setCondition(worker, conditionAvailable, metav1.ConditionFalse, "WorkloadUnavailable", "Worker workload is not yet available")
	}
	if err := r.updateStatus(ctx, statusBase, worker); err != nil {
		return ctrl.Result{}, err
	}
	if !available {
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}
	return ctrl.Result{}, nil
}

func (r *DatadogObservabilityPipelinesWorkerReconciler) applyObject(ctx context.Context, owner *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker, desired client.Object) error {
	if err := controllerutil.SetControllerReference(owner, desired, r.Scheme); err != nil {
		return wrapApplyError(desired, err)
	}
	gvk, err := apiutil.GVKForObject(desired, r.Scheme)
	if err != nil {
		return wrapApplyError(desired, err)
	}
	desired.GetObjectKind().SetGroupVersionKind(gvk)
	if err := r.Client.Patch(ctx, desired, client.Apply, client.ForceOwnership, client.FieldOwner(datadogObservabilityPipelinesWorkerFieldOwner)); err != nil {
		return wrapApplyError(desired, err)
	}
	return nil
}

func (r *DatadogObservabilityPipelinesWorkerReconciler) deleteObsoleteResources(ctx context.Context, worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker, resources *workerresources.Resources) error {
	for _, object := range resources.ObsoleteObjects() {
		if err := deleteOwnedIfExists(ctx, r.Client, worker, object); err != nil {
			return err
		}
	}
	return nil
}

func (r *DatadogObservabilityPipelinesWorkerReconciler) fail(ctx context.Context, statusBase, worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker, reason string, reconcileErr error) error {
	worker.Status.ObservedGeneration = new(worker.Generation)
	r.setCondition(worker, conditionReconciled, metav1.ConditionFalse, reason, reconcileErr.Error())
	r.setCondition(worker, conditionAvailable, metav1.ConditionFalse, reason, reconcileErr.Error())
	if statusErr := r.updateStatus(ctx, statusBase, worker); statusErr != nil {
		return errors.Join(reconcileErr, statusErr)
	}
	return reconcileErr
}

func (r *DatadogObservabilityPipelinesWorkerReconciler) setCondition(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker, conditionType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&worker.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: worker.Generation,
	})
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
