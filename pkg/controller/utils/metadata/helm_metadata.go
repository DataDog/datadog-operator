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

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/version"
)

type HelmMetadataForwarder struct {
	*SharedMetadata

	// Helm-specific fields
	payloadHeader http.Header
	HelmMetadata  HelmMetadata
}

type HelmMetadataPayload struct {
	Hostname  string       `json:"hostname"`
	Timestamp int64        `json:"timestamp"`
	Metadata  HelmMetadata `json:"datadog_helm_metadata"`
}

type HelmMetadata struct {
	OperatorVersion   string `json:"operator_version"`
	KubernetesVersion string `json:"kubernetes_version"`
	ClusterID         string `json:"cluster_id"`
	ClusterName       string `json:"cluster_name"`
}

// NewHelmMetadataForwarder creates a new instance of the helm metadata forwarder
func NewHelmMetadataForwarder(logger logr.Logger, k8sClient client.Reader, kubernetesVersion string, operatorVersion string) *HelmMetadataForwarder {
	return &HelmMetadataForwarder{
		SharedMetadata: NewSharedMetadata(logger, k8sClient, kubernetesVersion, operatorVersion),
		HelmMetadata:   HelmMetadata{},
	}
}

// Start starts the helm metadata forwarder
func (hmf *HelmMetadataForwarder) Start() {
	err := hmf.setCredentials()
	if err != nil {
		hmf.logger.Error(err, "Could not set credentials; not starting helm metadata forwarder")
		return
	}

	if hmf.hostName == "" {
		hmf.logger.Error(ErrEmptyHostName, "Could not set host name; not starting helm metadata forwarder")
		return
	}

	hmf.payloadHeader = hmf.getHeaders()

	hmf.logger.Info("Starting helm metadata forwarder")

	ticker := time.NewTicker(defaultInterval)
	go func() {
		for range ticker.C {
			if err := hmf.sendMetadata(); err != nil {
				hmf.logger.Error(err, "Error while sending helm metadata")
			}
		}
	}()
}

func (hmf *HelmMetadataForwarder) sendMetadata() error {
	payload := hmf.GetPayload()

	hmf.logger.Info("Helm metadata payload", "payload", string(payload))

	hmf.logger.V(1).Info("Sending helm metadata to URL", "url", hmf.requestURL)

	reader := bytes.NewReader(payload)
	req, err := http.NewRequestWithContext(context.TODO(), "POST", hmf.requestURL, reader)
	if err != nil {
		hmf.logger.Error(err, "Error creating request", "url", hmf.requestURL, "reader", reader)
		return err
	}
	req.Header = hmf.payloadHeader

	resp, err := hmf.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error sending helm metadata request: %w", err)
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read helm metadata response body: %w", err)
	}

	hmf.logger.V(1).Info("Read helm metadata response", "status code", resp.StatusCode, "body", string(body))
	return nil
}

func (hmf *HelmMetadataForwarder) GetPayload() []byte {
	now := time.Now().Unix()

	hmf.HelmMetadata.ClusterName = hmf.clusterName
	hmf.HelmMetadata.OperatorVersion = hmf.operatorVersion
	hmf.HelmMetadata.KubernetesVersion = hmf.kubernetesVersion

	clusterUID, err := hmf.SharedMetadata.GetOrCreateClusterUID()
	if err != nil {
		hmf.logger.Error(err, "Error getting cluster UID")
	} else {
		hmf.HelmMetadata.ClusterID = clusterUID
	}

	payload := HelmMetadataPayload{
		Hostname:  hmf.hostName,
		Timestamp: now,
		Metadata:  hmf.HelmMetadata,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		hmf.logger.Error(err, "Error marshaling payload to json")
	}

	return jsonPayload
}

// setupFromOperator delegates to SharedMetadata setupFromOperator method
func (hmf *HelmMetadataForwarder) setupFromOperator() error {
	return hmf.SharedMetadata.setupFromOperator()
}

// setupFromDDA delegates to SharedMetadata setupFromDDA method
func (hmf *HelmMetadataForwarder) setupFromDDA(dda *v2alpha1.DatadogAgent) error {
	return hmf.SharedMetadata.setupFromDDA(dda)
}

func (hmf *HelmMetadataForwarder) setCredentials() error {
	err := hmf.setupFromOperator()
	if err == nil && hmf.clusterName != "" {
		return nil
	}

	dda, err := hmf.SharedMetadata.getDatadogAgent()
	if err != nil {
		return err
	}

	return hmf.setupFromDDA(dda)
}

func (hmf *HelmMetadataForwarder) getHeaders() http.Header {
	headers := hmf.GetBaseHeaders()
	headers.Set(userAgentHTTPHeaderKey, fmt.Sprintf("Datadog Operator/%s", version.GetVersion()))
	return headers
}
