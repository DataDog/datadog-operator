// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadogagentdeployment

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

func (r *ReconcileDatadogAgentDeployment) manageClusterAgentSecret(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus) (reconcile.Result, error) {
	if !needClusterAgentSecret(dad) {
		result, err := r.cleanupClusterAgentSecret(logger, dad, newStatus)
		return result, err
	}
	now := metav1.NewTime(time.Now())
	// checks token secret
	secretName := utils.GetAppKeySecretName(dad)
	secret := &corev1.Secret{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: dad.Namespace, Name: secretName}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			if dad.Spec.Credentials.AppKeyExistingSecret == "" {
				return r.createClusterAgentSecret(logger, dad, newStatus)
			}
			// return error since the secret didn't exist and we are not responsible to create it.
			err = fmt.Errorf("Secret %s didn't exist", secretName)
			condition.UpdateDatadogAgentDeploymentStatusCondition(newStatus, now, datadoghqv1alpha1.ConditionTypeSecretError, corev1.ConditionTrue, fmt.Sprintf("%v", err), false)
		}
		return reconcile.Result{}, err
	}

	return r.updateIfNeededClusterAgentSecret(logger, dad, secret, newStatus)
}

func (r *ReconcileDatadogAgentDeployment) createClusterAgentSecret(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus) (reconcile.Result, error) {
	newSecret := newClusterAgentSecret(dad)
	// Set DatadogAgentDeployment instance  instance as the owner and controller
	if err := controllerutil.SetControllerReference(dad, newSecret, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	if err := r.client.Create(context.TODO(), newSecret); err != nil {
		return reconcile.Result{}, err
	}
	logger.Info("Create Cluster Agent Secret", "name", newSecret.Name)
	r.recordEvent(dad, corev1.EventTypeNormal, "Create Cluster Agent Secret", fmt.Sprintf("%s/%s", newSecret.Namespace, newSecret.Name), datadog.CreationEvent)

	return reconcile.Result{Requeue: true}, nil
}

func (r *ReconcileDatadogAgentDeployment) updateIfNeededClusterAgentSecret(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, currentSecret *corev1.Secret, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus) (reconcile.Result, error) {
	if !ownedByDatadogOperator(currentSecret.OwnerReferences) {
		return reconcile.Result{}, nil
	}
	newSecret := newClusterAgentSecret(dad)
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
		r.recordEvent(dad, corev1.EventTypeNormal, "Update Secret", fmt.Sprintf("%s/%s", updatedSecret.Namespace, updatedSecret.Name), datadog.UpdateEvent)
		result.Requeue = true
	}

	return result, nil
}

func (r *ReconcileDatadogAgentDeployment) cleanupClusterAgentSecret(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus) (reconcile.Result, error) {
	// checks token secret
	secretName := utils.GetAppKeySecretName(dad)
	secret := &corev1.Secret{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: dad.Namespace, Name: secretName}, secret)
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

func newClusterAgentSecret(dad *datadoghqv1alpha1.DatadogAgentDeployment) *corev1.Secret {
	labels := getDefaultLabels(dad, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dad))
	annotations := getDefaultAnnotations(dad)

	data := make(map[string][]byte)
	if dad.Spec.Credentials.APIKey != "" {
		data[datadoghqv1alpha1.DefaultAPIKeyKey] = []byte(base64.StdEncoding.EncodeToString([]byte(dad.Spec.Credentials.APIKey)))
	}
	if dad.Spec.Credentials.AppKey != "" {
		data[datadoghqv1alpha1.DefaultAPPKeyKey] = []byte(base64.StdEncoding.EncodeToString([]byte(dad.Spec.Credentials.AppKey)))
	}
	if dad.Spec.Credentials.Token != "" {
		data[datadoghqv1alpha1.DefaultTokenKey] = []byte(base64.StdEncoding.EncodeToString([]byte(dad.Spec.Credentials.Token)))
	} else if dad.Status.ClusterAgent != nil {
		data[datadoghqv1alpha1.DefaultTokenKey] = []byte(base64.StdEncoding.EncodeToString([]byte(dad.Status.ClusterAgent.GeneratedToken)))
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        utils.GetAppKeySecretName(dad),
			Namespace:   dad.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}

	return secret
}

func needClusterAgentSecret(dad *datadoghqv1alpha1.DatadogAgentDeployment) bool {
	if dad.Spec.ClusterAgent != nil && !datadoghqv1alpha1.BoolValue(dad.Spec.Credentials.UseSecretBackend) {
		return true
	}

	return false
}
