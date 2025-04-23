// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/defaults"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/global"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/override"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/store"
	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
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
	// We need to apply the defaults to be able to delete the resources
	// associated with those defaults.
	dda := obj.(*datadoghqv2alpha1.DatadogAgent).DeepCopy()
	defaults.DefaultDatadogAgent(dda)

	if r.options.OperatorMetricsEnabled {
		r.forwarders.Unregister(dda)
	}

	// To delete the resources associated with the DatadogAgent that we need to
	// delete, we figure out its dependencies, store them in the dependencies
	// store, and then call the DeleteAll function of the store.

	_, enabledFeatures, requiredComponents := feature.BuildFeatures(
		dda, reconcilerOptionsToFeatureOptions(&r.options, reqLogger))

	storeOptions := &store.StoreOptions{
		SupportCilium: r.options.SupportCilium,
		Logger:        reqLogger,
		Scheme:        r.scheme,
		PlatformInfo:  r.platformInfo,
	}
	depsStore := store.NewStore(dda, storeOptions)
	resourceManagers := feature.NewResourceManagers(depsStore)

	var errs []error

	// Global dependencies
	if err := global.ApplyGlobalDependencies(reqLogger, dda, resourceManagers); len(err) > 0 {
		errs = append(errs, err...)
	}
	if err := global.ApplyGlobalComponentDependencies(reqLogger, dda, resourceManagers, datadoghqv2alpha1.ClusterAgentComponentName, requiredComponents.ClusterAgent); len(err) > 0 {
		errs = append(errs, err...)
	}
	if err := global.ApplyGlobalComponentDependencies(reqLogger, dda, resourceManagers, datadoghqv2alpha1.NodeAgentComponentName, requiredComponents.Agent); len(err) > 0 {
		errs = append(errs, err...)
	}
	if err := global.ApplyGlobalComponentDependencies(reqLogger, dda, resourceManagers, datadoghqv2alpha1.ClusterChecksRunnerComponentName, requiredComponents.ClusterChecksRunner); len(err) > 0 {
		errs = append(errs, err...)
	}

	// Set up dependencies required by enabled features
	for _, feat := range enabledFeatures {
		if featErr := feat.ManageDependencies(resourceManagers); featErr != nil {
			errs = append(errs, featErr)
		}
	}

	// Examine user configuration to override any external dependencies (e.g. RBACs)
	errs = append(errs, override.Dependencies(reqLogger, resourceManagers, dda)...)

	if len(errs) > 0 {
		return fmt.Errorf("errors calculating dependencies while finalizing the DatadogAgent: %v", errs)
	}

	deleteErrs := depsStore.DeleteAll(context.TODO(), r.client)
	if len(deleteErrs) > 0 {
		return fmt.Errorf("error deleting dependencies while finalizing the DatadogAgent: %v", deleteErrs)
	}

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
