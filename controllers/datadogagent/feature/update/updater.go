package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/config/remote/service"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component"
	"github.com/DataDog/datadog-operator/pkg/version"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	kubeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const pollInterval = 5 * time.Second

const (
	defaultSite = "datadoghq.com"
)

var updateMockConfig = ""

type Updater struct {
	client kubeclient.Client
}

type FeatureActivationConfig struct {
	CWS FeatureConfig `json:"CWS"`
	NPM FeatureConfig `json:"NPM"`
}

type FeatureConfig struct {
	Enabled bool `json:"enabled"`
}

func getResolvedDDUrl(c model.Reader, urlKey string) string {
	resolvedDDURL := c.GetString(urlKey)
	if c.IsSet("site") {
		log.Infof("'site' and '%s' are both set in config: setting main endpoint to '%s': \"%s\"", urlKey, urlKey, c.GetString(urlKey))
	}
	return resolvedDDURL
}

// getMainEndpoint returns the main DD URL defined in the config, based on `site` and the prefix, or ddURLKey
func getMainEndpoint(c model.Reader, prefix string, ddURLKey string) string {
	// value under ddURLKey takes precedence over 'site'
	if c.IsSet(ddURLKey) && c.GetString(ddURLKey) != "" {
		return getResolvedDDUrl(c, ddURLKey)
	} else if c.GetString("site") != "" {
		return prefix + strings.TrimSpace(c.GetString("site"))
	}
	return prefix + defaultSite
}

func (u *Updater) Start(log logr.Logger) error {
	log.Info("Listening for remote configuration updates")

	cfg := model.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	cfg.BindEnvAndSetDefault("site", "datadoghq.com")
	cfg.BindEnv("api_key")
	cfg.BindEnv("app_key")
	cfg.BindEnv("remote_configuration.config_root")
	cfg.BindEnv("remote_configuration.director_root")
	cfg.BindEnv("remote_configuration.refresh_interval")

	//cfg.AutomaticEnv()
	apiKey := cfg.GetString("api_key")
	hostname, _ := os.Hostname()
	baseRawURL := getMainEndpoint(cfg, "https://config.", "remote_configuration.rc_dd_url")
	rcService, err := service.NewService(cfg, apiKey, baseRawURL, hostname, nil, version.Version)
	if err != nil {
		log.Error(err, "Failed to create remote config service")
		return err
	}

	ctx := context.Background()

	rcClient, err := client.NewClient(rcService,
		client.WithAgent("datadog-operator", "9.9.9"),
		client.WithProducts([]data.Product{state.ProductTesting1}),
		client.WithDirectorRootOverride(cfg.GetString("remote_configuration.director_root")),
	)
	if err != nil {
		log.Error(err, "Failed to create remote config client")
		return err
	}

	rcService.Start(ctx)

	rcClient.Start()

	go func() {
		// defer rcService.Stop()
		time.Sleep(5 * time.Second)

		rcClient.Subscribe(string(state.ProductTesting1), func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
			if updateMockConfig != "" {
				if err := json.Unmarshal([]byte(updateMockConfig), &update); err != nil {
					log.Error(err, "invalid mocked config")
				}
			}

			if len(update) == 0 {
				return
			}

			var cfg FeatureActivationConfig
			for _, update := range update {
				log.Info("Content: %s", string(update.Config))
				if err := json.Unmarshal(update.Config, &cfg); err != nil {
					log.Error(err, "failed to apply config %s", update.Metadata.ID)
					continue
				}
			}

			ddAgents, err := u.getUpdatableAgents(ctx)
			if err != nil {
				log.Error(err, "failed to get updatable agents")
			}

			u.applyConfiguration(ctx, ddAgents, cfg)
		})
	}()

	return nil
}

func (u *Updater) getUpdatableAgents(ctx context.Context) ([]v2alpha1.DatadogAgent, error) {
	userNamespace := "default"

	ddList := &v2alpha1.DatadogAgentList{}
	if err := u.client.List(context.TODO(), ddList, &kubeclient.ListOptions{Namespace: userNamespace}); err != nil {
		return nil, fmt.Errorf("unable to list DatadogAgent: %w", err)
	}
	if len(ddList.Items) == 0 {
		return nil, errors.New("cannot find any DatadogAgent")
	}

	return ddList.Items, nil
}

func (u *Updater) getDaemonset(dda v2alpha1.DatadogAgent) (*appsv1.DaemonSet, error) {
	nsName := types.NamespacedName{
		Name:      component.GetAgentName(&dda),
		Namespace: dda.GetNamespace(),
	}

	currentDaemonset := &appsv1.DaemonSet{}
	if err := u.client.Get(context.TODO(), nsName, currentDaemonset); err != nil {
		return nil, err
	}

	return currentDaemonset, nil
}

func (u *Updater) enableFeature(dd v2alpha1.DatadogAgent, cfg FeatureActivationConfig) error {
	newDD := dd.DeepCopy()

	if newDD.Spec.Features == nil {
		newDD.Spec.Features = &v2alpha1.DatadogFeatures{}
	}

	if newDD.Spec.Features.CWS == nil {
		newDD.Spec.Features.CWS = &v2alpha1.CWSFeatureConfig{}
	}

	if newDD.Spec.Features.CWS.Enabled == nil {
		newDD.Spec.Features.CWS.Enabled = new(bool)
	}

	*newDD.Spec.Features.CWS.Enabled = cfg.CWS.Enabled

	if !apiutils.IsEqualStruct(dd.Spec, newDD.Spec) {
		return u.client.Update(context.TODO(), newDD)
	}

	return nil
}

func (u *Updater) applyConfiguration(ctx context.Context, ddAgents []v2alpha1.DatadogAgent, cfg FeatureActivationConfig) error {
	for _, dd := range ddAgents {
		if err := u.enableFeature(dd, cfg); err != nil {
			return err
		}
	}

	return nil
}

func NewUpdater(client kubeclient.Client) *Updater {
	return &Updater{
		client: client,
	}
}
