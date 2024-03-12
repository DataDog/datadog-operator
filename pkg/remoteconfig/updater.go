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
	"github.com/DataDog/datadog-operator/pkg/version"
)

const (
	defaultSite  = "datadoghq.com"
	pollInterval = 10 * time.Second
)

var updateMockConfig = ""

type RemoteConfigUpdater struct {
	client kubeclient.Client
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

func (r *RemoteConfigUpdater) Setup(log logr.Logger, creds config.Creds) error {
	log.Info("Starting Remote Configuration client and service")

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
	// TODO decide what to put for version, since NewService is expecting agentVersion
	rcService, err := service.NewService(cfg, creds.APIKey, baseRawURL, hostname, nil, drct, version.Version, service.WithDatabaseFileName(filepath.Join(baseDir, "remote-config.db")))

	if err != nil {
		log.Error(err, "Failed to create Remote Configuration service")
		return err
	}

	rcClient, err := client.NewClient(rcService,
		client.WithAgent("datadog-operator", "9.9.9"),
		client.WithProducts(state.ProductTesting1),
		client.WithDirectorRootOverride(cfg.GetString("remote_configuration.director_root")),
		client.WithPollInterval(5*time.Second),
	)
	if err != nil {
		log.Error(err, "Failed to create Remote Configuration client")
		return err
	}

	ctx := context.Background()

	rcService.Start()
	defer rcService.Stop()

	rcClient.Start()

	go func() {
		// TODO
		rcClient.Subscribe(string(state.ProductTesting1), func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
			if updateMockConfig != "" {
				if err := json.Unmarshal([]byte(updateMockConfig), &update); err != nil {
					log.Error(err, "invalid mocked config")
				}
			}

			if len(update) == 0 {
				return
			}

			var cfg DatadogAgentRemoteConfig
			for _, update := range update {
				log.Info("Content: %s", string(update.Config))
				if err := json.Unmarshal(update.Config, &cfg); err != nil {
					log.Error(err, "failed to apply config %s", update.Metadata.ID)
					continue
				}
			}

			dda, err := r.getDatadogAgentInstance(ctx)
			if err != nil {
				log.Error(err, "failed to get updatable agents")
			}

			r.applyConfig(ctx, dda, cfg)
		})
	}()
	// TODO
	time.Sleep(pollInterval)

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
	userNamespace := "default"

	ddaList := &v2alpha1.DatadogAgentList{}
	if err := r.client.List(context.TODO(), ddaList, &kubeclient.ListOptions{Namespace: userNamespace}); err != nil {
		return v2alpha1.DatadogAgent{}, fmt.Errorf("unable to list DatadogAgents: %w", err)
	}

	if len(ddaList.Items) == 0 {
		return v2alpha1.DatadogAgent{}, errors.New("cannot find any DatadogAgent")
	}

	// Return first DatadogAgent as only one is supported
	return ddaList.Items[0], nil
}

func (r *RemoteConfigUpdater) updateInstance(dda v2alpha1.DatadogAgent, cfg DatadogAgentRemoteConfig) error {

	// Return if cfg.Features.CWS.Enabled is not defined
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

func NewRemoteConfigUpdater(client kubeclient.Client) *RemoteConfigUpdater {
	return &RemoteConfigUpdater{
		client: client,
	}
}
