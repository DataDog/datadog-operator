// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoggenericcrd

import (
	"context"
	"fmt"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
)

const (
	datadogGenericCRDFinalizer = "finalizer.datadoghq.com/genericcrd"
)

func (r *Reconciler) handleFinalizer(logger logr.Logger, instance *datadoghqv1alpha1.DatadogGenericCRD) (ctrl.Result, error) {
	// Check if the DatadogGenericCRD instance is marked to be deleted, which is indicated by the deletion timestamp being set.
	if instance.GetDeletionTimestamp() != nil {
		if utils.ContainsString(instance.GetFinalizers(), datadogGenericCRDFinalizer) {
			r.finalizeDatadogCustomResource(logger, instance)

			instance.SetFinalizers(utils.RemoveString(instance.GetFinalizers(), datadogGenericCRDFinalizer))
			err := r.client.Update(context.TODO(), instance)
			if err != nil {
				return ctrl.Result{RequeueAfter: defaultErrRequeuePeriod}, err
			}
		}

		// Requeue until the object is properly deleted by Kubernetes
		return ctrl.Result{RequeueAfter: defaultRequeuePeriod}, nil
	}

	// Add finalizer for this resource if it doesn't already exist.
	if !utils.ContainsString(instance.GetFinalizers(), datadogGenericCRDFinalizer) {
		if err := r.addFinalizer(logger, instance); err != nil {
			return ctrl.Result{RequeueAfter: defaultErrRequeuePeriod}, err
		}

		return ctrl.Result{Requeue: true}, nil
	}

	// Proceed in reconcile loop.
	return ctrl.Result{}, nil
}

func (r *Reconciler) finalizeDatadogCustomResource(logger logr.Logger, instance *datadoghqv1alpha1.DatadogGenericCRD) {
	var err error
	// TODO: use interface possibly for CRUD
	switch instance.Spec.Type {
	case "synthetics_browser_test":
		err = deleteSyntheticTest(r.datadogAuth, r.datadogSyntheticsClient, instance.Status.ID)
	case "notebook":
		err = deleteNotebook(r.datadogAuth, r.datadogNotebooksClient, instance.Status.ID)
	default:
		logger.Error(err, "failed to finalize: unsupported type", "custom resource ID", fmt.Sprint(instance.Status.ID))
		return
	}
	if err != nil {
		logger.Error(err, "failed to finalize ", "custom resource ID", fmt.Sprint(instance.Status.ID))

		return
	}
	logger.Info("Successfully finalized DatadogGenericCRD", "custom resource ID", fmt.Sprint(instance.Status.ID))
	event := buildEventInfo(instance.Name, instance.Namespace, datadog.DeletionEvent)
	r.recordEvent(instance, event)
}

func (r *Reconciler) addFinalizer(logger logr.Logger, instance *datadoghqv1alpha1.DatadogGenericCRD) error {
	logger.Info("Adding Finalizer for the DatadogGenericCRD")

	instance.SetFinalizers(append(instance.GetFinalizers(), datadogGenericCRDFinalizer))

	err := r.client.Update(context.TODO(), instance)
	if err != nil {
		logger.Error(err, "failed to update DatadogGenericCRD with finalizer", "custom resource ID", fmt.Sprint(instance.Status.ID))
		return err
	}

	return nil
}
