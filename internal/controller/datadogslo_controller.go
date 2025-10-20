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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogslo"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/datadogclient"
)

type DatadogSLOReconciler struct {
	Client   client.Client
	DDClient datadogclient.DatadogSLOClient
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	internal *datadogslo.Reconciler
}

// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogslos,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogslos/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogslos/finalizers,verbs=get;list;watch;create;update;patch;delete

// Reconcile loop for Datadog SLO
func (r *DatadogSLOReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	return r.internal.Reconcile(ctx, req)
}

func (r *DatadogSLOReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.internal = datadogslo.NewReconciler(r.Client, r.DDClient, r.Log, r.Recorder)

	builder := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.DatadogSLO{}).
		WithEventFilter(predicate.GenerationChangedPredicate{})

	err := builder.Complete(r)
	if err != nil {
		return err
	}
	return nil
}

var _ reconcile.Reconciler = (*DatadogSLOReconciler)(nil)

// Callback function for credential change from credential manager
func (r *DatadogSLOReconciler) onCredentialChange(newCreds config.Creds) error {
	return r.internal.UpdateDatadogClient(newCreds)
}
