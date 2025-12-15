// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package metadata

import (
	"encoding/json"
	"testing"

	"github.com/DataDog/datadog-operator/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// Test that payload generation works correctly for CRD metadata
func Test_CRDBuildPayload(t *testing.T) {
	expectedKubernetesVersion := "v1.28.0"
	expectedOperatorVersion := "v1.19.0"
	expectedClusterUID := "test-cluster-uid-12345"
	expectedCRDKind := "DatadogAgent"
	expectedCRDName := "my-datadog-agent"
	expectedCRDNamespace := "datadog"
	expectedCRDAPIVersion := "datadoghq.com/v2alpha1"
	expectedCRDUID := "crd-uid-67890"

	cmf := NewCRDMetadataForwarder(
		zap.New(zap.UseDevMode(true)),
		nil,
		expectedKubernetesVersion,
		expectedOperatorVersion,
		config.NewCredentialManager(fake.NewFakeClient()),
		EnabledCRDKindsConfig{
			DatadogAgentEnabled:         true,
			DatadogAgentInternalEnabled: true,
			DatadogAgentProfileEnabled:  true,
		},
	)

	// Create a test CRD instance
	testSpec := map[string]interface{}{
		"global": map[string]interface{}{
			"credentials": map[string]interface{}{
				"apiKey": "secret-key",
			},
		},
	}
	testLabels := map[string]string{
		"app": "datadog-agent",
		"env": "test",
	}
	testAnnotations := map[string]string{
		"owner":   "sre-team",
		"version": "1.0",
	}

	crdInstance := CRDInstance{
		Kind:        expectedCRDKind,
		Name:        expectedCRDName,
		Namespace:   expectedCRDNamespace,
		APIVersion:  expectedCRDAPIVersion,
		UID:         expectedCRDUID,
		Spec:        testSpec,
		Labels:      testLabels,
		Annotations: testAnnotations,
	}

	payload := cmf.buildPayload(expectedClusterUID, crdInstance)

	// Verify payload is valid JSON
	if len(payload) == 0 {
		t.Error("buildPayload() returned empty payload")
	}

	// Parse JSON to validate specific values
	var parsed map[string]interface{}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("buildPayload() returned invalid JSON: %v", err)
	}

	if timestamp, ok := parsed["timestamp"].(float64); !ok || timestamp <= 0 {
		t.Errorf("buildPayload() timestamp = %v, want positive number", timestamp)
	}

	if uuid, ok := parsed["uuid"].(string); !ok || uuid != expectedClusterUID {
		t.Errorf("buildPayload() uuid = %v, want %v", uuid, expectedClusterUID)
	}

	if clusterID, ok := parsed["cluster_id"].(string); !ok || clusterID != expectedClusterUID {
		t.Errorf("buildPayload() cluster_id = %v, want %v", clusterID, expectedClusterUID)
	}

	// Validate metadata object exists
	metadata, ok := parsed["datadog_operator_crd_metadata"].(map[string]interface{})
	if !ok {
		t.Fatal("buildPayload() missing or invalid datadog_operator_crd_metadata")
	}

	// Validate CRD-specific fields in metadata
	if operatorVersion, ok := metadata["operator_version"].(string); !ok || operatorVersion != expectedOperatorVersion {
		t.Errorf("buildPayload() metadata.operator_version = %v, want %v", operatorVersion, expectedOperatorVersion)
	}

	if kubernetesVersion, ok := metadata["kubernetes_version"].(string); !ok || kubernetesVersion != expectedKubernetesVersion {
		t.Errorf("buildPayload() metadata.kubernetes_version = %v, want %v", kubernetesVersion, expectedKubernetesVersion)
	}

	if clusterID, ok := metadata["cluster_id"].(string); !ok || clusterID != expectedClusterUID {
		t.Errorf("buildPayload() metadata.cluster_id = %v, want %v", clusterID, expectedClusterUID)
	}

	if crdKind, ok := metadata["crd_kind"].(string); !ok || crdKind != expectedCRDKind {
		t.Errorf("buildPayload() metadata.crd_kind = %v, want %v", crdKind, expectedCRDKind)
	}

	if crdName, ok := metadata["crd_name"].(string); !ok || crdName != expectedCRDName {
		t.Errorf("buildPayload() metadata.crd_name = %v, want %v", crdName, expectedCRDName)
	}

	if crdNamespace, ok := metadata["crd_namespace"].(string); !ok || crdNamespace != expectedCRDNamespace {
		t.Errorf("buildPayload() metadata.crd_namespace = %v, want %v", crdNamespace, expectedCRDNamespace)
	}

	if crdAPIVersion, ok := metadata["crd_api_version"].(string); !ok || crdAPIVersion != expectedCRDAPIVersion {
		t.Errorf("buildPayload() metadata.crd_api_version = %v, want %v", crdAPIVersion, expectedCRDAPIVersion)
	}

	if crdUID, ok := metadata["crd_uid"].(string); !ok || crdUID != expectedCRDUID {
		t.Errorf("buildPayload() metadata.crd_uid = %v, want %v", crdUID, expectedCRDUID)
	}

	// Validate crd_spec_full exists and is valid JSON
	if crdSpecFull, ok := metadata["crd_spec_full"].(string); !ok || crdSpecFull == "" {
		t.Errorf("buildPayload() metadata.crd_spec_full = %v, want non-empty JSON string", crdSpecFull)
	} else {
		// Verify it's valid JSON
		var specParsed map[string]interface{}
		if err := json.Unmarshal([]byte(crdSpecFull), &specParsed); err != nil {
			t.Errorf("buildPayload() metadata.crd_spec_full is not valid JSON: %v", err)
		}
	}

	// Validate crd_labels (stored as JSON string in the payload)
	if crdLabelsJSON, ok := metadata["crd_labels"].(string); !ok {
		t.Errorf("buildPayload() metadata.crd_labels type = %T, want string", metadata["crd_labels"])
	} else {
		// Parse the JSON string to validate contents
		var labels map[string]string
		if err := json.Unmarshal([]byte(crdLabelsJSON), &labels); err != nil {
			t.Errorf("buildPayload() metadata.crd_labels invalid JSON: %v", err)
		} else if labels["app"] != "datadog-agent" || labels["env"] != "test" {
			t.Errorf("buildPayload() metadata.crd_labels = %v, want app=datadog-agent, env=test", labels)
		}
	}

	// Validate crd_annotations (stored as JSON string in the payload)
	if crdAnnotationsJSON, ok := metadata["crd_annotations"].(string); !ok {
		t.Errorf("buildPayload() metadata.crd_annotations type = %T, want string", metadata["crd_annotations"])
	} else {
		// Parse the JSON string to validate contents
		var annotations map[string]string
		if err := json.Unmarshal([]byte(crdAnnotationsJSON), &annotations); err != nil {
			t.Errorf("buildPayload() metadata.crd_annotations invalid JSON: %v", err)
		} else if annotations["owner"] != "sre-team" || annotations["version"] != "1.0" {
			t.Errorf("buildPayload() metadata.crd_annotations = %v, want owner=sre-team, version=1.0", annotations)
		}
	}
}

