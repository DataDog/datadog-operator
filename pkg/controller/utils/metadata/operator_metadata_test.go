// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metadata

import (
	"encoding/json"
	"os"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/pkg/config"
)

func Test_getURL(t *testing.T) {
	tests := []struct {
		name     string
		loadFunc func()
		wantURL  string
	}{
		{
			name: "default case",
			loadFunc: func() {
			},
			wantURL: "https://app.datadoghq.com/api/v1/metadata",
		},
		{
			name: "set DD_SITE",
			loadFunc: func() {
				os.Clearenv()
				os.Setenv("DD_SITE", "datad0g.com")
			},
			wantURL: "https://app.datad0g.com/api/v1/metadata",
		},
		{
			name: "set DD_URL",
			loadFunc: func() {
				os.Clearenv()
				os.Setenv("DD_URL", "https://app.datad0g.com")
			},
			wantURL: "https://app.datad0g.com/api/v1/metadata",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.loadFunc()

			// Create SharedMetadata to test URL generation
			sm := NewSharedMetadata(zap.New(zap.UseDevMode(true)), nil, "v1.28.0", "v1.19.0", config.NewCredentialManager())

			if sm.requestURL != tt.wantURL {
				t.Errorf("getURL() url = %v, want %v", sm.requestURL, tt.wantURL)

			}
		})
	}
}

// Test setup of API key, cluster name and URL with Operator and DDA
func Test_setup(t *testing.T) {
	fakeAPIKeyDDA := "fake_api_key_dda"
	fakeAPPKeyDDA := "fake_app_key_dda"
	fakeClusterNameDDA := "fake_cluster_name_dda"

	fakeAPIKeyOperator := "fake_api_key_operator"
	fakeClusterNameOperator := "fake_cluster_name_operator"

	tests := []struct {
		name            string
		loadFunc        func()
		dda             *v2alpha1.DatadogAgent
		wantClusterName string
		wantAPIKey      string
		wantURL         string
	}{
		{
			name: "default case, credentials set in Operator, empty DDA",
			loadFunc: func() {
				os.Clearenv()
				os.Setenv("DD_API_KEY", fakeAPIKeyOperator)
				os.Setenv("DD_APP_KEY", fakeAPPKeyDDA)
				os.Setenv("DD_CLUSTER_NAME", fakeClusterNameOperator)
			},
			dda:             &v2alpha1.DatadogAgent{},
			wantClusterName: "fake_cluster_name_operator",
			wantAPIKey:      "fake_api_key_operator",
			wantURL:         "https://app.datadoghq.com/api/v1/metadata",
		},
		{
			name: "cluster name set in Operator, API key set in DDA",
			loadFunc: func() {
				os.Clearenv()
				os.Setenv("DD_CLUSTER_NAME", fakeClusterNameOperator)
			},
			dda: &v2alpha1.DatadogAgent{
				Spec: v2alpha1.DatadogAgentSpec{
					Global: &v2alpha1.GlobalConfig{
						ClusterName: apiutils.NewStringPointer(fakeClusterNameDDA),
						Credentials: &v2alpha1.DatadogCredentials{
							APIKey: apiutils.NewStringPointer(fakeAPIKeyDDA),
						},
					},
				},
			},
			wantClusterName: "fake_cluster_name_operator",
			wantAPIKey:      "fake_api_key_dda",
			wantURL:         "https://app.datadoghq.com/api/v1/metadata",
		},
		{
			name: "credentials and site set in DDA",
			loadFunc: func() {
				os.Clearenv()
			},
			dda: &v2alpha1.DatadogAgent{
				Spec: v2alpha1.DatadogAgentSpec{
					Global: &v2alpha1.GlobalConfig{
						ClusterName: apiutils.NewStringPointer(fakeClusterNameDDA),
						Credentials: &v2alpha1.DatadogCredentials{
							APIKey: apiutils.NewStringPointer(fakeAPIKeyDDA),
						},
						Site: apiutils.NewStringPointer("datad0g.com"),
					},
				},
			},
			wantClusterName: "fake_cluster_name_dda",
			wantAPIKey:      "fake_api_key_dda",
			wantURL:         "https://app.datad0g.com/api/v1/metadata",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()

			// Create OperatorMetadataForwarder with the new structure
			omf := &OperatorMetadataForwarder{
				SharedMetadata: NewSharedMetadata(zap.New(zap.UseDevMode(true)), nil, "v1.28.0", "v1.19.0", config.NewCredentialManager()),
				OperatorMetadata: OperatorMetadata{
					ResourceCounts: make(map[string]int),
				},
			}

			tt.loadFunc()

			_ = omf.setupFromOperator()

			_ = omf.setupFromDDA(tt.dda)

			if omf.clusterName != tt.wantClusterName {
				t.Errorf("setupFromDDA() clusterName = %v, want %v", omf.clusterName, tt.wantClusterName)
			}

			if omf.apiKey != tt.wantAPIKey {
				t.Errorf("setupFromDDA() apiKey = %v, want %v", omf.apiKey, tt.wantAPIKey)
			}

			if omf.requestURL != tt.wantURL {
				t.Errorf("setupFromDDA() url = %v, want %v", omf.requestURL, tt.wantURL)
			}
		})
	}
}

