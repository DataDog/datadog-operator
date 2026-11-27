// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlbuilder "sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlcontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogmonitor"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
)

// DatadogMonitorReconciler reconciles a DatadogMonitor object.
type DatadogMonitorReconciler struct {
	Client                 client.Client
	CredsManager           *config.CredentialManager
	Log                    logr.Logger
	Scheme                 *runtime.Scheme
	Recorder               record.EventRecorder
	operatorMetricsEnabled bool
	Options                DatadogMonitorReconcilerOptions
	internal               *datadogmonitor.Reconciler
}

type DatadogMonitorReconcilerOptions struct {
	MaxConcurrentReconciles int
	RequeuePeriod           time.Duration
}

// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogmonitors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogmonitors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogmonitors/finalizers,verbs=get;list;watch;create;update;patch;delete

// Reconcile loop for DatadogMonitor.
func (r *DatadogMonitorReconciler) Reconcile(ctx context.Context, instance *datadoghqv1alpha1.DatadogMonitor) (ctrl.Result, error) {
	return r.internal.Reconcile(ctx, instance)
}

// SetupWithManager creates a new DatadogMonitor controller.
func (r *DatadogMonitorReconciler) SetupWithManager(mgr ctrl.Manager, metricForwardersMgr datadog.MetricsForwardersManager) error {
	r.internal = datadogmonitor.NewReconciler(r.Client, r.CredsManager, r.Scheme, r.Log, r.Recorder, r.operatorMetricsEnabled, metricForwardersMgr, datadogmonitor.ReconcilerOptions{
		APIReader:     mgr.GetAPIReader(),
		RequeuePeriod: r.Options.RequeuePeriod,
	})

	builder := ctrl.NewControllerManagedBy(mgr)

	var builderOptions []ctrlbuilder.ForOption
	if r.operatorMetricsEnabled {
		builderOptions = append(builderOptions, ctrlbuilder.WithPredicates(monitorMetricsForwarderPredicate(metricForwardersMgr)))
	}
	or := reconcile.AsReconciler[*datadoghqv1alpha1.DatadogMonitor](r.Client, r)
	// Keep informer events at the default priority, including the initial list
	// delivered after controller startup. This ensures that every existing
	// DatadogMonitor is reconciled promptly; only controller-scheduled
	// background requeues are assigned low priority by the internal reconciler.
	if err := builder.For(&datadoghqv1alpha1.DatadogMonitor{}, builderOptions...).
		WithEventFilter(datadogResourcePredicate()).
		WithOptions(ctrlcontroller.Options{
			MaxConcurrentReconciles: r.Options.MaxConcurrentReconciles,
		}).
		WithLogConstructor(reconcileLogConstructor(mgr.GetLogger(), monitorControllerName)).
		Complete(or); err != nil {
		return err
	}

	return nil
}

func monitorMetricsForwarderPredicate(metricForwardersMgr datadog.MetricsForwardersManager) predicate.Predicate {
	return predicate.Funcs{
		// On `DatadogMonitor` object creation, we register a metrics forwarder for it
		// unless it already has the orphan management policy (removing the annotation
		// auto-registers it on the next reconcile).
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Object != nil && !datadoghqv1alpha1.IsOrphaned(e.Object) {
				metricForwardersMgr.Register(e.Object)
			}
			return true
		},
	}
}
