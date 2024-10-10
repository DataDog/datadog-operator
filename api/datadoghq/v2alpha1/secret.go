// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v2alpha1

import (
	"fmt"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetDefaultCredentialsSecretName returns the default name for credentials secret
func GetDefaultCredentialsSecretName(dda metav1.Object) string {
	return fmt.Sprintf("%s-secret", dda.GetName())
}

// GetDefaultDCATokenSecretName returns the default name for cluster-agent secret
func GetDefaultDCATokenSecretName(dda metav1.Object) string {
	return fmt.Sprintf("%s-token", dda.GetName())
}

// GetAPIKeySecret returns the API key secret name and the key inside the secret
// returns <isSet>, secretName, secretKey
// Note that the default name can differ depending on where this is called
func GetAPIKeySecret(credentials *DatadogCredentials, defaultName string) (bool, string, string) {
	isSet := false
	secretName := defaultName
	secretKey := DefaultAPIKeyKey

	if credentials.APISecret != nil {
		isSet = true
		secretName = credentials.APISecret.SecretName
		if credentials.APISecret.KeyName != "" {
			secretKey = credentials.APISecret.KeyName
		}
	} else if credentials.APIKey != nil && *credentials.APIKey != "" {
		isSet = true
	}

	return isSet, secretName, secretKey
}

// GetAppKeySecret returns the APP key secret name and the key inside the secret
// returns <isSet>, secretName, secretKey
// Note that the default name can differ depending on where this is called
func GetAppKeySecret(credentials *DatadogCredentials, defaultName string) (bool, string, string) {
	isSet := false
	secretName := defaultName
	secretKey := DefaultAPPKeyKey

	if credentials.AppSecret != nil {
		isSet = true
		secretName = credentials.AppSecret.SecretName
		if credentials.AppSecret.KeyName != "" {
			secretKey = credentials.AppSecret.KeyName
		}
	} else if credentials.AppKey != nil && *credentials.AppKey != "" {
		isSet = true
	}

	return isSet, secretName, secretKey
}

// GetKeysFromCredentials returns any key data that need to be stored in a new secret
func GetKeysFromCredentials(credentials *DatadogCredentials) map[string][]byte {
	data := make(map[string][]byte)
	// Create secret using DatadogAgent credentials if it exists
	if credentials.APIKey != nil && *credentials.APIKey != "" {
		data[DefaultAPIKeyKey] = []byte(*credentials.APIKey)
	}
	if credentials.AppKey != nil && *credentials.AppKey != "" {
		data[DefaultAPPKeyKey] = []byte(*credentials.AppKey)
	}

	return data
}

// CheckAPIKeySufficiency use to check for the API key if:
// 1. an existing secret is defined, or
// 2. the corresponding env var is defined (whether in ENC format or not)
// If either of these is true, the secret is not needed.
func CheckAPIKeySufficiency(creds *DatadogCredentials, envVarName string) bool {
	return ((creds.APISecret != nil && creds.APISecret.SecretName != "") ||
		os.Getenv(envVarName) != "")
}

// CheckAppKeySufficiency use to check for the APP key if:
// 1. an existing secret is defined, or
// 2. the corresponding env var is defined (whether in ENC format or not)
// If either of these is true, the secret is not needed.
func CheckAppKeySufficiency(creds *DatadogCredentials, envVarName string) bool {
	return ((creds.AppSecret != nil && creds.AppSecret.SecretName != "") ||
		os.Getenv(envVarName) != "")
}
