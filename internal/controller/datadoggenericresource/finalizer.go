// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoggenericresource

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
)

const (
	datadogGenericResourceFinalizer = "finalizer.datadoghq.com/genericresource"
)

func (r *Reconciler) handleFinalizer(ctx context.Context, instance *datadoghqv1alpha1.DatadogGenericResource) (ctrl.Result, error) {
	// Check if the DatadogGenericResource instance is marked to be deleted, which is indicated by the deletion timestamp being set.
	if instance.GetDeletionTimestamp() != nil {
		if controllerutil.ContainsFinalizer(instance, datadogGenericResourceFinalizer) {
			r.finalizeDatadogCustomResource(ctx, instance)

			controllerutil.RemoveFinalizer(instance, datadogGenericResourceFinalizer)
			err := r.client.Update(ctx, instance)
			if err != nil {
				return ctrl.Result{RequeueAfter: defaultErrRequeuePeriod}, err
			}
		}

		// Requeue until the object is properly deleted by Kubernetes
		return ctrl.Result{RequeueAfter: defaultRequeuePeriod}, nil
	}

	// Add finalizer for this resource if it doesn't already exist.
	if !controllerutil.ContainsFinalizer(instance, datadogGenericResourceFinalizer) {
		if err := r.addFinalizer(ctx, instance); err != nil {
			return ctrl.Result{RequeueAfter: defaultErrRequeuePeriod}, err
		}

		return ctrl.Result{Requeue: true}, nil
	}

	// Proceed in reconcile loop.
	return ctrl.Result{}, nil
}

func (r *Reconciler) finalizeDatadogCustomResource(ctx context.Context, instance *datadoghqv1alpha1.DatadogGenericResource) {
	logger := ctrl.LoggerFrom(ctx)
	err := apiDelete(r, instance)
	if err != nil {
		logger.Error(err, "failed to finalize", "custom resource Id", fmt.Sprint(instance.Status.Id))

		return
	}
	logger.V(1).Info("Successfully finalized DatadogGenericResource", "custom resource Id", fmt.Sprint(instance.Status.Id))
	event := buildEventInfo(instance.Name, instance.Namespace, datadog.DeletionEvent)
	r.recordEvent(instance, event)
}

func (r *Reconciler) addFinalizer(ctx context.Context, instance *datadoghqv1alpha1.DatadogGenericResource) error {
	logger := ctrl.LoggerFrom(ctx)
	logger.V(1).Info("Adding finalizer")

	controllerutil.AddFinalizer(instance, datadogGenericResourceFinalizer)

	err := r.client.Update(ctx, instance)
	if err != nil {
		logger.Error(err, "failed to update DatadogGenericResource with finalizer", "custom resource Id", fmt.Sprint(instance.Status.Id))
		return err
	}

	return nil
}
