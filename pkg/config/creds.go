// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package config

import (
	"errors"
	"os"

	"github.com/DataDog/datadog-operator/pkg/secrets"
)

// Creds holds the api and app keys
type Creds struct {
	APIKey string
	AppKey string
}

// GetCredentials returns the API and APP keys respectively from the operator configurations
// This function tries to decrypt the secrets using the secret backend if needed
// It returns an error if the creds aren't configured or if the secret backend fails to decrypt
func GetCredentials() (Creds, error) {
	return getCredentials(secrets.NewSecretBackend())
}

// getCredentials used to ease testing
func getCredentials(decryptor secrets.Decryptor) (Creds, error) {
	apiKey := os.Getenv(DDAPIKeyEnvVar)
	appKey := os.Getenv(DDAppKeyEnvVar)

	if apiKey == "" || appKey == "" {
		return Creds{}, errors.New("empty API key and/or App key")
	}

	var encrypted []string
	if secrets.IsEnc(apiKey) {
		encrypted = append(encrypted, apiKey)
	}

	if secrets.IsEnc(appKey) {
		encrypted = append(encrypted, appKey)
	}

	if len(encrypted) == 0 {
		// Nothing to decrypt
		return Creds{APIKey: apiKey, AppKey: appKey}, nil
	}

	decrypted, err := decryptor.Decrypt(encrypted)
	if err != nil {
		return Creds{}, err
	}

	if val, found := decrypted[apiKey]; found {
		apiKey = val
	}

	if val, found := decrypted[appKey]; found {
		appKey = val
	}

	return Creds{APIKey: apiKey, AppKey: appKey}, nil
}