// Test that hash-based change detection works correctly
func Test_CRDCacheDetection(t *testing.T) {
	cmf := NewCRDMetadataForwarder(
		zap.New(zap.UseDevMode(true)),
		nil,
		"v1.28.0",
		"v1.19.0",
		config.NewCredentialManager(fake.NewFakeClient()),
		EnabledCRDKindsConfig{
			DatadogAgentEnabled:         true,
			DatadogAgentInternalEnabled: true,
			DatadogAgentProfileEnabled:  true,
		},
	)

	crd1 := CRDInstance{
		Kind:        "DatadogAgent",
		Name:        "test-agent",
		Namespace:   "default",
		Spec:        map[string]interface{}{"version": "7.50.0"},
		Labels:      map[string]string{"app": "agent"},
		Annotations: map[string]string{"owner": "team-a"},
	}

	crd2 := CRDInstance{
		Kind:        "DatadogAgent",
		Name:        "test-agent-2",
		Namespace:   "default",
		Spec:        map[string]interface{}{"version": "7.51.0"},
		Labels:      map[string]string{"app": "agent"},
		Annotations: map[string]string{"owner": "team-b"},
	}
	// First call - both CRDs should be new (changed)
	changed := cmf.getChangedCRDs([]CRDInstance{crd1, crd2})
	if len(changed) != 2 {
		t.Errorf("Expected 2 changed CRDs on first run, got %d", len(changed))
	}

	// Second call with same specs - no changes expected
	changed = cmf.getChangedCRDs([]CRDInstance{crd1, crd2})
	if len(changed) != 0 {
		t.Errorf("Expected 0 changed CRDs on second run, got %d", len(changed))
	}

	// Modify crd1 spec
	crd1Modified := crd1
	crd1Modified.Spec = map[string]interface{}{"version": "7.52.0"}

	// Third call with modified crd1 spec - only 1 change expected
	changed = cmf.getChangedCRDs([]CRDInstance{crd1Modified, crd2})
	if len(changed) != 1 {
		t.Errorf("Expected 1 changed CRD after spec modification, got %d", len(changed))
	}
	if len(changed) > 0 && changed[0].Name != "test-agent" {
		t.Errorf("Expected changed CRD to be 'test-agent', got '%s'", changed[0].Name)
	}

	// Modify crd1 labels - should detect change
	crd1ModifiedLabels := crd1
	crd1ModifiedLabels.Labels = map[string]string{"app": "agent", "env": "prod"}

	changed = cmf.getChangedCRDs([]CRDInstance{crd1ModifiedLabels, crd2})
	if len(changed) != 1 {
		t.Errorf("Expected 1 changed CRD after label modification, got %d", len(changed))
	}

	// Modify crd1 annotations - should detect change
	crd1ModifiedAnnotations := crd1ModifiedLabels
	crd1ModifiedAnnotations.Annotations = map[string]string{"owner": "team-c"}

	changed = cmf.getChangedCRDs([]CRDInstance{crd1ModifiedAnnotations, crd2})
	if len(changed) != 1 {
		t.Errorf("Expected 1 changed CRD after annotation modification, got %d", len(changed))
	}
}

