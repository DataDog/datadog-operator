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

	"github.com/DataDog/datadog-operator/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// createTestForwarder creates a minimal test forwarder
func createTestForwarder() *HelmMetadataForwarder {
	return NewHelmMetadataForwarderWithManager(
		zap.New(zap.UseDevMode(true)),
		nil,
		nil,
		"v1.28.0",
		"v1.19.0",
		config.NewCredentialManager(fake.NewFakeClient()),
	)
}

// createValidReleaseData creates valid test release data
func createValidReleaseData() ([]byte, error) {
	release := HelmReleaseMinimal{
		Name:      "test-release",
		Namespace: "default",
		Version:   1,
	}
	release.Info.Status = "deployed"
	release.Chart.Metadata.Name = "datadog"
	release.Chart.Metadata.Version = "3.10.0"
	release.Chart.Metadata.AppVersion = "7.50.0"
	release.Config = map[string]interface{}{"key": "value"}
	release.Chart.Values = map[string]interface{}{"default": "value"}

	jsonData, err := json.Marshal(release)
	if err != nil {
		return nil, err
	}

	var compressed []byte
	writer := gzip.NewWriter(&testWriter{buf: &compressed})
	writer.Write(jsonData)
	writer.Close()

	return []byte(base64.StdEncoding.EncodeToString(compressed)), nil
}

func Test_workqueueInitialization(t *testing.T) {
	hmf := createTestForwarder()

	if hmf.queue == nil {
		t.Fatal("Workqueue not initialized")
	}

	hmf.queue.Add("test/key")
	if hmf.queue.Len() != 1 {
		t.Errorf("Queue length = %d, want 1", hmf.queue.Len())
	}

	key, shutdown := hmf.queue.Get()
	if shutdown {
		t.Error("Queue should not be shutting down")
	}
	if key != "test/key" {
		t.Errorf("Got key %v, want test/key", key)
	}

	hmf.queue.Done(key)
	hmf.queue.ShutDown()

	if !hmf.queue.ShuttingDown() {
		t.Error("Queue should be marked as shutting down")
	}
}

func Test_parseHelmResource(t *testing.T) {
	hmf := createTestForwarder()
	encoded, err := createValidReleaseData()
	if err != nil {
		t.Fatalf("Failed to create test data: %v", err)
	}

	tests := []struct {
		name            string
		resourceName    string
		data            []byte
		wantReleaseName string
		wantRevision    int
		wantOk          bool
		wantNilRelease  bool
	}{
		{
			name:            "valid resource",
			resourceName:    "sh.helm.release.v1.my-release.v5",
			data:            encoded,
			wantReleaseName: "my-release",
			wantRevision:    5,
			wantOk:          true,
		},
		{
			name:         "invalid format",
			resourceName: "invalid.my-release.v1",
			data:         encoded,
			wantOk:       false,
		},
		{
			name:            "nil data - deletion",
			resourceName:    "sh.helm.release.v1.my-release.v3",
			data:            nil,
			wantReleaseName: "my-release",
			wantRevision:    3,
			wantOk:          true,
			wantNilRelease:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			release, releaseName, revision, ok := hmf.parseHelmResource(tt.resourceName, tt.data)

			if ok != tt.wantOk {
				t.Errorf("ok = %v, want %v", ok, tt.wantOk)
			}
			if !tt.wantOk {
				return
			}
			if releaseName != tt.wantReleaseName {
				t.Errorf("releaseName = %v, want %v", releaseName, tt.wantReleaseName)
			}
			if revision != tt.wantRevision {
				t.Errorf("revision = %v, want %v", revision, tt.wantRevision)
			}
			if tt.wantNilRelease && release != nil {
				t.Error("expected nil release for deletion")
			}
			if !tt.wantNilRelease && release == nil {
				t.Error("expected non-nil release")
			}
		})
	}
}

func Test_releaseSnapshots(t *testing.T) {
	hmf := createTestForwarder()

	snapshot := &ReleaseSnapshot{
		ReleaseName: "test-release",
		Namespace:   "default",
		Revision:    1,
	}

	// Store
	key := "default/test-release"
	hmf.releaseSnapshots.Store(key, snapshot)

	// Load
	val, ok := hmf.releaseSnapshots.Load(key)
	if !ok {
		t.Fatal("Failed to load snapshot")
	}
	if val.(*ReleaseSnapshot).Revision != 1 {
		t.Errorf("Revision = %v, want 1", val.(*ReleaseSnapshot).Revision)
	}

	// Delete
	hmf.releaseSnapshots.Delete(key)
	if _, ok := hmf.releaseSnapshots.Load(key); ok {
		t.Error("Snapshot still exists after deletion")
	}
}

