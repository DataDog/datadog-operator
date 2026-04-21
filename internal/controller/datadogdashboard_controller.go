// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogdashboard"
	"github.com/DataDog/datadog-operator/pkg/config"
)

// DatadogDashboardReconciler reconciles a DatadogDashboard object
type DatadogDashboardReconciler struct {
	Client       client.Client
	CredsManager *config.CredentialManager
	Log          logr.Logger
	Scheme       *runtime.Scheme
	Recorder     record.EventRecorder
	internal     *datadogdashboard.Reconciler
}

//+kubebuilder:rbac:groups=datadoghq.com,resources=datadogdashboards,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=datadoghq.com,resources=datadogdashboards/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=datadoghq.com,resources=datadogdashboards/finalizers,verbs=update

// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.4/pkg/reconcile
func (r *DatadogDashboardReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.internal.Reconcile(ctx, req)
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatadogDashboardReconciler) SetupWithManager(mgr ctrl.Manager) error {
	internal, err := datadogdashboard.NewReconciler(r.Client, r.CredsManager, r.Scheme, r.Log, r.Recorder)
	if err != nil {
		return err
	}
	r.internal = internal

	builder := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.DatadogDashboard{}).
		WithEventFilter(predicate.GenerationChangedPredicate{})

	err = builder.Complete(r)

	if err != nil {
		return err
	}
	return nil
}
