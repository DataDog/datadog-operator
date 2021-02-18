// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadogagent

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/condition"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
)

func (r *Reconciler) manageAgentSecret(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	if !needAgentSecret(dda) {
		result, err := r.cleanupAgentSecret(dda)
		return result, err
	}
	now := metav1.NewTime(time.Now())
	// checks token secret
	secret := &corev1.Secret{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: dda.Namespace, Name: dda.Name}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			if (dda.Spec.Credentials.AppKeyExistingSecret == "" && dda.Spec.Credentials.APPSecret == nil) ||
				(dda.Spec.Credentials.APIKeyExistingSecret == "" && dda.Spec.Credentials.APISecret == nil) ||
				dda.Spec.ClusterAgent != nil {
				return r.createAgentSecret(logger, dda)
			}
			// return error since the secret didn't exist and we are not responsible to create it.
			err = fmt.Errorf("secret %s didn't exist", dda.Name)
			condition.UpdateDatadogAgentStatusConditions(newStatus, now, datadoghqv1alpha1.DatadogAgentConditionTypeSecretError, corev1.ConditionTrue, fmt.Sprintf("%v", err), false)
		}
		return reconcile.Result{}, err
	}

	return r.updateIfNeededAgentSecret(dda, secret)
}

func (r *Reconciler) createAgentSecret(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	newSecret := newAgentSecret(dda)
	// Set DatadogAgent instance  instance as the owner and controller
	if err := controllerutil.SetControllerReference(dda, newSecret, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	if err := r.client.Create(context.TODO(), newSecret); err != nil {
		return reconcile.Result{}, err
	}
	logger.Info("Create Agent Secret", "name", newSecret.Name)
	event := buildEventInfo(newSecret.Name, newSecret.Namespace, secretKind, datadog.CreationEvent)
	r.recordEvent(dda, event)

	return reconcile.Result{Requeue: true}, nil
}

func (r *Reconciler) updateIfNeededAgentSecret(dda *datadoghqv1alpha1.DatadogAgent, currentSecret *corev1.Secret) (reconcile.Result, error) {
	if !ownedByDatadogOperator(currentSecret.OwnerReferences) {
		return reconcile.Result{}, nil
	}
	newSecret := newAgentSecret(dda)
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

		if err := r.client.Update(context.TODO(), updatedSecret); err != nil {
			return reconcile.Result{}, err
		}
		event := buildEventInfo(updatedSecret.Name, updatedSecret.Namespace, secretKind, datadog.UpdateEvent)
		r.recordEvent(dda, event)
		result.Requeue = true
	}

	return result, nil
}

func (r *Reconciler) cleanupAgentSecret(dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	// checks token secret
	secret := &corev1.Secret{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: dda.Namespace, Name: dda.Name}, secret)
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

func newAgentSecret(dda *datadoghqv1alpha1.DatadogAgent) *corev1.Secret {
	labels := getDefaultLabels(dda, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dda))
	annotations := getDefaultAnnotations(dda)

	data := make(map[string][]byte)
	// Create secret using DatadogAgent credentials if it exists, otherwise use Datadog Operator env var
	if dda.Spec.Credentials.APIKey != "" {
		data[datadoghqv1alpha1.DefaultAPIKeyKey] = []byte(dda.Spec.Credentials.APIKey)
	} else if os.Getenv(config.DDAPIKeyEnvVar) != "" {
		data[datadoghqv1alpha1.DefaultAPIKeyKey] = []byte(os.Getenv(config.DDAPIKeyEnvVar))
	}
	if dda.Spec.Credentials.AppKey != "" {
		data[datadoghqv1alpha1.DefaultAPPKeyKey] = []byte(dda.Spec.Credentials.AppKey)
	} else if os.Getenv(config.DDAppKeyEnvVar) != "" {
		data[datadoghqv1alpha1.DefaultAPPKeyKey] = []byte(os.Getenv(config.DDAppKeyEnvVar))
	}
	if dda.Spec.Credentials.Token != "" {
		data[datadoghqv1alpha1.DefaultTokenKey] = []byte(dda.Spec.Credentials.Token)
	} else if dda.Status.ClusterAgent != nil {
		data[datadoghqv1alpha1.DefaultTokenKey] = []byte(dda.Status.ClusterAgent.GeneratedToken)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        dda.Name,
			Namespace:   dda.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}

	return secret
}

// needAgentSecret checks if a secret should be used or created due to the cluster agent being defined, or if any api or app key
// is configured, AND the secret backend is not used
func needAgentSecret(dda *datadoghqv1alpha1.DatadogAgent) bool {
	return (dda.Spec.ClusterAgent != nil || (dda.Spec.Credentials.APIKey != "" || os.Getenv(config.DDAPIKeyEnvVar) != "") || (dda.Spec.Credentials.AppKey != "" || os.Getenv(config.DDAppKeyEnvVar) != "")) &&
		!datadoghqv1alpha1.BoolValue(dda.Spec.Credentials.UseSecretBackend)
}
