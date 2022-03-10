// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"os"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/pkg/secrets"
)

// getKeysFromCredentials returns any key data that need to be stored in a new secret
func getKeysFromCredentials(credentials *datadoghqv1alpha1.DatadogCredentials) map[string][]byte {
	data := make(map[string][]byte)
	// Create secret using DatadogAgent credentials if it exists
	if credentials.APIKey != "" && !secrets.IsEnc(credentials.APIKey) {
		data[datadoghqv1alpha1.DefaultAPIKeyKey] = []byte(credentials.APIKey)
	}
	if credentials.AppKey != "" && !secrets.IsEnc(credentials.AppKey) {
		data[datadoghqv1alpha1.DefaultAPPKeyKey] = []byte(credentials.AppKey)
	}

	return data
}

// For each of the API key and app key, check if:
// 1. an existing secret is defined, or
// 2. the secret backend should be used (from the credentials key), or
// 3. the corresponding env var is defined (whether in ENC format or not)
// If any of these is true, the secret is not needed for that particular key.
func checkAPIKeySufficiency(creds *datadoghqv1alpha1.DatadogCredentials, useSecretBackend bool, envVar string) bool {
	return ((creds.APISecret != nil && creds.APISecret.SecretName != "") ||
		creds.APIKeyExistingSecret != "" ||
		(secrets.IsEnc(creds.APIKey) && useSecretBackend) ||
		os.Getenv(envVar) != "")
}

func checkAppKeySufficiency(creds *datadoghqv1alpha1.DatadogCredentials, useSecretBackend bool, envVar string) bool {
	return ((creds.APPSecret != nil && creds.APPSecret.SecretName != "") ||
		creds.AppKeyExistingSecret != "" ||
		(secrets.IsEnc(creds.AppKey) && useSecretBackend) ||
		os.Getenv(envVar) != "")
}

// Check if the token should come from the secret backend.
// If so, the secret is not needed for the token.
func checkTokenSufficiency(creds *datadoghqv1alpha1.AgentCredentials) bool {
	return secrets.IsEnc(creds.Token) && apiutils.BoolValue(creds.UseSecretBackend)
}
