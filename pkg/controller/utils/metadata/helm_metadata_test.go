// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package metadata

import (
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/DataDog/datadog-operator/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func Test_HelmMetadataForwarder_getPayload(t *testing.T) {
	expectedKubernetesVersion := "v1.28.0"
	expectedOperatorVersion := "v1.19.0"
	expectedClusterUID := "test-cluster-uid-123"
	expectedReleaseName := "my-release"
	expectedNamespace := "default"
	expectedChartName := "datadog"
	expectedChartVersion := "3.10.0"
	expectedAppVersion := "7.50.0"

	client := newFakeClientWithKubeSystem("test-cluster-uid-123")

	hmf := NewHelmMetadataForwarder(zap.New(zap.UseDevMode(true)), client, expectedKubernetesVersion, expectedOperatorVersion, config.NewCredentialManager(client))

	release := HelmReleaseData{
		ReleaseName:        expectedReleaseName,
		Namespace:          expectedNamespace,
		ChartName:          expectedChartName,
		ChartVersion:       expectedChartVersion,
		AppVersion:         expectedAppVersion,
		ConfigMapUID:       "configmap-uid-123",
		ProvidedValuesYAML: "datadog:\n  apiKey: xxx",
		FullValuesYAML:     "datadog:\n  apiKey: xxx\n  site: datadoghq.com",
		Revision:           1,
		Status:             "deployed",
	}

	payload := hmf.buildPayload(release)

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
	if timestamp, ok := parsed["timestamp"].(float64); !ok || timestamp <= 0 {
		t.Errorf("buildPayload() timestamp = %v, want positive number", timestamp)
	}

	// Validate metadata object exists
	metadata, ok := parsed["datadog_operator_helm_metadata"].(map[string]interface{})
	if !ok {
		t.Fatal("buildPayload() missing or invalid datadog_operator_helm_metadata")
	}

	// Validate specific metadata values
	if operatorVersion, ok := metadata["operator_version"].(string); !ok || operatorVersion != expectedOperatorVersion {
		t.Errorf("buildPayload() operator_version = %v, want %v", operatorVersion, expectedOperatorVersion)
	}

	if kubernetesVersion, ok := metadata["kubernetes_version"].(string); !ok || kubernetesVersion != expectedKubernetesVersion {
		t.Errorf("buildPayload() kubernetes_version = %v, want %v", kubernetesVersion, expectedKubernetesVersion)
	}

	if clusterID, ok := metadata["cluster_id"].(string); !ok || clusterID != expectedClusterUID {
		t.Errorf("buildPayload() cluster_id = %v, want %v", clusterID, expectedClusterUID)
	}

	if chartName, ok := metadata["chart_name"].(string); !ok || chartName != expectedChartName {
		t.Errorf("buildPayload() chart_name = %v, want %v", chartName, expectedChartName)
	}

	if releaseName, ok := metadata["chart_release_name"].(string); !ok || releaseName != expectedReleaseName {
		t.Errorf("buildPayload() chart_release_name = %v, want %v", releaseName, expectedReleaseName)
	}

	if chartVersion, ok := metadata["chart_version"].(string); !ok || chartVersion != expectedChartVersion {
		t.Errorf("buildPayload() chart_version = %v, want %v", chartVersion, expectedChartVersion)
	}

	if namespace, ok := metadata["chart_namespace"].(string); !ok || namespace != expectedNamespace {
		t.Errorf("buildPayload() chart_namespace = %v, want %v", namespace, expectedNamespace)
	}

	// Verify all expected fields exist
	expectedFields := []string{
		"operator_version",
		"kubernetes_version",
		"cluster_id",
		"chart_name",
		"chart_release_name",
		"chart_app_version",
		"chart_version",
		"chart_namespace",
		"chart_configmap_uid",
		"helm_provided_configuration",
		"helm_full_configuration",
	}

	for _, field := range expectedFields {
		if _, exists := metadata[field]; !exists {
			t.Errorf("buildPayload() missing expected field: %s", field)
		}
	}
}

func Test_parseHelmResource(t *testing.T) {
	client := newFakeClientWithKubeSystem("test-cluster-uid-123")

	hmf := NewHelmMetadataForwarder(zap.New(zap.UseDevMode(true)), client, "v1.28.0", "v1.19.0", config.NewCredentialManager(client))

	// Create a minimal valid Helm release JSON
	releaseData := HelmReleaseMinimal{
		Name:      "my-release",
		Namespace: "default",
		Version:   1,
	}
	releaseData.Info.Status = "deployed"
	releaseData.Chart.Metadata.Name = "datadog"
	releaseData.Chart.Metadata.Version = "3.10.0"
	releaseData.Chart.Metadata.AppVersion = "7.50.0"
	releaseData.Config = map[string]interface{}{"key": "value"}
	releaseData.Chart.Values = map[string]interface{}{"default": "value"}

	// Marshal to JSON
	jsonData, err := json.Marshal(releaseData)
	if err != nil {
		t.Fatalf("Failed to marshal test data: %v", err)
	}

	// Gzip compress
	var compressed []byte
	{
		writer := gzip.NewWriter(&testWriter{buf: &compressed})
		_, _ = writer.Write(jsonData)
		writer.Close()
	}

	// Base64 encode
	encoded := base64.StdEncoding.EncodeToString(compressed)

	tests := []struct {
		name            string
		resourceName    string
		data            []byte
		wantReleaseName string
		wantRevision    int
		wantOk          bool
	}{
		{
			name:            "valid secret name with revision 1",
			resourceName:    "sh.helm.release.v1.my-release.v1",
			data:            []byte(encoded),
			wantReleaseName: "my-release",
			wantRevision:    1,
			wantOk:          true,
		},
		{
			name:            "valid secret name with revision 5",
			resourceName:    "sh.helm.release.v1.my-app.v5",
			data:            []byte(encoded),
			wantReleaseName: "my-app",
			wantRevision:    5,
			wantOk:          true,
		},
		{
			name:         "invalid name format - no version",
			resourceName: "sh.helm.release.v1.my-release",
			data:         []byte(encoded),
			wantOk:       false,
		},
		{
			name:         "invalid name format - wrong prefix",
			resourceName: "invalid.my-release.v1",
			data:         []byte(encoded),
			wantOk:       false,
		},
		{
			name:         "invalid data - empty",
			resourceName: "sh.helm.release.v1.my-release.v1",
			data:         []byte{},
			wantOk:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			release, releaseName, revision, ok := hmf.parseHelmResource(tt.resourceName, tt.data)

			if ok != tt.wantOk {
				t.Errorf("parseHelmResource() ok = %v, want %v", ok, tt.wantOk)
				return
			}

			if !tt.wantOk {
				return // Skip further checks if we expected failure
			}

			if releaseName != tt.wantReleaseName {
				t.Errorf("parseHelmResource() releaseName = %v, want %v", releaseName, tt.wantReleaseName)
			}

			if revision != tt.wantRevision {
				t.Errorf("parseHelmResource() revision = %v, want %v", revision, tt.wantRevision)
			}

			if release == nil {
				t.Error("parseHelmResource() release is nil")
			}
		})
	}
}

