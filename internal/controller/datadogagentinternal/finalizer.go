// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	stderrors "errors"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/internal/controller/finalizer"
	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

var errPreparedRolloutWorkloadsTerminating = stderrors.New("waiting for prepared blue/green Agent workloads to terminate")

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

	if ddai, ok := obj.(*datadoghqv1alpha1.DatadogAgentInternal); ok {
		pending, err := r.preparedRolloutWorkloadsCleanup(ctx, ddai)
		if err != nil {
			return err
		}
		if pending {
			// Keep the DDAI owner and its finalizer until foreground deletion has
			// removed both DaemonSets and their Pods. Releasing node labels first
			// would make blue eligible again while a green Pod may still own the
			// host-local component locks.
			return errPreparedRolloutWorkloadsTerminating
		}
		if err := r.preparedRolloutLabelsCleanup(ctx, ddai); err != nil {
			return err
		}
	}
	// Profile labels also affect which Agent workload owns a node. Remove them
	// only after prepared workloads and their lock owners are fully gone.
	if err := r.profilesCleanup(ctx); err != nil {
		return err
	}

	logger.Info("Successfully finalized DatadogAgentInternal")
	return nil
}

func (r *Reconciler) preparedRolloutWorkloadsCleanup(ctx context.Context, ddai *datadoghqv1alpha1.DatadogAgentInternal) (bool, error) {
	daemonSets := appsv1.DaemonSetList{}
	if err := r.client.List(ctx, &daemonSets, client.InNamespace(ddai.Namespace)); err != nil {
		return false, err
	}

	pending := false
	foreground := metav1.DeletePropagationForeground
	for i := range daemonSets.Items {
		daemonSet := &daemonSets.Items[i]
		if !metav1.IsControlledBy(daemonSet, ddai) || !preparedDaemonSetInitialized(daemonSet) {
			continue
		}
		pending = true
		if daemonSet.DeletionTimestamp != nil {
			continue
		}
		if err := r.client.Delete(ctx, daemonSet, &client.DeleteOptions{PropagationPolicy: &foreground}); err != nil && !errors.IsNotFound(err) {
			return false, err
		}
	}
	return pending, nil
}

func (r *Reconciler) preparedRolloutLabelsCleanup(ctx context.Context, ddai *datadoghqv1alpha1.DatadogAgentInternal) error {
	key := preparedRolloutNodeLabelKey(ddai)
	candidateKey := preparedRolloutCandidateAnnotationKey(key)
	nodes := corev1.NodeList{}
	if err := r.client.List(ctx, &nodes); err != nil {
		return err
	}
	for i := range nodes.Items {
		node := &nodes.Items[i]
		_, hasLabel := node.Labels[key]
		_, hasCandidate := node.Annotations[candidateKey]
		if !hasLabel && !hasCandidate {
			continue
		}
		updated := node.DeepCopy()
		delete(updated.Labels, key)
		delete(updated.Annotations, candidateKey)
		if err := r.client.Patch(ctx, updated, client.MergeFrom(node)); err != nil && !errors.IsNotFound(err) {
			return err
		}
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
