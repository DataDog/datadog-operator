// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package metadata

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/DataDog/datadog-operator/pkg/config"
)

const (
	// releasePrefix is the prefix for Helm release ConfigMaps and Secrets
	releasePrefix = "sh.helm.release.v1."
	// helmHeartbeatInterval is how often the heartbeat sends all snapshots
	helmHeartbeatInterval = 5 * time.Minute
	// helmNumWorkers is the number of concurrent workers
	helmNumWorkers = 3
)

var (
	versionRegexp = regexp.MustCompile(`\.v(\d+)$`)
	allowedCharts = map[string]bool{
		"datadog":          true,
		"datadog-operator": true,
		"datadog-agent":    true,
	}
)

type HelmMetadataForwarder struct {
	*SharedMetadata

	mgr manager.Manager

	// runner owns the workqueue, worker pool, and heartbeat ticker.
	runner *InformerWorkQueue

	// Track latest snapshot of each release
	// Key: "namespace/releaseName"
	// Value: *ReleaseEntry
	releaseSnapshots sync.Map
}

// ReleaseEntry wraps a ReleaseSnapshot with a mutex for safe concurrent access
type ReleaseEntry struct {
	mu       sync.Mutex
	snapshot *ReleaseSnapshot
}

// ReleaseSnapshot holds a snapshot of a Helm release
type ReleaseSnapshot struct {
	Release            *HelmReleaseMinimal
	ReleaseName        string
	Namespace          string
	ChartName          string
	ChartVersion       string
	AppVersion         string
	ConfigMapUID       string
	ProvidedValuesYAML string
	FullValuesYAML     string
	Revision           int
	Status             string
}

type HelmMetadataPayload struct {
	UUID      string       `json:"uuid"`
	Timestamp int64        `json:"timestamp"`
	ClusterID string       `json:"cluster_id"`
	Metadata  HelmMetadata `json:"datadog_operator_helm_metadata"`
}

type HelmMetadata struct {
	// Shared
	OperatorVersion   string `json:"operator_version"`
	KubernetesVersion string `json:"kubernetes_version"`
	ClusterID         string `json:"cluster_id"`

	ChartName                 string `json:"chart_name"`
	ChartReleaseName          string `json:"chart_release_name"`
	ChartAppVersion           string `json:"chart_app_version"`
	ChartVersion              string `json:"chart_version"`
	ChartNamespace            string `json:"chart_namespace"`
	ChartConfigMapUID         string `json:"chart_configmap_uid"`
	HelmProvidedConfiguration string `json:"helm_provided_configuration"` // User-provided values only
	HelmFullConfiguration     string `json:"helm_full_configuration"`     // Includes defaults
}

// HelmReleaseData contains all data for a single Helm release
type HelmReleaseData struct {
	ReleaseName        string
	Namespace          string
	ChartName          string
	ChartVersion       string
	AppVersion         string
	ConfigMapUID       string
	ProvidedValuesYAML string // User-provided values only
	FullValuesYAML     string // Includes defaults
	Revision           int
	Status             string
}

// HelmReleaseMinimal represents the minimal structure we care about from Helm release
type HelmReleaseMinimal struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Info      struct {
		Status string `json:"status"`
	} `json:"info"`
	Config map[string]any `json:"config"` // User-provided values only
	Chart  struct {
		Metadata struct {
			Name       string `json:"name"`
			Version    string `json:"version"`
			AppVersion string `json:"appVersion"`
		} `json:"metadata"`
		Values map[string]any `json:"values"` // Defaults
	} `json:"chart"`
	Version int `json:"version"` // Revision number
}

// NewHelmMetadataForwarderWithManager creates a new instance of the helm metadata forwarder
func NewHelmMetadataForwarderWithManager(logger logr.Logger, mgr manager.Manager, k8sClient client.Reader, kubernetesVersion string, operatorVersion string, credsManager *config.CredentialManager) *HelmMetadataForwarder {
	forwarderLogger := logger.WithName("helm")

	hmf := &HelmMetadataForwarder{
		SharedMetadata: NewSharedMetadata(forwarderLogger, k8sClient, kubernetesVersion, operatorVersion, credsManager),
		mgr:            mgr,
	}
	hmf.runner = NewInformerWorkQueue(
		forwarderLogger,
		mgr,
		helmNumWorkers,
		helmHeartbeatInterval,
		hmf.processKey,
		hmf.handleDelete,
		hmf.heartbeat,
	)
	return hmf
}

