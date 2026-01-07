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
	toolscache "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/DataDog/datadog-operator/pkg/config"
)

const (
	// releasePrefix is the prefix for Helm release ConfigMaps and Secrets
	releasePrefix = "sh.helm.release.v1."
	// revisionSeparator separates release name from revision in tracking keys
	revisionSeparator = "/rev-"
)

var (
	versionRegexp = regexp.MustCompile(`\.v(\d+)$`)
	allowedCharts = map[string]bool{
		"datadog":          true,
		"datadog-operator": true,
	}
)

type HelmMetadataForwarder struct {
	*SharedMetadata

	mgr manager.Manager

	// Track processed releases to avoid duplicate sends
	// Key: namespace/releaseName/rev-N
	processedReleases sync.Map
}

type HelmMetadataPayload struct {
	Hostname  string       `json:"hostname"`
	Timestamp int64        `json:"timestamp"`
	ClusterID string       `json:"cluster_id"`
	Metadata  HelmMetadata `json:"datadog_operator_helm_metadata"`
}

type HelmMetadata struct {
	OperatorVersion           string `json:"operator_version"`
	KubernetesVersion         string `json:"kubernetes_version"`
	ClusterID                 string `json:"cluster_id"`
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
	}
}

// getWatchNamespacesForHelm retrieves the list of namespaces to watch from environment variables
func getWatchNamespacesForHelm(logger logr.Logger) []string {
	nsMap := config.GetWatchNamespacesFromEnv(logger, config.AgentWatchNamespaceEnvVar)

	namespaces := make([]string, 0, len(nsMap))
	for ns := range nsMap {
		if ns == cache.AllNamespaces {
			logger.V(1).Info("Watching all namespaces")
			return []string{""}
		}
		namespaces = append(namespaces, ns)
	}

	logger.V(1).Info("Watching specific namespaces", "namespaces", namespaces)
	return namespaces
}

// Start starts the helm metadata forwarder with informer-based event handling
func (hmf *HelmMetadataForwarder) Start() {
	cmInformer, err := hmf.mgr.GetCache().GetInformer(context.Background(), &corev1.ConfigMap{})
	if err != nil {
		hmf.logger.Error(err, "Error getting ConfigMap informer")
		return
	}
	secretInformer, err := hmf.mgr.GetCache().GetInformer(context.Background(), &corev1.Secret{})
	if err != nil {
		hmf.logger.Error(err, "Error getting Secret informer")
		return
	}

	_, err = cmInformer.AddEventHandler(toolscache.FilteringResourceEventHandler{
		FilterFunc: func(obj any) bool {
			cm, ok := obj.(*corev1.ConfigMap)
			return ok &&
				cm.Labels["owner"] == "helm" &&
				strings.HasPrefix(cm.Name, releasePrefix)
			// Chart name filtering happens after decoding in handler
		},
		Handler: toolscache.ResourceEventHandlerFuncs{
			AddFunc: func(obj any) {
				cm, _ := obj.(*corev1.ConfigMap)
				hmf.logger.V(1).Info("ConfigMap added", "name", cm.GetName(), "namespace", cm.GetNamespace())
				hmf.processHelmConfigMap(cm)
			},
			UpdateFunc: func(oldObj, newObj any) {
				cm, _ := newObj.(*corev1.ConfigMap)
				hmf.logger.V(1).Info("ConfigMap updated", "name", cm.GetName(), "namespace", cm.GetNamespace())
				hmf.processHelmConfigMap(cm)
			},
			DeleteFunc: func(obj any) {
				cm, _ := obj.(*corev1.ConfigMap)
				hmf.logger.V(1).Info("ConfigMap deleted", "name", cm.GetName(), "namespace", cm.GetNamespace())
			},
		},
	})

	if err != nil {
		hmf.logger.Error(err, "Error adding event handler to ConfigMap informer")
		return
	}

	_, err = secretInformer.AddEventHandler(toolscache.FilteringResourceEventHandler{
		FilterFunc: func(obj any) bool {
			secret, ok := obj.(*corev1.Secret)
			return ok &&
				secret.Labels["owner"] == "helm" &&
				strings.HasPrefix(secret.Name, releasePrefix)
			// Chart name filtering happens after decoding in handler
		},
		Handler: toolscache.ResourceEventHandlerFuncs{
			AddFunc: func(obj any) {
				secret, _ := obj.(*corev1.Secret)
				hmf.processHelmSecret(secret)
			},
		},
	})

	if err != nil {
		hmf.logger.Error(err, "Error adding event handler to Secret informer")
		return
	}

	if !hmf.mgr.GetCache().WaitForCacheSync(context.Background()) {
		hmf.logger.Error(err, "Error waiting for cache sync")
		return
	}

	if hmf.hostName == "" {
		hmf.logger.Error(ErrEmptyHostName, "Could not set host name; not starting metadata forwarder")
		return
	}

	hmf.logger.Info("Starting metadata forwarder with informers")
}

// buildRevisionTrackingKey creates a key for tracking processed revisions: "namespace/releaseName/rev-N"
func buildRevisionTrackingKey(namespace, releaseName string, revision int) string {
	return fmt.Sprintf("%s/%s%s%d", namespace, releaseName, revisionSeparator, revision)
}

