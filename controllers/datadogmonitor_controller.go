// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package controllers

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlbuilder "sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/controllers/datadogmonitor"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/datadogclient"
)

const kindDatadogMonitor = "DatadogMonitor"

// DatadogMonitorReconciler reconciles a DatadogMonitor object.
type DatadogMonitorReconciler struct {
	Client      client.Client
	DDClient    datadogclient.DatadogClient
	VersionInfo *version.Info
	Log         logr.Logger
	Scheme      *runtime.Scheme
	Recorder    record.EventRecorder
	internal    *datadogmonitor.Reconciler
	Options     datadogmonitor.ReconcilerOptions
}

// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogmonitors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogmonitors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogmonitors/finalizers,verbs=get;list;watch;create;update;patch;delete

// Reconcile loop for DatadogMonitor.
func (r *DatadogMonitorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.internal.Reconcile(ctx, req)
}

// SetupWithManager creates a new DatadogMonitor controller.
func (r *DatadogMonitorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	var metricForwarder datadog.MetricForwardersManager
	var builderOptions []ctrlbuilder.ForOption
	if r.Options.OperatorMetricsEnabled {
		metricForwarder = datadog.NewForwardersManager(r.Client)
		builderOptions = append(builderOptions, ctrlbuilder.WithPredicates(predicate.Funcs{
			// On `DatadogMonitor` controller creation, we register a metrics forwarder for it.
			CreateFunc: func(e event.CreateEvent) bool {
				// On brand new object creation the event's Object's Kind is "" so can't rely on it here
				metricForwarder.Register(e.Object, kindDatadogMonitor)
				return true
			},
		}))
	}

	internal, err := datadogmonitor.NewReconciler(r.Options, r.Client, r.DDClient, r.VersionInfo, r.Scheme, r.Log, r.Recorder, metricForwarder)
	if err != nil {
		return err
	}
	r.internal = internal

	builder := ctrl.NewControllerManagedBy(mgr).For(&datadoghqv1alpha1.DatadogMonitor{}, builderOptions...)

	err = builder.Complete(r)
	if err != nil {
		return err
	}

	return nil
}
