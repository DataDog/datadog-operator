// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metadata

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/secrets"
)

const (
	apiHTTPHeaderKey     = "Dd-Api-Key"
	contentTypeHeaderKey = "Content-Type"
	acceptHeaderKey      = "Accept"

	// URL constants for metadata endpoints
	defaultURLScheme     = "https"
	defaultURLHost       = "app.datadoghq.com"
	defaultURLHostPrefix = "app."
	defaultURLPath       = "api/v1/metadata"
)

var (
	// ErrEmptyAPIKey empty APIKey error
	ErrEmptyAPIKey = errors.New("empty api key")
	// ErrEmptyHostName empty HostName error
	ErrEmptyHostName = errors.New("empty host name")
)

// SharedMetadata contains the common metadata shared across all forwarders
type SharedMetadata struct {
	k8sClient client.Reader
	logger    logr.Logger

	// Shared metadata fields
	apiKey            string
	clusterUID        string
	clusterName       string
	operatorVersion   string
	kubernetesVersion string
	requestURL        string
	hostName          string
	httpClient        *http.Client

	// Shared credential management
	credsManager *config.CredentialManager
	decryptor    secrets.Decryptor
	creds        sync.Map
}

// NewSharedMetadata creates a new instance of shared metadata
func NewSharedMetadata(logger logr.Logger, k8sClient client.Reader, kubernetesVersion string, operatorVersion string, credsManager *config.CredentialManager) *SharedMetadata {
	return &SharedMetadata{
		k8sClient:         k8sClient,
		logger:            logger,
		operatorVersion:   operatorVersion,
		kubernetesVersion: kubernetesVersion,
		requestURL:        getURL(),
		hostName:          os.Getenv(constants.DDHostName),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		credsManager: credsManager,
		decryptor:    secrets.NewSecretBackend(),
	}
}

// GetOrCreateClusterUID retrieves the cluster UID from kube-system namespace
func (sm *SharedMetadata) GetOrCreateClusterUID() (string, error) {
	if sm.clusterUID != "" {
		return sm.clusterUID, nil
	}

	kubeSystemNS := &corev1.Namespace{}
	err := sm.k8sClient.Get(context.TODO(), types.NamespacedName{Name: "kube-system"}, kubeSystemNS)
	if err != nil {
		return "", fmt.Errorf("failed to get kube-system namespace: %w", err)
	}

	sm.clusterUID = string(kubeSystemNS.UID)
	return sm.clusterUID, nil
}

func (sm *SharedMetadata) setupFromOperator() error {
	sm.clusterName = os.Getenv(constants.DDClusterName)

	if sm.credsManager == nil {
		return errors.New("credentials Manager is undefined")
	}

	creds, err := sm.credsManager.GetCredentials()
	if err != nil {
		return err
	}

	// API key
	sm.apiKey = creds.APIKey

	return nil
}

func (sm *SharedMetadata) setupFromDDA(dda *v2alpha1.DatadogAgent) error {
	if sm.clusterName == "" {
		if dda.Spec.Global != nil && dda.Spec.Global.ClusterName != nil {
			sm.clusterName = *dda.Spec.Global.ClusterName
		}
	}

	if sm.apiKey == "" {
		apiKey, err := sm.getCredentialsFromDDA(dda)
		if err != nil {
			return err
		}
		sm.apiKey = apiKey

		// if API key is set from DDA, also update request URL if needed
		if dda.Spec.Global != nil && dda.Spec.Global.Site != nil {
			mdfURL := url.URL{
				Scheme: defaultURLScheme,
				Host:   defaultURLHostPrefix + *dda.Spec.Global.Site,
				Path:   defaultURLPath,
			}
			sm.requestURL = mdfURL.String()
		}
	}

	return nil
}

// setCredentials attempts to set up credentials and cluster name from the operator configuration first.
// If cluster name is empty (even when credentials are successfully retrieved from operator),
// it falls back to setting up from DatadogAgent to ensure we have a valid cluster name.
func (sm *SharedMetadata) setCredentials() error {
	err := sm.setupFromOperator()
	if err == nil && sm.clusterName != "" {
		return nil
	}

	dda, err := sm.getDatadogAgent()
	if err != nil {
		return err
	}

	return sm.setupFromDDA(dda)
}

// getDatadogAgent retrieves the DatadogAgent using Get client method
func (sm *SharedMetadata) getDatadogAgent() (*v2alpha1.DatadogAgent, error) {
	// Note: If there are no DDAs present when the Operator starts, the metadata forwarder does not re-try to get credentials from a future DDA
	ddaList := v2alpha1.DatadogAgentList{}

	// Create new client because manager client requires manager to start first
	cfg := ctrl.GetConfigOrDie()
	s := runtime.NewScheme()
	newclient, err := client.New(cfg, client.Options{Scheme: s})
	if err != nil {
		return nil, err
	}
	_ = v2alpha1.AddToScheme(s)

	if err := newclient.List(context.TODO(), &ddaList); err != nil {
		return nil, err
	}

	if len(ddaList.Items) == 0 {
		return nil, errors.New("DatadogAgent not found")
	}

	return &ddaList.Items[0], nil
}