// Test that cache cleanup works correctly
func Test_CRDCacheCleanup(t *testing.T) {
	cmf := NewCRDMetadataForwarder(
		zap.New(zap.UseDevMode(true)),
		nil,
		"v1.28.0",
		"v1.19.0",
		config.NewCredentialManager(fake.NewFakeClient()),
		EnabledCRDKindsConfig{DatadogAgentEnabled: true},
	)

	crd1 := CRDInstance{
		Kind:      "DatadogAgent",
		Name:      "test-agent",
		Namespace: "default",
		Spec:      map[string]interface{}{"version": "7.50.0"},
	}

	crd2 := CRDInstance{
		Kind:      "DatadogAgent",
		Name:      "test-agent-2",
		Namespace: "default",
		Spec:      map[string]interface{}{"version": "7.51.0"},
	}

	successfulKinds := map[string]bool{"DatadogAgent": true}

	// Add both CRDs to cache
	cmf.getChangedCRDs([]CRDInstance{crd1, crd2})

	cmf.cacheMutex.RLock()
	initialCacheSize := len(cmf.crdCache)
	cmf.cacheMutex.RUnlock()
	if initialCacheSize != 2 {
		t.Errorf("Expected cache size 2, got %d", initialCacheSize)
	}

	// Remove crd2 and cleanup
	cmf.cleanupDeletedCRDs([]CRDInstance{crd1}, successfulKinds)

	cmf.cacheMutex.RLock()
	finalCacheSize := len(cmf.crdCache)
	cmf.cacheMutex.RUnlock()
	if finalCacheSize != 1 {
		t.Errorf("Expected cache size 1 after cleanup, got %d", finalCacheSize)
	}
}

