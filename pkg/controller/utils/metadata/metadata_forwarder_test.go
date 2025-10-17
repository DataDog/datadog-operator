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

			u := getURL()

			if u != tt.wantURL {
				t.Errorf("getURL() url = %v, want %v", u, tt.wantURL)

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

			// Create MetadataForwarder with the new structure
			mdf := &MetadataForwarder{
				SharedMetadata: NewSharedMetadata(zap.New(zap.UseDevMode(true)), nil, "v1.28.0", "v1.19.0"),
				requestURL:     getURL(),
				credsManager:   config.NewCredentialManager(),
			}

			tt.loadFunc()

			_ = mdf.setupFromOperator()

			_ = mdf.setupFromDDA(tt.dda)

			if mdf.clusterName != tt.wantClusterName {
				t.Errorf("setupFromDDA() clusterName = %v, want %v", mdf.clusterName, tt.wantClusterName)
			}

			if mdf.apiKey != tt.wantAPIKey {
				t.Errorf("setupFromDDA() apiKey = %v, want %v", mdf.apiKey, tt.wantAPIKey)
			}

			if mdf.requestURL != tt.wantURL {
				t.Errorf("setupFromDDA() url = %v, want %v", mdf.requestURL, tt.wantURL)
			}
		})
	}
}

// Test that payload generation works correctly
func Test_GetPayload(t *testing.T) {
	expectedKubernetesVersion := "v1.28.0"
	expectedOperatorVersion := "v1.19.0"
	expectedClusterName := "test-cluster"
	expectedHostname := "test-host"

	mdf := &MetadataForwarder{
		SharedMetadata: NewSharedMetadata(zap.New(zap.UseDevMode(true)), nil, expectedKubernetesVersion, expectedOperatorVersion),
		hostName:       expectedHostname,

		OperatorMetadata: OperatorMetadata{
			ClusterName: expectedClusterName,
			IsLeader:    true,
		},
	}

	// Set cluster name in SharedMetadata to simulate it being populated
	mdf.clusterName = expectedClusterName

	payload := mdf.GetPayload()

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
	if hostname, ok := parsed["hostname"].(string); !ok || hostname != expectedHostname {
		t.Errorf("GetPayload() hostname = %v, want %v", hostname, expectedHostname)
	}

	if timestamp, ok := parsed["timestamp"].(float64); !ok || timestamp <= 0 {
		t.Errorf("GetPayload() timestamp = %v, want positive number", timestamp)
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
		"leader_election_enabled",
		"extendeddaemonset_enabled",
		"remote_config_enabled",
		"introspection_enabled",
		"cluster_name",
		"config_dd_url",
		"config_site",
	}

	for _, field := range expectedFields {
		if _, exists := metadata[field]; !exists {
			t.Errorf("GetPayload() missing expected field: %s", field)
		}
	}
}
