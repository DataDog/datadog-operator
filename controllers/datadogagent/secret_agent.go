// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadogagent

import (
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/go-logr/logr"
)

func (r *Reconciler) manageAgentSecret(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	return r.manageSecret(logger, managedSecret{name: utils.GetDefaultCredentialsSecretName(dda), requireFunc: needAgentSecret, createFunc: newAgentSecret}, dda, newStatus)
}

func newAgentSecret(name string, dda *datadoghqv1alpha1.DatadogAgent) (*corev1.Secret, error) {
	if !((dda.Spec.Credentials.AppKeyExistingSecret == "" && dda.Spec.Credentials.APPSecret == nil) ||
		(dda.Spec.Credentials.APIKeyExistingSecret == "" && dda.Spec.Credentials.APISecret == nil) ||
		dda.Spec.ClusterAgent != nil) {
		return nil, fmt.Errorf("unable to create agent secret: missing data in .spec.Credentials")
	}

	labels := getDefaultLabels(dda, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dda))
	annotations := getDefaultAnnotations(dda)

	data := dataFromCredentials(&dda.Spec.Credentials.DatadogCredentials)
	// Agent credentials has two more fields
	if dda.Spec.Credentials.Token != "" {
		data[datadoghqv1alpha1.DefaultTokenKey] = []byte(dda.Spec.Credentials.Token)
	} else if dda.Status.ClusterAgent != nil {
		data[datadoghqv1alpha1.DefaultTokenKey] = []byte(dda.Status.ClusterAgent.GeneratedToken)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   dda.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}

	return secret, nil
}

// needAgentSecret checks if a secret should be used or created due to the cluster agent being defined, or if any api or app key
// is configured, AND the secret backend is not used
func needAgentSecret(dda *datadoghqv1alpha1.DatadogAgent) bool {
	return (dda.Spec.ClusterAgent != nil || (dda.Spec.Credentials.APIKey != "" || os.Getenv(config.DDAPIKeyEnvVar) != "") || (dda.Spec.Credentials.AppKey != "" || os.Getenv(config.DDAppKeyEnvVar) != "")) &&
		!datadoghqv1alpha1.BoolValue(dda.Spec.Credentials.UseSecretBackend)
}
