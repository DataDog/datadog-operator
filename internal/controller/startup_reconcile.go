// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

type startupReconcileRunnable struct {
	name  string
	start func(context.Context) error
}

func (r startupReconcileRunnable) Start(ctx context.Context) error {
	return r.start(ctx)
}

func (r startupReconcileRunnable) NeedLeaderElection() bool {
	return true
}

func addDatadogAgentStartupReconcile(mgr manager.Manager, r *DatadogAgentReconciler) error {
	return mgr.Add(startupReconcileRunnable{
		name: "DatadogAgent",
		start: func(ctx context.Context) error {
			logger := ctrl.LoggerFrom(ctx).WithName("startup").WithName("DatadogAgent")
			list := &v2alpha1.DatadogAgentList{}
			if err := r.Client.List(ctx, list); err != nil {
				return err
			}
			for i := range list.Items {
				dda := list.Items[i].DeepCopy()
				logger.Info("Reconciling existing DatadogAgent on startup", "namespace", dda.Namespace, "name", dda.Name)
				if _, err := r.Reconcile(ctx, dda); err != nil {
					logger.Error(err, "Failed to reconcile existing DatadogAgent on startup", "namespace", dda.Namespace, "name", dda.Name)
				}
			}
			return nil
		},
	})
}

func addDatadogAgentInternalStartupReconcile(mgr manager.Manager, r *DatadogAgentInternalReconciler) error {
	return mgr.Add(startupReconcileRunnable{
		name: "DatadogAgentInternal",
		start: func(ctx context.Context) error {
			logger := ctrl.LoggerFrom(ctx).WithName("startup").WithName("DatadogAgentInternal")
			list := &v1alpha1.DatadogAgentInternalList{}
			if err := r.Client.List(ctx, list); err != nil {
				return err
			}
			for i := range list.Items {
				ddai := list.Items[i].DeepCopy()
				logger.Info("Reconciling existing DatadogAgentInternal on startup", "namespace", ddai.Namespace, "name", ddai.Name)
				if _, err := r.Reconcile(ctx, ddai); err != nil {
					logger.Error(err, "Failed to reconcile existing DatadogAgentInternal on startup", "namespace", ddai.Namespace, "name", ddai.Name)
				}
			}
			return nil
		},
	})
}

func addDatadogCSIDriverStartupReconcile(mgr manager.Manager, r *DatadogCSIDriverReconciler) error {
	return mgr.Add(startupReconcileRunnable{
		name: "DatadogCSIDriver",
		start: func(ctx context.Context) error {
			logger := ctrl.LoggerFrom(ctx).WithName("startup").WithName("DatadogCSIDriver")
			list := &v1alpha1.DatadogCSIDriverList{}
			if err := r.Client.List(ctx, list); err != nil {
				return err
			}
			for i := range list.Items {
				ddcsi := list.Items[i].DeepCopy()
				logger.Info("Reconciling existing DatadogCSIDriver on startup", "namespace", ddcsi.Namespace, "name", ddcsi.Name)
				if _, err := r.Reconcile(ctx, ddcsi); err != nil {
					logger.Error(err, "Failed to reconcile existing DatadogCSIDriver on startup", "namespace", ddcsi.Namespace, "name", ddcsi.Name)
				}
			}
			return nil
		},
	})
}
