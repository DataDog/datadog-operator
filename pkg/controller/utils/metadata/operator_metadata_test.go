// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metadata

import (
	"encoding/json"
	"os"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	testutils_test "github.com/DataDog/datadog-operator/internal/controller/datadogagent/testutils"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/stretchr/testify/assert"
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
				os.Clearenv()
				os.Setenv("DD_CLUSTER_NAME", "cluster")
				os.Setenv("DD_API_KEY", "api-key")
				os.Setenv("DD_APP_KEY", "app-key")
			},
			wantURL: "https://app.datadoghq.com/api/v1/metadata",
		},
		{
			name: "set DD_SITE",
			loadFunc: func() {
				os.Clearenv()
				os.Setenv("DD_SITE", "datad0g.com")
				os.Setenv("DD_API_KEY", "api-key")
				os.Setenv("DD_APP_KEY", "app-key")
			},
			wantURL: "https://app.datad0g.com/api/v1/metadata",
		},
		{
			name: "set DD_URL",
			loadFunc: func() {
				os.Clearenv()
				os.Setenv("DD_URL", "https://app.datad0g.com")
				os.Setenv("DD_API_KEY", "api-key")
				os.Setenv("DD_APP_KEY", "app-key")
			},
			wantURL: "https://app.datad0g.com/api/v1/metadata",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.loadFunc()
			s := testutils_test.TestScheme()

			clientObjects := []client.Object{}
			kubeSystem := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "kube-system",
					UID:  "test-cluster-uid",
				},
			}
			clientObjects = append(clientObjects, kubeSystem)

			client := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&v2alpha1.DatadogAgent{}).WithObjects(clientObjects...).Build()
			// Create BaseForwarder to test URL generation
			sm := NewBaseForwarder(zap.New(zap.UseDevMode(true)), client, config.NewCredentialManager(client))
			request, err := sm.createRequest([]byte("test"))
			assert.Nil(t, err)

			if request.URL.String() != tt.wantURL {
				t.Errorf("getURL() url = %v, want %v", request.URL, tt.wantURL)

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
			tt.loadFunc()

			s := testutils_test.TestScheme()
			kubeSystem := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "kube-system",
					UID:  "test-cluster-uid",
				},
			}
			client := fake.NewClientBuilder().
				WithScheme(s).
				WithStatusSubresource(&v2alpha1.DatadogAgent{}).
				WithObjects(tt.dda, kubeSystem).
				Build()
			// Create OperatorMetadataForwarder with the new structure
			sharedMetadata, _ := NewSharedMetadata("v1.19.0", "v1.28.0", client)
			omf := &OperatorMetadataForwarder{
				BaseForwarder:  NewBaseForwarder(zap.New(zap.UseDevMode(true)), client, config.NewCredentialManager(client)),
				SharedMetadata: sharedMetadata,
				OperatorMetadata: OperatorMetadata{
					ResourceCounts: make(map[string]int),
				},
			}

			_, err := omf.createRequest([]byte("test"))
			assert.Nil(t, err)
			apiKey, requestURL, err := omf.getApiKeyAndURL()

			assert.Nil(t, err)
			assert.Equal(t, tt.wantAPIKey, *apiKey)
			assert.Equal(t, tt.wantURL, *requestURL)
		})
	}
}

// Test that payload generation works correctly
func Test_GetPayload(t *testing.T) {
	expectedKubernetesVersion := "v1.28.0"
	expectedOperatorVersion := "v1.19.0"
	expectedClusterUID := "test-cluster-uid-12345"

	s := testutils_test.TestScheme()
	kubeSystem := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-system",
			UID:  "test-cluster-uid-12345",
		},
	}
	client := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&v2alpha1.DatadogAgent{}, kubeSystem).Build()
	sharedMetadata, _ := NewSharedMetadata(expectedOperatorVersion, expectedKubernetesVersion, client)
	omf := &OperatorMetadataForwarder{
		BaseForwarder:  NewBaseForwarder(zap.New(zap.UseDevMode(true)), client, config.NewCredentialManager(client)),
		SharedMetadata: sharedMetadata,
		OperatorMetadata: OperatorMetadata{
			IsLeader:       true,
			ResourceCounts: make(map[string]int),
		},
	}

	payload := omf.GetPayload()

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

// Test that GetPayload is safe for concurrent access (no data races)
func Test_GetPayload_Concurrent(t *testing.T) {
	s := testutils_test.TestScheme()
	kubeSystem := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-system",
			UID:  "test-cluster-uid-12345",
		},
	}
	client := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&v2alpha1.DatadogAgent{}, kubeSystem).Build()
	sharedMetadata, _ := NewSharedMetadata("v1.19.0", "v1.28.0", client)
	omf := &OperatorMetadataForwarder{
		BaseForwarder:  NewBaseForwarder(zap.New(zap.UseDevMode(true)), client, config.NewCredentialManager(client)),
		SharedMetadata: sharedMetadata,
		OperatorMetadata: OperatorMetadata{
			IsLeader:                    true,
			DatadogAgentEnabled:         true,
			DatadogMonitorEnabled:       true,
			DatadogAgentInternalEnabled: true,
			ResourceCounts:              map[string]int{"datadogagent": 5, "datadogmonitor": 10},
		},
	}

	// Run GetPayload concurrently from multiple goroutines
	const numGoroutines = 50
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			// Call GetPayload multiple times
			for j := 0; j < 10; j++ {
				payload := omf.GetPayload()
				if len(payload) == 0 {
					t.Errorf("Goroutine %d: GetPayload() returned empty payload", id)
				}
			}
			done <- true
		}(i)
	}

	updateDone := make(chan bool, 1)
	go func() {
		for i := 0; i < 10; i++ {
			omf.updateResourceCounts()
		}
		updateDone <- true
	}()

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}
	<-updateDone
}
