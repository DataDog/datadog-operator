// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
)

const (
	datadogAgentFinalizer = "finalizer.agent.datadoghq.com"
)

type finalizerDadFunc func(reqLogger logr.Logger, dda client.Object)

func (r *Reconciler) handleFinalizer(reqLogger logr.Logger, dda client.Object, finalizerDad finalizerDadFunc) (reconcile.Result, error) {
	// Check if the DatadogAgent instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	isDadMarkedToBeDeleted := dda.GetDeletionTimestamp() != nil
	if isDadMarkedToBeDeleted {
		if utils.ContainsString(dda.GetFinalizers(), datadogAgentFinalizer) {
			// Run finalization logic for datadogAgentFinalizer. If the
			// finalization logic fails, don't remove the finalizer so
			// that we can retry during the next reconciliation.
			finalizerDad(reqLogger, dda)

			// Remove datadogAgentFinalizer. Once all finalizers have been
			// removed, the object will be deleted.
			dda.SetFinalizers(utils.RemoveString(dda.GetFinalizers(), datadogAgentFinalizer))
			err := r.client.Update(context.TODO(), dda)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{Requeue: true}, nil
	}

	// Add finalizer for this CR
	if !utils.ContainsString(dda.GetFinalizers(), datadogAgentFinalizer) {
		if err := r.addFinalizer(reqLogger, dda); err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{Requeue: true}, nil
	}

	return reconcile.Result{}, nil
}

func (r *Reconciler) finalizeDadV1(reqLogger logr.Logger, obj client.Object) {
	dda := obj.(*datadoghqv1alpha1.DatadogAgent)
	_, err := r.cleanupMetricsServerAPIService(reqLogger, dda)
	if err != nil {
		reqLogger.Error(err, "Could not delete Metrics Server API Service")
	}

	for _, rbacName := range rbacNamesForDda(dda, r.versionInfo) {
		if _, err = r.cleanupClusterRoleBinding(reqLogger, dda, rbacName); err != nil {
			reqLogger.Error(err, "Could not delete cluster role binding", "name", rbacName)
		}

		if _, err = r.cleanupClusterRole(reqLogger, dda, rbacName); err != nil {
			reqLogger.Error(err, "Could not delete cluster role", "name", rbacName)
		}
	}

	r.forwarders.Unregister(dda)
	reqLogger.Info("Successfully finalized DatadogAgent")
}

func (r *Reconciler) addFinalizer(reqLogger logr.Logger, dda client.Object) error {
	reqLogger.Info("Adding Finalizer for the DatadogAgent")
	dda.SetFinalizers(append(dda.GetFinalizers(), datadogAgentFinalizer))

	// Update CR
	err := r.client.Update(context.TODO(), dda)
	if err != nil {
		reqLogger.Error(err, "Failed to update DatadogAgent with finalizer")
		return err
	}
	return nil
}
