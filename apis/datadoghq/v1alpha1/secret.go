// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	"os"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
)

// GetDefaultCredentialsSecretName returns the default name for credentials secret
func GetDefaultCredentialsSecretName(dda *DatadogAgent) string {
	return dda.Name
}

// GetAPIKeySecret returns the API key secret name and the key inside the secret
// returns <is_set>, secretName, secretKey
func GetAPIKeySecret(credentials *DatadogCredentials, defaultName string) (bool, string, string) {
	if credentials.APISecret != nil {
		if credentials.APISecret.KeyName != "" {
			return true, credentials.APISecret.SecretName, credentials.APISecret.KeyName
		}

		return true, credentials.APISecret.SecretName, apicommon.DefaultAPIKeyKey
	}

	if credentials.APIKeyExistingSecret != "" {
		return true, credentials.APIKeyExistingSecret, apicommon.DefaultAPIKeyKey
	}

	if credentials.APIKey != "" {
		return true, defaultName, apicommon.DefaultAPIKeyKey
	}

	return false, defaultName, apicommon.DefaultAPIKeyKey
}

// GetAppKeySecret returns the APP key secret name and the key inside the secret
// returns <is_set>, secretName, secretKey
func GetAppKeySecret(credentials *DatadogCredentials, defaultName string) (bool, string, string) {
	if credentials.APPSecret != nil {
		if credentials.APPSecret.KeyName != "" {
			return true, credentials.APPSecret.SecretName, credentials.APPSecret.KeyName
		}

		return true, credentials.APPSecret.SecretName, apicommon.DefaultAPPKeyKey
	}

	if credentials.AppKeyExistingSecret != "" {
		return true, credentials.AppKeyExistingSecret, apicommon.DefaultAPPKeyKey
	}

	if credentials.AppKey != "" {
		return true, defaultName, apicommon.DefaultAPPKeyKey
	}

	return false, defaultName, apicommon.DefaultAPPKeyKey
}

// GetKeysFromCredentials returns any key data that need to be stored in a new secret
func GetKeysFromCredentials(credentials *DatadogCredentials) map[string][]byte {
	data := make(map[string][]byte)
	// Create secret using DatadogAgent credentials if it exists
	if credentials.APIKey != "" {
		data[apicommon.DefaultAPIKeyKey] = []byte(credentials.APIKey)
	}
	if credentials.AppKey != "" {
		data[apicommon.DefaultAPPKeyKey] = []byte(credentials.AppKey)
	}

	return data
}

// CheckAPIKeySufficiency use to check for the API key if:
// 1. an existing secret is defined, or
// 2. the corresponding env var is defined (whether in ENC format or not)
// If either of these is true, the secret is not needed.
func CheckAPIKeySufficiency(creds *DatadogCredentials, envVarName string) bool {
	return ((creds.APISecret != nil && creds.APISecret.SecretName != "") ||
		creds.APIKeyExistingSecret != "" ||
		os.Getenv(envVarName) != "")
}

// CheckAppKeySufficiency use to check for the APP key if:
// 1. an existing secret is defined, or
// 2. the corresponding env var is defined (whether in ENC format or not)
// If either of these is true, the secret is not needed.
func CheckAppKeySufficiency(creds *DatadogCredentials, envVarName string) bool {
	return ((creds.APPSecret != nil && creds.APPSecret.SecretName != "") ||
		creds.AppKeyExistingSecret != "" ||
		os.Getenv(envVarName) != "")
}
