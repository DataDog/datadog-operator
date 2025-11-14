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
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/version"
)

const (
	// helmValuesCacheTTL is the time-to-live for the cached Helm values (~5 minutes)
	helmValuesCacheTTL = 5 * time.Minute
	// releasePrefix is the prefix for Helm release ConfigMaps and Secrets
	releasePrefix = "sh.helm.release.v1."
)

var (
	versionRegexp = regexp.MustCompile(`\.v(\d+)$`)
	allowedCharts = map[string]bool{
		"datadog":          true,
		"datadog-operator": true,
		"datadog-agent":    true, // internal agent chart
	}
)

type HelmMetadataForwarder struct {
	*SharedMetadata

	// Helm-specific fields
	payloadHeader        http.Header
	allHelmReleasesCache allHelmReleasesCache
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

// allHelmReleasesCache holds cached Helm releases with timestamp
type allHelmReleasesCache struct {
	mu        sync.RWMutex
	releases  []HelmReleaseData
	timestamp time.Time
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
func NewHelmMetadataForwarder(logger logr.Logger, k8sClient client.Reader, kubernetesVersion string, operatorVersion string, credsManager *config.CredentialManager) *HelmMetadataForwarder {
	return &HelmMetadataForwarder{
		SharedMetadata: NewSharedMetadata(logger, k8sClient, kubernetesVersion, operatorVersion, credsManager),
	}
}

// getWatchNamespacesForHelm retrieves the list of namespaces to watch from environment variables
func getWatchNamespacesForHelm(logger logr.Logger) []string {
	nsMap := config.GetWatchNamespacesFromEnv(logger, config.AgentWatchNamespaceEnvVar)

	namespaces := make([]string, 0, len(nsMap))
	for ns := range nsMap {
		if ns == cache.AllNamespaces {
			logger.V(1).Info("Helm metadata watching all namespaces")
			return []string{""}
		}
		namespaces = append(namespaces, ns)
	}

	logger.V(1).Info("Helm metadata watching specific namespaces", "namespaces", namespaces)
	return namespaces
}

// Start starts the helm metadata forwarder
func (hmf *HelmMetadataForwarder) Start() {
	err := hmf.setCredentials()
	if err != nil {
		hmf.logger.Error(err, "Could not set credentials; not starting helm metadata forwarder")
		return
	}

	if hmf.hostName == "" {
		hmf.logger.Error(ErrEmptyHostName, "Could not set host name; not starting helm metadata forwarder")
		return
	}

	hmf.payloadHeader = hmf.getHeaders()

	hmf.logger.Info("Starting helm metadata forwarder")

	ticker := time.NewTicker(defaultInterval)
	go func() {
		for range ticker.C {
			if err := hmf.sendMetadata(); err != nil {
				hmf.logger.Error(err, "Error while sending helm metadata")
			}
		}
	}()
}

func (hmf *HelmMetadataForwarder) sendMetadata() error {
	ctx := context.Background()

	releases, err, notScrubbed := hmf.discoverAllHelmReleases(ctx)
	if err != nil {
		hmf.logger.Error(err, "Failed to discover Helm releases")
		return err
	}

	hmf.logger.Info("Discovered Helm releases", "count", len(releases))

	clusterUID, err := hmf.SharedMetadata.GetOrCreateClusterUID()
	if err != nil {
		hmf.logger.Error(err, "Error getting cluster UID")
	}

	var sendErrors []error
	for _, release := range releases {
		hmf.logger.V(1).Info("Processing Helm release",
			"release", release.ReleaseName,
			"namespace", release.Namespace,
			"chart", release.ChartName,
			"chart_version", release.ChartVersion)

		if err := hmf.sendSingleReleasePayload(release, clusterUID, notScrubbed); err != nil {
			hmf.logger.Error(err, "Failed to send payload for release",
				"release", release.ReleaseName,
				"namespace", release.Namespace)
			sendErrors = append(sendErrors, err)
		} else {
			hmf.logger.V(1).Info("Successfully sent Helm metadata",
				"release", release.ReleaseName,
				"namespace", release.Namespace)
		}
	}

	if len(sendErrors) > 0 {
		return fmt.Errorf("failed to send %d/%d helm release payloads", len(sendErrors), len(releases))
	}

	hmf.logger.V(1).Info("Successfully sent all Helm release metadata", "count", len(releases))
	return nil
}

func (hmf *HelmMetadataForwarder) sendSingleReleasePayload(release HelmReleaseData, clusterUID string, notScrubbed bool) error {
	payload := hmf.buildPayload(release, clusterUID)
	if notScrubbed {
		hmf.logger.V(1).Info("Built Helm metadata payload (not scrubbed)",
			"release", release.ReleaseName,
			"namespace", release.Namespace,
			"chart", release.ChartName,
			"payload_size", len(payload))
	} else {
		hmf.logger.V(1).Info("Built Helm metadata payload",
			"release", release.ReleaseName,
			"namespace", release.Namespace,
			"chart", release.ChartName,
			"payload_size", len(payload),
			"payload", string(payload))
	}

	hmf.logger.V(1).Info("Sending helm metadata HTTP request",
		"release", release.ReleaseName,
		"url", hmf.requestURL)

	reader := bytes.NewReader(payload)
	req, err := http.NewRequestWithContext(context.TODO(), "POST", hmf.requestURL, reader)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}
	req.Header = hmf.payloadHeader

	resp, err := hmf.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error sending helm metadata request: %w", err)
	}
	defer resp.Body.Close()

	hmf.logger.V(1).Info("Received HTTP response for Helm metadata",
		"release", release.ReleaseName,
		"status_code", resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read helm metadata response body: %w", err)
	}

	hmf.logger.V(1).Info("Read helm metadata response",
		"release", release.ReleaseName,
		"status_code", resp.StatusCode,
		"response_body", string(body))

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
		ClusterName:               hmf.clusterName,
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
		ClusterName: hmf.clusterName,
		Metadata:    helmMetadata,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		hmf.logger.Error(err, "Error marshaling payload to json",
			"release", release.ReleaseName)
	}

	return jsonPayload
}

