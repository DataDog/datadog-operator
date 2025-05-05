// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

const (
	datadogAgentFinalizer = "finalizer.agent.datadoghq.com"
)

type finalizerDadFunc func(reqLogger logr.Logger, dda client.Object) error

func (r *Reconciler) handleFinalizer(reqLogger logr.Logger, dda client.Object, finalizerDad finalizerDadFunc) (reconcile.Result, error) {
	// Check if the DatadogAgent instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	isDadMarkedToBeDeleted := dda.GetDeletionTimestamp() != nil
	if isDadMarkedToBeDeleted {
		if utils.ContainsString(dda.GetFinalizers(), datadogAgentFinalizer) {
			// Run finalization logic for datadogAgentFinalizer. If the
			// finalization logic fails, don't remove the finalizer so
			// that we can retry during the next reconciliation.
			if err := finalizerDad(reqLogger, dda); err != nil {
				return reconcile.Result{}, err
			}

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

func (r *Reconciler) finalizeDadV2(reqLogger logr.Logger, obj client.Object) error {
	if r.options.OperatorMetricsEnabled {
		r.forwarders.Unregister(obj)
	}

	// Namespaced resources from the store should be deleted automatically due to owner reference
	// Delete cluster level resources
	r.cleanUpClusterLevelResources(reqLogger, obj)

	if err := r.profilesCleanup(); err != nil {
		return err
	}

	reqLogger.Info("Successfully finalized DatadogAgent")
	return nil
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

// profilesCleanup performs the cleanups required for the profiles feature. The
// only thing that we need to do is to ensure that no nodes are left with the
// profile label.
func (r *Reconciler) profilesCleanup() error {
	nodeList := corev1.NodeList{}
	if err := r.client.List(context.TODO(), &nodeList); err != nil {
		return err
	}

	for _, node := range nodeList.Items {
		_, profileLabelExists := node.Labels[agentprofile.ProfileLabelKey]
		_, oldProfileLabelExists := node.Labels[agentprofile.OldProfileLabelKey]
		if !profileLabelExists && !oldProfileLabelExists {
			continue
		}

		newLabels := map[string]string{}
		for k, v := range node.Labels {
			// Remove profile labels from nodes
			if k == agentprofile.OldProfileLabelKey || k == agentprofile.ProfileLabelKey {
				continue
			}
			newLabels[k] = v
		}

		modifiedNode := node.DeepCopy()
		modifiedNode.Labels = newLabels

		err := r.client.Patch(context.TODO(), modifiedNode, client.MergeFrom(&node))
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

func (r *Reconciler) cleanUpClusterLevelResources(reqLogger logr.Logger, dda client.Object) error {
	// Cluster level resources must be deleted manually since they cannot have an owner reference
	deleteObjectsForResource(r.client, dda, kubernetes.ObjectFromKind(kubernetes.ClusterRolesKind, r.platformInfo))
	deleteObjectsForResource(r.client, dda, kubernetes.ObjectFromKind(kubernetes.ClusterRoleBindingKind, r.platformInfo))
	deleteObjectsForResource(r.client, dda, kubernetes.ObjectFromKind(kubernetes.APIServiceKind, r.platformInfo))

	return nil
}

func deleteObjectsForResource(c client.Client, dda client.Object, kind client.Object) error {
	matchingLabels := client.MatchingLabels{
		store.OperatorStoreLabelKey:              "true",
		kubernetes.AppKubernetesPartOfLabelKey:   object.NewPartOfLabelValue(dda).String(),
		kubernetes.AppKubernetesManageByLabelKey: "datadog-operator",
	}
	if err := c.DeleteAllOf(context.TODO(), kind, matchingLabels); err != nil {
		return err
	}
	return nil
}
