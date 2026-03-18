// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

type finalizerDDAIFunc func(ctx context.Context, dda client.Object) error

func (r *Reconciler) handleFinalizer(ctx context.Context, ddai client.Object, finalizerDDAI finalizerDDAIFunc) (reconcile.Result, error) {
	// Check if the DatadogAgentInternal instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	isDDAIMarkedToBeDeleted := ddai.GetDeletionTimestamp() != nil
	if isDDAIMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(ddai, constants.DatadogAgentInternalFinalizer) {
			// Run finalization logic for datadogAgentFinalizer. If the
			// finalization logic fails, don't remove the finalizer so
			// that we can retry during the next reconciliation.
			if err := finalizerDDAI(ctx, ddai); err != nil {
				return reconcile.Result{}, err
			}

			// Remove datadogAgentFinalizer. Once all finalizers have been
			// removed, the object will be deleted.
			controllerutil.RemoveFinalizer(ddai, constants.DatadogAgentInternalFinalizer)
			err := r.client.Update(ctx, ddai)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{Requeue: true}, nil
	}

	// Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(ddai, constants.DatadogAgentInternalFinalizer) {
		if err := r.addFinalizer(ctx, ddai); err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{Requeue: true}, nil
	}

	return reconcile.Result{}, nil
}

func (r *Reconciler) finalizeDDAI(ctx context.Context, obj client.Object) error {
	logger := ctrl.LoggerFrom(ctx)
	if r.options.OperatorMetricsEnabled {
		r.forwarders.Unregister(obj)
	}

	// Namespaced resources from the store are deleted thanks to owner references.
	// Cluster level resources must be deleted manually since they cannot have an owner reference.
	if err := r.cleanUpClusterLevelResources(ctx, obj); err != nil {
		return err
	}

	if err := r.profilesCleanup(ctx); err != nil {
		return err
	}

	logger.Info("Successfully finalized DatadogAgentInternal")
	return nil
}

func (r *Reconciler) addFinalizer(ctx context.Context, ddai client.Object) error {
	logger := ctrl.LoggerFrom(ctx)
	logger.Info("Adding Finalizer for the DatadogAgentInternal")
	controllerutil.AddFinalizer(ddai, constants.DatadogAgentInternalFinalizer)

	// Update CR
	err := r.client.Update(ctx, ddai)
	if err != nil {
		logger.Error(err, "Failed to update DatadogAgentInternal with finalizer")
		return err
	}
	return nil
}

// profilesCleanup performs the cleanups required for the profiles feature. The
// only thing that we need to do is to ensure that no nodes are left with the
// profile label.
func (r *Reconciler) profilesCleanup(ctx context.Context) error {
	nodeList := corev1.NodeList{}
	if err := r.client.List(ctx, &nodeList); err != nil {
		return err
	}

	for _, node := range nodeList.Items {
		_, profileLabelExists := node.Labels[constants.ProfileLabelKey]
		_, oldProfileLabelExists := node.Labels[agentprofile.OldProfileLabelKey]
		if !profileLabelExists && !oldProfileLabelExists {
			continue
		}

		newLabels := map[string]string{}
		for k, v := range node.Labels {
			// Remove profile labels from nodes
			if k == agentprofile.OldProfileLabelKey || k == constants.ProfileLabelKey {
				continue
			}
			newLabels[k] = v
		}

		modifiedNode := node.DeepCopy()
		modifiedNode.Labels = newLabels

		err := r.client.Patch(ctx, modifiedNode, client.MergeFrom(&node))
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

func (r *Reconciler) cleanUpClusterLevelResources(ctx context.Context, ddai client.Object) error {
	// Cluster level resources must be deleted manually since they cannot have an owner reference
	if err := deleteObjectsForResource(ctx, r.client, ddai, kubernetes.ObjectFromKind(kubernetes.ClusterRolesKind, r.platformInfo)); err != nil {
		return err
	}
	if err := deleteObjectsForResource(ctx, r.client, ddai, kubernetes.ObjectFromKind(kubernetes.ClusterRoleBindingKind, r.platformInfo)); err != nil {
		return err
	}
	if err := deleteObjectsForResource(ctx, r.client, ddai, kubernetes.ObjectFromKind(kubernetes.APIServiceKind, r.platformInfo)); err != nil {
		return err
	}

	return nil
}

func deleteObjectsForResource(ctx context.Context, c client.Client, ddai client.Object, kind client.Object) error {
	matchingLabels := client.MatchingLabels{
		store.OperatorStoreLabelKey:              "true",
		kubernetes.AppKubernetesPartOfLabelKey:   object.NewPartOfLabelValue(ddai).String(),
		kubernetes.AppKubernetesManageByLabelKey: "datadog-operator",
	}
	if err := c.DeleteAllOf(ctx, kind, matchingLabels); err != nil {
		return err
	}
	return nil
}
