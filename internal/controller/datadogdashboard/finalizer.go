// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogdashboard

import (
	"context"
	"fmt"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	datadogDashboardFinalizer = "finalizer.datadoghq.com/dashboard"
)

func (r *Reconciler) handleFinalizer(logger logr.Logger, db *datadoghqv1alpha1.DatadogDashboard) (ctrl.Result, error) {
	// Check if the DatadogDashboard instance is marked to be deleted, which is indicated by the deletion timestamp being set.
	if db.GetDeletionTimestamp() != nil {
		if controllerutil.ContainsFinalizer(db, datadogDashboardFinalizer) {
			r.finalizeDatadogDashboard(logger, db)

			controllerutil.RemoveFinalizer(db, datadogDashboardFinalizer)
			err := r.client.Update(context.TODO(), db)
			if err != nil {
				return ctrl.Result{RequeueAfter: defaultErrRequeuePeriod}, err
			}
		}

		// Requeue until the object is properly deleted by Kubernetes
		return ctrl.Result{RequeueAfter: defaultRequeuePeriod}, nil
	}

	// Add finalizer for this resource if it doesn't already exist.
	if !controllerutil.ContainsFinalizer(db, datadogDashboardFinalizer) {
		if err := r.addFinalizer(logger, db); err != nil {
			return ctrl.Result{RequeueAfter: defaultErrRequeuePeriod}, err
		}

		return ctrl.Result{Requeue: true}, nil
	}

	// Proceed in reconcile loop.
	return ctrl.Result{}, nil
}

func (r *Reconciler) finalizeDatadogDashboard(logger logr.Logger, db *datadoghqv1alpha1.DatadogDashboard) {
	err := deleteDashboard(r.datadogAuth, r.datadogClient, db.Status.ID)
	if err != nil {
		logger.Error(err, "failed to finalize dashboard", "dashboard ID", fmt.Sprint(db.Status.ID))

		return
	}
	logger.Info("Successfully finalized DatadogDashboard", "dashboard ID", fmt.Sprint(db.Status.ID))
	event := buildEventInfo(db.Name, db.Namespace, datadog.DeletionEvent)
	r.recordEvent(db, event)
}

func (r *Reconciler) addFinalizer(logger logr.Logger, db *datadoghqv1alpha1.DatadogDashboard) error {
	logger.Info("Adding Finalizer for the DatadogDashboard")

	controllerutil.AddFinalizer(db, datadogDashboardFinalizer)

	err := r.client.Update(context.TODO(), db)
	if err != nil {
		logger.Error(err, "failed to update DatadogDashboard with finalizer", "dashboard ID", fmt.Sprint(db.Status.ID))
		return err
	}

	return nil
}