// Start implements manager.Runnable.
func (hmf *HelmMetadataForwarder) Start(ctx context.Context) error {
	cmFilter := func(obj any) bool {
		cm, ok := obj.(*corev1.ConfigMap)
		return ok && cm.Labels["owner"] == "helm" && strings.HasPrefix(cm.Name, releasePrefix)
	}
	secretFilter := func(obj any) bool {
		s, ok := obj.(*corev1.Secret)
		return ok && s.Labels["owner"] == "helm" && strings.HasPrefix(s.Name, releasePrefix)
	}

	hmf.runner.AddWatch(ctx, WatchTarget{
		Object: &corev1.ConfigMap{}, Resource: "configmaps", Kind: "ConfigMap", Filter: cmFilter,
	})
	hmf.runner.AddWatch(ctx, WatchTarget{
		Object: &corev1.Secret{}, Resource: "secrets", Kind: "Secret", Filter: secretFilter,
	})

	go hmf.runner.Run(ctx)
	return nil
}

// NeedLeaderElection implements manager.LeaderElectionRunnable
func (hmf *HelmMetadataForwarder) NeedLeaderElection() bool {
	return true
}

// processKey processes a single Helm release by its kind and namespaced key
func (hmf *HelmMetadataForwarder) processKey(ctx context.Context, kind, namespace, name string) error {
	switch kind {
	case "ConfigMap":
		cm := &corev1.ConfigMap{}
		if err := hmf.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, cm); err != nil {
			if errors.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("failed to get ConfigMap: %w", err)
		}
		if cm.Labels["owner"] != "helm" {
			return nil
		}
		hmf.handleHelmResource(ctx, cm.Name, cm.Namespace, string(cm.UID), []byte(cm.Data["release"]))
	case "Secret":
		secret := &corev1.Secret{}
		if err := hmf.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, secret); err != nil {
			if errors.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("failed to get Secret: %w", err)
		}
		if secret.Labels["owner"] != "helm" {
			return nil
		}
		hmf.handleHelmResource(ctx, secret.Name, secret.Namespace, string(secret.UID), secret.Data["release"])
	default:
		hmf.logger.V(1).Info("Unknown kind in processKey", "kind", kind)
	}
	return nil
}

// handleDelete handles deletion of a Helm release
func (hmf *HelmMetadataForwarder) handleDelete(_ /*kind*/ string, namespace, name string) {
	_, releaseName, revision, ok := hmf.parseHelmResource(name, nil)
	if !ok || releaseName == "" {
		return
	}

	releaseKey := fmt.Sprintf("%s/%s", namespace, releaseName)

	if existing, loaded := hmf.releaseSnapshots.Load(releaseKey); loaded {
		entry := existing.(*ReleaseEntry)
		entry.mu.Lock()
		defer entry.mu.Unlock()

		if entry.snapshot != nil && entry.snapshot.Revision == revision {
			hmf.releaseSnapshots.Delete(releaseKey)
			hmf.logger.V(1).Info("Deleted release snapshot", "releaseKey", releaseKey, "revision", revision)
		}
	}
}

