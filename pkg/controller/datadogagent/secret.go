// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadogagent

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/condition"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
)

func (r *ReconcileDatadogAgent) manageClusterAgentSecret(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	if !needClusterAgentSecret(dda) {
		result, err := r.cleanupClusterAgentSecret(logger, dda, newStatus)
		return result, err
	}
	now := metav1.NewTime(time.Now())
	// checks token secret
	secretName := utils.GetAppKeySecretName(dda)
	secret := &corev1.Secret{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: dda.Namespace, Name: secretName}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			if dda.Spec.Credentials.AppKeyExistingSecret == "" {
				return r.createClusterAgentSecret(logger, dda, newStatus)
			}
			// return error since the secret didn't exist and we are not responsible to create it.
			err = fmt.Errorf("Secret %s didn't exist", secretName)
			condition.UpdateDatadogAgentStatusCondition(newStatus, now, datadoghqv1alpha1.ConditionTypeSecretError, corev1.ConditionTrue, fmt.Sprintf("%v", err), false)
		}
		return reconcile.Result{}, err
	}

	return r.updateIfNeededClusterAgentSecret(logger, dda, secret, newStatus)
}

func (r *ReconcileDatadogAgent) createClusterAgentSecret(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	newSecret := newClusterAgentSecret(dda)
	// Set DatadogAgent instance  instance as the owner and controller
	if err := controllerutil.SetControllerReference(dda, newSecret, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	if err := r.client.Create(context.TODO(), newSecret); err != nil {
		return reconcile.Result{}, err
	}
	logger.Info("Create Cluster Agent Secret", "name", newSecret.Name)
	eventInfo := buildEventInfo(newSecret.Name, newSecret.Namespace, secretKind, datadog.CreationEvent)
	r.recordEvent(dda, eventInfo)

	return reconcile.Result{Requeue: true}, nil
}

func (r *ReconcileDatadogAgent) updateIfNeededClusterAgentSecret(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, currentSecret *corev1.Secret, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	if !ownedByDatadogOperator(currentSecret.OwnerReferences) {
		return reconcile.Result{}, nil
	}
	newSecret := newClusterAgentSecret(dda)
	result := reconcile.Result{}
	if !(apiequality.Semantic.DeepEqual(newSecret.Data, currentSecret.Data) &&
		apiequality.Semantic.DeepEqual(newSecret.Labels, currentSecret.Labels) &&
		apiequality.Semantic.DeepEqual(newSecret.Annotations, currentSecret.Annotations)) {

		updatedSecret := currentSecret.DeepCopy()
		updatedSecret.Labels = newSecret.Labels
		updatedSecret.Annotations = newSecret.Annotations
		updatedSecret.Type = newSecret.Type
		for key, val := range newSecret.Data {
			updatedSecret.Data[key] = val
		}

		if err := r.client.Update(context.TODO(), updatedSecret); err != nil {
			return reconcile.Result{}, err
		}
		eventInfo := buildEventInfo(updatedSecret.Name, updatedSecret.Namespace, secretKind, datadog.UpdateEvent)
		r.recordEvent(dda, eventInfo)
		result.Requeue = true
	}

	return result, nil
}

func (r *ReconcileDatadogAgent) cleanupClusterAgentSecret(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	// checks token secret
	secretName := utils.GetAppKeySecretName(dda)
	secret := &corev1.Secret{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: dda.Namespace, Name: secretName}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	if ownedByDatadogOperator(secret.OwnerReferences) {
		err = r.client.Delete(context.TODO(), secret)
	}

	return reconcile.Result{}, err
}

func newClusterAgentSecret(dda *datadoghqv1alpha1.DatadogAgent) *corev1.Secret {
	labels := getDefaultLabels(dda, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dda))
	annotations := getDefaultAnnotations(dda)

	data := make(map[string][]byte)
	if dda.Spec.Credentials.APIKey != "" {
		data[datadoghqv1alpha1.DefaultAPIKeyKey] = []byte(base64.StdEncoding.EncodeToString([]byte(dda.Spec.Credentials.APIKey)))
	}
	if dda.Spec.Credentials.AppKey != "" {
		data[datadoghqv1alpha1.DefaultAPPKeyKey] = []byte(base64.StdEncoding.EncodeToString([]byte(dda.Spec.Credentials.AppKey)))
	}
	if dda.Spec.Credentials.Token != "" {
		data[datadoghqv1alpha1.DefaultTokenKey] = []byte(base64.StdEncoding.EncodeToString([]byte(dda.Spec.Credentials.Token)))
	} else if dda.Status.ClusterAgent != nil {
		data[datadoghqv1alpha1.DefaultTokenKey] = []byte(base64.StdEncoding.EncodeToString([]byte(dda.Status.ClusterAgent.GeneratedToken)))
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        utils.GetAppKeySecretName(dda),
			Namespace:   dda.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}

	return secret
}

func needClusterAgentSecret(dda *datadoghqv1alpha1.DatadogAgent) bool {
	if dda.Spec.ClusterAgent != nil && !datadoghqv1alpha1.BoolValue(dda.Spec.Credentials.UseSecretBackend) {
		return true
	}

	return false
}
