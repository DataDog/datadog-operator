// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package controllers

// import (
// 	"context"

// 	"github.com/go-logr/logr"
// 	"k8s.io/apimachinery/pkg/runtime"
// 	ctrl "sigs.k8s.io/controller-runtime"
// 	"sigs.k8s.io/controller-runtime/pkg/client"

// 	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
// )

// // DatadogMonitorReconciler reconciles a DatadogMonitor object
// type DatadogMonitorReconciler struct {
// 	client.Client
// 	Log    logr.Logger
// 	Scheme *runtime.Scheme
// }

// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogmonitors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogmonitors/status,verbs=get;update;patch

// func (r *DatadogMonitorReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
// 	_ = context.Background()
// 	_ = r.Log.WithValues("datadogmonitor", req.NamespacedName)

// 	// your logic here

// 	return ctrl.Result{}, nil
// }

// func (r *DatadogMonitorReconciler) SetupWithManager(mgr ctrl.Manager) error {
// 	return ctrl.NewControllerManagedBy(mgr).
// 		For(&datadoghqv1alpha1.DatadogMonitor{}).
// 		Complete(r)
// }