func (sm *SharedMetadata) getCredentialsFromDDA(dda *v2alpha1.DatadogAgent) (string, error) {
	if dda.Spec.Global == nil || dda.Spec.Global.Credentials == nil {
		return "", errors.New("credentials not configured in the DatadogAgent")
	}

	defaultSecretName := secrets.GetDefaultCredentialsSecretName(dda)

	var err error
	apiKey := ""

	if dda.Spec.Global != nil && dda.Spec.Global.Credentials != nil && dda.Spec.Global.Credentials.APIKey != nil && *dda.Spec.Global.Credentials.APIKey != "" {
		apiKey = *dda.Spec.Global.Credentials.APIKey
	} else {
		_, secretName, secretKeyName := secrets.GetAPIKeySecret(dda.Spec.Global.Credentials, defaultSecretName)
		apiKey, err = sm.getKeyFromSecret(dda.Namespace, secretName, secretKeyName)
		if err != nil {
			return "", err
		}
	}

	if apiKey == "" {
		return "", ErrEmptyAPIKey
	}

	return sm.resolveSecretsIfNeeded(apiKey)
}

// getKeyFromSecret is used to retrieve an API or App key from a secret object
func (sm *SharedMetadata) getKeyFromSecret(namespace, secretName, dataKey string) (string, error) {
	secret := &corev1.Secret{}
	err := sm.k8sClient.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: secretName}, secret)
	if err != nil {
		return "", err
	}

	return string(secret.Data[dataKey]), nil
}

// resolveSecretsIfNeeded calls the secret backend if creds are encrypted
func (sm *SharedMetadata) resolveSecretsIfNeeded(apiKey string) (string, error) {
	if !secrets.IsEnc(apiKey) {
		// Credentials are not encrypted
		return apiKey, nil
	}

	// Try to get secrets from the local cache
	if decAPIKey, cacheHit := sm.getSecretsFromCache(apiKey); cacheHit {
		// Creds are found in local cache
		return decAPIKey, nil
	}

	// Cache miss, call the secret decryptor
	decrypted, err := sm.decryptor.Decrypt([]string{apiKey})
	if err != nil {
		sm.logger.Error(err, "cannot decrypt secrets")
		return "", err
	}

	// Update the local cache with the decrypted secrets
	sm.resetSecretsCache(decrypted)

	return decrypted[apiKey], nil
}

// getSecretsFromCache returns the cached and decrypted values of encrypted creds
func (sm *SharedMetadata) getSecretsFromCache(encAPIKey string) (string, bool) {
	decAPIKey, found := sm.creds.Load(encAPIKey)
	if !found {
		return "", false
	}

	return decAPIKey.(string), true
}

// resetSecretsCache updates the local secret cache with new secret values
func (sm *SharedMetadata) resetSecretsCache(newSecrets map[string]string) {
	sm.cleanSecretsCache()
	for k, v := range newSecrets {
		sm.creds.Store(k, v)
	}
}

// cleanSecretsCache deletes all cached secrets
func (sm *SharedMetadata) cleanSecretsCache() {
	sm.creds.Range(func(k, v any) bool {
		sm.creds.Delete(k)
		return true
	})
}

// GetBaseHeaders returns the common HTTP headers for API requests
func (sm *SharedMetadata) GetBaseHeaders() http.Header {
	header := http.Header{}
	header.Set(apiHTTPHeaderKey, sm.apiKey)
	header.Set(contentTypeHeaderKey, "application/json")
	header.Set(acceptHeaderKey, "application/json")
	return header
}

func getURL() string {
	mdfURL := url.URL{
		Scheme: defaultURLScheme,
		Host:   defaultURLHost,
		Path:   defaultURLPath,
	}

	// check site env var
	// example: datadoghq.com
	if siteFromEnvVar := os.Getenv("DD_SITE"); siteFromEnvVar != "" {
		mdfURL.Host = defaultURLHostPrefix + siteFromEnvVar
	}
	// check url env var
	// example: https://app.datadoghq.com
	if urlFromEnvVar := os.Getenv("DD_URL"); urlFromEnvVar != "" {
		tempURL, err := url.Parse(urlFromEnvVar)
		if err == nil {
			mdfURL.Host = tempURL.Host
			mdfURL.Scheme = tempURL.Scheme
		}
	}

	return mdfURL.String()
}
