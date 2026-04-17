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

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/internal/controller/finalizer"
	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

const (
	datadogAgentFinalizer = "finalizer.agent.datadoghq.com"
)

func (r *Reconciler) deleteResource(reqLogger logr.Logger) finalizer.ResourceDeleteFunc {
	return func(ctx context.Context, k8sObj client.Object, datadogID string) error {
		return r.finalizeDadV2(reqLogger, k8sObj)
	}
}

func (r *Reconciler) finalizeDadV2(reqLogger logr.Logger, obj client.Object) error {
	if r.options.OperatorMetricsEnabled {
		r.forwarders.Unregister(obj)
	}

	if !r.options.DatadogAgentInternalEnabled {
		// Namespaced resources from the store should be deleted automatically due to owner reference
		// Delete cluster level resources
		r.cleanUpClusterLevelResources(reqLogger, obj)
	}

	if err := r.profilesCleanup(); err != nil {
		return err
	}

	reqLogger.Info("Successfully finalized DatadogAgent")
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

		err := r.client.Patch(context.TODO(), modifiedNode, client.MergeFrom(&node))
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

func (r *Reconciler) cleanUpClusterLevelResources(reqLogger logr.Logger, dda client.Object) error {
	// Cluster level resources must be deleted manually since they cannot have an owner reference
	r.log.Info("Cleaning up cluster level resources")
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
