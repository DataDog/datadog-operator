// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/secrets"
)

var (
	// ErrEmptyAPIKey empty APIKey error
	ErrEmptyAPIKey = errors.New("empty api key")
)

// Creds holds the api and app keys.
type Creds struct {
	APIKey string
	AppKey string
	Site   *string
	URL    *string
}

// CredentialManager provides the credentials from the operator configuration.
type CredentialManager struct {
	client           client.Client
	secretBackend    secrets.Decryptor
	creds            Creds
	credsMutex       sync.Mutex
	decryptorBackoff wait.Backoff
	callbacks        []CredentialChangeCallback
	callbackMutex    sync.RWMutex

	ddaDecryptor secrets.Decryptor
	ddaCredsMap  sync.Map
}

type CredentialChangeCallback func(newCreds Creds) error

func (cm *CredentialManager) RegisterCallback(cb CredentialChangeCallback) {
	cm.callbackMutex.Lock()
	defer cm.callbackMutex.Unlock()
	cm.callbacks = append(cm.callbacks, cb)
}

// NewCredentialManager returns a CredentialManager.
func NewCredentialManagerWithDecryptor(client client.Client, decryptor secrets.Decryptor) *CredentialManager {
	return &CredentialManager{
		client:        client,
		secretBackend: decryptor,
		creds:         Creds{},
		decryptorBackoff: wait.Backoff{
			Steps:    5,
			Duration: 10 * time.Millisecond,
			Factor:   5.0,
			Cap:      20 * time.Second,
		},
		ddaDecryptor: decryptor,
		ddaCredsMap:  sync.Map{},
	}
}

// TODO deprecate in favor of NewCredentialManagerWithDecryptor
// NewCredentialManager returns a CredentialManager.
func NewCredentialManager(client client.Client) *CredentialManager {
	return &CredentialManager{
		client:        client,
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

func (cm *CredentialManager) GetCredsWithDDAFallback(dda *v2alpha1.DatadogAgent) (Creds, error) {
	creds, err := cm.GetCredentials()
	if err == nil {
		if os.Getenv("DD_SITE") != "" {
			site := os.Getenv("DD_SITE")
			creds.Site = &site
		}
		if os.Getenv("DD_URL") != "" {
			url := os.Getenv("DD_URL")
			creds.URL = &url
		}
		return creds, nil
	}

	creds, err = cm.getCredentialsFromDDA(dda)
	if err != nil {
		return Creds{}, err
	}

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

// GetCredentialsFromDDA retrieves the API key from the DatadogAgent and decrypts it if needed
// It returns the API key and the site if set in the DatadogAgent
func (cm *CredentialManager) getCredentialsFromDDA(dda *v2alpha1.DatadogAgent) (Creds, error) {
	creds := Creds{}
	if dda == nil {
		return creds, fmt.Errorf("DatadogAgent is nil")
	}

	if dda.Spec.Global == nil || dda.Spec.Global.Credentials == nil {
		return creds, fmt.Errorf("credentials not configured in the DatadogAgent")
	}

	defaultSecretName := secrets.GetDefaultCredentialsSecretName(dda)

	apiKey := ""
	err := error(nil)
	if dda.Spec.Global != nil && dda.Spec.Global.Credentials != nil && dda.Spec.Global.Credentials.APIKey != nil && *dda.Spec.Global.Credentials.APIKey != "" {
		apiKey = *dda.Spec.Global.Credentials.APIKey
	} else {
		_, secretName, secretKeyName := secrets.GetAPIKeySecret(dda.Spec.Global.Credentials, defaultSecretName)
		apiKey, err = cm.getKeyFromSecret(dda.Namespace, secretName, secretKeyName)
		if err != nil {
			return creds, err
		}
	}

	if apiKey == "" {
		return creds, ErrEmptyAPIKey
	}

	apiKey, err = cm.resolveSecretsIfNeeded(apiKey)
	if err != nil {
		return creds, err
	}

	creds.APIKey = apiKey
	if dda.Spec.Global != nil && dda.Spec.Global.Site != nil {
		creds.Site = dda.Spec.Global.Site
	}

	return creds, nil
}

// getKeyFromSecret is used to retrieve an API or App key from a secret object
func (cm *CredentialManager) getKeyFromSecret(namespace, secretName, dataKey string) (string, error) {
	secret := &corev1.Secret{}
	err := cm.client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: secretName}, secret)
	if err != nil {
		return "", err
	}

	return string(secret.Data[dataKey]), nil
}

// resolveSecretsIfNeeded calls the secret backend if creds are encrypted
func (cm *CredentialManager) resolveSecretsIfNeeded(apiKey string) (string, error) {
	if !secrets.IsEnc(apiKey) {
		// Credentials are not encrypted
		return apiKey, nil
	}

	// Try to get secrets from the local cache
	if decAPIKey, cacheHit := cm.getSecretsFromCache(apiKey); cacheHit {
		// Creds are found in local cache
		return decAPIKey, nil
	}

	// Cache miss, call the secret decryptor
	decrypted, err := cm.ddaDecryptor.Decrypt([]string{apiKey})
	if err != nil {
		// TODO cm.logger.Error(err, "cannot decrypt secrets")
		return "", err
	}

	// Update the local cache with the decrypted secrets
	cm.resetSecretsCache(decrypted)

	return decrypted[apiKey], nil
}

// getSecretsFromCache returns the cached and decrypted values of encrypted creds
func (cm *CredentialManager) getSecretsFromCache(encAPIKey string) (string, bool) {
	decAPIKey, found := cm.ddaCredsMap.Load(encAPIKey)
	if !found {
		return "", false
	}

	return decAPIKey.(string), true
}

// resetSecretsCache updates the local secret cache with new secret values
func (cm *CredentialManager) resetSecretsCache(newSecrets map[string]string) {
	cm.cleanSecretsCache()
	for k, v := range newSecrets {
		cm.ddaCredsMap.Store(k, v)
	}
}

// cleanSecretsCache deletes all cached secrets
func (cm *CredentialManager) cleanSecretsCache() {
	cm.ddaCredsMap.Range(func(k, v any) bool {
		cm.ddaCredsMap.Delete(k)
		return true
	})
}