// handleHelmResource processes a Helm resource event and updates the snapshot
func (hmf *HelmMetadataForwarder) handleHelmResource(ctx context.Context, name, namespace, uid string, data []byte) {
	release, releaseName, revision, ok := hmf.parseHelmResource(name, data)
	if !ok || release == nil {
		return
	}

	// Filter for allowed charts (after decoding)
	if !allowedCharts[release.Chart.Metadata.Name] {
		hmf.logger.V(2).Info("Skipping non-allowed chart",
			"chart", release.Chart.Metadata.Name,
			"release", releaseName)
		return
	}

	key := fmt.Sprintf("%s/%s", namespace, releaseName)

	// Get or create entry for this release
	value, _ := hmf.releaseSnapshots.LoadOrStore(key, &ReleaseEntry{})
	entry := value.(*ReleaseEntry)

	entry.mu.Lock()
	defer entry.mu.Unlock()

	// Check if we should update (prevent old revisions)
	if entry.snapshot != nil && entry.snapshot.Revision >= revision {
		hmf.logger.V(2).Info("Skipping old/same revision",
			"key", key,
			"existing", entry.snapshot.Revision,
			"new", revision)
		return
	}

	// Build snapshot
	snapshot := hmf.buildSnapshot(release, releaseName, namespace, uid, revision)
	if snapshot == nil {
		return
	}

	// Send immediately
	releaseData := hmf.snapshotToReleaseData(snapshot)
	if err := hmf.sendSingleReleasePayload(ctx, releaseData); err != nil {
		hmf.logger.V(1).Info("Failed to send release",
			"key", key,
			"error", err)
		// Don't update snapshot if send failed
		return
	}

	entry.snapshot = snapshot

	hmf.logger.V(1).Info("Updated release snapshot",
		"key", key,
		"revision", revision,
		"chart", release.Chart.Metadata.Name)
}

// buildSnapshot constructs a ReleaseSnapshot from a parsed release
func (hmf *HelmMetadataForwarder) buildSnapshot(
	release *HelmReleaseMinimal,
	releaseName, namespace, uid string,
	revision int,
) *ReleaseSnapshot {
	providedValuesYAML, err := yaml.Marshal(release.Config)
	if err != nil {
		hmf.logger.V(1).Info("Failed to marshal Helm provided values", "release", releaseName, "error", err)
		return nil
	}

	fullValues := hmf.mergeValues(release.Chart.Values, release.Config)
	fullValuesYAML, err := yaml.Marshal(fullValues)
	if err != nil {
		hmf.logger.V(1).Info("Failed to marshal Helm full values", "release", releaseName, "error", err)
		fullValuesYAML = providedValuesYAML
	}

	return &ReleaseSnapshot{
		Release:            release,
		ReleaseName:        releaseName,
		Namespace:          namespace,
		ChartName:          release.Chart.Metadata.Name,
		ChartVersion:       release.Chart.Metadata.Version,
		AppVersion:         release.Chart.Metadata.AppVersion,
		ConfigMapUID:       uid,
		ProvidedValuesYAML: string(providedValuesYAML),
		FullValuesYAML:     string(fullValuesYAML),
		Revision:           revision,
		Status:             release.Info.Status,
	}
}

// snapshotToReleaseData converts a ReleaseSnapshot to HelmReleaseData
func (hmf *HelmMetadataForwarder) snapshotToReleaseData(snapshot *ReleaseSnapshot) HelmReleaseData {
	return HelmReleaseData{
		ReleaseName:        snapshot.ReleaseName,
		Namespace:          snapshot.Namespace,
		ChartName:          snapshot.ChartName,
		ChartVersion:       snapshot.ChartVersion,
		AppVersion:         snapshot.AppVersion,
		ConfigMapUID:       snapshot.ConfigMapUID,
		ProvidedValuesYAML: snapshot.ProvidedValuesYAML,
		FullValuesYAML:     snapshot.FullValuesYAML,
		Revision:           snapshot.Revision,
		Status:             snapshot.Status,
	}
}

// heartbeat sends all release snapshots and is called by the runner on every tick.
func (hmf *HelmMetadataForwarder) heartbeat(ctx context.Context) {
	hmf.releaseSnapshots.Range(func(key, value any) bool {
		entry := value.(*ReleaseEntry)
		entry.mu.Lock()
		snapshot := entry.snapshot
		entry.mu.Unlock()

		if snapshot == nil {
			return true
		}

		releaseData := hmf.snapshotToReleaseData(snapshot)
		if err := hmf.sendSingleReleasePayload(ctx, releaseData); err != nil {
			hmf.logger.V(1).Info("Failed to send snapshot during heartbeat", "key", key, "error", err)
		}
		return true
	})
}

