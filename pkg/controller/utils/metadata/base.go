// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metadata

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	apiHTTPHeaderKey     = "Dd-Api-Key"
	contentTypeHeaderKey = "Content-Type"
	acceptHeaderKey      = "Accept"
)

// SharedMetadata contains the common metadata shared across all forwarders
type SharedMetadata struct {
	k8sClient client.Reader
	logger    logr.Logger

	apiKey            string
	clusterUID        string
	clusterName       string
	operatorVersion   string
	kubernetesVersion string
}

// NewSharedMetadata creates a new instance of shared metadata
func NewSharedMetadata(logger logr.Logger, k8sClient client.Reader, kubernetesVersion string, operatorVersion string) *SharedMetadata {
	return &SharedMetadata{
		k8sClient:         k8sClient,
		logger:            logger,
		operatorVersion:   operatorVersion,
		kubernetesVersion: kubernetesVersion,
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

// GetBaseHeaders returns the common HTTP headers for API requests
func (sm *SharedMetadata) GetBaseHeaders() http.Header {
	header := http.Header{}
	header.Set(apiHTTPHeaderKey, sm.apiKey)
	header.Set(contentTypeHeaderKey, "application/json")
	header.Set(acceptHeaderKey, "application/json")
	return header
}
