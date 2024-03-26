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
	client kubeclient.Client
	logger logr.Logger
}

// DatadogAgentRemoteConfig contains the struct used to update DatadogAgent object from RemoteConfig
type DatadogAgentRemoteConfig struct {
	Features *FeaturesConfig `json:"features"`
}

type FeaturesConfig struct {
	CWS *FeatureEnabledConfig `json:"CWS"`
}

type FeatureEnabledConfig struct {
	Enabled *bool `json:"enabled"`
}

// TODO replace
type dummyRcTelemetryReporter struct{}

func (d dummyRcTelemetryReporter) IncRateLimit() {}
func (d dummyRcTelemetryReporter) IncTimeout()   {}

func (r *RemoteConfigUpdater) Setup(creds config.Creds) error {
	r.logger.Info("Starting Remote Configuration client and service")

	if creds.APIKey == "" || creds.AppKey == "" {
		return errors.New("error obtaining API key and/or app key")
	}

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

	drct := dummyRcTelemetryReporter{}
	// TODO decide what to put for version, since NewService is expecting agentVersion (even "1.50.0" for operator doesn't work)
	rcService, err := service.NewService(cfg, creds.APIKey, baseRawURL, hostname, nil, drct, "7.50.0", service.WithDatabaseFileName(filepath.Join(baseDir, "remote-config.db")))

	if err != nil {
		r.logger.Error(err, "Failed to create Remote Configuration service")
		return err
	}

	rcClient, err := client.NewClient(rcService,
		// TODO update version
		client.WithAgent("datadog-operator", "9.9.9"),
		client.WithProducts(state.ProductAgentConfig),
		client.WithDirectorRootOverride(cfg.GetString("remote_configuration.director_root")),
		client.WithPollInterval(5*time.Second),
	)
	if err != nil {
		r.logger.Error(err, "Failed to create Remote Configuration client")
		return err
	}

	ctx := context.Background()

	rcService.Start()
	// defer rcService.Stop()

	rcClient.Subscribe(string(state.ProductAgentConfig), func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
		r.logger.Info("Subscribe is called")

		// ---------- Section to use when mocking config ----------
		// Comment out this section when testing remote config updates
		mockFeatureConfig := `{"features":{"cws":{"enabled":true}}}` //`{"some":"json"}`

		mockMetadata := state.Metadata{
			Product:   "testProduct",
			ID:        "testID",
			Name:      "testName",
			Version:   9,
			RawLength: 20,
		}
		mockRawConfig := state.RawConfig{
			Config:   []byte(mockFeatureConfig),
			Metadata: mockMetadata,
		}
		var mockUpdate = make(map[string]state.RawConfig)
		mockUpdate["testConfigPath"] = mockRawConfig

		// r.logger.Info(string(mockUpdate["testConfigPath"].Config))

		update = mockUpdate
		// ---------- End section to use when mocking config ----------

		// TODO
		// For now, only single default config path is present (key of update[key])
		tempstring := ""
		for k := range update {
			tempstring += k
		}

		r.logger.Info(tempstring)

		applyStateCallback(tempstring, state.ApplyStatus{State: state.ApplyStateUnacknowledged, Error: ""})

		if len(update) == 0 {
			return
		}

		var cfg DatadogAgentRemoteConfig
		for _, update := range update {
			r.logger.Info("Content", "update.Config", string(update.Config))
			if err := json.Unmarshal(update.Config, &cfg); err != nil {
				r.logger.Error(err, "failed to marshal config", "updateMetadata.ID", update.Metadata.ID)
				return
			}
		}

		dda, err := r.getDatadogAgentInstance(ctx)
		if err != nil {
			r.logger.Error(err, "failed to get updatable agents")
		}

		if err := r.applyConfig(ctx, dda, cfg); err != nil {
			r.logger.Error(err, "failed to apply config")
			applyStateCallback(tempstring, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			return
		}

		r.logger.Info("Successfully applied config!")

		applyStateCallback(tempstring, state.ApplyStatus{State: state.ApplyStateAcknowledged, Error: ""})
	})

	rcClient.Start()

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

func (r *RemoteConfigUpdater) getDatadogAgentInstance(ctx context.Context) (v2alpha1.DatadogAgent, error) {
	ddaList := &v2alpha1.DatadogAgentList{}
	if err := r.client.List(context.TODO(), ddaList); err != nil {
		return v2alpha1.DatadogAgent{}, fmt.Errorf("unable to list DatadogAgents: %w", err)
	}

	if len(ddaList.Items) == 0 {
		return v2alpha1.DatadogAgent{}, errors.New("cannot find any DatadogAgent")
	}

	// Return first DatadogAgent as only one is supported
	return ddaList.Items[0], nil
}

func (r *RemoteConfigUpdater) updateInstance(dda v2alpha1.DatadogAgent, cfg DatadogAgentRemoteConfig) error {

	if cfg.Features == nil {
		return nil
	}

	if cfg.Features.CWS == nil {
		return nil
	}

	if cfg.Features.CWS.Enabled == nil {
		return nil
	}

	newdda := dda.DeepCopy()

	if newdda.Spec.Features == nil {
		newdda.Spec.Features = &v2alpha1.DatadogFeatures{}
	}

	if newdda.Spec.Features.CWS == nil {
		newdda.Spec.Features.CWS = &v2alpha1.CWSFeatureConfig{}
	}

	if newdda.Spec.Features.CWS.Enabled == nil {
		newdda.Spec.Features.CWS.Enabled = new(bool)
	}

	newdda.Spec.Features.CWS.Enabled = cfg.Features.CWS.Enabled

	if !apiutils.IsEqualStruct(dda.Spec, newdda.Spec) {
		return r.client.Update(context.TODO(), newdda)
	}

	return nil
}

func (r *RemoteConfigUpdater) applyConfig(ctx context.Context, dda v2alpha1.DatadogAgent, cfg DatadogAgentRemoteConfig) error {
	if err := r.updateInstance(dda, cfg); err != nil {
		return err
	}

	return nil
}

func NewRemoteConfigUpdater(client kubeclient.Client, logger logr.Logger) *RemoteConfigUpdater {
	return &RemoteConfigUpdater{
		client: client,
		logger: logger,
	}
}
