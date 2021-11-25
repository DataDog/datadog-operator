// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/go-logr/logr"
)

func (r *Reconciler) manageAgentSecret(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	return r.manageSecret(logger, managedSecret{name: utils.GetDefaultCredentialsSecretName(dda), requireFunc: needAgentSecret, createFunc: newAgentSecret}, dda, newStatus)
}

func newAgentSecret(name string, dda *datadoghqv1alpha1.DatadogAgent) (*corev1.Secret, error) {
	if err := checkCredentials(dda); err != nil {
		return nil, err
	}

	labels := getDefaultLabels(dda, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dda))
	annotations := getDefaultAnnotations(dda)

	creds := dda.Spec.Credentials
	data := dataFromCredentials(&creds.DatadogCredentials)

	// Agent credentials has two more fields
	if creds.Token != "" {
		data[datadoghqv1alpha1.DefaultTokenKey] = []byte(creds.Token)
	} else if isClusterAgentEnabled(dda.Spec.ClusterAgent) && dda.Status.ClusterAgent != nil {
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
	if dda.Spec.Credentials == nil {
		// If Credentials are not specified we fail downstream to have the error surfaced in the status of the DatadogAgent
		return false
	}
	return (isClusterAgentEnabled(dda.Spec.ClusterAgent) || (dda.Spec.Credentials.APIKey != "" || os.Getenv(config.DDAPIKeyEnvVar) != "") || (dda.Spec.Credentials.AppKey != "" || os.Getenv(config.DDAppKeyEnvVar) != "")) &&
		!datadoghqv1alpha1.BoolValue(dda.Spec.Credentials.UseSecretBackend)
}

func checkCredentials(dda *datadoghqv1alpha1.DatadogAgent) error {
	if dda.Spec.Credentials == nil {
		return fmt.Errorf("unable to create agent secret: missing .spec.Credentials")
	}

	creds := dda.Spec.Credentials

	if !checkKeyAndSecret(creds.APIKey, creds.APIKeyExistingSecret, creds.APISecret) {
		if os.Getenv(config.DDAPIKeyEnvVar) == "" {
			return fmt.Errorf("unable to create agent credential secret: missing Api-Key information")
		}
	}

	if !checkKeyAndSecret(creds.AppKey, creds.AppKeyExistingSecret, creds.APPSecret) {
		if os.Getenv(config.DDAppKeyEnvVar) == "" {
			return fmt.Errorf("unable to create agent credential secret: missing App-Key information")
		}
	}
	return nil
}

func checkKeyAndSecret(value, secretName string, secret *datadoghqv1alpha1.Secret) bool {
	if value != "" || secretName != "" || (secret != nil && secret.SecretName != "") {
		return true
	}
	return false
}
