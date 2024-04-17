// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	kubeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/config/remote/service"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/pkg/version"
)

const (
	defaultSite           = "datadoghq.com"
	pollInterval          = 10 * time.Second
	remoteConfigUrlPrefix = "https://config."
)

type RemoteConfigUpdater struct {
	kubeClient  kubeclient.Client
	rcClient    *client.Client
	rcService   *service.Service
	serviceConf RcServiceConfiguration
	logger      logr.Logger
}

type RcServiceConfiguration struct {
	cfg               model.Config
	apiKey            string
	site              string
	baseRawURL        string
	hostname          string
	clusterName       string
	telemetryReporter service.RcTelemetryReporter
	agentVersion      string
	rcDatabaseDir     string
}

// DatadogAgentRemoteConfig contains the struct used to update DatadogAgent object from RemoteConfig
type DatadogAgentRemoteConfig struct {
	ID            string                       `json:"name"`
	CoreAgent     *CoreAgentFeaturesConfig     `json:"config"`
	SystemProbe   *SystemProbeFeaturesConfig   `json:"system_probe"`
	SecurityAgent *SecurityAgentFeaturesConfig `json:"security_agent"`
}

type CoreAgentFeaturesConfig struct {
	SBOM *SbomConfig `json:"sbom"`
}

type SystemProbeFeaturesConfig struct {
	CWS *FeatureEnabledConfig `json:"runtime_security_config"`
	USM *FeatureEnabledConfig `json:"service_monitoring_config"`
}

type SecurityAgentFeaturesConfig struct {
	CSPM *FeatureEnabledConfig `json:"compliance_config"`
}
type SbomConfig struct {
	Enabled        *bool                 `json:"enabled"`
	Host           *FeatureEnabledConfig `json:"host"`
	ContainerImage *FeatureEnabledConfig `json:"container_image"`
}
type FeatureEnabledConfig struct {
	Enabled *bool `json:"enabled"`
}

type agentConfigOrder struct {
	Order         []string `json:"order"`
	InternalOrder []string `json:"internal_order"`
}

// TODO replace
type dummyRcTelemetryReporter struct{}

func (d dummyRcTelemetryReporter) IncRateLimit() {}
func (d dummyRcTelemetryReporter) IncTimeout()   {}

func (r *RemoteConfigUpdater) Setup(dda *v2alpha1.DatadogAgent) error {
	// Get API Key from DatadogAgent
	apiKey, err := r.getAPIKeyFromDatadogAgent(dda)
	if err != nil {
		return err
	}

	// Extract needed configs from the DatadogAgent
	var site string
	if dda.Spec.Global.Site != nil && *dda.Spec.Global.Site != "" {
		site = *dda.Spec.Global.Site
	}

	var clusterName string
	if dda.Spec.Global.ClusterName != nil && *dda.Spec.Global.ClusterName != "" {
		clusterName = *dda.Spec.Global.ClusterName
	}

	var rcDirectorRoot string
	if dda.Spec.Features.RemoteConfiguration.DirectorRoot != nil && *dda.Spec.Features.RemoteConfiguration.DirectorRoot != "" {
		rcDirectorRoot = *dda.Spec.Features.RemoteConfiguration.DirectorRoot
	}
	var rcConfigRoot string
	if dda.Spec.Features.RemoteConfiguration.ConfigRoot != nil && *dda.Spec.Features.RemoteConfiguration.ConfigRoot != "" {
		rcConfigRoot = *dda.Spec.Features.RemoteConfiguration.ConfigRoot
	}

	var rcEndpoint string
	if dda.Spec.Features.RemoteConfiguration.Endpoint != nil && *dda.Spec.Features.RemoteConfiguration.Endpoint != "" {
		rcEndpoint = *dda.Spec.Features.RemoteConfiguration.Endpoint
	}

	if r.rcClient == nil && r.rcService == nil {
		// If rcClient && rcService not setup yet
		err = r.Start(apiKey, site, clusterName, rcDirectorRoot, rcConfigRoot, rcEndpoint)
		if err != nil {
			return err
		}
	} else if apiKey != r.serviceConf.apiKey || site != r.serviceConf.site || clusterName != r.serviceConf.clusterName || rcEndpoint != r.serviceConf.baseRawURL {
		// If one of configs has been updated
		err := r.Stop()
		if err != nil {
			return err
		}
		err = r.Start(apiKey, site, clusterName, rcDirectorRoot, rcConfigRoot, rcEndpoint)
		if err != nil {
			return err
		}
	}
	return nil

}

