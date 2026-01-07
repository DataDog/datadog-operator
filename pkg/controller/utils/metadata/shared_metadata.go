// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metadata

import (
	"bytes"
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

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
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
	operatorVersion   string
	kubernetesVersion string
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
		hostName:          os.Getenv(constants.DDHostName),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		credsManager: credsManager,
	}
}

func (sm *SharedMetadata) createRequest(payload []byte) (*http.Request, error) {
	if sm.hostName == "" {
		sm.logger.Error(ErrEmptyHostName, "Could not set host name; not starting metadata forwarder")
		return nil, ErrEmptyHostName
	}

	apiKey, requestURL, err := sm.getApiKeyAndURL()
	if err != nil {
		sm.logger.V(1).Info("Could not get credentials", "error", err)
		return nil, err
	}
	payloadHeader := sm.GetHeaders(*apiKey)

	sm.logger.V(1).Info("Sending metadata to URL", "url", *requestURL)

	reader := bytes.NewReader(payload)
	req, err := http.NewRequestWithContext(context.TODO(), "POST", *requestURL, reader)
	if err != nil {
		sm.logger.V(1).Info("Error creating request", "error", err, "url", *requestURL)
		return nil, err
	}
	req.Header = payloadHeader
	return req, nil
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
	creds, err := sm.credsManager.GetCredsWithDDAFallback(sm.getDatadogAgent)
	if err != nil {
		return nil, nil, err
	}

	mdfURL := url.URL{
		Scheme: defaultURLScheme,
		Host:   defaultURLHost,
		Path:   defaultURLPath,
	}
	if creds.Site != nil {
		mdfURL.Host = defaultURLHostPrefix + *creds.Site
	}

	if creds.URL != nil {
		tempURL, err := url.Parse(*creds.URL)
		if err == nil {
			mdfURL.Host = tempURL.Host
			mdfURL.Scheme = tempURL.Scheme
		}
	}
	requestURL := mdfURL.String()
	return &creds.APIKey, &requestURL, nil
}

// getDatadogAgent retrieves the DatadogAgent using Get client method
func (sm *SharedMetadata) getDatadogAgent() (*v2alpha1.DatadogAgent, error) {
	ddaList := v2alpha1.DatadogAgentList{}

	if err := sm.k8sClient.List(context.TODO(), &ddaList); err != nil {
		return nil, err
	}

	if len(ddaList.Items) == 0 {
		return nil, errors.New("DatadogAgent not found")
	}

	return &ddaList.Items[0], nil
}

// GetBaseHeaders returns the common HTTP headers for API requests
func (sm *SharedMetadata) GetHeaders(apiKey string) http.Header {
	header := http.Header{}
	header.Set(apiHTTPHeaderKey, apiKey)
	header.Set(contentTypeHeaderKey, "application/json")
	header.Set(acceptHeaderKey, "application/json")
	header.Set(userAgentHTTPHeaderKey, fmt.Sprintf("Datadog Operator/%s", version.GetVersion()))
	return header
}
