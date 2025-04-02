// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

// DatadogAgentInternalReconciler reconciles a DatadogAgentInternal object
type DatadogAgentInternalReconciler struct {
	client client.Client
	scheme *runtime.Scheme
	log    logr.Logger
}

//+kubebuilder:rbac:groups=datadoghq.com,resources=datadogagentinternals,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=datadoghq.com,resources=datadogagentinternals/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=datadoghq.com,resources=datadogagentinternals/finalizers,verbs=update

// NewReconciler returns a new Reconciler object
func NewReconciler(client client.Client, scheme *runtime.Scheme, log logr.Logger) (*DatadogAgentInternalReconciler, error) {
	return &DatadogAgentInternalReconciler{
		client: client,
		scheme: scheme,
		log:    log,
	}, nil
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the DatadogAgentInternal object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *DatadogAgentInternalReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// TODO(user): your logic here

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatadogAgentInternalReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&datadoghqv1alpha1.DatadogAgentInternal{}).
		Complete(r)
}
