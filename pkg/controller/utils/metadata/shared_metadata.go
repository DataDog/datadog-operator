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
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/version"
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
	// ErrEmptyHostName empty HostName error
	ErrEmptyHostName = errors.New("empty host name")
)

// SharedMetadata contains the common metadata shared across all forwarders
type SharedMetadata struct {
	k8sClient client.Reader
	logger    logr.Logger

	// Shared metadata fields
	clusterUID        string
	clusterName       string
	operatorVersion   string
	kubernetesVersion string
	requestURL        string
	hostName          string
	httpClient        *http.Client

	// Shared credential management
	credsManager *config.CredentialManager
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

// getApiKeyAndURL retrieves the API key and request URL from the operator or DDA
// and sets the cluster name from the operator or DDA in the SharedMetadata struct
func (sm *SharedMetadata) getApiKeyAndURL() (*string, *string, error) {
	// Get credentials (operator → DDA fallback handled internally)
	creds, err := sm.credsManager.GetCredsWithDDAFallback()
	if err != nil {
		return nil, nil, err
	}

	// Set cluster name - try operator first, then DDA
	// TODO: not ideal really; maybe we could drop cluster name from metadata or extract it as part of rest of metadata instead of tieing with credentials
	sm.clusterName = os.Getenv(constants.DDClusterName)
	if sm.clusterName == "" {
		// Fallback to DDA cluster name
		dda, err := sm.credsManager.GetDatadogAgent()
		if err == nil && dda.Spec.Global != nil && dda.Spec.Global.ClusterName != nil {
			sm.clusterName = *dda.Spec.Global.ClusterName
		}
	}

	if creds.Site != nil {
		mdfURL := url.URL{
			Scheme: defaultURLScheme,
			Host:   defaultURLHostPrefix + *creds.Site,
			Path:   defaultURLPath,
		}
		requestURL := mdfURL.String()
		return &creds.APIKey, &requestURL, nil
	}
	return &creds.APIKey, nil, nil
}

// setCredentials attempts to set up credentials and cluster name from the operator configuration first.
// If cluster name is empty (even when credentials are successfully retrieved from operator),
// it falls back to setting up from DatadogAgent to ensure we have a valid cluster name.
// func (sm *SharedMetadata) setCredentials() error {
// 	apiKey, requestURL, err := sm.getApiKeyAndURL()
// 	if err != nil {
// 		return err
// 	}
// 	sm.apiKey = *apiKey
// 	// request can still be nil
// 	if requestURL != nil {
// 		sm.requestURL = *requestURL
// 	}
// 	return nil
// }

// GetBaseHeaders returns the common HTTP headers for API requests
func (sm *SharedMetadata) GetHeaders(apiKey string) http.Header {
	header := http.Header{}
	header.Set(apiHTTPHeaderKey, apiKey)
	header.Set(contentTypeHeaderKey, "application/json")
	header.Set(acceptHeaderKey, "application/json")
	header.Set(userAgentHTTPHeaderKey, fmt.Sprintf("Datadog Operator/%s", version.GetVersion()))
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
