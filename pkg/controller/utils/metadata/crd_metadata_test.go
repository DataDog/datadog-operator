// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package metadata

import (
	"encoding/json"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// Test that payload generation works correctly for CRD metadata
func Test_CRDBuildPayload(t *testing.T) {
	expectedKubernetesVersion := "v1.28.0"
	expectedOperatorVersion := "v1.19.0"
	expectedClusterName := "test-cluster"
	expectedClusterUID := "test-cluster-uid-12345"
	expectedHostname := "test-host"

	cmf := &CRDMetadataForwarder{
		SharedMetadata: NewSharedMetadata(zap.New(zap.UseDevMode(true)), nil, expectedKubernetesVersion, expectedOperatorVersion, nil),
	}

	// Set hostname in SharedMetadata to simulate it being populated
	cmf.hostName = expectedHostname

	// Set cluster name in SharedMetadata to simulate it being populated
	cmf.clusterName = expectedClusterName

	// Set cluster UID in SharedMetadata to simulate it being populated
	cmf.clusterUID = expectedClusterUID

	payload := cmf.buildPayload(expectedClusterUID)

	// Verify payload is valid JSON
	if len(payload) == 0 {
		t.Error("buildPayload() returned empty payload")
	}

	// Parse JSON to validate specific values
	var parsed map[string]interface{}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("buildPayload() returned invalid JSON: %v", err)
	}

	// Validate top-level fields
	if hostname, ok := parsed["hostname"].(string); !ok || hostname != expectedHostname {
		t.Errorf("buildPayload() hostname = %v, want %v", hostname, expectedHostname)
	}

	if timestamp, ok := parsed["timestamp"].(float64); !ok || timestamp <= 0 {
		t.Errorf("buildPayload() timestamp = %v, want positive number", timestamp)
	}

	if clusterID, ok := parsed["cluster_id"].(string); !ok || clusterID != expectedClusterUID {
		t.Errorf("buildPayload() cluster_id = %v, want %v", clusterID, expectedClusterUID)
	}

	if clusterName, ok := parsed["clustername"].(string); !ok || clusterName != expectedClusterName {
		t.Errorf("buildPayload() cluster_name = %v, want %v", clusterName, expectedClusterName)
	}

	// Validate metadata object exists
	metadata, ok := parsed["datadog_crd_metadata"].(map[string]interface{})
	if !ok {
		t.Fatal("buildPayload() missing or invalid datadog_crd_metadata")
	}

	// TODO: Add validation for specific CRD metadata fields once they are populated
	_ = metadata
}
