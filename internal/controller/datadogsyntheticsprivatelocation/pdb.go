// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogsyntheticsprivatelocation

import (
	"context"
	"fmt"

	policyv1 "k8s.io/api/policy/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

func pdbEnabled(instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation) bool {
	return instance.Spec.Worker != nil &&
		instance.Spec.Worker.PodDisruptionBudget != nil &&
		instance.Spec.Worker.PodDisruptionBudget.Enabled
}

func buildPodDisruptionBudget(instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation) *policyv1.PodDisruptionBudget {
	p := instance.Spec.Worker.PodDisruptionBudget
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
			Labels:    splLabels(instance),
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable:   p.MinAvailable,
			MaxUnavailable: p.MaxUnavailable,
			Selector:       &metav1.LabelSelector{MatchLabels: selectorLabels(instance)},
		},
	}
}

// reconcilePodDisruptionBudget creates, updates or deletes the worker
// PodDisruptionBudget depending on spec.worker.podDisruptionBudget.enabled.
func reconcilePodDisruptionBudget(ctx context.Context, kubeClient client.Client, scheme *runtime.Scheme, instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation) error {
	logger := ctrl.LoggerFrom(ctx)

	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
			Labels:    splLabels(instance),
		},
	}
	err := kubeClient.Get(ctx, client.ObjectKey{Name: pdb.Name, Namespace: pdb.Namespace}, pdb)

	if !pdbEnabled(instance) {
		// The PDB was disabled after being managed: remove the stale object.
		if err == nil {
			logger.Info("Deleting pod disruption budget", "pdb", pdb.Name)
			if delErr := kubeClient.Delete(ctx, pdb); delErr != nil && !apierrors.IsNotFound(delErr) {
				return fmt.Errorf("deleting pod disruption budget: %w", delErr)
			}
		}
		return nil
	}

	desired := buildPodDisruptionBudget(instance)
	if refErr := controllerutil.SetControllerReference(instance, desired, scheme); refErr != nil {
		return fmt.Errorf("setting owner reference on pod disruption budget: %w", refErr)
	}

	switch {
	case apierrors.IsNotFound(err):
		logger.Info("Creating pod disruption budget", "pdb", desired.Name)
		if createErr := kubeClient.Create(ctx, desired); createErr != nil {
			return fmt.Errorf("creating pod disruption budget: %w", createErr)
		}
		return nil
	case err != nil:
		return fmt.Errorf("getting pod disruption budget: %w", err)
	}

	if !apiequality.Semantic.DeepEqual(pdb.Spec, desired.Spec) ||
		!apiequality.Semantic.DeepEqual(pdb.Labels, desired.Labels) {
		logger.Info("Updating pod disruption budget", "pdb", desired.Name)
		pdb.Spec = desired.Spec
		pdb.Labels = desired.Labels
		if updateErr := kubeClient.Update(ctx, pdb); updateErr != nil {
			return fmt.Errorf("updating pod disruption budget: %w", updateErr)
		}
	}

	return nil
}
