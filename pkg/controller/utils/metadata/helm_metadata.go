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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	k8sclientset "k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/pkg/config"
)

const (
	// releasePrefix is the prefix for Helm release ConfigMaps and Secrets
	releasePrefix = "sh.helm.release.v1."
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

	// Track processed releases to avoid duplicate sends
	// Key: namespace/name/resourceVersion
	processedReleases sync.Map

	// Context for watch cancellation
	ctx    context.Context
	cancel context.CancelFunc

	// Kubernetes clientset for direct watch API access
	clientset *k8sclientset.Clientset
}

type HelmMetadataPayload struct {
	Hostname    string       `json:"hostname"`
	Timestamp   int64        `json:"timestamp"`
	ClusterID   string       `json:"cluster_id"`
	ClusterName string       `json:"clustername"`
	Metadata    HelmMetadata `json:"datadog_operator_helm_metadata"`
}

type HelmMetadata struct {
	OperatorVersion           string `json:"operator_version"`
	KubernetesVersion         string `json:"kubernetes_version"`
	ClusterID                 string `json:"cluster_id"`
	ClusterName               string `json:"cluster_name"`
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

// NewHelmMetadataForwarder creates a new instance of the helm metadata forwarder
func NewHelmMetadataForwarder(logger logr.Logger, k8sClient client.Reader, clientset *k8sclientset.Clientset, kubernetesVersion string, operatorVersion string, credsManager *config.CredentialManager) *HelmMetadataForwarder {
	forwarderLogger := logger.WithName("helm")
	ctx, cancel := context.WithCancel(context.Background())
	return &HelmMetadataForwarder{
		SharedMetadata: NewSharedMetadata(forwarderLogger, k8sClient, kubernetesVersion, operatorVersion, credsManager),
		ctx:            ctx,
		cancel:         cancel,
		clientset:      clientset,
	}
}

// Stop stops the helm metadata forwarder and cancels all watches
func (hmf *HelmMetadataForwarder) Stop() {
	hmf.logger.Info("Stopping metadata forwarder")
	if hmf.cancel != nil {
		hmf.cancel()
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

// Start starts the helm metadata forwarder with watch-based event handling
func (hmf *HelmMetadataForwarder) Start() {
	if hmf.hostName == "" {
		hmf.logger.Error(ErrEmptyHostName, "Could not set host name; not starting metadata forwarder")
		return
	}

	hmf.logger.Info("Starting metadata forwarder")

	namespacesToWatch := getWatchNamespacesForHelm(hmf.logger)

	for _, namespace := range namespacesToWatch {
		for chartName := range allowedCharts {
			// Start watch for Secrets
			go hmf.watchHelmResources(namespace, chartName, true)
			// Start watch for ConfigMaps
			go hmf.watchHelmResources(namespace, chartName, false)
		}
	}
}

// watchHelmResources watches for Helm release changes in Secrets or ConfigMaps
func (hmf *HelmMetadataForwarder) watchHelmResources(namespace, chartName string, isSecret bool) {
	resourceType := "ConfigMap"
	if isSecret {
		resourceType = "Secret"
	}

	hmf.logger.V(1).Info("Starting watch for Helm releases",
		"namespace", namespace,
		"chart", chartName,
		"resource_type", resourceType)

	for {
		select {
		case <-hmf.ctx.Done():
			hmf.logger.V(1).Info("Watch cancelled", "namespace", namespace, "chart", chartName, "resource_type", resourceType)
			return
		default:
		}

		if err := hmf.watchLoop(namespace, chartName, isSecret); err != nil {
			hmf.logger.Info("Watch error, will retry",
				"error", err,
				"namespace", namespace,
				"chart", chartName,
				"resource_type", resourceType)
			// Backoff before retry
			time.Sleep(5 * time.Second)
		}
	}
}

// watchLoop performs a single watch cycle
func (hmf *HelmMetadataForwarder) watchLoop(namespace, chartName string, isSecret bool) error {
	labelSelector := fmt.Sprintf("owner=helm,name=%s", chartName)

	watchNamespace := namespace
	if watchNamespace == "" {
		watchNamespace = metav1.NamespaceAll
	}

	watchOpts := metav1.ListOptions{
		LabelSelector: labelSelector,
		Watch:         true,
	}

	var watcher watch.Interface

	if isSecret {
		secretList, err := hmf.clientset.CoreV1().Secrets(watchNamespace).List(hmf.ctx, metav1.ListOptions{
			LabelSelector: labelSelector,
		})
		if err != nil {
			return fmt.Errorf("failed to list secrets: %w", err)
		}

		// Filter to only process the latest revision of each release
		latestSecrets := hmf.findLatestSecretRevisions(secretList.Items)
		for i := range latestSecrets {
			hmf.processHelmSecret(&latestSecrets[i])
		}
		hmf.logger.V(1).Info("Processed existing secrets, starting watch", "total", len(secretList.Items), "latest_only", len(latestSecrets), "namespace", watchNamespace)

		// Start watching for new/updated releases
		watcher, err = hmf.clientset.CoreV1().Secrets(watchNamespace).Watch(hmf.ctx, watchOpts)
		if err != nil {
			return fmt.Errorf("failed to start secret watch: %w", err)
		}
	} else {
		cmList, err := hmf.clientset.CoreV1().ConfigMaps(watchNamespace).List(hmf.ctx, metav1.ListOptions{
			LabelSelector: labelSelector,
		})
		if err != nil {
			return fmt.Errorf("failed to list configmaps: %w", err)
		}

		// Filter to only process the latest revision of each release
		latestCMs := hmf.findLatestConfigMapRevisions(cmList.Items)
		for i := range latestCMs {
			hmf.processHelmConfigMap(&latestCMs[i])
		}
		hmf.logger.V(1).Info("Processed existing configmaps, starting watch", "total", len(cmList.Items), "latest_only", len(latestCMs), "namespace", watchNamespace)

		// Start watching for new/updated releases
		watcher, err = hmf.clientset.CoreV1().ConfigMaps(watchNamespace).Watch(hmf.ctx, watchOpts)
		if err != nil {
			return fmt.Errorf("failed to start configmap watch: %w", err)
		}
	}

	defer watcher.Stop()

	// Process watch events until context is cancelled or watch fails
	for {
		select {
		case <-hmf.ctx.Done():
			hmf.logger.V(1).Info("Watch context cancelled", "namespace", watchNamespace, "chart", chartName)
			return nil

		case event, ok := <-watcher.ResultChan():
			if !ok {
				return fmt.Errorf("watch channel closed, will reconnect")
			}

			switch event.Type {
			case watch.Added:
				if isSecret {
					if secret, ok := event.Object.(*corev1.Secret); ok {
						hmf.processHelmSecret(secret)
					}
				} else {
					if cm, ok := event.Object.(*corev1.ConfigMap); ok {
						hmf.processHelmConfigMap(cm)
					}
				}

			case watch.Error:
				hmf.logger.V(1).Info("Watch error event received",
					"namespace", watchNamespace,
					"chart", chartName)
				return fmt.Errorf("watch error event")
			}
		}
	}
}

// findLatestSecretRevisions filters a list of Secrets to only include the latest revision of each Helm release
func (hmf *HelmMetadataForwarder) findLatestSecretRevisions(secrets []corev1.Secret) []corev1.Secret {
	latestRevisions := make(map[string]struct {
		secret   *corev1.Secret
		revision int
	})

	for i := range secrets {
		secret := &secrets[i]
		if !strings.HasPrefix(secret.Name, releasePrefix) {
			continue
		}

		_, releaseName, revision, ok := hmf.parseHelmResource(secret.Name, secret.Data["release"])
		if !ok {
			continue
		}

		key := fmt.Sprintf("%s/%s", secret.Namespace, releaseName)
		if existing, exists := latestRevisions[key]; !exists || revision > existing.revision {
			latestRevisions[key] = struct {
				secret   *corev1.Secret
				revision int
			}{
				secret:   secret,
				revision: revision,
			}
		}
	}

	result := make([]corev1.Secret, 0, len(latestRevisions))
	for _, data := range latestRevisions {
		result = append(result, *data.secret)
	}
	return result
}

// findLatestConfigMapRevisions filters a list of ConfigMaps to only include the latest revision of each Helm release
func (hmf *HelmMetadataForwarder) findLatestConfigMapRevisions(configMaps []corev1.ConfigMap) []corev1.ConfigMap {
	latestRevisions := make(map[string]struct {
		cm       *corev1.ConfigMap
		revision int
	})

	for i := range configMaps {
		cm := &configMaps[i]
		if !strings.HasPrefix(cm.Name, releasePrefix) {
			continue
		}

		_, releaseName, revision, ok := hmf.parseHelmResource(cm.Name, []byte(cm.Data["release"]))
		if !ok {
			continue
		}

		key := fmt.Sprintf("%s/%s", cm.Namespace, releaseName)
		if existing, exists := latestRevisions[key]; !exists || revision > existing.revision {
			latestRevisions[key] = struct {
				cm       *corev1.ConfigMap
				revision int
			}{
				cm:       cm,
				revision: revision,
			}
		}
	}

	result := make([]corev1.ConfigMap, 0, len(latestRevisions))
	for _, data := range latestRevisions {
		result = append(result, *data.cm)
	}
	return result
}

// processHelmSecret processes a Helm release Secret and sends metadata if it's new
func (hmf *HelmMetadataForwarder) processHelmSecret(secret *corev1.Secret) {
	if !strings.HasPrefix(secret.Name, releasePrefix) {
		return
	}

	trackingKey := fmt.Sprintf("%s/%s/%s", secret.Namespace, secret.Name, secret.ResourceVersion)
	if _, exists := hmf.processedReleases.LoadOrStore(trackingKey, true); exists {
		hmf.logger.V(1).Info("Skipping already processed secret", "secret", secret.Name, "resourceVersion", secret.ResourceVersion)
		return
	}

	release, releaseName, revision, ok := hmf.parseHelmResource(secret.Name, secret.Data["release"])
	if !ok {
		return
	}

	hmf.logger.V(1).Info("Processing new/updated Helm release from Secret",
		"release", releaseName,
		"namespace", secret.Namespace,
		"revision", revision)

	releaseData := hmf.buildReleaseData(release, releaseName, revision, string(secret.UID), secret.Namespace)
	if releaseData == nil {
		return
	}

	if err := hmf.sendSingleReleasePayload(*releaseData); err != nil {
		hmf.logger.Info("Failed to send metadata for Helm release",
			"error", err,
			"release", releaseName,
			"namespace", secret.Namespace)
		hmf.processedReleases.Delete(trackingKey)
	} else {
		hmf.logger.Info("Successfully sent metadata for Helm release",
			"release", releaseName,
			"namespace", secret.Namespace,
			"revision", revision)
	}
}

// processHelmConfigMap processes a Helm release ConfigMap and sends metadata if new or updated
func (hmf *HelmMetadataForwarder) processHelmConfigMap(cm *corev1.ConfigMap) {
	if !strings.HasPrefix(cm.Name, releasePrefix) {
		return
	}

	trackingKey := fmt.Sprintf("%s/%s/%s", cm.Namespace, cm.Name, cm.ResourceVersion)
	if _, exists := hmf.processedReleases.LoadOrStore(trackingKey, true); exists {
		hmf.logger.V(1).Info("Skipping already processed configmap", "configmap", cm.Name, "resourceVersion", cm.ResourceVersion)
		return
	}

	release, releaseName, revision, ok := hmf.parseHelmResource(cm.Name, []byte(cm.Data["release"]))
	if !ok {
		return
	}

	hmf.logger.V(1).Info("Processing new/updated Helm release from ConfigMap",
		"release", releaseName,
		"namespace", cm.Namespace,
		"revision", revision)

	releaseData := hmf.buildReleaseData(release, releaseName, revision, string(cm.UID), cm.Namespace)
	if releaseData == nil {
		return
	}

	if err := hmf.sendSingleReleasePayload(*releaseData); err != nil {
		hmf.logger.Info("Failed to send metadata for Helm release",
			"error", err,
			"release", releaseName,
			"namespace", cm.Namespace)
		hmf.processedReleases.Delete(trackingKey)
	} else {
		hmf.logger.Info("Successfully sent metadata for Helm release",
			"release", releaseName,
			"namespace", cm.Namespace,
			"revision", revision)
	}
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

	hmf.logger.V(1).Info("Built metadata payload",
		"release", release.ReleaseName,
		"namespace", release.Namespace,
		"chart", release.ChartName,
		"payload_size", len(payload))

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

	hmf.logger.V(1).Info("Read metadata response",
		"release", release.ReleaseName,
		"status_code", resp.StatusCode)

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
		ClusterName:               hmf.GetOrCreateClusterName(),
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
		Hostname:    hmf.hostName,
		Timestamp:   now,
		ClusterID:   clusterUID,
		ClusterName: hmf.GetOrCreateClusterName(),
		Metadata:    helmMetadata,
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
