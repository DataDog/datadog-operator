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
	"sync"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kubeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/config/remote/service"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
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
	rcService   *service.CoreAgentService
	serviceConf RcServiceConfiguration
	logger      logr.Logger
	mu          sync.RWMutex
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

// DatadogProductRemoteConfig  is an interface for Datadog product remote configuration
type DatadogProductRemoteConfig interface {
	// GetID returns the ID of the configuration
	GetID() string
}

// DatadogAgentRemoteConfig contains the struct used to update DatadogAgent object from RemoteConfig
type DatadogAgentRemoteConfig struct {
	ID            string                       `json:"id,omitempty"`
	Name          string                       `json:"name,omitempty"`
	CoreAgent     *CoreAgentFeaturesConfig     `json:"config,omitempty"`
	SystemProbe   *SystemProbeFeaturesConfig   `json:"system_probe,omitempty"`
	SecurityAgent *SecurityAgentFeaturesConfig `json:"security_agent,omitempty"`
}

// GetID returns the ID of the configuration
func (d DatadogAgentRemoteConfig) GetID() string {
	return d.ID
}

// OrchestratorK8sCRDRemoteConfig contains the struct used to update OrchestratorK8sCRD object from RemoteConfig
type OrchestratorK8sCRDRemoteConfig struct {
	ID   string                        `json:"id,omitempty"`
	Name string                        `json:"name,omitempty"`
	CRDs *CustomResourceDefinitionURLs `json:"crds,omitempty"`
}

// GetID returns the ID of the configuration
func (d OrchestratorK8sCRDRemoteConfig) GetID() string {
	return d.ID
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
		// Setup rcClient and rcService
		err := r.Start(apiKey, site, clusterName, directorRoot, configRoot, endpoint)
		if err != nil {
			return err
		}
	}

	return nil

}

func (r *RemoteConfigUpdater) Start(apiKey string, site string, clusterName string, directorRoot string, configRoot string, endpoint string) error {

	r.logger.Info("Starting Remote Configuration client and service")

	err := r.configureService(apiKey, site, clusterName, directorRoot, configRoot, endpoint)
	if err != nil {
		r.logger.Error(err, "Failed to configure Remote Configuration service")
		return err
	}

	rcService, err := service.NewService(
		r.serviceConf.cfg,
		"",
		r.serviceConf.baseRawURL,
		r.serviceConf.hostname,
		func() []string { return []string{"cluster_name:" + r.serviceConf.clusterName} },
		r.serviceConf.telemetryReporter,
		r.serviceConf.agentVersion,
		service.WithAPIKey(apiKey),
		service.WithDatabaseFileName(filepath.Join(r.serviceConf.rcDatabaseDir, fmt.Sprintf("remote-config-%s.db", uuid.New()))),
		service.WithDirectorRootOverride(r.serviceConf.cfg.GetString("site"), r.serviceConf.cfg.GetString("remote_configuration.director_root")),
		service.WithConfigRootOverride(r.serviceConf.cfg.GetString("site"), r.serviceConf.cfg.GetString("remote_configuration.config_root")),
	)
	if err != nil {
		r.logger.Error(err, "Failed to create Remote Configuration service")
		return err
	}
	r.rcService = rcService

	rcClient, err := client.NewClient(
		rcService,
		client.WithAgent("datadog-operator", version.Version),
		client.WithProducts(state.ProductAgentConfig, state.ProductOrchestratorK8sCRDs),
		client.WithDirectorRootOverride(r.serviceConf.cfg.GetString("site"), r.serviceConf.cfg.GetString("remote_configuration.director_root")),
		client.WithPollInterval(pollInterval),
	)
	if err != nil {
		r.logger.Error(err, "Failed to create Remote Configuration client")
		return err
	}
	r.rcClient = rcClient

	rcService.Start()
	r.logger.Info("Remote Configuration service started")

	rcClient.Start()
	r.logger.Info("Remote Configuration client started")

	rcClient.Subscribe(string(state.ProductAgentConfig), r.agentConfigUpdateCallback)

	rcClient.Subscribe(string(state.ProductOrchestratorK8sCRDs), r.crdConfigUpdateCallback)

	return nil
}

