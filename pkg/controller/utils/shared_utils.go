// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package utils

import (
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
)

// GetAPIKeySecret returns the API key secret name and the key inside the secret
// TODO these can become more general if they instead accept *datadoghqv1alpha1.DatadogCredentials
// and have that be a common struct in both CRs
func GetAPIKeySecret(dda *datadoghqv1alpha1.DatadogAgent) (string, string) {
	if dda.Spec.Credentials.APISecret != nil {
		if dda.Spec.Credentials.APISecret.KeyName != "" {
			return dda.Spec.Credentials.APISecret.SecretName, dda.Spec.Credentials.APISecret.KeyName
		}
		return dda.Spec.Credentials.APISecret.SecretName, datadoghqv1alpha1.DefaultAPIKeyKey
	}
	if dda.Spec.Credentials.APIKeyExistingSecret != "" {
		return dda.Spec.Credentials.APIKeyExistingSecret, datadoghqv1alpha1.DefaultAPIKeyKey
	}
	return dda.Name, datadoghqv1alpha1.DefaultAPIKeyKey
}

// GetAppKeySecret returns the APP key secret name and the key inside the secret
func GetAppKeySecret(dda *datadoghqv1alpha1.DatadogAgent) (string, string) {
	if dda.Spec.Credentials.APPSecret != nil {
		if dda.Spec.Credentials.APPSecret.KeyName != "" {
			return dda.Spec.Credentials.APPSecret.SecretName, dda.Spec.Credentials.APPSecret.KeyName
		}
		return dda.Spec.Credentials.APPSecret.SecretName, datadoghqv1alpha1.DefaultAPPKeyKey
	}
	if dda.Spec.Credentials.AppKeyExistingSecret != "" {
		return dda.Spec.Credentials.AppKeyExistingSecret, datadoghqv1alpha1.DefaultAPPKeyKey
	}
	return dda.Name, datadoghqv1alpha1.DefaultAPPKeyKey
}
