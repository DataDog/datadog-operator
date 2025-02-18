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
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent"
	ddai "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// DatadogAgentInternalReconciler reconciles a DatadogAgentInternal object.
type DatadogAgentInternalReconciler struct {
	client.Client
	PlatformInfo kubernetes.PlatformInfo
	Log          logr.Logger
	Scheme       *runtime.Scheme
	Recorder     record.EventRecorder
	Options      datadogagent.ReconcilerOptions
	internal     *ddai.Reconciler
}

// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogagentinternals,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogagentinternals/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogagentinternals/finalizers,verbs=get;list;watch;create;update;patch;delete

// Reconcile loop for DatadogAgentInternal.
func (r *DatadogAgentInternalReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.internal.Reconcile(ctx, req)
}

// SetupWithManager creates a new DatadogAgentInternal controller.
func (r *DatadogAgentInternalReconciler) SetupWithManager(mgr ctrl.Manager, metricForwardersMgr datadog.MetricForwardersManager) error {
	internal, err := ddai.NewReconciler(r.Options, r.Client, r.PlatformInfo, r.Scheme, r.Log, r.Recorder, metricForwardersMgr)
	if err != nil {
		return err
	}
	r.internal = internal

	builder := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.DatadogAgentInternal{}).
		WithEventFilter(predicate.GenerationChangedPredicate{})

	err = builder.Complete(r)
	if err != nil {
		return err
	}

	return nil
}
