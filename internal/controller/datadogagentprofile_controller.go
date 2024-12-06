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

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/crds/datadoghq/v1alpha1"
	dap "github.com/DataDog/datadog-operator/internal/controller/datadogagentprofile"
)

// DatadogAgentProfileReconciler reconciles a DatadogAgentProfile object.
type DatadogAgentProfileReconciler struct {
	Client   client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	internal *dap.Reconciler
}

// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogagentprofiles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogagentprofiles/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogagentprofiles/finalizers,verbs=get;list;watch;create;update;patch;delete

// Reconcile loop for DatadogAgentProfile.
func (r *DatadogAgentProfileReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.internal.Reconcile(ctx, req)
}

// SetupWithManager creates a new DatadogAgentProfile controller.
func (r *DatadogAgentProfileReconciler) SetupWithManager(mgr ctrl.Manager) error {
	internal, err := dap.NewReconciler(r.Client, r.Scheme, r.Log)
	if err != nil {
		return err
	}
	r.internal = internal

	builder := ctrl.NewControllerManagedBy(mgr).
		For(&datadoghqv1alpha1.DatadogAgentProfile{})

	err = builder.Complete(r)
	if err != nil {
		return err
	}

	return nil
}
