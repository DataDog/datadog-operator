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

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/internal/controller/finalizer"
	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func (r *Reconciler) deleteResource() finalizer.ResourceDeleteFunc {
	return func(ctx context.Context, k8sObj client.Object, datadogID string) error {
		return r.finalizeDDAI(ctx, k8sObj)
	}
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
