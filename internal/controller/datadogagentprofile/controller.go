// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoghq

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Reconciler reconciles a DatadogAgentProfile object
type Reconciler struct {
	client client.Client
	scheme *runtime.Scheme
	log    logr.Logger
}

//+kubebuilder:rbac:groups=datadoghq.com,resources=datadogagentprofiles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=datadoghq.com,resources=datadogagentprofiles/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=datadoghq.com,resources=datadogagentprofiles/finalizers,verbs=update

// NewReconciler returns a new Reconciler object
func NewReconciler(client client.Client, scheme *runtime.Scheme, log logr.Logger) (*Reconciler, error) {
	return &Reconciler{
		client: client,
		scheme: scheme,
		log:    log,
	}, nil
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) {
	// _ = log.FromContext(ctx)

	// // TODO(user): your logic here

	// return ctrl.Result{}, nil
	return r.internalReconcile(ctx, req)
}

func (r *Reconciler) internalReconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	reqLogger := r.log.WithValues("id", uuid.New().String(), "datadogagentprofile", req.NamespacedName)
	reqLogger.Info("Reconciling DatadogAgentProfile")

	var result reconcile.Result

	return result, nil
}
