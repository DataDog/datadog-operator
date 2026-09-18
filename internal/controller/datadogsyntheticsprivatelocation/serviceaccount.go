// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogsyntheticsprivatelocation

import (
	"context"
	"fmt"
	"maps"

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

// serviceAccountName returns the ServiceAccount name for the worker pods:
// a user-provided name when referencing an existing one, or the CR name by
// default.
func serviceAccountName(instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation) string {
	if instance.Spec.Worker != nil && instance.Spec.Worker.ServiceAccount != nil && instance.Spec.Worker.ServiceAccount.Name != "" {
		return instance.Spec.Worker.ServiceAccount.Name
	}
	return instance.Name
}

// reconcileServiceAccount creates the worker ServiceAccount when
// spec.worker.serviceAccount.create is set (the default when a serviceAccount
// block is absent is to manage the "<cr-name>" ServiceAccount), or simply
// returns the referenced name when the user brings their own. It returns the
// ServiceAccount name to use in the Deployment.
func reconcileServiceAccount(ctx context.Context, kubeClient client.Client, scheme *runtime.Scheme, instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation) (string, error) {
	logger := ctrl.LoggerFrom(ctx)

	var saConfig *datadoghqv1alpha1.DatadogSPLServiceAccount
	if instance.Spec.Worker != nil {
		saConfig = instance.Spec.Worker.ServiceAccount
	}
	if saConfig != nil && !saConfig.Create && saConfig.Name != "" {
		// Reference an existing ServiceAccount managed elsewhere.
		return saConfig.Name, nil
	}

	name := serviceAccountName(instance)
	annotations := map[string]string{}
	if saConfig != nil {
		maps.Copy(annotations, saConfig.Annotations)
	}

	desired := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   instance.Namespace,
			Labels:      splLabels(instance),
			Annotations: annotations,
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
		if !apiequality.Semantic.DeepEqual(current.Labels, desired.Labels) ||
			!apiequality.Semantic.DeepEqual(current.Annotations, desired.Annotations) {
			logger.Info("Updating service account", "serviceaccount", name)
			current.Labels = desired.Labels
			current.Annotations = desired.Annotations
			if updateErr := kubeClient.Update(ctx, current); updateErr != nil {
				return "", fmt.Errorf("updating service account: %w", updateErr)
			}
		}
	}

	return name, nil
}
