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
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	toolscache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/DataDog/datadog-operator/pkg/config"
)

const (
	// releasePrefix is the prefix for Helm release ConfigMaps and Secrets
	releasePrefix = "sh.helm.release.v1."
	// tickerInterval is how often the ticker sends all snapshots
	tickerInterval = 5 * time.Minute
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

	// Workqueue for processing Helm releases
	queue workqueue.TypedRateLimitingInterface[string]

	// Track latest snapshot of each release
	// Key: "namespace/releaseName"
	// Value: *ReleaseSnapshot
	releaseSnapshots sync.Map
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
	Config map[string]interface{} `json:"config"` // User-provided values only
	Chart  struct {
		Metadata struct {
			Name       string `json:"name"`
			Version    string `json:"version"`
			AppVersion string `json:"appVersion"`
		} `json:"metadata"`
		Values map[string]interface{} `json:"values"` // Defaults
	} `json:"chart"`
	Version int `json:"version"` // Revision number
}

// NewHelmMetadataForwarderWithManager creates a new instance of the helm metadata forwarder
func NewHelmMetadataForwarderWithManager(logger logr.Logger, mgr manager.Manager, k8sClient client.Reader, kubernetesVersion string, operatorVersion string, credsManager *config.CredentialManager) *HelmMetadataForwarder {
	forwarderLogger := logger.WithName("helm")

	return &HelmMetadataForwarder{
		SharedMetadata: NewSharedMetadata(forwarderLogger, k8sClient, kubernetesVersion, operatorVersion, credsManager),
		mgr:            mgr,
		queue:          workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[string]()),
	}
}

// Start starts the helm metadata forwarder with informer-based event handling
func (hmf *HelmMetadataForwarder) Start() {
	cmInformer, err := hmf.mgr.GetCache().GetInformer(context.Background(), &corev1.ConfigMap{})
	if err != nil {
		hmf.logger.Info("Error getting ConfigMap informer", "error", err)
		return
	}
	secretInformer, err := hmf.mgr.GetCache().GetInformer(context.Background(), &corev1.Secret{})
	if err != nil {
		hmf.logger.Info("Error getting Secret informer", "error", err)
		return
	}

	_, err = cmInformer.AddEventHandler(toolscache.FilteringResourceEventHandler{
		FilterFunc: func(obj any) bool {
			cm, ok := obj.(*corev1.ConfigMap)
			return ok &&
				cm.Labels["owner"] == "helm" &&
				strings.HasPrefix(cm.Name, releasePrefix)
		},
		Handler: toolscache.ResourceEventHandlerFuncs{
			AddFunc: func(obj any) {
				if key, keyErr := toolscache.MetaNamespaceKeyFunc(obj); keyErr == nil {
					hmf.queue.Add(key)
					hmf.logger.V(1).Info("Enqueued ConfigMap for processing", "key", key)
				}
			},
			DeleteFunc: func(obj any) {
				if key, keyErr := toolscache.DeletionHandlingMetaNamespaceKeyFunc(obj); keyErr == nil {
					hmf.queue.Add(key)
					hmf.logger.V(1).Info("Enqueued ConfigMap deletion for processing", "key", key)
				}
			},
		},
	})

	if err != nil {
		hmf.logger.Info("Error adding event handler to ConfigMap informer", "error", err)
		return
	}

	_, err = secretInformer.AddEventHandler(toolscache.FilteringResourceEventHandler{
		FilterFunc: func(obj any) bool {
			secret, ok := obj.(*corev1.Secret)
			return ok &&
				secret.Labels["owner"] == "helm" &&
				strings.HasPrefix(secret.Name, releasePrefix)
		},
		Handler: toolscache.ResourceEventHandlerFuncs{
			AddFunc: func(obj any) {
				if key, keyErr := toolscache.MetaNamespaceKeyFunc(obj); keyErr == nil {
					hmf.queue.Add(key)
				}
			},
			DeleteFunc: func(obj any) {
				if key, keyErr := toolscache.DeletionHandlingMetaNamespaceKeyFunc(obj); keyErr == nil {
					hmf.queue.Add(key)
				}
			},
		},
	})

	if err != nil {
		hmf.logger.Info("Error adding event handler to Secret informer", "error", err)
		return
	}

	if !hmf.mgr.GetCache().WaitForCacheSync(context.Background()) {
		hmf.logger.Info("Error waiting for cache sync", "error", err)
		return
	}

	// Start worker goroutine
	go hmf.runWorker()

	// Start ticker for periodic sends
	go hmf.tickerLoop()

	hmf.logger.Info("Started Helm metadata forwarder with workqueue")
}

// runWorker is a long-running function that will continually process items from the workqueue
func (hmf *HelmMetadataForwarder) runWorker() {
	for {
		key, shutdown := hmf.queue.Get()
		if shutdown {
			break
		}

		if err := hmf.processKey(key); err != nil {
			hmf.queue.AddRateLimited(key)
			hmf.logger.V(1).Info("Error processing key, will retry", "key", key, "error", err)
		} else {
			hmf.queue.Forget(key)
		}
		hmf.queue.Done(key)
	}
}

