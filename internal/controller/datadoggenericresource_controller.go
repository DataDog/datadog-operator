// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	ddgr "github.com/DataDog/datadog-operator/internal/controller/datadoggenericresource"
	"github.com/DataDog/datadog-operator/pkg/datadogclient"
	"github.com/go-logr/logr"
)

// DatadogGenericCRReconciler reconciles a DatadogGenericCR object
type DatadogGenericCRReconciler struct {
	Client   client.Client
	DDClient datadogclient.DatadogGenericClient
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	internal *ddgr.Reconciler
}

//+kubebuilder:rbac:groups=datadoghq.com,resources=datadoggenericcrs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=datadoghq.com,resources=datadoggenericcrs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=datadoghq.com,resources=datadoggenericcrs/finalizers,verbs=update

// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.4/pkg/reconcile
func (r *DatadogGenericCRReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.internal.Reconcile(ctx, req)
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatadogGenericCRReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.internal = ddgr.NewReconciler(r.Client, r.DDClient, r.Scheme, r.Log, r.Recorder)

	builder := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.DatadogGenericResource{}).
		WithEventFilter(predicate.GenerationChangedPredicate{})

	err := builder.Complete(r)

	if err != nil {
		return err
	}
	return nil
}
