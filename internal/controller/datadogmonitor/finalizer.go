// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogmonitor

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
)

const (
	datadogMonitorFinalizer = "finalizer.monitor.datadoghq.com"
)

func (r *Reconciler) handleFinalizer(auth context.Context, logger logr.Logger, dm *datadoghqv1alpha1.DatadogMonitor) (ctrl.Result, error) {
	// Check if the DatadogMonitor instance is marked to be deleted, which is indicated by the deletion timestamp being set.
	if dm.GetDeletionTimestamp() != nil {
		if controllerutil.ContainsFinalizer(dm, datadogMonitorFinalizer) {
			r.finalizeDatadogMonitor(auth, logger, dm)

			controllerutil.RemoveFinalizer(dm, datadogMonitorFinalizer)
			err := r.client.Update(context.TODO(), dm)
			if err != nil {
				return ctrl.Result{RequeueAfter: defaultErrRequeuePeriod}, err
			}
		}

		// Requeue until the object was properly deleted by Kuberentes
		return ctrl.Result{RequeueAfter: defaultRequeuePeriod}, nil
	}

	// Add finalizer for this resource if it doesn't already exist.
	if !controllerutil.ContainsFinalizer(dm, datadogMonitorFinalizer) {
		if err := r.addFinalizer(logger, dm); err != nil {
			return ctrl.Result{RequeueAfter: defaultErrRequeuePeriod}, err
		}

		return ctrl.Result{RequeueAfter: defaultRequeuePeriod}, nil
	}

	// Proceed in reconcile loop.
	return ctrl.Result{}, nil
}

func (r *Reconciler) finalizeDatadogMonitor(auth context.Context, logger logr.Logger, dm *datadoghqv1alpha1.DatadogMonitor) {
	if dm.Status.Primary {
		err := deleteMonitor(auth, r.datadogClient, dm.Status.ID)
		if err != nil {
			logger.Error(err, "failed to finalize monitor", "Monitor ID", fmt.Sprint(dm.Status.ID))

			return
		}
		logger.Info("Successfully finalized DatadogMonitor", "Monitor ID", fmt.Sprint(dm.Status.ID))
		event := buildEventInfo(dm.Name, dm.Namespace, datadog.DeletionEvent)
		r.recordEvent(dm, event)
	}
}

func (r *Reconciler) addFinalizer(logger logr.Logger, dm *datadoghqv1alpha1.DatadogMonitor) error {
	logger.Info("Adding Finalizer for the DatadogMonitor")

	controllerutil.AddFinalizer(dm, datadogMonitorFinalizer)

	err := r.client.Update(context.TODO(), dm)
	if err != nil {
		logger.Error(err, "failed to update DatadogMonitor with finalizer", "Monitor ID", fmt.Sprint(dm.Status.ID))
		return err
	}

	return nil
}