// configureService fills the configuration needed to start the rc service
func (r *RemoteConfigUpdater) configureService(apiKey, site, clusterName, directorRoot, configRoot, endpoint string) error {
	cfg := model.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))

	cfg.SetWithoutSource("api_key", apiKey)
	cfg.SetWithoutSource("site", site)
	cfg.SetWithoutSource("remote_configuration.config_root", configRoot)
	cfg.SetWithoutSource("remote_configuration.director_root", directorRoot)
	hostname, _ := os.Hostname()

	if endpoint == "" {
		endpoint = getEndpoint(remoteConfigUrlPrefix, site)
	}

	// TODO consider different dir
	baseDir := filepath.Join(os.TempDir(), "datadog-operator")
	if err := os.MkdirAll(baseDir, 0777); err != nil {
		return err
	}

	serviceConf := RcServiceConfiguration{
		cfg:               cfg,
		apiKey:            apiKey,
		baseRawURL:        endpoint,
		hostname:          hostname,
		clusterName:       clusterName,
		telemetryReporter: dummyTelemetryReporter{},
		// TODO fix when other values accepted
		agentVersion:  "7.50.0",
		rcDatabaseDir: baseDir,
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

// getAndUpdateDatadogAgent is used to prevent race conditions when updating the DDA's status
// we do not want to modify the status without using this function or we could have conflicts
func (r *RemoteConfigUpdater) getAndUpdateDatadogAgent(ctx context.Context, cfg DatadogProductRemoteConfig, f func(v2alpha1.DatadogAgent, DatadogProductRemoteConfig) error) error {
	// Only one instance of this can run at a time
	r.mu.Lock()
	defer r.mu.Unlock()

	ddaList := &v2alpha1.DatadogAgentList{}
	if err := r.kubeClient.List(context.TODO(), ddaList); err != nil {
		return fmt.Errorf("unable to list DatadogAgents: %w", err)
	}

	if len(ddaList.Items) == 0 {
		return errors.New("cannot find any DatadogAgent")
	}

	// Return first DatadogAgent as only one is supported
	dda := ddaList.Items[0]

	return f(dda, cfg)
}

func (r *RemoteConfigUpdater) getAndUpdateDatadogAgentWithRetry(ctx context.Context, cfg DatadogProductRemoteConfig, f func(v2alpha1.DatadogAgent, DatadogProductRemoteConfig) error) error {
	var err error

	operation := func() error {
		err = r.getAndUpdateDatadogAgent(ctx, cfg, f)
		if err != nil {
			r.logger.Error(err, "Failed to get and patch agents, retrying...")
			return err
		}
		return nil
	}

	// Create a backoff strategy
	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = 1 * time.Second
	expBackoff.MaxInterval = 10 * time.Second
	expBackoff.MaxElapsedTime = 3 * time.Minute

	err = backoff.Retry(operation, expBackoff)
	if err != nil {
		r.logger.Error(err, "Failed to get and patch agents after retries")
		return err
	}
	return nil

}

func (r *RemoteConfigUpdater) agentConfigUpdateCallback(updates map[string]state.RawConfig, applyStatus func(string, state.ApplyStatus)) {

	ctx := context.Background()

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

	if mergedUpdate.ID == "" {
		r.logger.Info("No configuration updates received")
		// Continue through the function so that any existing configuration can be reset
	} else {
		r.logger.Info("Merged", "update", mergedUpdate)
	}

	if err := r.getAndUpdateDatadogAgentWithRetry(ctx, mergedUpdate, r.updateInstanceStatus); err != nil {
		r.logger.Error(err, "Failed to update status")
		applyStatus(configIDs[len(configIDs)-1], state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
		return
	}

	// Acknowledge that configs were received
	for _, id := range configIDs {
		applyStatus(id, state.ApplyStatus{State: state.ApplyStateAcknowledged, Error: ""})
	}

	r.logger.Info("Successfully applied configuration")
}

func (r *RemoteConfigUpdater) parseReceivedUpdates(updates map[string]state.RawConfig, applyStatus func(string, state.ApplyStatus)) (DatadogAgentRemoteConfig, error) {

	// Unmarshal configs and config order
	var order agentConfigOrder
	configByID := make(map[string]DatadogAgentRemoteConfig)
	for configPath, c := range updates {
		if c.Metadata.ID == "configuration_order" {
			if err := json.Unmarshal(c.Config, &order); err != nil {
				r.logger.Info("Error unmarshalling configuration_order:", "err", err)
				applyStatus(configPath, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
				return DatadogAgentRemoteConfig{}, fmt.Errorf("could not unmarshal configuration order")
			}
		} else {
			var configData DatadogAgentRemoteConfig
			if err := json.Unmarshal(c.Config, &configData); err != nil {
				applyStatus(configPath, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
				r.logger.Info("Error unmarshalling JSON:", "err", err)
				return DatadogAgentRemoteConfig{}, fmt.Errorf("could not unmarshal configuration %s", c.Metadata.ID)
			} else {
				configData.ID = c.Metadata.ID
				configByID[configData.ID] = configData
			}
		}
	}

	// Merge configs
	var finalConfig DatadogAgentRemoteConfig

	for _, id := range order.Order {
		if config, found := configByID[id]; found {
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

func (r *RemoteConfigUpdater) updateInstanceStatus(dda v2alpha1.DatadogAgent, config DatadogProductRemoteConfig) error {
	cfg, ok := config.(DatadogAgentRemoteConfig)
	if !ok {
		return fmt.Errorf("invalid config type: %T", config)
	}

	newddaStatus := dda.Status.DeepCopy()
	if newddaStatus.RemoteConfigConfiguration == nil {
		newddaStatus.RemoteConfigConfiguration = &v2alpha1.RemoteConfigConfiguration{}
	}

	if newddaStatus.RemoteConfigConfiguration.Features == nil {
		newddaStatus.RemoteConfigConfiguration.Features = &v2alpha1.DatadogFeatures{}
	}

	// CWS
	if cfg.SystemProbe != nil && cfg.SystemProbe.CWS != nil {
		if newddaStatus.RemoteConfigConfiguration.Features.CWS == nil {
			newddaStatus.RemoteConfigConfiguration.Features.CWS = &v2alpha1.CWSFeatureConfig{}
		}
		if newddaStatus.RemoteConfigConfiguration.Features.CWS.Enabled == nil {
			newddaStatus.RemoteConfigConfiguration.Features.CWS.Enabled = new(bool)
		}
		newddaStatus.RemoteConfigConfiguration.Features.CWS.Enabled = cfg.SystemProbe.CWS.Enabled
	} else {
		newddaStatus.RemoteConfigConfiguration.Features.CWS = nil
	}

	// CSPM
	if cfg.SecurityAgent != nil && cfg.SecurityAgent.CSPM != nil {
		if newddaStatus.RemoteConfigConfiguration.Features.CSPM == nil {
			newddaStatus.RemoteConfigConfiguration.Features.CSPM = &v2alpha1.CSPMFeatureConfig{}
		}
		if newddaStatus.RemoteConfigConfiguration.Features.CSPM.Enabled == nil {
			newddaStatus.RemoteConfigConfiguration.Features.CSPM.Enabled = new(bool)
		}
		newddaStatus.RemoteConfigConfiguration.Features.CSPM.Enabled = cfg.SecurityAgent.CSPM.Enabled
	} else {
		newddaStatus.RemoteConfigConfiguration.Features.CSPM = nil
	}

	// SBOM
	if cfg.CoreAgent != nil && cfg.CoreAgent.SBOM != nil {
		if newddaStatus.RemoteConfigConfiguration.Features.SBOM == nil {
			newddaStatus.RemoteConfigConfiguration.Features.SBOM = &v2alpha1.SBOMFeatureConfig{}
		}
		if newddaStatus.RemoteConfigConfiguration.Features.SBOM.Enabled == nil {
			newddaStatus.RemoteConfigConfiguration.Features.SBOM.Enabled = new(bool)
		}
		newddaStatus.RemoteConfigConfiguration.Features.SBOM.Enabled = cfg.CoreAgent.SBOM.Enabled

		// SBOM HOST
		if cfg.CoreAgent.SBOM.Host != nil {
			if newddaStatus.RemoteConfigConfiguration.Features.SBOM.Host == nil {
				newddaStatus.RemoteConfigConfiguration.Features.SBOM.Host = &v2alpha1.SBOMHostConfig{}
			}
			if newddaStatus.RemoteConfigConfiguration.Features.SBOM.Host.Enabled == nil {
				newddaStatus.RemoteConfigConfiguration.Features.SBOM.Host.Enabled = new(bool)
			}
			newddaStatus.RemoteConfigConfiguration.Features.SBOM.Host.Enabled = cfg.CoreAgent.SBOM.Host.Enabled
		} else {
			newddaStatus.RemoteConfigConfiguration.Features.SBOM.Host = nil
		}

		// SBOM CONTAINER IMAGE
		if cfg.CoreAgent.SBOM.ContainerImage != nil {
			if newddaStatus.RemoteConfigConfiguration.Features.SBOM.ContainerImage == nil {
				newddaStatus.RemoteConfigConfiguration.Features.SBOM.ContainerImage = &v2alpha1.SBOMContainerImageConfig{}
			}
			if newddaStatus.RemoteConfigConfiguration.Features.SBOM.ContainerImage.Enabled == nil {
				newddaStatus.RemoteConfigConfiguration.Features.SBOM.ContainerImage.Enabled = new(bool)
			}
			newddaStatus.RemoteConfigConfiguration.Features.SBOM.ContainerImage.Enabled = cfg.CoreAgent.SBOM.ContainerImage.Enabled
		} else {
			newddaStatus.RemoteConfigConfiguration.Features.SBOM.ContainerImage = nil
		}

	} else {
		newddaStatus.RemoteConfigConfiguration.Features.SBOM = nil
	}

	// USM
	if cfg.SystemProbe != nil && cfg.SystemProbe.USM != nil {
		if newddaStatus.RemoteConfigConfiguration.Features.USM == nil {
			newddaStatus.RemoteConfigConfiguration.Features.USM = &v2alpha1.USMFeatureConfig{}
		}
		if newddaStatus.RemoteConfigConfiguration.Features.USM.Enabled == nil {
			newddaStatus.RemoteConfigConfiguration.Features.USM.Enabled = new(bool)
		}
		newddaStatus.RemoteConfigConfiguration.Features.USM.Enabled = cfg.SystemProbe.USM.Enabled
	} else {
		newddaStatus.RemoteConfigConfiguration.Features.USM = nil
	}

	if !apiequality.Semantic.DeepEqual(&dda.Status, newddaStatus) {
		ddaUpdate := dda.DeepCopy()
		ddaUpdate.Status = *newddaStatus
		if err := r.kubeClient.Status().Update(context.TODO(), ddaUpdate); err != nil {
			if apierrors.IsConflict(err) {
				r.logger.Info("unable to update DatadogAgent status due to update conflict")
				return nil
			}
			r.logger.Error(err, "unable to update DatadogAgent status")
			return err
		}
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
