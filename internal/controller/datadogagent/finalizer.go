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

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/finalizer"
	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/trace"
)

const (
	datadogAgentFinalizer = "finalizer.agent.datadoghq.com"
)

func (r *Reconciler) handleFinalizer(ctx context.Context, reqLogger logr.Logger, instance *v2alpha1.DatadogAgent) (result reconcile.Result, err error) {
	span, ctx := startDDASpan(ctx)
	defer trace.FinishSpan(span, &err)

	final := finalizer.NewFinalizer(reqLogger, r.client, r.deleteResource(reqLogger), defaultRequeuePeriod, defaultErrRequeuePeriod)
	return final.HandleFinalizer(ctx, instance, "", datadogAgentFinalizer)
}

func (r *Reconciler) deleteResource(reqLogger logr.Logger) finalizer.ResourceDeleteFunc {
	return func(ctx context.Context, k8sObj client.Object, datadogID string) error {
		return r.finalizeDad(reqLogger, k8sObj)
	}
}

func (r *Reconciler) finalizeDad(reqLogger logr.Logger, obj client.Object) error {
	if r.options.OperatorMetricsEnabled {
		r.forwarders.Unregister(obj)
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
