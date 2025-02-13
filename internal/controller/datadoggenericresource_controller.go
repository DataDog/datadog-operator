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
	ddgr "github.com/DataDog/datadog-operator/internal/controller/datadoggenericresource"
	"github.com/DataDog/datadog-operator/pkg/datadogclient"
)

// DatadogGenericResourceReconciler reconciles a DatadogGenericResource object
type DatadogGenericResourceReconciler struct {
	Client   client.Client
	DDClient datadogclient.DatadogGenericClient
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	internal *ddgr.Reconciler
}

// +kubebuilder:rbac:groups=datadoghq.com,resources=datadoggenericresources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadoggenericresources/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadoggenericresources/finalizers,verbs=get;list;watch;create;update;patch;delete

func (r *DatadogGenericResourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.internal.Reconcile(ctx, req)
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatadogGenericResourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
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