func (r *RemoteConfigUpdater) Start(apiKey string, site string, clusterName string, rcDirectorRoot string, rcConfigRoot string, rcEndpoint string) error {

	r.logger.Info("starting Remote Configuration client and service")

	// Fill in rc service configuration
	err := r.configureService(apiKey, site, clusterName, rcDirectorRoot, rcConfigRoot, rcEndpoint)
	if err != nil {
		r.logger.Error(err, "Failed to configure Remote Configuration service")
		return err
	}

	rcService, err := service.NewService(
		r.serviceConf.cfg,
		r.serviceConf.apiKey,
		r.serviceConf.baseRawURL,
		r.serviceConf.hostname,
		[]string{fmt.Sprintf("cluster_name:%s", r.serviceConf.clusterName)},
		r.serviceConf.telemetryReporter,
		r.serviceConf.agentVersion,
		service.WithDatabaseFileName(filepath.Join(r.serviceConf.rcDatabaseDir, fmt.Sprintf("remote-config-%s.db", uuid.New()))))
	if err != nil {
		r.logger.Error(err, "Failed to create Remote Configuration service")
		return err
	}
	r.rcService = rcService

	rcClient, err := client.NewClient(
		rcService,
		client.WithAgent("datadog-operator", version.Version),
		client.WithProducts(state.ProductAgentConfig),
		client.WithDirectorRootOverride(r.serviceConf.cfg.GetString("remote_configuration.director_root")),
		client.WithPollInterval(10*time.Second))
	if err != nil {
		r.logger.Error(err, "Failed to create Remote Configuration client")
		return err
	}
	r.rcClient = rcClient

	rcService.Start()
	r.logger.Info("rcService started")

	rcClient.Start()
	r.logger.Info("rcClient started")

	rcClient.Subscribe(string(state.ProductAgentConfig), r.agentConfigUpdateCallback)

	return nil
}

func (r *RemoteConfigUpdater) agentConfigUpdateCallback(updates map[string]state.RawConfig, applyStatus func(string, state.ApplyStatus)) {

	ctx := context.Background()

	r.logger.Info("agentConfigUpdateCallback is called")

	// Tell rc that we have received the configurations
	var configIDs []string
	for id := range updates {
		applyStatus(id, state.ApplyStatus{State: state.ApplyStateUnacknowledged, Error: ""})
		configIDs = append(configIDs, id)
	}

	mergedUpdate, err := r.parseReceivedUpdates(updates, applyStatus)
	if err != nil {
		r.logger.Error(err, "failed to merge updates")
		return
	}
	r.logger.Info("Merged", "update", mergedUpdate)

	dda, err := r.getDatadogAgentInstance(ctx)
	if err != nil {
		r.logger.Error(err, "failed to get updatable agents")
	}

	if err := r.applyConfig(ctx, dda, mergedUpdate); err != nil {
		r.logger.Error(err, "failed to apply config")
		applyStatus(configIDs[len(configIDs)-1], state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
		return
	}

	// Tell rc that we have received the configurations
	for _, id := range configIDs {
		applyStatus(id, state.ApplyStatus{State: state.ApplyStateAcknowledged, Error: ""})
	}

	r.logger.Info("successfully applied config!")
}

func (r *RemoteConfigUpdater) getAPIKeyFromDatadogAgent(dda *v2alpha1.DatadogAgent) (string, error) {
	var err error
	apiKey := ""

	// If APIKey is set directly
	if dda.Spec.Global != nil && dda.Spec.Global.Credentials != nil && dda.Spec.Global.Credentials.APIKey != nil && *dda.Spec.Global.Credentials.APIKey != "" {
		return *dda.Spec.Global.Credentials.APIKey, nil
	}

	// If a secret is used
	if dda.Spec.Global != nil && dda.Spec.Global.Credentials != nil {
		isSet, secretName, secretKeyName := v2alpha1.GetAPIKeySecret(dda.Spec.Global.Credentials, v2alpha1.GetDefaultCredentialsSecretName(dda))
		if isSet {
			apiKey, err = r.getKeyFromSecret(dda.Namespace, secretName, secretKeyName)
			if err != nil {
				return "", err
			}
			return apiKey, nil
		}
	}
	return "", fmt.Errorf("no APIKey set")

}

func (r *RemoteConfigUpdater) getKeyFromSecret(namespace, secretName, dataKey string) (string, error) {
	secret := &corev1.Secret{}
	err := r.kubeClient.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: secretName}, secret)
	if err != nil {
		return "", err
	}

	return string(secret.Data[dataKey]), nil
}

