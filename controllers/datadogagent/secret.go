// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

type managedSecret struct {
	name        string
	requireFunc func(dda *datadoghqv1alpha1.DatadogAgent) bool
	createFunc  func(name string, dda *datadoghqv1alpha1.DatadogAgent) *corev1.Secret
}

func (r *Reconciler) manageSecret(logger logr.Logger, secret managedSecret, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	if !secret.requireFunc(dda) {
		result, err := r.cleanupSecret(dda.Namespace, secret.name, dda)
		return result, err
	}

	secretObj := &corev1.Secret{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: dda.Namespace, Name: secret.name}, secretObj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			s := secret.createFunc(secret.name, dda)

			return r.createSecret(logger, s, dda)
		}
		return reconcile.Result{}, err
	}

	return r.updateIfNeededSecret(secret, dda, secretObj)
}

func (r *Reconciler) createSecret(logger logr.Logger, newSecret *corev1.Secret, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	// Set DatadogAgent instance  instance as the owner and controller
	if err := controllerutil.SetControllerReference(dda, newSecret, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	if err := r.client.Create(context.TODO(), newSecret); err != nil {
		return reconcile.Result{}, err
	}
	logger.Info("Create Secret", "name", newSecret.Name)
	event := buildEventInfo(newSecret.Name, newSecret.Namespace, secretKind, datadog.CreationEvent)
	r.recordEvent(dda, event)

	return reconcile.Result{Requeue: true}, nil
}

func (r *Reconciler) updateIfNeededSecret(secret managedSecret, dda *datadoghqv1alpha1.DatadogAgent, currentSecret *corev1.Secret) (reconcile.Result, error) {
	if !CheckOwnerReference(dda, currentSecret) {
		return reconcile.Result{}, nil
	}

	newSecret := secret.createFunc(secret.name, dda)

	result := reconcile.Result{}
	if !(apiequality.Semantic.DeepEqual(newSecret.Data, currentSecret.Data) &&
		apiequality.Semantic.DeepEqual(newSecret.Labels, currentSecret.Labels) &&
		apiequality.Semantic.DeepEqual(newSecret.Annotations, currentSecret.Annotations)) {
		updatedSecret := currentSecret.DeepCopy()
		updatedSecret.Labels = newSecret.Labels
		updatedSecret.Annotations = newSecret.Annotations
		updatedSecret.Type = newSecret.Type
		if updatedSecret.Data == nil {
			updatedSecret.Data = make(map[string][]byte)
		}
		for key, val := range newSecret.Data {
			updatedSecret.Data[key] = val
		}

		if err := kubernetes.UpdateFromObject(context.TODO(), r.client, newSecret, currentSecret.ObjectMeta); err != nil {
			return reconcile.Result{}, err
		}
		event := buildEventInfo(updatedSecret.Name, updatedSecret.Namespace, secretKind, datadog.UpdateEvent)
		r.recordEvent(dda, event)
		result.Requeue = true
	}

	return result, nil
}

func (r *Reconciler) cleanupSecret(namespace, name string, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	secret := &corev1.Secret{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: name}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	if CheckOwnerReference(dda, secret) {
		err = r.client.Delete(context.TODO(), secret)
	}

	return reconcile.Result{}, err
}