// processKey processes a single Helm release by its namespaced key
func (hmf *HelmMetadataForwarder) processKey(key string) error {
	namespace, name, err := toolscache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid key format: %w", err)
	}

	// Try to get as ConfigMap first
	cm := &corev1.ConfigMap{}
	err = hmf.k8sClient.Get(context.Background(), client.ObjectKey{Namespace: namespace, Name: name}, cm)
	if err == nil && cm.Labels["owner"] == "helm" {
		hmf.handleHelmResource(cm.Name, cm.Namespace, string(cm.UID), []byte(cm.Data["release"]))
		return nil
	}

	// Try as Secret
	secret := &corev1.Secret{}
	err = hmf.k8sClient.Get(context.Background(), client.ObjectKey{Namespace: namespace, Name: name}, secret)
	if err == nil && secret.Labels["owner"] == "helm" {
		hmf.handleHelmResource(secret.Name, secret.Namespace, string(secret.UID), secret.Data["release"])
		return nil
	}

	// If not found, it was likely deleted
	if errors.IsNotFound(err) {
		hmf.handleDelete(key)
		return nil
	}

	return fmt.Errorf("failed to get resource: %w", err)
}

// handleDelete handles deletion of a Helm release
func (hmf *HelmMetadataForwarder) handleDelete(key string) {
	namespace, name, _ := toolscache.SplitMetaNamespaceKey(key)

	// Parse the release name from the resource name
	_, releaseName, _, ok := hmf.parseHelmResource(name, nil)
	if !ok || releaseName == "" {
		return
	}

	releaseKey := fmt.Sprintf("%s/%s", namespace, releaseName)
	if _, exists := hmf.releaseSnapshots.Load(releaseKey); exists {
		hmf.releaseSnapshots.Delete(releaseKey)
		hmf.logger.Info("Deleted release snapshot for release", "releaseKey", releaseKey)
	}
}

// handleHelmResource processes a Helm resource event and updates the snapshot
func (hmf *HelmMetadataForwarder) handleHelmResource(name, namespace, uid string, data []byte) {
	release, releaseName, revision, ok := hmf.parseHelmResource(name, data)
	if !ok || release == nil {
		return
	}

	// Filter for allowed charts (after decoding)
	if !allowedCharts[release.Chart.Metadata.Name] {
		hmf.logger.V(1).Info("Skipping non-allowed chart",
			"chart", release.Chart.Metadata.Name,
			"release", releaseName)
		return
	}

	key := fmt.Sprintf("%s/%s", namespace, releaseName)

	// Check if we should update (prevent old revisions)
	if existing, loaded := hmf.releaseSnapshots.Load(key); loaded {
		existingSnapshot := existing.(*ReleaseSnapshot)
		if existingSnapshot.Revision >= revision {
			hmf.logger.V(1).Info("Skipping old/same revision",
				"key", key,
				"existing", existingSnapshot.Revision,
				"new", revision)
			return
		}
		hmf.logger.V(1).Info("Updating to newer revision",
			"key", key,
			"old", existingSnapshot.Revision,
			"new", revision)
	}

	// Build snapshot
	snapshot := hmf.buildSnapshot(release, releaseName, namespace, uid, revision)
	if snapshot == nil {
		return
	}

	// Send immediately
	releaseData := hmf.snapshotToReleaseData(snapshot)
	if err := hmf.sendSingleReleasePayload(releaseData); err != nil {
		hmf.logger.V(1).Info("Failed to send release",
			"key", key,
			"error", err)
		// Don't store in map if send failed
		return
	}

	// Store in map after successful send
	hmf.releaseSnapshots.Store(key, snapshot)

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

// tickerLoop runs the periodic ticker to send all snapshots
func (hmf *HelmMetadataForwarder) tickerLoop() {
	ticker := time.NewTicker(tickerInterval)
	defer ticker.Stop()

	for range ticker.C {
		hmf.sendAllSnapshots()
	}
}

// sendAllSnapshots sends all release snapshots
func (hmf *HelmMetadataForwarder) sendAllSnapshots() {
	hmf.logger.V(1).Info("Ticker: sending all Helm release snapshots")

	count := 0
	errors := 0

	hmf.releaseSnapshots.Range(func(key, value interface{}) bool {
		snapshot := value.(*ReleaseSnapshot)

		releaseData := hmf.snapshotToReleaseData(snapshot)
		if err := hmf.sendSingleReleasePayload(releaseData); err != nil {
			hmf.logger.V(1).Info("Failed to send snapshot",
				"key", key,
				"error", err)
			errors++
		} else {
			count++
		}

		return true
	})

	if count > 0 {
		hmf.logger.V(1).Info("Ticker: sent Helm release snapshots",
			"sent", count,
			"errors", errors)
	}
}

func (hmf *HelmMetadataForwarder) sendSingleReleasePayload(release HelmReleaseData) error {
	clusterUID, err := hmf.GetOrCreateClusterUID()
	if err != nil {
		return fmt.Errorf("error getting cluster UID: %w", err)
	}
	payload := hmf.buildPayload(release, clusterUID)

	req, err := hmf.createRequest(payload)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

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
func (hmf *HelmMetadataForwarder) mergeValues(defaults, overrides map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	for k, v := range defaults {
		result[k] = v
	}

	for k, v := range overrides {
		if existingVal, exists := result[k]; exists {
			if existingMap, ok := existingVal.(map[string]interface{}); ok {
				if overrideMap, ok := v.(map[string]interface{}); ok {
					result[k] = hmf.mergeValues(existingMap, overrideMap)
					continue
				}
			}
		}
		result[k] = v
	}

	return result
}
