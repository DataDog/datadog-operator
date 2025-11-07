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
		nil,
		EnabledCRDKindsConfig{
			DatadogAgentEnabled:         true,
			DatadogAgentInternalEnabled: true,
			DatadogAgentProfileEnabled:  true,
		},
	)

	// Set cluster name in SharedMetadata to simulate it being populated
	cmf.clusterName = expectedClusterName

	// Set cluster UID in SharedMetadata to simulate it being populated
	cmf.clusterUID = expectedClusterUID

	// Create a test CRD instance
	testSpec := map[string]interface{}{
		"global": map[string]interface{}{
			"credentials": map[string]interface{}{
				"apiKey": "secret-key",
			},
		},
	}

	crdInstance := CRDInstance{
		Kind:       expectedCRDKind,
		Name:       expectedCRDName,
		Namespace:  expectedCRDNamespace,
		APIVersion: expectedCRDAPIVersion,
		UID:        expectedCRDUID,
		Spec:       testSpec,
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

	if clusterName, ok := metadata["cluster_name"].(string); !ok || clusterName != expectedClusterName {
		t.Errorf("buildPayload() metadata.cluster_name = %v, want %v", clusterName, expectedClusterName)
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
}
