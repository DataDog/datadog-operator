// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package controllers

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	datadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/DataDog/extendeddaemonset/controllers/podtemplate"
)

// PodTemplateReconciler reconciles a PodTemplate object.
type PodTemplateReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Options  podtemplate.ReconcilerOptions
	internal *podtemplate.Reconciler
}

// +kubebuilder:rbac:groups=datadoghq.com,resources=extendeddaemonsets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=podtemplates,verbs=get;list;watch;create;update;patch;delete

// Reconcile loop for PodTemplate.
func (r *PodTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.internal.Reconcile(ctx, req)
}

// SetupWithManager creates a new PodTemplate controller.
func (r *PodTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	internal, err := podtemplate.NewReconciler(r.Options, r.Client, r.Scheme, r.Log, r.Recorder)
	if err != nil {
		return err
	}

	r.internal = internal

	return ctrl.NewControllerManagedBy(mgr).
		For(&datadoghqv1alpha1.ExtendedDaemonSet{}).
		Owns(&corev1.PodTemplate{}).
		Complete(r)
}
