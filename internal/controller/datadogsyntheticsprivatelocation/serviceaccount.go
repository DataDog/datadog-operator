// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogsyntheticsprivatelocation

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

// reconcileServiceAccount creates the worker ServiceAccount, named after the
// DatadogSyntheticsPrivateLocation, and returns its name. Only the labels are
// managed, so annotations added by users (for example for workload identity)
// are kept.
func reconcileServiceAccount(ctx context.Context, kubeClient client.Client, scheme *runtime.Scheme, instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation) (string, error) {
	logger := ctrl.LoggerFrom(ctx)

	name := instance.Name
	desired := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: instance.Namespace,
			Labels:    splLabels(instance),
		},
	}
	if refErr := controllerutil.SetControllerReference(instance, desired, scheme); refErr != nil {
		return "", fmt.Errorf("setting owner reference on service account: %w", refErr)
	}

	current := &corev1.ServiceAccount{}
	err := kubeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: instance.Namespace}, current)
	switch {
	case apierrors.IsNotFound(err):
		logger.Info("Creating service account", "serviceaccount", name)
		if createErr := kubeClient.Create(ctx, desired); createErr != nil {
			return "", fmt.Errorf("creating service account: %w", createErr)
		}
	case err != nil:
		return "", fmt.Errorf("getting service account: %w", err)
	default:
		if !apiequality.Semantic.DeepEqual(current.Labels, desired.Labels) {
			logger.Info("Updating service account", "serviceaccount", name)
			current.Labels = desired.Labels
			if updateErr := kubeClient.Update(ctx, current); updateErr != nil {
				return "", fmt.Errorf("updating service account: %w", updateErr)
			}
		}
	}

	return name, nil
}
