// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
)

// GetDefaultCredentialsSecretName returns the default name for credentials secret
func GetDefaultCredentialsSecretName(dda *datadoghqv1alpha1.DatadogAgent) string {
	return dda.Name
}

// GetAPIKeySecret returns the API key secret name and the key inside the secret
// returns <is_set>, secretName, secretKey
func GetAPIKeySecret(credentials *datadoghqv1alpha1.DatadogCredentials, defaultName string) (bool, string, string) {
	if credentials.APISecret != nil {
		if credentials.APISecret.KeyName != "" {
			return true, credentials.APISecret.SecretName, credentials.APISecret.KeyName
		}
		return true, credentials.APISecret.SecretName, datadoghqv1alpha1.DefaultAPIKeyKey
	}
	if credentials.APIKeyExistingSecret != "" {
		return true, credentials.APIKeyExistingSecret, datadoghqv1alpha1.DefaultAPIKeyKey
	}
	if credentials.APIKey != "" {
		return true, defaultName, datadoghqv1alpha1.DefaultAPIKeyKey
	}
	return false, defaultName, datadoghqv1alpha1.DefaultAPIKeyKey
}

// GetAppKeySecret returns the APP key secret name and the key inside the secret
// returns <is_set>, secretName, secretKey
func GetAppKeySecret(credentials *datadoghqv1alpha1.DatadogCredentials, defaultName string) (bool, string, string) {
	if credentials.APPSecret != nil {
		if credentials.APPSecret.KeyName != "" {
			return true, credentials.APPSecret.SecretName, credentials.APPSecret.KeyName
		}
		return true, credentials.APPSecret.SecretName, datadoghqv1alpha1.DefaultAPPKeyKey
	}
	if credentials.AppKeyExistingSecret != "" {
		return true, credentials.AppKeyExistingSecret, datadoghqv1alpha1.DefaultAPPKeyKey
	}
	if credentials.AppKey != "" {
		return true, defaultName, datadoghqv1alpha1.DefaultAPPKeyKey
	}
	return false, defaultName, datadoghqv1alpha1.DefaultAPPKeyKey
}
