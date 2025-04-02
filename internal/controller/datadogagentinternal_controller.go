// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package controller

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	ddai "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal"
)

// DatadogAgentInternalReconciler reconciles a DatadogAgentInternal object.
type DatadogAgentInternalReconciler struct {
	Client   client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	internal *ddai.Reconciler
}

// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogagentinternals,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogagentinternals/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogagentinternals/finalizers,verbs=get;list;watch;create;update;patch;delete

// Reconcile loop for DatadogAgentInternal.
func (r *DatadogAgentInternalReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.internal.Reconcile(ctx, req)
}

// SetupWithManager creates a new DatadogAgentInternal controller.
func (r *DatadogAgentInternalReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.internal = ddai.NewReconciler(r.Client, r.Scheme, r.Log)

	builder := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.DatadogAgentInternal{})
		// TODO: Possibly only watch for spec changes, not status changes.
		// .WithEventFilter(predicate.GenerationChangedPredicate{})

	err := builder.Complete(r)
	if err != nil {
		return err
	}

	return nil
}
