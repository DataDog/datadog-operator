// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"errors"
	"os"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"

	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/secrets"
)

// Creds holds the api and app keys.
type Creds struct {
	APIKey string
	AppKey string
}

// CredentialManager provides the credentials from the operator configuration.
type CredentialManager struct {
	secretBackend    secrets.Decryptor
	creds            Creds
	credsMutex       sync.Mutex
	decryptorBackoff wait.Backoff
	callbacks        []CredentialChangeCallback
	callbackMutex    sync.RWMutex
}

type CredentialChangeCallback func(newCreds Creds) error

func (cm *CredentialManager) RegisterCallback(cb CredentialChangeCallback) {
	cm.callbackMutex.Lock()
	defer cm.callbackMutex.Unlock()
	cm.callbacks = append(cm.callbacks, cb)
}

// NewCredentialManager returns a CredentialManager.
func NewCredentialManager() *CredentialManager {
	return &CredentialManager{
		secretBackend: secrets.NewSecretBackend(),
		creds:         Creds{},
		decryptorBackoff: wait.Backoff{
			Steps:    5,
			Duration: 10 * time.Millisecond,
			Factor:   5.0,
			Cap:      20 * time.Second,
		},
	}
}

// GetCredentials returns the API and APP keys respectively from the operator configurations.
// This function tries to decrypt the secrets using the secret backend if needed.
// It returns an error if the creds aren't configured or if the secret backend fails to decrypt.
func (cm *CredentialManager) GetCredentials() (Creds, error) {
	if creds, found := cm.getCredsFromCache(); found {
		return creds, nil
	}

	apiKey := os.Getenv(constants.DDAPIKey)
	appKey := os.Getenv(constants.DDAppKey)

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

	if len(encrypted) > 0 {
		decrypted := map[string]string{}
		var decErr error
		if err := retry.OnError(cm.decryptorBackoff, secrets.Retriable, func() error {
			decrypted, decErr = cm.secretBackend.Decrypt(encrypted)

			return decErr
		}); err != nil {
			return Creds{}, err
		}

		if val, found := decrypted[apiKey]; found {
			apiKey = val
		}

		if val, found := decrypted[appKey]; found {
			appKey = val
		}
	}

	creds := Creds{APIKey: apiKey, AppKey: appKey}
	cm.cacheCreds(creds)

	return creds, nil
}

func (cm *CredentialManager) cacheCreds(creds Creds) {
	cm.credsMutex.Lock()
	defer cm.credsMutex.Unlock()
	cm.creds = creds
}

func (cm *CredentialManager) getCredsFromCache() (Creds, bool) {
	cm.credsMutex.Lock()
	defer cm.credsMutex.Unlock()
	if cm.creds.APIKey != "" && cm.creds.AppKey != "" {
		return cm.creds, true
	}

	return Creds{}, false
}

func (cm *CredentialManager) refresh(logger logr.Logger) error {
	cm.credsMutex.Lock()
	oldCreds := cm.creds
	cm.credsMutex.Unlock()
	cm.creds = Creds{}

	newCreds, err := cm.GetCredentials()

	if err != nil {
		return err
	}

	if oldCreds != newCreds {
		logger.Info("Credentials have changed, updating creds")
		// callbacks
		err = cm.notifyCallbacks(newCreds)

		if err != nil {
			return err
		}
	}
	return nil
}

// Recreate custom resource clients
func (cm *CredentialManager) notifyCallbacks(newCreds Creds) error {
	cm.callbackMutex.RLock()
	defer cm.callbackMutex.RUnlock()

	var errs []error
	for _, cb := range cm.callbacks {
		if err := cb(newCreds); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		// combine multiple errors
		return errors.Join(errs...)
	}
	return nil
}

func (cm *CredentialManager) StartCredentialRefreshRoutine(interval time.Duration, logger logr.Logger) {
	logger.Info("Starting secret refresh routine", "interval", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		<-ticker.C
		if err := cm.refresh(logger); err != nil {
			logger.Error(err, "Failed to refresh credentials")
		}
	}
}