func Test_revisionLogic(t *testing.T) {
	hmf := createTestForwarder()
	key := "default/test-release"

	// Store revision 1
	hmf.releaseSnapshots.Store(key, &ReleaseSnapshot{Revision: 1})

	// Update to revision 3
	hmf.releaseSnapshots.Store(key, &ReleaseSnapshot{Revision: 3})
	stored, _ := hmf.releaseSnapshots.Load(key)
	if stored.(*ReleaseSnapshot).Revision != 3 {
		t.Errorf("Revision = %v, want 3", stored.(*ReleaseSnapshot).Revision)
	}

	// Verify >= check (simulates handleHelmResource logic)
	existing := stored.(*ReleaseSnapshot)
	if existing.Revision < 2 {
		t.Error("Should skip older revision 2")
	}
}

func Test_buildSnapshot(t *testing.T) {
	hmf := createTestForwarder()

	release := &HelmReleaseMinimal{
		Name:      "test-release",
		Namespace: "default",
		Version:   2,
	}
	release.Info.Status = "deployed"
	release.Chart.Metadata.Name = "datadog"
	release.Chart.Metadata.Version = "3.10.0"
	release.Config = map[string]interface{}{"datadog": map[string]interface{}{"apiKey": "key"}}
	release.Chart.Values = map[string]interface{}{"datadog": map[string]interface{}{"site": "datadoghq.com"}}

	snapshot := hmf.buildSnapshot(release, "test-release", "default", "uid-123", 2)

	if snapshot == nil {
		t.Fatal("buildSnapshot() returned nil")
	}
	if snapshot.Revision != 2 {
		t.Errorf("Revision = %v, want 2", snapshot.Revision)
	}
	if snapshot.ProvidedValuesYAML == "" {
		t.Error("ProvidedValuesYAML empty")
	}
	if snapshot.FullValuesYAML == "" {
		t.Error("FullValuesYAML empty")
	}
	if snapshot.Release == nil {
		t.Error("Release reference nil")
	}
}

func Test_snapshotToReleaseData(t *testing.T) {
	hmf := createTestForwarder()

	snapshot := &ReleaseSnapshot{
		ReleaseName:  "test-release",
		Namespace:    "default",
		ChartName:    "datadog",
		ChartVersion: "3.10.0",
		Revision:     2,
		Status:       "deployed",
	}

	releaseData := hmf.snapshotToReleaseData(snapshot)

	if releaseData.ReleaseName != "test-release" {
		t.Errorf("ReleaseName = %v, want test-release", releaseData.ReleaseName)
	}
	if releaseData.Revision != 2 {
		t.Errorf("Revision = %v, want 2", releaseData.Revision)
	}
}

func Test_mergeValues(t *testing.T) {
	hmf := createTestForwarder()

	defaults := map[string]interface{}{
		"datadog": map[string]interface{}{
			"apiKey": "default-key",
			"site":   "datadoghq.com",
		},
	}
	overrides := map[string]interface{}{
		"datadog": map[string]interface{}{
			"apiKey": "user-key",
		},
	}

	result := hmf.mergeValues(defaults, overrides)

	datadog := result["datadog"].(map[string]interface{})
	if datadog["apiKey"] != "user-key" {
		t.Errorf("apiKey = %v, want user-key", datadog["apiKey"])
	}
	if datadog["site"] != "datadoghq.com" {
		t.Errorf("site = %v, want datadoghq.com", datadog["site"])
	}
}

func Test_buildPayload(t *testing.T) {
	hmf := createTestForwarder()
	hmf.hostName = "test-host"

	release := HelmReleaseData{
		ReleaseName:  "my-release",
		ChartName:    "datadog",
		ChartVersion: "3.10.0",
		Revision:     1,
	}

	payload := hmf.buildPayload(release, "cluster-123")

	var parsed map[string]interface{}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	if parsed["hostname"] != "test-host" {
		t.Errorf("hostname = %v, want test-host", parsed["hostname"])
	}

	metadata := parsed["datadog_operator_helm_metadata"].(map[string]interface{})
	if metadata["chart_name"] != "datadog" {
		t.Errorf("chart_name = %v, want datadog", metadata["chart_name"])
	}
}

// Helper for gzip compression
type testWriter struct {
	buf *[]byte
}

func (tw *testWriter) Write(p []byte) (n int, err error) {
	*tw.buf = append(*tw.buf, p...)
	return len(p), nil
}
