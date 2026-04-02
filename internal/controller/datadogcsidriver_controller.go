// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"context"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlbuilder "sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogcsidriver"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// DatadogCSIDriverReconciler reconciles a DatadogCSIDriver object.
type DatadogCSIDriverReconciler struct {
	Client   client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	internal *datadogcsidriver.Reconciler
}

// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogcsidrivers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogcsidrivers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogcsidrivers/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=storage.k8s.io,resources=csidrivers,verbs=get;list;watch;create;update;patch;delete

// Reconcile loop for DatadogCSIDriver.
func (r *DatadogCSIDriverReconciler) Reconcile(ctx context.Context, instance *datadoghqv1alpha1.DatadogCSIDriver) (ctrl.Result, error) {
	return r.internal.Reconcile(ctx, instance)
}

// SetupWithManager creates a new DatadogCSIDriver controller.
func (r *DatadogCSIDriverReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.internal = datadogcsidriver.NewReconciler(r.Client, r.Scheme, r.Log, r.Recorder)

	or := reconcile.AsReconciler[*datadoghqv1alpha1.DatadogCSIDriver](r.Client, r)
	return ctrl.NewControllerManagedBy(mgr).
		// GenerationChangedPredicate on For() only — filters spec-only changes
		// on the primary resource without suppressing status updates on owned resources.
		For(&datadoghqv1alpha1.DatadogCSIDriver{}, ctrlbuilder.WithPredicates(predicate.GenerationChangedPredicate{})).
		// Owns DaemonSet — no predicate so we reconcile on status changes (pod readiness) too.
		Owns(&appsv1.DaemonSet{}).
		// CSIDriver is cluster-scoped so we can't use Owns(). Watch with label-based enqueue
		// to detect drift (manual edits, external deletions).
		Watches(&storagev1.CSIDriver{}, handler.EnqueueRequestsFromMapFunc(enqueueIfOwnedCSIDriver)).
		Complete(or)
}

// enqueueIfOwnedCSIDriver maps a CSIDriver change back to the owning DatadogCSIDriver CR
// using the app.kubernetes.io/part-of label and the app.kubernetes.io/managed-by label.
// If the label is not set, the object is not owned by a DatadogCSIDriver and we do nothing.
// If the label is set, we enqueue the owning DatadogCSIDriver CR.
func enqueueIfOwnedCSIDriver(_ context.Context, obj client.Object) []reconcile.Request {
	labels := obj.GetLabels()
	if labels[kubernetes.AppKubernetesManageByLabelKey] != "datadog-operator" {
		return nil
	}

	partOfLabelVal := object.PartOfLabelValue{Value: labels[kubernetes.AppKubernetesPartOfLabelKey]}
	owner := partOfLabelVal.NamespacedName()
	if owner.Name == "" {
		return nil
	}

	return []reconcile.Request{{NamespacedName: owner}}
}
