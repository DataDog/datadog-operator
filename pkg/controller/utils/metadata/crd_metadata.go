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
	"maps"
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
	UUID      string      `json:"uuid"`
	Timestamp int64       `json:"timestamp"`
	ClusterID string      `json:"cluster_id"`
	Metadata  CRDMetadata `json:"datadog_operator_crd_metadata"`
}

type CRDMetadata struct {
	// Shared
	OperatorVersion   string `json:"operator_version"`
	KubernetesVersion string `json:"kubernetes_version"`
	ClusterID         string `json:"cluster_id"`

	CRDKind            string `json:"crd_kind"`
	CRDName            string `json:"crd_name"`
	CRDNamespace       string `json:"crd_namespace"`
	CRDAPIVersion      string `json:"crd_api_version"`
	CRDUID             string `json:"crd_uid"`
	CRDSpecFull        string `json:"crd_spec_full"`
	CRDLabelsJSON      string `json:"crd_labels,omitempty"`
	CRDAnnotationsJSON string `json:"crd_annotations,omitempty"`
}

type CRDInstance struct {
	Kind        string            `json:"kind"`
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	APIVersion  string            `json:"api_version"`
	UID         string            `json:"uid"`
	Spec        interface{}       `json:"spec"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
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

	clusterUID, err := cmf.GetOrCreateClusterUID()
	if err != nil {
		cmf.logger.Error(err, "Could not get cluster UID; not starting CRD metadata forwarder")
		return
	}

	cmf.payloadHeader = cmf.getHeaders()

	cmf.logger.Info("Starting CRD metadata forwarder")

	ticker := time.NewTicker(crdMetadataInterval)
	go func() {
		for range ticker.C {
			if err := cmf.sendMetadata(clusterUID); err != nil {
				cmf.logger.Error(err, "Error while sending CRD metadata")
			}
		}
	}()
}

func (cmf *CRDMetadataForwarder) sendMetadata(clusterUID string) error {
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

// marshalToJSON marshals data to JSON, returning empty object on error
func (cmf *CRDMetadataForwarder) marshalToJSON(data interface{}, fieldName string, crdInstance CRDInstance) []byte {
	if data == nil {
		return nil
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		cmf.logger.Error(err, "Error marshaling CRD field to JSON",
			"field", fieldName,
			"kind", crdInstance.Kind,
			"name", crdInstance.Name)
		return []byte("{}")
	}
	return jsonBytes
}

func (cmf *CRDMetadataForwarder) buildPayload(clusterUID string, crdInstance CRDInstance) []byte {
	now := time.Now().Unix()

	specJSON := cmf.marshalToJSON(crdInstance.Spec, "spec", crdInstance)
	labelsJSON := cmf.marshalToJSON(crdInstance.Labels, "labels", crdInstance)
	annotationsJSON := cmf.marshalToJSON(crdInstance.Annotations, "annotations", crdInstance)

	crdMetadata := CRDMetadata{
		OperatorVersion:    cmf.operatorVersion,
		KubernetesVersion:  cmf.kubernetesVersion,
		ClusterID:          clusterUID,
		CRDKind:            crdInstance.Kind,
		CRDName:            crdInstance.Name,
		CRDNamespace:       crdInstance.Namespace,
		CRDAPIVersion:      crdInstance.APIVersion,
		CRDUID:             crdInstance.UID,
		CRDSpecFull:        string(specJSON),
		CRDLabelsJSON:      string(labelsJSON),
		CRDAnnotationsJSON: string(annotationsJSON),
	}

	payload := CRDMetadataPayload{
		UUID:      clusterUID,
		Timestamp: now,
		ClusterID: clusterUID,
		Metadata:  crdMetadata,
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
				annotations := maps.Clone(dda.Annotations)
				delete(annotations, "kubectl.kubernetes.io/last-applied-configuration")

				crds = append(crds, CRDInstance{
					Kind:        "DatadogAgent",
					Name:        dda.Name,
					Namespace:   dda.Namespace,
					APIVersion:  dda.APIVersion,
					UID:         string(dda.UID),
					Spec:        dda.Spec,
					Labels:      dda.Labels,
					Annotations: annotations,
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
				annotations := maps.Clone(ddai.Annotations)
				delete(annotations, "kubectl.kubernetes.io/last-applied-configuration")

				crds = append(crds, CRDInstance{
					Kind:        "DatadogAgentInternal",
					Name:        ddai.Name,
					Namespace:   ddai.Namespace,
					APIVersion:  ddai.APIVersion,
					UID:         string(ddai.UID),
					Spec:        ddai.Spec,
					Labels:      ddai.Labels,
					Annotations: annotations,
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
				annotations := maps.Clone(dap.Annotations)
				delete(annotations, "kubectl.kubernetes.io/last-applied-configuration")

				crds = append(crds, CRDInstance{
					Kind:        "DatadogAgentProfile",
					Name:        dap.Name,
					Namespace:   dap.Namespace,
					APIVersion:  dap.APIVersion,
					UID:         string(dap.UID),
					Spec:        dap.Spec,
					Labels:      dap.Labels,
					Annotations: annotations,
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
		key := buildCacheKey(crd)
		newHash, err := hashCRD(crd)
		if err != nil {
			cmf.logger.Error(err, "Failed to hash CRD", "key", key)
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
		currentKeys[buildCacheKey(crd)] = true
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
// buildCacheKey builds a unique key for a CRD instance, with format "kind/namespace/name"
func buildCacheKey(crd CRDInstance) string {
	return fmt.Sprintf("%s/%s/%s", crd.Kind, crd.Namespace, crd.Name)
}

// hashCRD computes a SHA256 hash of the CRD spec, labels, and annotations for change detection
func hashCRD(crd CRDInstance) (string, error) {
	// Hash spec, labels, and annotations together
	hashable := struct {
		Spec        interface{}       `json:"spec"`
		Labels      map[string]string `json:"labels,omitempty"`
		Annotations map[string]string `json:"annotations,omitempty"`
	}{
		Spec:        crd.Spec,
		Labels:      crd.Labels,
		Annotations: crd.Annotations,
	}

	hashableJSON, err := json.Marshal(hashable)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(hashableJSON)
	return fmt.Sprintf("%x", hash), nil
}