func (r *RemoteConfigUpdater) Stop() error {
	if r.rcService != nil {
		err := r.rcService.Stop()
		if err != nil {
			return err
		}
	}
	if r.rcClient != nil {
		r.rcClient.Close()
	}
	r.rcService = nil
	r.rcClient = nil
	return nil
}

func (r *RemoteConfigUpdater) parseReceivedUpdates(updates map[string]state.RawConfig, applyStatus func(string, state.ApplyStatus)) (DatadogAgentRemoteConfig, error) {

	// Unmarshall configs and config order
	var order agentConfigOrder
	var configs []DatadogAgentRemoteConfig
	for configPath, c := range updates {
		if c.Metadata.ID == "configuration_order" {
			if err := json.Unmarshal(c.Config, &order); err != nil {
				r.logger.Info("Error unmarshalling configuration_order:", "err", err)
				applyStatus(configPath, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
				return DatadogAgentRemoteConfig{}, fmt.Errorf("could not unmarshall configuration order")
			}
		} else {
			var configData DatadogAgentRemoteConfig
			if err := json.Unmarshal(c.Config, &configData); err != nil {
				applyStatus(configPath, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
				r.logger.Info("Error unmarshalling JSON:", "err", err)
				return DatadogAgentRemoteConfig{}, fmt.Errorf("could not unmarshall configuration %s", c.Metadata.ID)

			} else {
				configs = append(configs, configData)
			}
		}
	}

	// Merge configs
	var finalConfig DatadogAgentRemoteConfig

	configMap := make(map[string]DatadogAgentRemoteConfig)
	for _, config := range configs {
		configMap[config.ID] = config
	}

	for _, name := range order.Order {
		if config, found := configMap[name]; found {
			mergeConfigs(&finalConfig, &config)
		}
	}
	return finalConfig, nil
}

func mergeConfigs(dst, src *DatadogAgentRemoteConfig) {
	if src.ID != "" && dst.ID != "" {
		dst.ID = dst.ID + "|" + src.ID
	} else if src.ID != "" {
		dst.ID = src.ID
	}

	// CoreAgent
	if src.CoreAgent != nil {
		if dst.CoreAgent == nil {
			dst.CoreAgent = &CoreAgentFeaturesConfig{}
		}
		if src.CoreAgent.SBOM != nil {
			if dst.CoreAgent.SBOM == nil {
				dst.CoreAgent.SBOM = &SbomConfig{}
			}
			if src.CoreAgent.SBOM.Enabled != nil {
				dst.CoreAgent.SBOM.Enabled = src.CoreAgent.SBOM.Enabled
			}
			// Merging SbomConfig's Host
			if src.CoreAgent.SBOM.Host != nil {
				if dst.CoreAgent.SBOM.Host == nil {
					dst.CoreAgent.SBOM.Host = &FeatureEnabledConfig{}
				}
				if src.CoreAgent.SBOM.Host.Enabled != nil {
					dst.CoreAgent.SBOM.Host.Enabled = src.CoreAgent.SBOM.Host.Enabled
				}
			}
			// Merging SbomConfig's ContainerImage
			if src.CoreAgent.SBOM.ContainerImage != nil {
				if dst.CoreAgent.SBOM.ContainerImage == nil {
					dst.CoreAgent.SBOM.ContainerImage = &FeatureEnabledConfig{}
				}
				if src.CoreAgent.SBOM.ContainerImage.Enabled != nil {
					dst.CoreAgent.SBOM.ContainerImage.Enabled = src.CoreAgent.SBOM.ContainerImage.Enabled
				}
			}
		}
	}

	// SystemProbe
	if src.SystemProbe != nil {
		if dst.SystemProbe == nil {
			dst.SystemProbe = &SystemProbeFeaturesConfig{}
		}
		// Merging USM
		if src.SystemProbe.USM != nil {
			if dst.SystemProbe.USM == nil {
				dst.SystemProbe.USM = &FeatureEnabledConfig{}
			}
			if src.SystemProbe.USM.Enabled != nil {
				dst.SystemProbe.USM.Enabled = src.SystemProbe.USM.Enabled
			}
		}
		// Merging CWS
		if src.SystemProbe.CWS != nil {
			if dst.SystemProbe.CWS == nil {
				dst.SystemProbe.CWS = &FeatureEnabledConfig{}
			}
			if src.SystemProbe.CWS.Enabled != nil {
				dst.SystemProbe.CWS.Enabled = src.SystemProbe.CWS.Enabled
			}
		}

	}

	// SecurityAgent
	if src.SecurityAgent != nil {
		if dst.SecurityAgent == nil {
			dst.SecurityAgent = &SecurityAgentFeaturesConfig{}
		}
		if src.SecurityAgent.CSPM != nil {
			if dst.SecurityAgent.CSPM == nil {
				dst.SecurityAgent.CSPM = &FeatureEnabledConfig{}
			}
			if src.SecurityAgent.CSPM.Enabled != nil {
				dst.SecurityAgent.CSPM.Enabled = src.SecurityAgent.CSPM.Enabled
			}
		}
	}

}

func (r *RemoteConfigUpdater) getDatadogAgentInstance(ctx context.Context) (v2alpha1.DatadogAgent, error) {
	ddaList := &v2alpha1.DatadogAgentList{}
	if err := r.kubeClient.List(context.TODO(), ddaList); err != nil {
		return v2alpha1.DatadogAgent{}, fmt.Errorf("unable to list DatadogAgents: %w", err)
	}

	if len(ddaList.Items) == 0 {
		return v2alpha1.DatadogAgent{}, errors.New("cannot find any DatadogAgent")
	}

	// Return first DatadogAgent as only one is supported
	return ddaList.Items[0], nil
}

func (r *RemoteConfigUpdater) applyConfig(ctx context.Context, dda v2alpha1.DatadogAgent, cfg DatadogAgentRemoteConfig) error {
	if err := r.updateInstance(dda, cfg); err != nil {
		return err
	}

	return nil
}

func (r *RemoteConfigUpdater) updateInstance(dda v2alpha1.DatadogAgent, cfg DatadogAgentRemoteConfig) error {

	newdda := dda.DeepCopy()
	if newdda.Spec.Features == nil {
		newdda.Spec.Features = &v2alpha1.DatadogFeatures{}
	}

	// CWS
	if cfg.SystemProbe != nil && cfg.SystemProbe.CWS != nil {
		if newdda.Spec.Features.CWS == nil {
			newdda.Spec.Features.CWS = &v2alpha1.CWSFeatureConfig{}
		}
		if newdda.Spec.Features.CWS.Enabled == nil {
			newdda.Spec.Features.CWS.Enabled = new(bool)
		}
		newdda.Spec.Features.CWS.Enabled = cfg.SystemProbe.CWS.Enabled
	}

	// CSPM
	if cfg.SecurityAgent != nil && cfg.SecurityAgent.CSPM != nil {
		if newdda.Spec.Features.CSPM == nil {
			newdda.Spec.Features.CSPM = &v2alpha1.CSPMFeatureConfig{}
		}
		if newdda.Spec.Features.CSPM.Enabled == nil {
			newdda.Spec.Features.CSPM.Enabled = new(bool)
		}
		newdda.Spec.Features.CSPM.Enabled = cfg.SecurityAgent.CSPM.Enabled
	}

	// SBOM
	if cfg.CoreAgent != nil && cfg.CoreAgent.SBOM != nil {
		if newdda.Spec.Features.SBOM == nil {
			newdda.Spec.Features.SBOM = &v2alpha1.SBOMFeatureConfig{}
		}
		if newdda.Spec.Features.SBOM.Enabled == nil {
			newdda.Spec.Features.SBOM.Enabled = new(bool)
		}
		newdda.Spec.Features.SBOM.Enabled = cfg.CoreAgent.SBOM.Enabled

		// SBOM HOST
		if cfg.CoreAgent.SBOM.Host != nil {
			if newdda.Spec.Features.SBOM.Host == nil {
				newdda.Spec.Features.SBOM.Host = &v2alpha1.SBOMTypeConfig{}
			}
			if newdda.Spec.Features.SBOM.Host.Enabled == nil {
				newdda.Spec.Features.SBOM.Host.Enabled = new(bool)
			}
			newdda.Spec.Features.SBOM.Host.Enabled = cfg.CoreAgent.SBOM.Host.Enabled
		}

		// SBOM CONTAINER IMAGE
		if cfg.CoreAgent.SBOM.ContainerImage != nil {
			if newdda.Spec.Features.SBOM.ContainerImage == nil {
				newdda.Spec.Features.SBOM.ContainerImage = &v2alpha1.SBOMTypeConfig{}
			}
			if newdda.Spec.Features.SBOM.ContainerImage.Enabled == nil {
				newdda.Spec.Features.SBOM.ContainerImage.Enabled = new(bool)
			}
			newdda.Spec.Features.SBOM.ContainerImage.Enabled = cfg.CoreAgent.SBOM.ContainerImage.Enabled
		}

	}

	// USM
	if cfg.SystemProbe != nil && cfg.SystemProbe.USM != nil {
		if newdda.Spec.Features.USM == nil {
			newdda.Spec.Features.USM = &v2alpha1.USMFeatureConfig{}
		}
		if newdda.Spec.Features.USM.Enabled == nil {
			newdda.Spec.Features.USM.Enabled = new(bool)
		}
		newdda.Spec.Features.USM.Enabled = cfg.SystemProbe.USM.Enabled
	}

	if !apiutils.IsEqualStruct(dda.Spec, newdda.Spec) {
		return r.kubeClient.Update(context.TODO(), newdda)
	}

	return nil
}

// configureService fills the configuration needed to start the rc service
func (r *RemoteConfigUpdater) configureService(apiKey, site, clusterName, rcDirectorRoot, rcConfigRoot, rcEndpoint string) error {
	cfg := model.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))

	cfg.SetWithoutSource("api_key", apiKey)
	cfg.SetWithoutSource("remote_configuration.config_root", rcConfigRoot)
	cfg.SetWithoutSource("remote_configuration.director_root", rcDirectorRoot)
	hostname, _ := os.Hostname()

	baseRawURL := getRemoteConfigEndpoint(rcEndpoint, remoteConfigUrlPrefix, site)

	// TODO change to a different dir
	baseDir := filepath.Join(os.TempDir(), "datadog-operator")
	if err := os.MkdirAll(baseDir, 0777); err != nil {
		return err
	}

	// TODO decide what to put for version, since NewService is expecting agentVersion (even "1.50.0" for operator doesn't work)
	serviceConf := RcServiceConfiguration{
		cfg:               cfg,
		apiKey:            apiKey,
		baseRawURL:        baseRawURL,
		hostname:          hostname,
		clusterName:       clusterName,
		telemetryReporter: dummyRcTelemetryReporter{},
		agentVersion:      "7.50.0",
		// agentVersion:  version.Version,
		rcDatabaseDir: baseDir,
	}
	r.serviceConf = serviceConf
	return nil
}

// getRemoteConfigEndpoint returns the main DD URL defined in the config, based on `site` and the prefix
func getRemoteConfigEndpoint(rcEndpoint, prefix, site string) string {
	if rcEndpoint != "" {
		return rcEndpoint
	}
	if site != "" {
		return prefix + strings.TrimSpace(site)
	}
	return prefix + defaultSite
}

func NewRemoteConfigUpdater(client kubeclient.Client, logger logr.Logger) *RemoteConfigUpdater {
	return &RemoteConfigUpdater{
		kubeClient: client,
		logger:     logger.WithName("remote-config"),
	}
}
