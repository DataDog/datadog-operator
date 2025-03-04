// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoggenericresource

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
)

const (
	datadogGenericResourceFinalizer = "finalizer.datadoghq.com/genericresource"
)

func (r *Reconciler) handleFinalizer(logger logr.Logger, instance *datadoghqv1alpha1.DatadogGenericResource) (ctrl.Result, error) {
	// Check if the DatadogGenericResource instance is marked to be deleted, which is indicated by the deletion timestamp being set.
	if instance.GetDeletionTimestamp() != nil {
		if utils.ContainsString(instance.GetFinalizers(), datadogGenericResourceFinalizer) {
			r.finalizeDatadogCustomResource(logger, instance)

			instance.SetFinalizers(utils.RemoveString(instance.GetFinalizers(), datadogGenericResourceFinalizer))
			err := r.client.Update(context.TODO(), instance)
			if err != nil {
				return ctrl.Result{RequeueAfter: defaultErrRequeuePeriod}, err
			}
		}

		// Requeue until the object is properly deleted by Kubernetes
		return ctrl.Result{RequeueAfter: defaultRequeuePeriod}, nil
	}

	// Add finalizer for this resource if it doesn't already exist.
	if !utils.ContainsString(instance.GetFinalizers(), datadogGenericResourceFinalizer) {
		if err := r.addFinalizer(logger, instance); err != nil {
			return ctrl.Result{RequeueAfter: defaultErrRequeuePeriod}, err
		}

		return ctrl.Result{Requeue: true}, nil
	}

	// Proceed in reconcile loop.
	return ctrl.Result{}, nil
}

func (r *Reconciler) finalizeDatadogCustomResource(logger logr.Logger, instance *datadoghqv1alpha1.DatadogGenericResource) {
	err := apiDelete(r, instance)
	if err != nil {
		logger.Error(err, "failed to finalize ", "custom resource Id", fmt.Sprint(instance.Status.Id))

		return
	}
	logger.Info("Successfully finalized DatadogGenericResource", "custom resource Id", fmt.Sprint(instance.Status.Id))
	event := buildEventInfo(instance.Name, instance.Namespace, datadog.DeletionEvent)
	r.recordEvent(instance, event)
}

func (r *Reconciler) addFinalizer(logger logr.Logger, instance *datadoghqv1alpha1.DatadogGenericResource) error {
	logger.Info("Adding Finalizer for the DatadogGenericResource")

	instance.SetFinalizers(append(instance.GetFinalizers(), datadogGenericResourceFinalizer))

	err := r.client.Update(context.TODO(), instance)
	if err != nil {
		logger.Error(err, "failed to update DatadogGenericResource with finalizer", "custom resource Id", fmt.Sprint(instance.Status.Id))
		return err
	}

	return nil
}
