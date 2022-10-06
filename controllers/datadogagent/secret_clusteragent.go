// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	componentdca "github.com/DataDog/datadog-operator/controllers/datadogagent/component/clusteragent"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object"
)

func (r *Reconciler) manageExternalMetricsSecret(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	return r.manageSecret(logger, managedSecret{name: componentdca.GetDefaultExternalMetricSecretName(dda), requireFunc: needExternalMetricsSecret, createFunc: newExternalMetricsSecret}, dda)
}

func newExternalMetricsSecret(name string, dda *datadoghqv1alpha1.DatadogAgent) *corev1.Secret {
	labels := object.GetDefaultLabels(dda, apicommon.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dda))
	annotations := object.GetDefaultAnnotations(dda)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   dda.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Type: corev1.SecretTypeOpaque,
		Data: datadoghqv1alpha1.GetKeysFromCredentials(dda.Spec.ClusterAgent.Config.ExternalMetrics.Credentials),
	}

	return secret
}

func needExternalMetricsSecret(dda *datadoghqv1alpha1.DatadogAgent) bool {
	// If the External Metrics Server is not enabled or the ExternalMetrics.Credentials don't contain API and app keys, we don't need a secret
	if !isClusterAgentEnabled(dda.Spec.ClusterAgent) ||
		dda.Spec.ClusterAgent.Config == nil ||
		dda.Spec.ClusterAgent.Config.ExternalMetrics == nil ||
		!*dda.Spec.ClusterAgent.Config.ExternalMetrics.Enabled ||
		dda.Spec.ClusterAgent.Config.ExternalMetrics.Credentials == nil ||
		(dda.Spec.ClusterAgent.Config.ExternalMetrics.Credentials.APIKey == "" && dda.Spec.ClusterAgent.Config.ExternalMetrics.Credentials.AppKey == "") {
		return false
	}

	// If API key and app key don't need a new secret, then don't create one.
	if datadoghqv1alpha1.CheckAPIKeySufficiency(dda.Spec.ClusterAgent.Config.ExternalMetrics.Credentials, apicommon.DDExternalMetricsProviderAPIKey) &&
		datadoghqv1alpha1.CheckAppKeySufficiency(dda.Spec.ClusterAgent.Config.ExternalMetrics.Credentials, apicommon.DDExternalMetricsProviderAPIKey) {
		return false
	}

	return true
}
