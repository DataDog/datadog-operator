// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlbuilder "sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogsyntheticsprivatelocation"
	"github.com/DataDog/datadog-operator/pkg/config"
)

// DatadogSyntheticsPrivateLocationReconciler reconciles a DatadogSyntheticsPrivateLocation object.
type DatadogSyntheticsPrivateLocationReconciler struct {
	Client       client.Client
	APIReader    client.Reader
	CredsManager *config.CredentialManager
	Scheme       *runtime.Scheme
	Recorder     record.EventRecorder
	internal     *datadogsyntheticsprivatelocation.Reconciler
}

// RBACs for DatadogSyntheticsPrivateLocation objects
//
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogsyntheticsprivatelocations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogsyntheticsprivatelocations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogsyntheticsprivatelocations/finalizers,verbs=update
//
// RBACs for worker resources owned by DatadogSyntheticsPrivateLocation
//
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete

// Reconcile loop for DatadogSyntheticsPrivateLocation.
func (r *DatadogSyntheticsPrivateLocationReconciler) Reconcile(ctx context.Context, instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation) (ctrl.Result, error) {
	return r.internal.Reconcile(ctx, instance)
}

// SetupWithManager creates a new DatadogSyntheticsPrivateLocation controller.
func (r *DatadogSyntheticsPrivateLocationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.internal = datadogsyntheticsprivatelocation.NewReconciler(r.Client, r.CredsManager, r.Scheme, ctrl.Log.WithName("controllers").WithName("DatadogSyntheticsPrivateLocation"), r.Recorder,
		datadogsyntheticsprivatelocation.ReconcilerOptions{APIReader: r.APIReader})

	or := reconcile.AsReconciler[*datadoghqv1alpha1.DatadogSyntheticsPrivateLocation](r.Client, r)
	// Predicates on For() only: spec and Datadog annotation changes on the
	// primary resource trigger reconciliation, while owned objects (notably
	// the worker Deployment) still reconcile on status changes.
	return ctrl.NewControllerManagedBy(mgr).
		For(&datadoghqv1alpha1.DatadogSyntheticsPrivateLocation{}, ctrlbuilder.WithPredicates(predicate.Or(predicate.GenerationChangedPredicate{}, datadogAnnotationChangedPredicate()))).
		Owns(&corev1.ServiceAccount{}).
		Owns(&corev1.Secret{}).
		Owns(&appsv1.Deployment{}).
		Complete(or)
}
