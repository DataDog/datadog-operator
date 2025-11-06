// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/version"
)

const (
	crdMetadataInterval = 1 * time.Minute
)

type CRDMetadataForwarder struct {
	*SharedMetadata

	// CRD-specific fields
	payloadHeader http.Header
}

type CRDMetadataPayload struct {
	Timestamp   int64       `json:"timestamp"`
	ClusterID   string      `json:"cluster_id"`
	ClusterName string      `json:"clustername"`
	Metadata    CRDMetadata `json:"datadog_crd_metadata"`
}

type CRDMetadata struct {
	// TODO: Add CRD-specific metadata fields here
}

// NewCRDMetadataForwarder creates a new instance of the CRD metadata forwarder
func NewCRDMetadataForwarder(logger logr.Logger, k8sClient client.Reader, kubernetesVersion string, operatorVersion string, credsManager *config.CredentialManager) *CRDMetadataForwarder {
	return &CRDMetadataForwarder{
		SharedMetadata: NewSharedMetadata(logger, k8sClient, kubernetesVersion, operatorVersion, credsManager),
	}
}

// Start starts the CRD metadata forwarder
func (cmf *CRDMetadataForwarder) Start() {
	err := cmf.setCredentials()
	if err != nil {
		cmf.logger.Error(err, "Could not set credentials; not starting CRD metadata forwarder")
		return
	}

	cmf.payloadHeader = cmf.getHeaders()

	cmf.logger.Info("Starting CRD metadata forwarder")

	ticker := time.NewTicker(crdMetadataInterval)
	go func() {
		for range ticker.C {
			if err := cmf.sendMetadata(); err != nil {
				cmf.logger.Error(err, "Error while sending CRD metadata")
			}
		}
	}()
}

func (cmf *CRDMetadataForwarder) sendMetadata() error {
	clusterUID, err := cmf.GetOrCreateClusterUID()
	if err != nil {
		cmf.logger.Error(err, "Failed to get cluster UID")
		return err
	}
	payload := cmf.buildPayload(clusterUID)

	cmf.logger.Info("CRD metadata payload", "payload", string(payload))

	cmf.logger.V(1).Info("Sending CRD metadata to URL", "url", cmf.requestURL)

	reader := bytes.NewReader(payload)
	req, err := http.NewRequestWithContext(context.TODO(), "POST", cmf.requestURL, reader)
	if err != nil {
		cmf.logger.Error(err, "Error creating request", "url", cmf.requestURL, "reader", reader)
		return err
	}
	req.Header = cmf.payloadHeader

	resp, err := cmf.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error sending CRD metadata request: %w", err)
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read CRD metadata response body: %w", err)
	}

	cmf.logger.V(1).Info("Read CRD metadata response", "status code", resp.StatusCode, "body", string(body))
	return nil
}

func (cmf *CRDMetadataForwarder) buildPayload(clusterUID string) []byte {
	now := time.Now().Unix()

	crdMetadata := CRDMetadata{
		// TODO: Populate CRD-specific metadata fields
	}

	payload := CRDMetadataPayload{
		Timestamp:   now,
		ClusterID:   clusterUID,
		ClusterName: cmf.clusterName,
		Metadata:    crdMetadata,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		cmf.logger.Error(err, "Error marshaling payload to json")
	}

	return jsonPayload
}

func (cmf *CRDMetadataForwarder) setCredentials() error {
	return cmf.SharedMetadata.setCredentials()
}

func (cmf *CRDMetadataForwarder) getHeaders() http.Header {
	headers := cmf.GetBaseHeaders()
	headers.Set(userAgentHTTPHeaderKey, fmt.Sprintf("Datadog Operator/%s", version.GetVersion()))
	return headers
}