func Test_allHelmReleasesCache(t *testing.T) {
	cache := &allHelmReleasesCache{}

	// Test empty cache
	if releases, ok := cache.getFromCache(); ok {
		t.Errorf("getFromCache() on empty cache returned ok=true, releases=%v", releases)
	}

	// Set cache with test data
	testReleases := []HelmReleaseData{
		{
			ReleaseName:  "release1",
			Namespace:    "default",
			ChartName:    "chart1",
			ChartVersion: "1.0.0",
		},
		{
			ReleaseName:  "release2",
			Namespace:    "kube-system",
			ChartName:    "chart2",
			ChartVersion: "2.0.0",
		},
	}
	cache.setCache(testReleases)

	// Test cache hit
	releases, ok := cache.getFromCache()
	if !ok {
		t.Error("getFromCache() after setCache returned ok=false")
	}
	if len(releases) != len(testReleases) {
		t.Errorf("getFromCache() returned %d releases, want %d", len(releases), len(testReleases))
	}

	// Test cache expiration
	cache.timestamp = time.Now().Add(-2 * helmValuesCacheTTL) // Set timestamp to expired
	if _, ok := cache.getFromCache(); ok {
		t.Error("getFromCache() on expired cache returned ok=true")
	}
}

func Test_mergeValues(t *testing.T) {
	client := newFakeClientWithKubeSystem("test-cluster-uid-123")

	hmf := NewHelmMetadataForwarder(zap.New(zap.UseDevMode(true)), client, "v1.28.0", "v1.19.0", config.NewCredentialManager(client))

	tests := []struct {
		name      string
		defaults  map[string]interface{}
		overrides map[string]interface{}
		want      map[string]interface{}
	}{
		{
			name: "simple override",
			defaults: map[string]interface{}{
				"key1": "default1",
				"key2": "default2",
			},
			overrides: map[string]interface{}{
				"key1": "override1",
			},
			want: map[string]interface{}{
				"key1": "override1",
				"key2": "default2",
			},
		},
		{
			name: "nested merge",
			defaults: map[string]interface{}{
				"datadog": map[string]interface{}{
					"apiKey": "default-key",
					"site":   "datadoghq.com",
				},
			},
			overrides: map[string]interface{}{
				"datadog": map[string]interface{}{
					"apiKey": "user-key",
				},
			},
			want: map[string]interface{}{
				"datadog": map[string]interface{}{
					"apiKey": "user-key",
					"site":   "datadoghq.com",
				},
			},
		},
		{
			name:     "empty defaults",
			defaults: map[string]interface{}{},
			overrides: map[string]interface{}{
				"key1": "value1",
			},
			want: map[string]interface{}{
				"key1": "value1",
			},
		},
		{
			name: "empty overrides",
			defaults: map[string]interface{}{
				"key1": "value1",
			},
			overrides: map[string]interface{}{},
			want: map[string]interface{}{
				"key1": "value1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hmf.mergeValues(tt.defaults, tt.overrides)

			// Deep comparison
			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(tt.want)

			if string(gotJSON) != string(wantJSON) {
				t.Errorf("mergeValues() = %s, want %s", string(gotJSON), string(wantJSON))
			}
		})
	}
}

// Helper type for gzip compression in tests
type testWriter struct {
	buf *[]byte
}

func (tw *testWriter) Write(p []byte) (n int, err error) {
	*tw.buf = append(*tw.buf, p...)
	return len(p), nil
}
