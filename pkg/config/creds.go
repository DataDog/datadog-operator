// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/secrets"
)

var (
	// ErrEmptyAPIKey empty APIKey error
	ErrEmptyAPIKey = errors.New("empty api key")
)

const apiURLPrefix = "https://api."

// Creds holds the api and app keys.
type Creds struct {
	APIKey string
	AppKey string
	Site   *string
	URL    *string
}

// parsedAPIUrl holds the parsed API URL components used for constructing auth contexts.
type parsedAPIUrl struct {
	Host     string
	Protocol string
}

// CredentialManager provides the credentials from the operator configuration.
type CredentialManager struct {
	logger           logr.Logger
	client           client.Reader
	secretBackend    secrets.Decryptor
	creds            Creds
	credsMutex       sync.Mutex
	decryptorBackoff wait.Backoff

	apiURL *parsedAPIUrl

	ddaDecryptor secrets.Decryptor
	ddaCredsMap  sync.Map
}

// NewCredentialManager returns a CredentialManager.
func NewCredentialManagerWithDecryptor(client client.Reader, decryptor secrets.Decryptor) *CredentialManager {
	cm := &CredentialManager{
		logger:        ctrl.Log.WithName("credentials-manager"),
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

	if err := cm.parseAPIURL(); err != nil {
		cm.logger.Error(err, "Failed to parse API URL")
	}
	return cm
}

// TODO deprecate in favor of NewCredentialManagerWithDecryptor
// NewCredentialManager returns a CredentialManager.
func NewCredentialManager(client client.Reader) *CredentialManager {
	cm := &CredentialManager{
		logger:        ctrl.Log.WithName("credentials-manager"),
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

	if err := cm.parseAPIURL(); err != nil {
		cm.logger.Error(err, "Failed to parse API URL")
	}
	return cm
}

// parseAPIURL parses the API URL from environment variables and stores it.
// Returns nil if no custom API URL is configured. Returns an error if the
// URL is set but invalid.
func (cm *CredentialManager) parseAPIURL() error {
	apiURL := ""
	if os.Getenv(constants.DDddURL) != "" {
		apiURL = os.Getenv(constants.DDddURL)
	} else if os.Getenv(constants.DDURL) != "" {
		apiURL = os.Getenv(constants.DDURL)
	} else if site := os.Getenv(constants.DDSite); site != "" {
		apiURL = apiURLPrefix + strings.TrimSpace(site)
	}

	if apiURL == "" {
		return nil
	}

	parsedAPIURL, parseErr := url.Parse(apiURL)
	if parseErr != nil {
		return fmt.Errorf("invalid API URL %q: %w", apiURL, parseErr)
	}
	if parsedAPIURL.Host == "" || parsedAPIURL.Scheme == "" {
		return fmt.Errorf("missing protocol or host in API URL: %s", apiURL)
	}

	cm.apiURL = &parsedAPIUrl{
		Host:     parsedAPIURL.Host,
		Protocol: parsedAPIURL.Scheme,
	}
	return nil
}

// GetAuth returns a fresh Datadog API authentication context using the latest
// credentials and the parsed API URL. This should be called on every reconcile.
func (cm *CredentialManager) GetAuth() (context.Context, error) {
	creds, err := cm.GetCredentials()
	if err != nil {
		return nil, err
	}

	auth := context.WithValue(
		context.Background(),
		datadogapi.ContextAPIKeys,
		map[string]datadogapi.APIKey{
			"apiKeyAuth": {
				Key: creds.APIKey,
			},
			"appKeyAuth": {
				Key: creds.AppKey,
			},
		},
	)

	if cm.apiURL != nil {
		auth = context.WithValue(auth, datadogapi.ContextServerIndex, 1)
		auth = context.WithValue(auth, datadogapi.ContextServerVariables, map[string]string{
			"name":     cm.apiURL.Host,
			"protocol": cm.apiURL.Protocol,
		})
	}

	return auth, nil
}

// GetCredentials returns the API and APP keys respectively from the operator configurations.
// This function tries to decrypt the secrets using the secret backend if needed.
// It returns an error if the creds aren't configured or if the secret backend fails to decrypt.
func (cm *CredentialManager) GetCredentials() (Creds, error) {
	if creds, found := cm.getCredsFromCache(); found {
		return creds, nil
	}

	creds, err := cm.fetchCredentials()
	if err != nil {
		return Creds{}, err
	}
	cm.cacheCreds(creds)
	return creds, nil
}

// fetchCredentials reads credentials from env vars and decrypts them if needed.
func (cm *CredentialManager) fetchCredentials() (Creds, error) {
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

	return Creds{APIKey: apiKey, AppKey: appKey}, nil
}

// GetCredentialsForMetadata retrieves credentials for metadata endpoints.
// Only DD_API_KEY is required; DD_APP_KEY is optional since metadata endpoints
// don't require application keys for authentication.
func (cm *CredentialManager) GetCredentialsForMetadata() (Creds, error) {
	if creds, found := cm.getCredsFromCache(); found {
		return creds, nil
	}

	apiKey := os.Getenv(constants.DDAPIKey)
	appKey := os.Getenv(constants.DDAppKey)

	if apiKey == "" {
		return Creds{}, errors.New("empty API key")
	}

	var encrypted []string
	if secrets.IsEnc(apiKey) {
		encrypted = append(encrypted, apiKey)
	}

	if appKey != "" && secrets.IsEnc(appKey) {
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

		if appKey != "" {
			if val, found := decrypted[appKey]; found {
				appKey = val
			}
		}
	}

	creds := Creds{APIKey: apiKey, AppKey: appKey}
	cm.cacheCreds(creds)

	return creds, nil
}

// GetCredsWithDDAFallback retrieves credentials for metadata endpoints with a three-tier fallback:
// 1. Operator environment variables (DD_API_KEY, DD_APP_KEY, DD_SITE, DD_URL)
// 2. ConfigMap-based credentials (Helm deployment with endpoint-config ConfigMap)
// 3. DatadogAgent custom resource
// Only DD_API_KEY is required; DD_APP_KEY is optional since this is exclusively
// used for metadata endpoints (/api/v1/metadata) which don't require application keys.
func (cm *CredentialManager) GetCredsWithDDAFallback(getDDA func() (*v2alpha1.DatadogAgent, error)) (Creds, error) {
	if creds, err := cm.GetCredentialsForMetadata(); err == nil {
		if site := os.Getenv("DD_SITE"); site != "" {
			creds.Site = &site
		}
		if url := os.Getenv("DD_URL"); url != "" {
			creds.URL = &url
		}
		return creds, nil
	}

	if creds, err := cm.getCredentialsFromConfigMap(); err == nil {
		return creds, nil
	}

	dda, err := getDDA()
	if err != nil {
		return Creds{}, err
	}

	return cm.getCredentialsFromDDA(dda)
}

// getCredentialsFromConfigMap retrieves credentials by reading the endpoint-config ConfigMap
// and the secrets it references
func (cm *CredentialManager) getCredentialsFromConfigMap() (Creds, error) {
	podName := os.Getenv("POD_NAME")
	namespace := os.Getenv("POD_NAMESPACE")

	if podName == "" || namespace == "" {
		return Creds{}, fmt.Errorf("POD_NAME and POD_NAMESPACE must be set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pod := &corev1.Pod{}
	if err := cm.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: podName}, pod); err != nil {
		return Creds{}, fmt.Errorf("failed to get operator pod: %w", err)
	}

	releaseName := pod.Labels["app.kubernetes.io/instance"]
	if releaseName == "" {
		return Creds{}, fmt.Errorf("app.kubernetes.io/instance label not found on pod (Helm deployment required)")
	}

	configMap := &corev1.ConfigMap{}
	configMapName := fmt.Sprintf("%s-endpoint-config", releaseName)
	if err := cm.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: configMapName}, configMap); err != nil {
		return Creds{}, err
	}

	apiKeySecretName := configMap.Data["api-key-secret-name"]
	if apiKeySecretName == "" {
		return Creds{}, fmt.Errorf("api-key-secret-name not found in endpoint-config ConfigMap")
	}

	apiKey, err := cm.getKeyFromSecret(namespace, apiKeySecretName, "api-key")
	if err != nil {
		return Creds{}, fmt.Errorf("failed to get API key from secret %s: %w", apiKeySecretName, err)
	}
	if apiKey == "" {
		return Creds{}, ErrEmptyAPIKey
	}

	apiKey, err = cm.resolveSecretsIfNeeded(apiKey)
	if err != nil {
		return Creds{}, fmt.Errorf("failed to decrypt API key: %w", err)
	}

	creds := Creds{APIKey: apiKey}

	if appKeySecretName := configMap.Data["app-key-secret-name"]; appKeySecretName != "" {
		if appKey, err := cm.getKeyFromSecret(namespace, appKeySecretName, "app-key"); err == nil && appKey != "" {
			if appKey, err = cm.resolveSecretsIfNeeded(appKey); err == nil {
				creds.AppKey = appKey
			}
		}
	}

	if site := configMap.Data["dd-site"]; site != "" {
		creds.Site = &site
	}

	if url := configMap.Data["dd-url"]; url != "" {
		creds.URL = &url
	}

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
	if cm.creds.APIKey != "" {
		return cm.creds, true
	}

	return Creds{}, false
}

func (cm *CredentialManager) refresh(logger logr.Logger) error {
	newCreds, err := cm.fetchCredentials()
	if err != nil {
		return err
	}

	cm.credsMutex.Lock()
	oldCreds := cm.creds
	cm.creds = newCreds
	cm.credsMutex.Unlock()

	if oldCreds != newCreds {
		logger.Info("Credentials have changed, cache updated")
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