func (hmf *HelmMetadataForwarder) setCredentials() error {
	return hmf.SharedMetadata.setCredentials()
}

func (hmf *HelmMetadataForwarder) getHeaders() http.Header {
	headers := hmf.GetBaseHeaders()
	headers.Set(userAgentHTTPHeaderKey, fmt.Sprintf("Datadog Operator/%s", version.GetVersion()))
	return headers
}

// getFromCache retrieves the cached Helm releases if they exist and are not expired
func (c *allHelmReleasesCache) getFromCache() ([]HelmReleaseData, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.releases == nil || time.Since(c.timestamp) > helmValuesCacheTTL {
		return nil, false
	}
	return c.releases, true
}

// setCache stores the Helm releases in the cache with the current timestamp
func (c *allHelmReleasesCache) setCache(releases []HelmReleaseData) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.releases = releases
	c.timestamp = time.Now()
}

// discoverAllHelmReleases finds all Helm releases in the watched namespaces
func (hmf *HelmMetadataForwarder) discoverAllHelmReleases(ctx context.Context) ([]HelmReleaseData, error, bool) {
	var notScrubbed bool

	// Check cache first
	if cachedReleases, ok := hmf.allHelmReleasesCache.getFromCache(); ok {
		hmf.logger.V(1).Info("Using cached Helm releases", "count", len(cachedReleases))
		return cachedReleases, nil, notScrubbed
	}

	hmf.logger.V(1).Info("Cache miss, discovering Helm releases from cluster")

	latestReleases := make(map[string]struct {
		release  HelmReleaseMinimal
		uid      string
		revision int
	})

	var allErrors []error

	namespacesToSearch := getWatchNamespacesForHelm(hmf.logger)
	for _, namespace := range namespacesToSearch {
		listOpts := []client.ListOption{
			client.MatchingLabels{"owner": "helm"},
		}
		if namespace != "" {
			listOpts = append(listOpts, client.InNamespace(namespace))
		}

		secretList := &corev1.SecretList{}
		if err := hmf.k8sClient.List(ctx, secretList, listOpts...); err != nil {
			hmf.logger.Error(err, "Error listing Secrets for Helm releases", "namespace", namespace)
			allErrors = append(allErrors, fmt.Errorf("secrets in namespace %s: %w", namespace, err))
		} else {
			hmf.logger.V(1).Info("Scanning Secrets for Helm releases", "namespace", namespace, "total_secrets", len(secretList.Items))
			for _, secret := range secretList.Items {
				if !strings.HasPrefix(secret.Name, releasePrefix) {
					continue
				}

				if release, releaseName, revision, ok := hmf.parseHelmResource(secret.Name, secret.Data["release"]); ok {
					if !allowedCharts[release.Chart.Metadata.Name] {
						continue
					}
					key := fmt.Sprintf("%s/%s", secret.Namespace, releaseName)
					if existing, exists := latestReleases[key]; !exists || revision > existing.revision {
						latestReleases[key] = struct {
							release  HelmReleaseMinimal
							uid      string
							revision int
						}{
							release:  *release,
							uid:      string(secret.UID),
							revision: revision,
						}
					}
				}
			}
		}

		cmList := &corev1.ConfigMapList{}
		if err := hmf.k8sClient.List(ctx, cmList, listOpts...); err != nil {
			hmf.logger.Error(err, "Error listing ConfigMaps for Helm releases", "namespace", namespace)
			allErrors = append(allErrors, fmt.Errorf("configmaps in namespace %s: %w", namespace, err))
		} else {
			hmf.logger.V(1).Info("Scanning ConfigMaps for Helm releases", "namespace", namespace, "total_configmaps", len(cmList.Items))
			for _, cm := range cmList.Items {
				if !strings.HasPrefix(cm.Name, releasePrefix) {
					continue
				}

				if release, releaseName, revision, ok := hmf.parseHelmResource(cm.Name, []byte(cm.Data["release"])); ok {
					if !allowedCharts[release.Chart.Metadata.Name] {
						continue
					}
					key := fmt.Sprintf("%s/%s", cm.Namespace, releaseName)
					if existing, exists := latestReleases[key]; !exists || revision > existing.revision {
						latestReleases[key] = struct {
							release  HelmReleaseMinimal
							uid      string
							revision int
						}{
							release:  *release,
							uid:      string(cm.UID),
							revision: revision,
						}
					}
				}
			}
		}
	}

	if len(allErrors) > 0 && len(latestReleases) == 0 {
		return nil, fmt.Errorf("failed to discover any Helm releases: %v", allErrors), notScrubbed
	}

	releases := make([]HelmReleaseData, 0, len(latestReleases))
	for _, data := range latestReleases {
		providedValuesYAML, err := yaml.Marshal(data.release.Config)
		if err != nil {
			hmf.logger.V(1).Info("Failed to marshal Helm provided values", "release", data.release.Name, "error", err)
			continue
		}

		fullValues := hmf.mergeValues(data.release.Chart.Values, data.release.Config)
		fullValuesYAML, err := yaml.Marshal(fullValues)
		if err != nil {
			hmf.logger.V(1).Info("Failed to marshal Helm full values", "release", data.release.Name, "error", err)
			// Fall back
			fullValuesYAML = providedValuesYAML
		}

		scrubbedProvidedYAML, err := scrubber.ScrubBytes(providedValuesYAML)
		if err != nil {
			hmf.logger.V(1).Info("Failed to scrub provided values, using unscrubbed", "release", data.release.Name, "error", err)
			scrubbedProvidedYAML = providedValuesYAML
			notScrubbed = true
		}

		scrubbedFullYAML, err := scrubber.ScrubBytes(fullValuesYAML)
		if err != nil {
			hmf.logger.V(1).Info("Failed to scrub full values, using unscrubbed", "release", data.release.Name, "error", err)
			scrubbedFullYAML = fullValuesYAML
			notScrubbed = true
		}
		// values will be scrubbed again in decoder layer

		releaseData := HelmReleaseData{
			ReleaseName:        data.release.Name,
			Namespace:          data.release.Namespace,
			ChartName:          data.release.Chart.Metadata.Name,
			ChartVersion:       data.release.Chart.Metadata.Version,
			AppVersion:         data.release.Chart.Metadata.AppVersion,
			ConfigMapUID:       data.uid,
			ProvidedValuesYAML: string(scrubbedProvidedYAML),
			FullValuesYAML:     string(scrubbedFullYAML),
			Revision:           data.revision,
			Status:             data.release.Info.Status,
		}
		releases = append(releases, releaseData)
	}

	hmf.allHelmReleasesCache.setCache(releases)

	return releases, nil, notScrubbed
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
