// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
	"github.com/go-logr/logr"
)

func (r *Reconciler) manageExternalMetricsSecret(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	return r.manageSecret(logger, managedSecret{name: getDefaultExternalMetricSecretName(dda), requireFunc: needExternalMetricsSecret, createFunc: newExternalMetricsSecret}, dda, newStatus)
}

func newExternalMetricsSecret(name string, dda *datadoghqv1alpha1.DatadogAgent) (*corev1.Secret, error) {
	if dda.Spec.ClusterAgent.Config == nil || dda.Spec.ClusterAgent.Config.ExternalMetrics == nil ||
		dda.Spec.ClusterAgent.Config.ExternalMetrics.Credentials == nil || dda.Spec.ClusterAgent.Config.ExternalMetrics.Credentials.APIKey == "" &&
		dda.Spec.ClusterAgent.Config.ExternalMetrics.Credentials.AppKey == "" {
		return nil, fmt.Errorf("unable to create external metrics secret, missing data in .Spec.ClusterAgent.Config.ExternalMetrics.Credentials")
	}

	labels := getDefaultLabels(dda, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dda))
	annotations := getDefaultAnnotations(dda)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   dda.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Type: corev1.SecretTypeOpaque,
		Data: dataFromCredentials(dda.Spec.ClusterAgent.Config.ExternalMetrics.Credentials),
	}

	return secret, nil
}

func needExternalMetricsSecret(dda *datadoghqv1alpha1.DatadogAgent) bool {
	if dda.Spec.ClusterAgent.Config == nil || dda.Spec.ClusterAgent.Config.ExternalMetrics == nil || dda.Spec.ClusterAgent.Config.ExternalMetrics.Credentials == nil {
		// If Credentials are not specified we fail downstream to have the error surfaced in the status of the DatadogAgent
		return false
	}
	return isClusterAgentEnabled(dda.Spec.ClusterAgent) &&
		(dda.Spec.ClusterAgent.Config.ExternalMetrics.Credentials.APIKey != "" || dda.Spec.ClusterAgent.Config.ExternalMetrics.Credentials.AppKey != "") &&
		!datadoghqv1alpha1.BoolValue(dda.Spec.Credentials.UseSecretBackend)
}

func getDefaultExternalMetricSecretName(dda *datadoghqv1alpha1.DatadogAgent) string {
	return fmt.Sprintf("%s-%s", dda.Name, "metrics-server")
}
