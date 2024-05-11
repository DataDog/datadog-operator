// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package controllers

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	datadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/DataDog/extendeddaemonset/controllers/extendeddaemonsetsetting"
)

// ExtendedDaemonsetSettingReconciler reconciles a ExtendedDaemonsetSetting object.
type ExtendedDaemonsetSettingReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Options  extendeddaemonsetsetting.ReconcilerOptions
	internal *extendeddaemonsetsetting.Reconciler
}

// +kubebuilder:rbac:groups=datadoghq.com,resources=extendeddaemonsetsettings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=datadoghq.com,resources=extendeddaemonsetsettings/status,verbs=get;update;patch

// Reconcile loop for ExtendedDaemonsetSetting.
func (r *ExtendedDaemonsetSettingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.internal.Reconcile(ctx, req)
}

// SetupWithManager creates a new ExtendedDaemonsetSetting controller.
func (r *ExtendedDaemonsetSettingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	internal, err := extendeddaemonsetsetting.NewReconciler(r.Options, r.Client, r.Scheme, r.Log, r.Recorder)
	if err != nil {
		return err
	}
	r.internal = internal

	return ctrl.NewControllerManagedBy(mgr).
		For(&datadoghqv1alpha1.ExtendedDaemonsetSetting{}).
		Complete(r)
}
