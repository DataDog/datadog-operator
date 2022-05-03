// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"os"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
)

// getKeysFromCredentials returns any key data that need to be stored in a new secret
func getKeysFromCredentials(credentials *datadoghqv1alpha1.DatadogCredentials) map[string][]byte {
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

// For each of the API key and app key, check if:
// 1. an existing secret is defined, or
// 2. the corresponding env var is defined (whether in ENC format or not)
// If either of these is true, the secret is not needed for that particular key.
func checkAPIKeySufficiency(creds *datadoghqv1alpha1.DatadogCredentials, envVarName string) bool {
	return ((creds.APISecret != nil && creds.APISecret.SecretName != "") ||
		creds.APIKeyExistingSecret != "" ||
		os.Getenv(envVarName) != "")
}

func checkAppKeySufficiency(creds *datadoghqv1alpha1.DatadogCredentials, envVarName string) bool {
	return ((creds.APPSecret != nil && creds.APPSecret.SecretName != "") ||
		creds.AppKeyExistingSecret != "" ||
		os.Getenv(envVarName) != "")
}
