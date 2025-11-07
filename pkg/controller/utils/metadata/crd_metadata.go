// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package metadata

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
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

	payloadHeader http.Header
	enabledCRDs   EnabledCRDKindsConfig

	crdCache   map[string]string // key: "kind/namespace/name", value: hash of spec
	cacheMutex sync.RWMutex
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

// EnabledCRDsConfig specifies which CRD kinds are enabled for metadata collection
type EnabledCRDKindsConfig struct {
	DatadogAgentEnabled         bool
	DatadogAgentInternalEnabled bool
	DatadogAgentProfileEnabled  bool
}

// NewCRDMetadataForwarder creates a new instance of the CRD metadata forwarder
func NewCRDMetadataForwarder(logger logr.Logger, k8sClient client.Reader, kubernetesVersion string, operatorVersion string, credsManager *config.CredentialManager, config EnabledCRDKindsConfig) *CRDMetadataForwarder {
	return &CRDMetadataForwarder{
		SharedMetadata: NewSharedMetadata(logger, k8sClient, kubernetesVersion, operatorVersion, credsManager),
		enabledCRDs:    config,
		crdCache:       make(map[string]string),
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

	allCRDs, listSuccess := cmf.getAllActiveCRDs()
	changedCRDs := cmf.getChangedCRDs(allCRDs)

	if len(changedCRDs) == 0 {
		cmf.logger.V(1).Info("No CRD changes detected")
		return nil
	}

	cmf.logger.Info("Detected CRD changes", "count", len(changedCRDs))

	// Send individual payloads for each changed CRD
	for _, crd := range changedCRDs {
		if err := cmf.sendCRDMetadata(clusterUID, crd); err != nil {
			cmf.logger.Error(err, "Failed to send CRD metadata",
				"kind", crd.Kind, "name", crd.Name, "namespace", crd.Namespace)
		}
	}

	cmf.cleanupDeletedCRDs(allCRDs, listSuccess)
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

// getAllActiveCRDs returns all active CRDs and a map of list successes for each CRD type
// Currently only DatadogAgent, DatadogAgentInternal, and DatadogAgentProfile are collected
func (cmf *CRDMetadataForwarder) getAllActiveCRDs() ([]CRDInstance, map[string]bool) {
	var crds []CRDInstance
	listSuccess := make(map[string]bool)

	if cmf.k8sClient == nil {
		return crds, listSuccess
	}
	// DDA
	if cmf.enabledCRDs.DatadogAgentEnabled {
		ddaList := &v2alpha1.DatadogAgentList{}
		if err := cmf.k8sClient.List(context.TODO(), ddaList); err == nil {
			listSuccess["DatadogAgent"] = true
			for _, dda := range ddaList.Items {
				crds = append(crds, CRDInstance{
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
	}

	// DDAI
	if cmf.enabledCRDs.DatadogAgentInternalEnabled {
		ddaiList := &v1alpha1.DatadogAgentInternalList{}
		if err := cmf.k8sClient.List(context.TODO(), ddaiList); err == nil {
			listSuccess["DatadogAgentInternal"] = true
			for _, ddai := range ddaiList.Items {
				crds = append(crds, CRDInstance{
					Kind:       "DatadogAgentInternal",
					Name:       ddai.Name,
					Namespace:  ddai.Namespace,
					APIVersion: ddai.APIVersion,
					UID:        string(ddai.UID),
					Spec:       ddai.Spec,
				})
			}
		} else {
			cmf.logger.Error(err, "Error listing DatadogAgentInternals")
		}
	}

	// DAP
	if cmf.enabledCRDs.DatadogAgentProfileEnabled {
		dapList := &v1alpha1.DatadogAgentProfileList{}
		if err := cmf.k8sClient.List(context.TODO(), dapList); err == nil {
			listSuccess["DatadogAgentProfile"] = true
			for _, dap := range dapList.Items {
				crds = append(crds, CRDInstance{
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
	}

	return crds, listSuccess
}

func (cmf *CRDMetadataForwarder) setCredentials() error {
	return cmf.SharedMetadata.setCredentials()
}

func (cmf *CRDMetadataForwarder) getHeaders() http.Header {
	headers := cmf.GetBaseHeaders()
	headers.Set(userAgentHTTPHeaderKey, fmt.Sprintf("Datadog Operator/%s", version.GetVersion()))
	return headers
}

// getChangedCRDs returns only CRDs whose specs have changed and updates the cache
func (cmf *CRDMetadataForwarder) getChangedCRDs(crds []CRDInstance) []CRDInstance {
	cmf.cacheMutex.Lock()
	defer cmf.cacheMutex.Unlock()

	var changed []CRDInstance
	for _, crd := range crds {
		key := getCacheKey(crd)
		newHash, err := hashCRDSpec(crd.Spec)
		if err != nil {
			cmf.logger.Error(err, "Failed to hash CRD spec", "key", key)
			continue
		}

		if oldHash, exists := cmf.crdCache[key]; !exists || oldHash != newHash {
			changed = append(changed, crd)
			cmf.crdCache[key] = newHash
		}
	}

	return changed
}

// cleanupDeletedCRDs removes cache entries for CRDs that got deleted
func (cmf *CRDMetadataForwarder) cleanupDeletedCRDs(currentCRDs []CRDInstance, successfulKinds map[string]bool) {
	cmf.cacheMutex.Lock()
	defer cmf.cacheMutex.Unlock()

	currentKeys := make(map[string]bool)
	for _, crd := range currentCRDs {
		currentKeys[getCacheKey(crd)] = true
	}

	for key := range cmf.crdCache {
		cachedKind, _, found := strings.Cut(key, "/")
		if !found {
			continue
		}

		// Only clean up cache for kinds that were successfully listed
		if successfulKinds[cachedKind] {
			if !currentKeys[key] {
				delete(cmf.crdCache, key)
				cmf.logger.V(1).Info("Removed deleted CRD from cache", "key", key)
			}
		}
	}
}

// Cache helper functions
// getCacheKey returns a unique key for a CRD instance, with format "kind/namespace/name"
func getCacheKey(crd CRDInstance) string {
	return fmt.Sprintf("%s/%s/%s", crd.Kind, crd.Namespace, crd.Name)
}

// hashCRDSpec computes a SHA256 hash of the CRD spec
func hashCRDSpec(spec interface{}) (string, error) {
	specJSON, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(specJSON)
	return fmt.Sprintf("%x", hash), nil
}
