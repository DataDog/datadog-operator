// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/finalizer"
	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

const (
	datadogAgentFinalizer = "finalizer.agent.datadoghq.com"
)

func (r *Reconciler) deleteResource() finalizer.ResourceDeleteFunc {
	return func(ctx context.Context, k8sObj client.Object, datadogID string) error {
		return r.finalizeDad(ctx, k8sObj)
	}
}

func (r *Reconciler) finalizeDad(ctx context.Context, obj client.Object) error {
	logger := ctrl.LoggerFrom(ctx)
	if r.options.OperatorMetricsEnabled {
		r.forwarders.Unregister(obj)
	}

	if err := r.profilesCleanup(ctx, obj.GetNamespace(), obj.GetName()); err != nil {
		return err
	}

	logger.Info("Successfully finalized DatadogAgent")
	return nil
}

// profilesCleanup performs the cleanups required for the profiles feature. The
// profile assignment labels for other Agent namespaces must be preserved.
func (r *Reconciler) profilesCleanup(ctx context.Context, namespace, ddaName string) error {
	profileLabelKey := agentprofile.NodeLabelKey(namespace)
	// Keep the legacy shared label while another DatadogAgent's DaemonSets still
	// select with it. This DatadogAgent's DaemonSets are about to be deleted.
	legacyLabelInUse, err := agentprofile.LegacyProfileLabelInUse(ctx, r.client, func(ds *appsv1.DaemonSet) bool {
		return ds.Namespace == namespace && ds.Labels[apicommon.AgentDeploymentNameLabelKey] == ddaName
	})
	if err != nil {
		return err
	}
	nodeList := corev1.NodeList{}
	if err := r.client.List(ctx, &nodeList); err != nil {
		return err
	}

	for _, node := range nodeList.Items {
		_, profileLabelExists := node.Labels[profileLabelKey]
		_, sharedProfileLabelExists := node.Labels[constants.ProfileLabelKey]
		sharedProfileLabelExists = sharedProfileLabelExists && !legacyLabelInUse
		_, oldProfileLabelExists := node.Labels[agentprofile.OldProfileLabelKey]
		if !profileLabelExists && !sharedProfileLabelExists && !oldProfileLabelExists {
			continue
		}

		newLabels := map[string]string{}
		for k, v := range node.Labels {
			// Remove profile labels from nodes
			if k == profileLabelKey || k == agentprofile.OldProfileLabelKey || (k == constants.ProfileLabelKey && sharedProfileLabelExists) {
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