func (hmf *HelmMetadataForwarder) sendSingleReleasePayload(ctx context.Context, release HelmReleaseData) error {
	clusterUID, err := hmf.GetOrCreateClusterUID(ctx)
	if err != nil {
		return fmt.Errorf("error getting cluster UID: %w", err)
	}
	payload := hmf.buildPayload(release, clusterUID)

	req, err := hmf.createRequest(payload)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	req = req.WithContext(ctx)

	resp, err := hmf.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error sending metadata request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read metadata response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("received error status code %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (hmf *HelmMetadataForwarder) buildPayload(release HelmReleaseData, clusterUID string) []byte {
	now := time.Now().Unix()

	helmMetadata := HelmMetadata{
		OperatorVersion:           hmf.operatorVersion,
		KubernetesVersion:         hmf.kubernetesVersion,
		ClusterID:                 clusterUID,
		ChartName:                 release.ChartName,
		ChartReleaseName:          release.ReleaseName,
		ChartAppVersion:           release.AppVersion,
		ChartVersion:              release.ChartVersion,
		ChartNamespace:            release.Namespace,
		ChartConfigMapUID:         release.ConfigMapUID,
		HelmProvidedConfiguration: release.ProvidedValuesYAML,
		HelmFullConfiguration:     release.FullValuesYAML,
	}

	payload := HelmMetadataPayload{
		UUID:      clusterUID,
		Timestamp: now,
		ClusterID: clusterUID,
		Metadata:  helmMetadata,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		hmf.logger.V(1).Info("Error marshaling payload to json",
			"error", err,
			"release", release.ReleaseName)
	}

	return jsonPayload
}

// parseHelmResource extracts release information from a Helm Secret or ConfigMap
func (hmf *HelmMetadataForwarder) parseHelmResource(name string, data []byte) (*HelmReleaseMinimal, string, int, bool) {
	// Format: sh.helm.release.v1.{release-name}.v{revision}
	if !strings.HasPrefix(name, releasePrefix) {
		return nil, "", 0, false
	}

	parts := strings.TrimPrefix(name, releasePrefix)
	match := versionRegexp.FindStringSubmatch(parts)
	if len(match) != 2 {
		return nil, "", 0, false
	}

	revision, err := strconv.Atoi(match[1])
	if err != nil {
		return nil, "", 0, false
	}

	releaseName := strings.TrimSuffix(parts, match[0])

	// If no data provided (e.g., during deletion), return name and revision only
	if len(data) == 0 {
		return nil, releaseName, revision, true
	}

	release, err := hmf.decodeHelmReleaseFromBytes(data)
	if err != nil {
		hmf.logger.V(1).Info("Failed to decode Helm release", "resource", name, "error", err)
		return nil, "", 0, false
	}

	return release, releaseName, revision, true
}

// decodeHelmReleaseFromBytes decodes and decompresses a Helm release from base64 gzipped bytes
func (hmf *HelmMetadataForwarder) decodeHelmReleaseFromBytes(data []byte) (*HelmReleaseMinimal, error) {
	decoded := data
	if decodedData, err := base64.StdEncoding.DecodeString(string(data)); err == nil {
		decoded = decodedData
	}

	gr, err := gzip.NewReader(bytes.NewReader(decoded))
	if err != nil {
		return nil, fmt.Errorf("gzip decompression error: %w", err)
	}
	defer gr.Close()

	var decompressed bytes.Buffer
	_, err = io.Copy(&decompressed, gr)
	if err != nil {
		return nil, fmt.Errorf("gzip read error: %w", err)
	}

	var release HelmReleaseMinimal
	if err := json.Unmarshal(decompressed.Bytes(), &release); err != nil {
		return nil, fmt.Errorf("unmarshal error: %w", err)
	}

	return &release, nil
}

// mergeValues merges chart default values with user-provided config
// User config takes precedence over defaults (similar to Helm's merge logic)
func (hmf *HelmMetadataForwarder) mergeValues(defaults, overrides map[string]any) map[string]any {
	result := make(map[string]any)

	maps.Copy(result, defaults)

	for k, v := range overrides {
		if existingVal, exists := result[k]; exists {
			if existingMap, ok := existingVal.(map[string]any); ok {
				if overrideMap, ok := v.(map[string]any); ok {
					result[k] = hmf.mergeValues(existingMap, overrideMap)
					continue
				}
			}
		}
		result[k] = v
	}

	return result
}
