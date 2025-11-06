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

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
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
	// Shared
	OperatorVersion   string `json:"operator_version"`
	KubernetesVersion string `json:"kubernetes_version"`
	ClusterID         string `json:"cluster_id"`
	ClusterName       string `json:"cluster_name"`

	CRDKind       string `json:"crd_kind"`
	CRDName       string `json:"crd_name"`
	CRDNamespace  string `json:"crd_namespace"`
	CRDAPIVersion string `json:"crd_api_version"`
	CRDUID        string `json:"crd_uid"`
	CRDSpecFull   string `json:"crd_spec_full"`
}

type CRDInstance struct {
	Kind       string      `json:"kind"`
	Name       string      `json:"name"`
	Namespace  string      `json:"namespace"`
	APIVersion string      `json:"api_version"`
	UID        string      `json:"uid"`
	Spec       interface{} `json:"spec"`
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

	allCRDs := cmf.getAllActiveCRDs()

	if len(allCRDs) == 0 {
		cmf.logger.V(1).Info("No CRD instances found to send metadata for")
		return nil
	}

	cmf.logger.Info("Detected CRD changes",
		"total_count", len(allCRDs))

	// Send individual payloads for each changed CRD
	for _, crdInstance := range allCRDs {
		if err := cmf.sendCRDMetadata(clusterUID, crdInstance); err != nil {
			cmf.logger.Error(err, "Failed to send metadata for CRD",
				"kind", crdInstance.Kind,
				"name", crdInstance.Name,
				"namespace", crdInstance.Namespace)
		}
	}

	return nil
}

func (cmf *CRDMetadataForwarder) sendCRDMetadata(clusterUID string, crdInstance CRDInstance) error {
	payload := cmf.buildPayload(clusterUID, crdInstance)

	cmf.logger.V(1).Info("CRD metadata payload", "payload", string(payload))

	reader := bytes.NewReader(payload)
	req, err := http.NewRequestWithContext(context.TODO(), "POST", cmf.requestURL, reader)
	if err != nil {
		cmf.logger.Error(err, "Error creating request", "url", cmf.requestURL)
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

	cmf.logger.V(1).Info("Read CRD metadata response",
		"status code", resp.StatusCode,
		"body", string(body),
		"kind", crdInstance.Kind,
		"name", crdInstance.Name)

	return nil
}

func (cmf *CRDMetadataForwarder) buildPayload(clusterUID string, crdInstance CRDInstance) []byte {
	now := time.Now().Unix()

	// Marshal the CRD spec to JSON string
	specJSON, err := json.Marshal(crdInstance.Spec)
	if err != nil {
		cmf.logger.Error(err, "Error marshaling CRD spec to JSON",
			"kind", crdInstance.Kind,
			"name", crdInstance.Name)
		specJSON = []byte("{}")
	}

	crdMetadata := CRDMetadata{
		OperatorVersion:   cmf.operatorVersion,
		KubernetesVersion: cmf.kubernetesVersion,
		ClusterID:         clusterUID,
		ClusterName:       cmf.clusterName,

		CRDKind:       crdInstance.Kind,
		CRDName:       crdInstance.Name,
		CRDNamespace:  crdInstance.Namespace,
		CRDAPIVersion: crdInstance.APIVersion,
		CRDUID:        crdInstance.UID,
		CRDSpecFull:   string(specJSON),
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

func (cmf *CRDMetadataForwarder) getAllActiveCRDs() []CRDInstance {
	var allCRDs []CRDInstance // only collecting DDA, DDAI, and DAP
	if cmf.k8sClient == nil {
		return allCRDs
	}
	// DDA
	ddaList := &v2alpha1.DatadogAgentList{}
	if err := cmf.k8sClient.List(context.TODO(), ddaList); err == nil {
		for _, dda := range ddaList.Items {
			allCRDs = append(allCRDs, CRDInstance{
				Kind:       "DatadogAgent",
				Name:       dda.Name,
				Namespace:  dda.Namespace,
				APIVersion: dda.APIVersion,
				UID:        string(dda.UID),
				Spec:       dda.Spec,
			})
		}
	} else {
		cmf.logger.Error(err, "Error listing DatadogAgents")
	}

	// DDAI
	ddaiList := &v1alpha1.DatadogAgentInternalList{}
	if err := cmf.k8sClient.List(context.TODO(), ddaiList); err == nil {
		for _, ddai := range ddaiList.Items {
			allCRDs = append(allCRDs, CRDInstance{
				Kind:       "DatadogAgentInternal",
				Name:       ddai.Name,
				Namespace:  ddai.Namespace,
				APIVersion: ddai.APIVersion,
				UID:        string(ddai.UID),
				Spec:       ddai.Spec,
			})
		}
	} else {
		cmf.logger.Error(err, "Error listing DatadogAgentInstances")
	}

	// DAP
	dapList := &v1alpha1.DatadogAgentProfileList{}
	if err := cmf.k8sClient.List(context.TODO(), dapList); err == nil {
		for _, dap := range dapList.Items {
			allCRDs = append(allCRDs, CRDInstance{
				Kind:       "DatadogAgentProfile",
				Name:       dap.Name,
				Namespace:  dap.Namespace,
				APIVersion: dap.APIVersion,
				UID:        string(dap.UID),
				Spec:       dap.Spec,
			})
		}
	} else {
		cmf.logger.Error(err, "Error listing DatadogAgentProfiles")
	}

	return allCRDs
}

func (cmf *CRDMetadataForwarder) setCredentials() error {
	return cmf.SharedMetadata.setCredentials()
}

func (cmf *CRDMetadataForwarder) getHeaders() http.Header {
	headers := cmf.GetBaseHeaders()
	headers.Set(userAgentHTTPHeaderKey, fmt.Sprintf("Datadog Operator/%s", version.GetVersion()))
	return headers
}
