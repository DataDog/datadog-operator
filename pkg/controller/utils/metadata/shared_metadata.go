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
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/version"
)

const (
	apiHTTPHeaderKey       = "Dd-Api-Key"
	contentTypeHeaderKey   = "Content-Type"
	acceptHeaderKey        = "Accept"
	userAgentHTTPHeaderKey = "User-Agent"

	// URL constants for metadata endpoints
	defaultURLScheme     = "https"
	defaultURLHost       = "app.datadoghq.com"
	defaultURLHostPrefix = "app."
	defaultURLPath       = "api/v1/metadata"
)

// SharedMetadata contains metadata values shared across all forwarders
type SharedMetadata struct {
	OperatorVersion   string `json:"operator_version"`
	KubernetesVersion string `json:"kubernetes_version"`
	ClusterID         string `json:"cluster_id"`
}

// NewSharedMetadata creates a new instance of shared metadata by fetching cluster UID
func NewSharedMetadata(operatorVersion, kubernetesVersion string, k8sClient client.Reader) (*SharedMetadata, error) {
	clusterUID, err := getClusterUID(k8sClient)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster UID: %w", err)
	}

	return &SharedMetadata{
		OperatorVersion:   operatorVersion,
		KubernetesVersion: kubernetesVersion,
		ClusterID:         clusterUID,
	}, nil
}

// getClusterUID retrieves the cluster UID from kube-system namespace
func getClusterUID(k8sClient client.Reader) (string, error) {
	kubeSystemNS := &corev1.Namespace{}
	err := k8sClient.Get(context.TODO(), types.NamespacedName{Name: "kube-system"}, kubeSystemNS)
	if err != nil {
		return "", fmt.Errorf("failed to get kube-system namespace: %w", err)
	}
	return string(kubeSystemNS.UID), nil
}

// BaseForwarder contains the common infrastructure shared across all forwarders
type BaseForwarder struct {
	k8sClient    client.Reader
	logger       logr.Logger
	httpClient   *http.Client
	credsManager *config.CredentialManager
}

// NewBaseForwarder creates a new instance of base forwarder
func NewBaseForwarder(logger logr.Logger, k8sClient client.Reader, credsManager *config.CredentialManager) *BaseForwarder {
	return &BaseForwarder{
		k8sClient: k8sClient,
		logger:    logger,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		credsManager: credsManager,
	}
}

// createRequest creates an HTTP request with the appropriate API key and URL
func (sfc *BaseForwarder) createRequest(payload []byte) (*http.Request, error) {
	apiKey, requestURL, err := sfc.getApiKeyAndURL()
	if err != nil {
		sfc.logger.V(1).Info("Could not get credentials", "error", err)
		return nil, err
	}
	payloadHeader := sfc.GetHeaders(*apiKey)

	sfc.logger.V(1).Info("Sending metadata to URL", "url", *requestURL)

	reader := bytes.NewReader(payload)
	req, err := http.NewRequestWithContext(context.TODO(), "POST", *requestURL, reader)
	if err != nil {
		sfc.logger.V(1).Info("Error creating request", "error", err, "url", *requestURL)
		return nil, err
	}
	req.Header = payloadHeader
	return req, nil
}

// getApiKeyAndURL retrieves the API key and request URL from the operator or DDA
// and sets the cluster name from the operator or DDA in the BaseForwarder struct
func (sfc *BaseForwarder) getApiKeyAndURL() (*string, *string, error) {
	creds, err := sfc.credsManager.GetCredsWithDDAFallback(sfc.getDatadogAgent)
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
func (sfc *BaseForwarder) getDatadogAgent() (*v2alpha1.DatadogAgent, error) {
	ddaList := v2alpha1.DatadogAgentList{}

	if err := sfc.k8sClient.List(context.TODO(), &ddaList); err != nil {
		return nil, err
	}

	if len(ddaList.Items) == 0 {
		return nil, errors.New("DatadogAgent not found")
	}

	return &ddaList.Items[0], nil
}

// GetBaseHeaders returns the common HTTP headers for API requests
func (sfc *BaseForwarder) GetHeaders(apiKey string) http.Header {
	header := http.Header{}
	header.Set(apiHTTPHeaderKey, apiKey)
	header.Set(contentTypeHeaderKey, "application/json")
	header.Set(acceptHeaderKey, "application/json")
	header.Set(userAgentHTTPHeaderKey, fmt.Sprintf("Datadog Operator/%s", version.GetVersion()))
	return header
}