// Test that per-kind error handling preserves cache correctly
func Test_CRDPerKindErrorHandling(t *testing.T) {
	cmf := NewCRDMetadataForwarder(
		zap.New(zap.UseDevMode(true)),
		nil,
		"v1.28.0",
		"v1.19.0",
		config.NewCredentialManager(fake.NewFakeClient()),
		EnabledCRDKindsConfig{
			DatadogAgentEnabled:         true,
			DatadogAgentInternalEnabled: true,
		},
	)

	ddaCRD := CRDInstance{
		Kind:      "DatadogAgent",
		Name:      "test-dda",
		Namespace: "default",
		Spec:      map[string]interface{}{"version": "7.50.0"},
	}

	ddaiCRD := CRDInstance{
		Kind:      "DatadogAgentInternal",
		Name:      "test-ddai",
		Namespace: "default",
		Spec:      map[string]interface{}{"version": "7.50.0"},
	}

	cmf.getChangedCRDs([]CRDInstance{ddaCRD, ddaiCRD})

	cmf.cacheMutex.RLock()
	cacheSize := len(cmf.crdCache)
	cmf.cacheMutex.RUnlock()
	if cacheSize != 2 {
		t.Errorf("Expected cache size 2, got %d", cacheSize)
	}

	// Second run: DatadogAgent successful, DatadogAgentInternal failed
	onlyDDASuccessful := map[string]bool{"DatadogAgent": true}

	// Filter should only process DDA (no changes since spec is same)
	changed := cmf.getChangedCRDs([]CRDInstance{ddaCRD})
	if len(changed) != 0 {
		t.Errorf("Expected 0 changed CRDs for DDA (unchanged spec), got %d", len(changed))
	}

	// Cleanup should only process DDA (DDAI cache should be preserved)
	cmf.cleanupDeletedCRDs([]CRDInstance{ddaCRD}, onlyDDASuccessful)

	// Verify cache still has 2 entries (DDAI not cleaned up because it failed to list)
	cmf.cacheMutex.RLock()
	finalCacheSize := len(cmf.crdCache)
	cmf.cacheMutex.RUnlock()
	if finalCacheSize != 2 {
		t.Errorf("Expected cache size 2 (DDAI preserved), got %d", finalCacheSize)
	}
}

// Test buildCacheKey function
func Test_BuildCacheKey(t *testing.T) {
	crd := CRDInstance{
		Kind:      "DatadogAgent",
		Name:      "my-agent",
		Namespace: "datadog",
	}

	key := buildCacheKey(crd)
	expected := "DatadogAgent/datadog/my-agent"
	if key != expected {
		t.Errorf("buildCacheKey() = %s, want %s", key, expected)
	}
}

// Test hashCRD function
func Test_HashCRD(t *testing.T) {
	crd1 := CRDInstance{
		Kind:      "DatadogAgent",
		Name:      "test",
		Namespace: "default",
		Spec: map[string]interface{}{
			"version": "7.50.0",
			"image":   "datadog/agent:7.50.0",
		},
		Labels:      map[string]string{"app": "agent"},
		Annotations: map[string]string{"owner": "team"},
	}

	crd2 := CRDInstance{
		Kind:      "DatadogAgent",
		Name:      "test",
		Namespace: "default",
		Spec: map[string]interface{}{
			"version": "7.50.0",
			"image":   "datadog/agent:7.50.0",
		},
		Labels:      map[string]string{"app": "agent"},
		Annotations: map[string]string{"owner": "team"},
	}

	crd3 := CRDInstance{
		Kind:      "DatadogAgent",
		Name:      "test",
		Namespace: "default",
		Spec: map[string]interface{}{
			"version": "7.51.0",
			"image":   "datadog/agent:7.51.0",
		},
		Labels:      map[string]string{"app": "agent"},
		Annotations: map[string]string{"owner": "team"},
	}

	crd4 := CRDInstance{
		Kind:      "DatadogAgent",
		Name:      "test",
		Namespace: "default",
		Spec: map[string]interface{}{
			"version": "7.50.0",
			"image":   "datadog/agent:7.50.0",
		},
		Labels:      map[string]string{"app": "agent", "env": "prod"}, // Different labels
		Annotations: map[string]string{"owner": "team"},
	}

	hash1, err := hashCRD(crd1)
	if err != nil {
		t.Fatalf("hashCRD failed: %v", err)
	}

	hash2, err := hashCRD(crd2)
	if err != nil {
		t.Fatalf("hashCRD failed: %v", err)
	}

	hash3, err := hashCRD(crd3)
	if err != nil {
		t.Fatalf("hashCRD failed: %v", err)
	}

	hash4, err := hashCRD(crd4)
	if err != nil {
		t.Fatalf("hashCRD failed: %v", err)
	}

	// Same CRDs (spec, labels, annotations) should produce same hash
	if hash1 != hash2 {
		t.Errorf("Expected same hash for identical CRDs, got %s and %s", hash1, hash2)
	}

	// Different specs should produce different hash
	if hash1 == hash3 {
		t.Errorf("Expected different hash for different specs, both got %s", hash1)
	}

	// Different labels should produce different hash
	if hash1 == hash4 {
		t.Errorf("Expected different hash for different labels, both got %s", hash1)
	}
}