// shouldProcessRevision checks if we should process this revision
func (hmf *HelmMetadataForwarder) shouldProcessRevision(namespace, releaseName string, revision int) bool {
	trackingKey := buildRevisionTrackingKey(namespace, releaseName, revision)

	// Check if we've already processed this exact revision
	if _, alreadyProcessed := hmf.processedReleases.LoadOrStore(trackingKey, true); alreadyProcessed {
		hmf.logger.V(1).Info("Skipping Helm revision",
			"release", releaseName,
			"revision", revision,
			"namespace", namespace,
			"reason", "already processed")
		return false
	}

	if hmf.hasNewerRevision(namespace, releaseName, revision) {
		hmf.processedReleases.Delete(trackingKey)
		hmf.logger.V(1).Info("Skipping Helm revision",
			"release", releaseName,
			"revision", revision,
			"namespace", namespace,
			"reason", "newer revision exists")
		return false
	}

	return true
}

// hasNewerRevision checks if a newer revision of this release has already been processed
func (hmf *HelmMetadataForwarder) hasNewerRevision(namespace, releaseName string, currentRevision int) bool {
	var hasNewer bool
	hmf.processedReleases.Range(func(key, value interface{}) bool {
		keyStr, ok := key.(string)
		if !ok {
			return true // continue iteration
		}

		// Check if this key matches our release (namespace/releaseName/rev-*)
		prefix := fmt.Sprintf("%s/%s%s", namespace, releaseName, revisionSeparator)
		if !strings.HasPrefix(keyStr, prefix) {
			return true // continue iteration
		}

		// Extract revision number from key
		revisionStr := strings.TrimPrefix(keyStr, prefix)
		if rev, err := strconv.Atoi(revisionStr); err == nil && rev > currentRevision {
			hasNewer = true
			return false // stop iteration
		}

		return true // continue iteration
	})
	return hasNewer
}

// processAndSendRelease builds release data and sends it, with proper error handling
func (hmf *HelmMetadataForwarder) processAndSendRelease(release *HelmReleaseMinimal, releaseName string, revision int, uid, namespace string) {
	releaseData := hmf.buildReleaseData(release, releaseName, revision, uid, namespace)
	if releaseData == nil {
		return
	}

	trackingKey := buildRevisionTrackingKey(namespace, releaseName, revision)

	if err := hmf.sendSingleReleasePayload(*releaseData); err != nil {
		hmf.logger.V(1).Info("Failed to send metadata for Helm release",
			"error", err,
			"release", releaseName,
			"namespace", namespace)
		hmf.processedReleases.Delete(trackingKey)
	} else {
		hmf.logger.V(1).Info("Successfully sent metadata for Helm release",
			"release", releaseName,
			"namespace", namespace,
			"revision", revision)
	}
}

// processHelmSecret processes a Helm release Secret and sends metadata if it's new
func (hmf *HelmMetadataForwarder) processHelmSecret(secret *corev1.Secret) {
	if !strings.HasPrefix(secret.Name, releasePrefix) {
		return
	}

	release, releaseName, revision, ok := hmf.parseHelmResource(secret.Name, secret.Data["release"])
	if !ok {
		return
	}

	// Filter for allowed charts (after decoding)
	if !allowedCharts[release.Chart.Metadata.Name] {
		hmf.logger.V(1).Info("Skipping non-allowed chart",
			"chart", release.Chart.Metadata.Name,
			"release", releaseName,
			"namespace", secret.Namespace)
		return
	}

	if !hmf.shouldProcessRevision(secret.Namespace, releaseName, revision) {
		return
	}

	hmf.logger.V(1).Info("Processing new/updated Helm release from Secret",
		"release", releaseName,
		"namespace", secret.Namespace,
		"revision", revision)

	hmf.processAndSendRelease(release, releaseName, revision, string(secret.UID), secret.Namespace)
}

// processHelmConfigMap processes a Helm release ConfigMap and sends metadata if new or updated
func (hmf *HelmMetadataForwarder) processHelmConfigMap(cm *corev1.ConfigMap) {
	if !strings.HasPrefix(cm.Name, releasePrefix) {
		return
	}

	release, releaseName, revision, ok := hmf.parseHelmResource(cm.Name, []byte(cm.Data["release"]))
	if !ok {
		return
	}

	// Filter for allowed charts (after decoding)
	if !allowedCharts[release.Chart.Metadata.Name] {
		hmf.logger.V(1).Info("Skipping non-allowed chart",
			"chart", release.Chart.Metadata.Name,
			"release", releaseName,
			"namespace", cm.Namespace)
		return
	}

	if !hmf.shouldProcessRevision(cm.Namespace, releaseName, revision) {
		return
	}

	hmf.logger.V(1).Info("Processing new/updated Helm release from ConfigMap",
		"release", releaseName,
		"namespace", cm.Namespace,
		"revision", revision)

	hmf.processAndSendRelease(release, releaseName, revision, string(cm.UID), cm.Namespace)
}

// buildReleaseData constructs HelmReleaseData from a parsed release
func (hmf *HelmMetadataForwarder) buildReleaseData(release *HelmReleaseMinimal, releaseName string, revision int, uid, namespace string) *HelmReleaseData {
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

	return &HelmReleaseData{
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

	hmf.logger.V(1).Info("Received HTTP response for metadata",
		"release", release.ReleaseName,
		"status_code", resp.StatusCode)

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
		Hostname:  hmf.hostName,
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
