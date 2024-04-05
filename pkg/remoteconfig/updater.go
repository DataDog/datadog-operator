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
	"reflect"
	"strings"
	"time"

	"github.com/go-logr/logr"
	kubeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/config/remote/service"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/pkg/config"
	// "github.com/DataDog/datadog-operator/pkg/version"
)

const (
	defaultSite  = "datadoghq.com"
	pollInterval = 10 * time.Second
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
	tags              []string
	telemetryReporter service.RcTelemetryReporter
	agentVersion      string
	rcDatabaseDir     string
}

// DatadogAgentRemoteConfig contains the struct used to update DatadogAgent object from RemoteConfig
type DatadogAgentRemoteConfig struct {
	Name     string          `json:"id"`
	Features *FeaturesConfig `json:"core_agent"`
}

type FeaturesConfig struct {
	CWS  *FeatureEnabledConfig `json:"runtime_security_config"`
	CSPM *FeatureEnabledConfig `json:"compliance_config"`
	SBOM *SbomConfig           `json:"sbom"`
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

func (r *RemoteConfigUpdater) Setup(creds config.Creds) error {
	r.logger.Info("Setting up Remote Configuration client and service")

	if creds.APIKey == "" || creds.AppKey == "" {
		return errors.New("error obtaining API key and/or app key")
	}

	// Fill in rc service configuration
	err := r.configureService(creds)
	if err != nil {
		r.logger.Error(err, "Failed to configure Remote Configuration service")
		return err
	}

	rcService, err := service.NewService(
		r.serviceConf.cfg,
		r.serviceConf.apiKey,
		r.serviceConf.baseRawURL,
		r.serviceConf.hostname,
		r.serviceConf.tags,
		r.serviceConf.telemetryReporter,
		r.serviceConf.agentVersion,
		service.WithDatabaseFileName(filepath.Join(r.serviceConf.rcDatabaseDir, "remote-config.db")))

	if err != nil {
		r.logger.Error(err, "Failed to create Remote Configuration service")
		return err
	}
	r.rcService = rcService

	rcClient, err := client.NewClient(
		rcService,
		client.WithAgent("datadog-operator", "9.9.9"),
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

	// defer rcService.Stop()

	rcClient.Start()
	r.logger.Info("rcClient started")

	rcClient.Subscribe(string(state.ProductAgentConfig), r.agentConfigUpdateCallback)

	return nil
}

func (r *RemoteConfigUpdater) agentConfigUpdateCallback(updates map[string]state.RawConfig, applyStatus func(string, state.ApplyStatus)) {

	ctx := context.Background()

	r.logger.Info("agentConfigUpdateCallback is called")
	r.logger.Info("Received", "updates", updates)
	mergedUpdate, err := r.parseReceivedUpdates(updates, applyStatus)
	if err != nil {
		r.logger.Error(err, "failed to merge updates")
		return
	}
	r.logger.Info("Merged", "update", mergedUpdate)

	tempstring := ""
	for k := range updates {
		tempstring += k
	}

	r.logger.Info(tempstring)

	applyStatus(tempstring, state.ApplyStatus{State: state.ApplyStateUnacknowledged, Error: ""})

	if len(updates) == 0 {
		return
	}

	dda, err := r.getDatadogAgentInstance(ctx)
	if err != nil {
		r.logger.Error(err, "failed to get updatable agents")
	}

	if err := r.applyConfig(ctx, dda, mergedUpdate); err != nil {
		r.logger.Error(err, "failed to apply config")
		applyStatus(tempstring, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
		return
	}

	r.logger.Info("Successfully applied config!")

	applyStatus(tempstring, state.ApplyStatus{State: state.ApplyStateAcknowledged, Error: ""})

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
		configMap[config.Name] = config
	}

	for _, name := range order.Order {
		if config, found := configMap[name]; found {
			mergeStructs(finalConfig, config)
		}
	}
	return finalConfig, nil
}

func mergeStructs(dst, src interface{}) {
	dstVal := reflect.ValueOf(dst).Elem()
	srcVal := reflect.ValueOf(src).Elem()

	for i := 0; i < srcVal.NumField(); i++ {
		srcField := srcVal.Field(i)
		dstField := dstVal.Field(i)

		// Check if the source field is non-zero and if the destination field can be set
		if srcField.IsValid() && dstField.CanSet() {
			if !reflect.DeepEqual(srcField.Interface(), reflect.Zero(srcField.Type()).Interface()) {
				dstField.Set(srcField)
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

	if cfg.Features == nil {
		return nil
	}

	newdda := dda.DeepCopy()
	if newdda.Spec.Features == nil {
		newdda.Spec.Features = &v2alpha1.DatadogFeatures{}
	}

	// CWS
	if cfg.Features.CWS != nil {
		if newdda.Spec.Features.CWS == nil {
			newdda.Spec.Features.CWS = &v2alpha1.CWSFeatureConfig{}
		}
		if newdda.Spec.Features.CWS.Enabled == nil {
			newdda.Spec.Features.CWS.Enabled = new(bool)
		}
		newdda.Spec.Features.CWS.Enabled = cfg.Features.CWS.Enabled
	}

	// CSPM
	if cfg.Features.CSPM != nil {
		if newdda.Spec.Features.CSPM == nil {
			newdda.Spec.Features.CSPM = &v2alpha1.CSPMFeatureConfig{}
		}
		if newdda.Spec.Features.CSPM.Enabled == nil {
			newdda.Spec.Features.CSPM.Enabled = new(bool)
		}
		newdda.Spec.Features.CSPM.Enabled = cfg.Features.CSPM.Enabled
	}

	// SBOM
	if cfg.Features.SBOM != nil {
		if newdda.Spec.Features.SBOM == nil {
			newdda.Spec.Features.SBOM = &v2alpha1.SBOMFeatureConfig{}
		}
		if newdda.Spec.Features.SBOM.Enabled == nil {
			newdda.Spec.Features.SBOM.Enabled = new(bool)
		}
		newdda.Spec.Features.SBOM.Enabled = cfg.Features.SBOM.Enabled

		// SBOM HOST
		if cfg.Features.SBOM.Host != nil {
			if newdda.Spec.Features.SBOM.Host == nil {
				newdda.Spec.Features.SBOM.Host = &v2alpha1.SBOMTypeConfig{}
			}
			if newdda.Spec.Features.SBOM.Host.Enabled == nil {
				newdda.Spec.Features.SBOM.Host.Enabled = new(bool)
			}
			newdda.Spec.Features.SBOM.Host.Enabled = cfg.Features.SBOM.Host.Enabled
		}

		// SBOM CONTAINER IMAGE
		if cfg.Features.SBOM.ContainerImage != nil {
			if newdda.Spec.Features.SBOM.ContainerImage == nil {
				newdda.Spec.Features.SBOM.ContainerImage = &v2alpha1.SBOMTypeConfig{}
			}
			if newdda.Spec.Features.SBOM.ContainerImage.Enabled == nil {
				newdda.Spec.Features.SBOM.ContainerImage.Enabled = new(bool)
			}
			newdda.Spec.Features.SBOM.ContainerImage.Enabled = cfg.Features.SBOM.ContainerImage.Enabled
		}

	}

	if !apiutils.IsEqualStruct(dda.Spec, newdda.Spec) {
		return r.kubeClient.Update(context.TODO(), newdda)
	}

	return nil
}

// configureService fills the configuration needed to start the rc service
func (r *RemoteConfigUpdater) configureService(creds config.Creds) error {
	ctx := context.TODO()

	// Create config required for RC service
	cfg := model.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	cfg.Set("api_key", creds.APIKey, model.SourceDefault)
	cfg.Set("app_key", creds.AppKey, model.SourceDefault)

	// Read and bind envvars (prefixed with DD_)
	cfg.BindEnvAndSetDefault("site", "datadoghq.com")
	cfg.BindEnvAndSetDefault("remote_configuration.config_root", "")
	cfg.BindEnvAndSetDefault("remote_configuration.director_root", "")
	cfg.BindEnv("remote_configuration.refresh_interval")

	hostname, _ := os.Hostname()
	baseRawURL := getRemoteConfigEndpoint(cfg, "https://config.", "remote_configuration.rc_dd_url")

	// TODO change to a different dir
	baseDir := filepath.Join(os.TempDir(), "datadog-operator")
	if err := os.MkdirAll(baseDir, 0777); err != nil {
		return err
	}

	dda, err := r.getDatadogAgentInstance(ctx)
	if err != nil {
		r.logger.Error(err, "failed to get dda")
	}

	var clusterName string

	if dda.Spec.Global != nil && dda.Spec.Global.ClusterName != nil {
		clusterName = *dda.Spec.Global.ClusterName
	}
	r.logger.Info("My", "clusterName", clusterName)

	// TODO decide what to put for version, since NewService is expecting agentVersion (even "1.50.0" for operator doesn't work)
	serviceConf := RcServiceConfiguration{
		cfg:               cfg,
		apiKey:            creds.APIKey,
		baseRawURL:        baseRawURL,
		hostname:          hostname,
		tags:              []string{"uniqueEnough:test-rc-operator-49"},
		telemetryReporter: dummyRcTelemetryReporter{},
		agentVersion:      "7.50.0",
		// agentVersion:  version.Version,
		rcDatabaseDir: baseDir,
	}
	r.serviceConf = serviceConf
	return nil
}

// getRemoteConfigEndpoint returns the main DD URL defined in the config, based on `site` and the prefix, or ddURLKey
func getRemoteConfigEndpoint(c model.Reader, prefix string, ddURLKey string) string {
	// Value under ddURLKey takes precedence over 'site'
	if c.IsSet(ddURLKey) && c.GetString(ddURLKey) != "" {
		return getResolvedDDUrl(c, ddURLKey)
	} else if c.GetString("site") != "" {
		return prefix + strings.TrimSpace(c.GetString("site"))
	}
	return prefix + defaultSite
}

func getResolvedDDUrl(c model.Reader, urlKey string) string {
	resolvedDDURL := c.GetString(urlKey)
	if c.IsSet("site") {
		log.Infof("'site' and '%s' are both set in config: setting main endpoint to '%s': \"%s\"", urlKey, urlKey, c.GetString(urlKey))
	}
	return resolvedDDURL
}

func NewRemoteConfigUpdater(client kubeclient.Client, logger logr.Logger) *RemoteConfigUpdater {
	return &RemoteConfigUpdater{
		kubeClient: client,
		logger:     logger,
	}
}
