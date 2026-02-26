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

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"
	"github.com/DataDog/datadog-operator/internal/controller/datadogdashboard"
	"github.com/DataDog/datadog-operator/pkg/config"
)

// DatadogDashboardV1Alpha2Reconciler reconciles a DatadogDashboard v1alpha2 object
type DatadogDashboardV1Alpha2Reconciler struct {
	Client   client.Client
	Creds    config.Creds
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	internal *datadogdashboard.Reconciler
}

//+kubebuilder:rbac:groups=datadoghq.com,resources=datadogdashboards,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=datadoghq.com,resources=datadogdashboards/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=datadoghq.com,resources=datadogdashboards/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *DatadogDashboardV1Alpha2Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.internal.Reconcile(ctx, req)
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatadogDashboardV1Alpha2Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	internal, err := datadogdashboard.NewReconciler(r.Client, r.Creds, r.Scheme, r.Log, r.Recorder)
	if err != nil {
		return err
	}
	r.internal = internal

	builder := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha2.DatadogDashboard{}).
		WithEventFilter(predicate.GenerationChangedPredicate{})

	err = builder.Complete(r)

	if err != nil {
		return err
	}
	return nil
}
