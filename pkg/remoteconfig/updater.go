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
	kubeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/config/remote/service"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-operator/apis/datadoghq/common"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/pkg/config"
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
type dummyTelemetryReporter struct{}

func (d dummyTelemetryReporter) IncRateLimit() {}
func (d dummyTelemetryReporter) IncTimeout()   {}

func (r *RemoteConfigUpdater) Setup(creds config.Creds) error {
	apiKey := creds.APIKey
	if apiKey == "" {
		return errors.New("error obtaining API key")
	}

	site := os.Getenv(common.DDSite) // TODO support DD_URL as well
	clusterName := os.Getenv(common.DDClusterName)
	directorRoot := os.Getenv("DD_REMOTE_CONFIGURATION_DIRECTOR_ROOT")
	configRoot := os.Getenv("DD_REMOTE_CONFIGURATION_CONFIG_ROOT")
	endpoint := os.Getenv("DD_REMOTE_CONFIGURATION_RC_DD_URL")

	if r.rcClient == nil && r.rcService == nil {
		// If rcClient && rcService not setup yet
		err := r.Start(apiKey, site, clusterName, directorRoot, configRoot, endpoint)
		if err != nil {
			return err
		}
	}

	return nil

}

func (r *RemoteConfigUpdater) Start(apiKey string, site string, clusterName string, directorRoot string, configRoot string, endpoint string) error {

	r.logger.Info("Starting Remote Configuration client and service")

	// Fill in rc service configuration
	err := r.configureService(apiKey, site, clusterName, directorRoot, configRoot, endpoint)
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

// configureService fills the configuration needed to start the rc service
func (r *RemoteConfigUpdater) configureService(apiKey, site, clusterName, directorRoot, configRoot, endpoint string) error {
	cfg := model.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))

	cfg.SetWithoutSource("api_key", apiKey)
	cfg.SetWithoutSource("remote_configuration.config_root", configRoot)
	cfg.SetWithoutSource("remote_configuration.director_root", directorRoot)
	hostname, _ := os.Hostname()

	if endpoint == "" {
		endpoint = getEndpoint(remoteConfigUrlPrefix, site)
	}

	// TODO change to a different dir
	baseDir := filepath.Join(os.TempDir(), "datadog-operator")
	if err := os.MkdirAll(baseDir, 0777); err != nil {
		return err
	}

	// TODO decide what to put for version, since NewService is expecting agentVersion (even "1.50.0" for operator doesn't work)
	serviceConf := RcServiceConfiguration{
		cfg:               cfg,
		apiKey:            apiKey,
		baseRawURL:        endpoint,
		hostname:          hostname,
		clusterName:       clusterName,
		telemetryReporter: dummyTelemetryReporter{},
		agentVersion:      "7.50.0",
		rcDatabaseDir:     baseDir,
	}
	r.serviceConf = serviceConf
	return nil
}

// getEndpoint returns the Remote Config endpoint, based on `site` and the prefix
func getEndpoint(prefix, site string) string {
	if site != "" {
		return prefix + strings.TrimSpace(site)
	}
	return prefix + defaultSite
}

func (r *RemoteConfigUpdater) agentConfigUpdateCallback(updates map[string]state.RawConfig, applyStatus func(string, state.ApplyStatus)) {

	ctx := context.Background()

	// TODO rm
	r.logger.Info("agentConfigUpdateCallback is called")

	// Tell rc that we have received the configurations
	var configIDs []string
	for id := range updates {
		applyStatus(id, state.ApplyStatus{State: state.ApplyStateUnacknowledged, Error: ""})
		configIDs = append(configIDs, id)
	}

	mergedUpdate, err := r.parseReceivedUpdates(updates, applyStatus)
	if err != nil {
		r.logger.Error(err, "Failed to merge updates")
		return
	}
	r.logger.Info("Merged", "update", mergedUpdate)

	dda, err := r.getDatadogAgentInstance(ctx)
	if err != nil {
		r.logger.Error(err, "Failed to get updatable agents")
		return
	}

	if err := r.applyConfig(ctx, dda, mergedUpdate); err != nil {
		r.logger.Error(err, "Failed to apply config")
		applyStatus(configIDs[len(configIDs)-1], state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
		return
	}

	// Tell rc that we have received the configurations
	for _, id := range configIDs {
		applyStatus(id, state.ApplyStatus{State: state.ApplyStateAcknowledged, Error: ""})
	}

	r.logger.Info("Successfully applied config")
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

func NewRemoteConfigUpdater(client kubeclient.Client, logger logr.Logger) *RemoteConfigUpdater {
	return &RemoteConfigUpdater{
		kubeClient: client,
		logger:     logger,
	}
}