// Test that payload generation works correctly
func Test_GetPayload(t *testing.T) {
	expectedKubernetesVersion := "v1.28.0"
	expectedOperatorVersion := "v1.19.0"
	expectedClusterName := "test-cluster"
	expectedClusterUID := "test-cluster-uid-12345"

	omf := &OperatorMetadataForwarder{
		SharedMetadata: NewSharedMetadata(zap.New(zap.UseDevMode(true)), nil, expectedKubernetesVersion, expectedOperatorVersion, config.NewCredentialManager()),
		OperatorMetadata: OperatorMetadata{
			ClusterName:    expectedClusterName,
			IsLeader:       true,
			ResourceCounts: make(map[string]int),
		},
	}

	// Set cluster name in SharedMetadata to simulate it being populated
	omf.clusterName = expectedClusterName

	// Set cluster UID in SharedMetadata to simulate it being populated
	omf.clusterUID = expectedClusterUID

	payload := omf.GetPayload(expectedClusterUID)

	// Verify payload is valid JSON
	if len(payload) == 0 {
		t.Error("GetPayload() returned empty payload")
	}

	// Parse JSON to validate specific values
	var parsed map[string]interface{}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("GetPayload() returned invalid JSON: %v", err)
	}

	// Validate top-level fields
	if timestamp, ok := parsed["timestamp"].(float64); !ok || timestamp <= 0 {
		t.Errorf("GetPayload() timestamp = %v, want positive number", timestamp)
	}

	if clusterID, ok := parsed["cluster_id"].(string); !ok || clusterID != expectedClusterUID {
		t.Errorf("GetPayload() cluster_id = %v, want %v", clusterID, expectedClusterUID)
	}

	if clusterName, ok := parsed["clustername"].(string); !ok || clusterName != expectedClusterName {
		t.Errorf("GetPayload() cluster_name = %v, want %v", clusterName, expectedClusterName)
	}

	// Validate metadata object exists
	metadata, ok := parsed["datadog_operator_metadata"].(map[string]interface{})
	if !ok {
		t.Fatal("GetPayload() missing or invalid datadog_operator_metadata")
	}

	// Validate specific metadata values
	if operatorVersion, ok := metadata["operator_version"].(string); !ok || operatorVersion != expectedOperatorVersion {
		t.Errorf("GetPayload() operator_version = %v, want %v", operatorVersion, expectedOperatorVersion)
	}

	if kubernetesVersion, ok := metadata["kubernetes_version"].(string); !ok || kubernetesVersion != expectedKubernetesVersion {
		t.Errorf("GetPayload() kubernetes_version = %v, want %v", kubernetesVersion, expectedKubernetesVersion)
	}

	if clusterName, ok := metadata["cluster_name"].(string); !ok || clusterName != expectedClusterName {
		t.Errorf("GetPayload() cluster_name = %v, want %v", clusterName, expectedClusterName)
	}

	if clusterID, ok := metadata["cluster_id"].(string); !ok || clusterID != expectedClusterUID {
		t.Errorf("GetPayload() cluster_id in metadata = %v, want %v", clusterID, expectedClusterUID)
	}

	if isLeader, ok := metadata["is_leader"].(bool); !ok || !isLeader {
		t.Errorf("GetPayload() is_leader = %v, want true", isLeader)
	}

	// Verify all expected fields exist
	expectedFields := []string{
		"operator_version",
		"kubernetes_version",
		"install_method_tool",
		"install_method_tool_version",
		"is_leader",
		"datadogagent_enabled",
		"datadogmonitor_enabled",
		"datadogdashboard_enabled",
		"datadogslo_enabled",
		"datadoggenericresource_enabled",
		"datadogagentprofile_enabled",
		"datadogagentinternal_enabled",
		"leader_election_enabled",
		"extendeddaemonset_enabled",
		"remote_config_enabled",
		"introspection_enabled",
		"cluster_id",
		"cluster_name",
		"config_dd_url",
		"config_site",
		"resource_count",
	}

	for _, field := range expectedFields {
		if _, exists := metadata[field]; !exists {
			t.Errorf("GetPayload() missing expected field: %s", field)
		}
	}

	// Verify resource_count is a valid map
	if resourceCount, ok := metadata["resource_count"].(map[string]interface{}); ok {
		// Valid map - verify it's structured correctly (values are numbers)
		for _, value := range resourceCount {
			if _, ok := value.(float64); !ok {
				t.Errorf("GetPayload() resource_count value is not a number: %v", value)
			}
		}
	} else {
		t.Errorf("GetPayload() resource_count is not a map, got: %T", metadata["resource_count"])
	}
}
