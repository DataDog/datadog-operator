// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadogagentdeployment

import (
	"context"

	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"
	"github.com/go-logr/logr"
)

const (
	datadogAgentDeploymentFinalizer = "finalizer.agentdeployment.datadoghq.com"
)

func (r *ReconcileDatadogAgentDeployment) handleFinalizer(reqLogger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment) (reconcile.Result, error) {
	// Check if the DatadogAgentDeployment instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	isDadMarkedToBeDeleted := dad.GetDeletionTimestamp() != nil
	if isDadMarkedToBeDeleted {
		if utils.ContainsString(dad.GetFinalizers(), datadogAgentDeploymentFinalizer) {
			// Run finalization logic for datadogAgentDeploymentFinalizer. If the
			// finalization logic fails, don't remove the finalizer so
			// that we can retry during the next reconciliation.
			r.finalizeDad(reqLogger, dad)

			// Remove datadogAgentDeploymentFinalizer. Once all finalizers have been
			// removed, the object will be deleted.
			dad.SetFinalizers(utils.RemoveString(dad.GetFinalizers(), datadogAgentDeploymentFinalizer))
			err := r.client.Update(context.TODO(), dad)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{Requeue: true}, nil
	}

	// Add finalizer for this CR
	if !utils.ContainsString(dad.GetFinalizers(), datadogAgentDeploymentFinalizer) {
		if err := r.addFinalizer(reqLogger, dad); err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{Requeue: true}, nil
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileDatadogAgentDeployment) finalizeDad(reqLogger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment) {
	r.forwarders.Unregister(dad)
	reqLogger.Info("Successfully finalized DatadogAgentDeployment")
}

func (r *ReconcileDatadogAgentDeployment) addFinalizer(reqLogger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment) error {
	reqLogger.Info("Adding Finalizer for the DatadogAgentDeployment")
	dad.SetFinalizers(append(dad.GetFinalizers(), datadogAgentDeploymentFinalizer))

	// Update CR
	err := r.client.Update(context.TODO(), dad)
	if err != nil {
		reqLogger.Error(err, "Failed to update DatadogAgentDeployment with finalizer")
		return err
	}
	return nil
}
