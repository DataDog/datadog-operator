// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

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
	"github.com/DataDog/datadog-operator/pkg/plugin/common"
	"github.com/go-logr/logr"

	kubeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const pollInterval = 5 * time.Second

// UpdateConfig describes a version update
type UpdateConfig struct {
	Agent UpdateVersion `json:"agent"`
}

// UpdateVersion describes a version candidate for update
type UpdateVersion struct {
	Version string `json:"version"`
	Image   string `json:"image"`
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

func (r *Reconciler) listenForRemoteConfiguration(log logr.Logger) error {
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
	rcService, err := service.NewService(cfg, apiKey, baseRawURL, hostname)
	if err != nil {
		log.Error(err, "Failed to create remote config service")
		return err
	}

	ctx := context.Background()

	rcService.Start(ctx)
	// defer rcService.Stop()

	rcClient, err := client.NewClient(rcService,
		client.WithAgent("datadog-operator", "9.9.9"),
		client.WithProducts([]data.Product{state.ProductDebug}),
		client.WithDirectorRootOverride(cfg.GetString("remote_configuration.director_root")),
	)
	if err != nil {
		log.Error(err, "Failed to create remote config client")
		return err
	}

	rcClient.Start()

	go func() {
		time.Sleep(5 * time.Second)

		rcClient.Subscribe(string(state.ProductDebug), func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
			//if len(updates) == 0 {
			//	return
			//}

			var serviceState bool
			for _, update := range update {
				log.Info("Content: %s", string(update.Config))
				var cfg state.ConfigContent
				if err := json.Unmarshal(update.Config, &cfg); err != nil {
					log.Error(err, "failed to apply config %s", update.Metadata.ID)
					continue
				}
				serviceState = cfg.Features.RuntimeSecurityEnabled == "true"
			}

			_ = serviceState
			r.enableFeatures(ctx, false) // serviceState)

			/*
				for _, update := range update {
					if update.Metadata.ID == "default" {
						var updateConfig UpdateConfig
						if err := json.Unmarshal(update.Config, &updateConfig); err != nil {
							log.Errorf("Error while retrieving configuration: %s", err)
							continue
						}

						if err := r.upgradeAgents(ctx, updateConfig.Agent.Image); err != nil {
							log.Errorf("Error while updating agent: %s", err)
							continue
						}
					}
				}
			*/

			if err := r.upgradeAgents(ctx, "agent:7.49.0"); err != nil {
				log.Error(err, "Error while updating agent")
			}

		})
	}()

	return nil
}

func (r *Reconciler) upgradeAgents(ctx context.Context, image string) error {
	userNamespace := "default"

	ddList := &v2alpha1.DatadogAgentList{}
	if err := r.client.List(context.TODO(), ddList, &kubeclient.ListOptions{Namespace: userNamespace}); err != nil {
		return fmt.Errorf("unable to list DatadogAgent: %w", err)
	}
	if len(ddList.Items) == 0 {
		return errors.New("cannot find any DatadogAgent")
	}

	for _, dd := range ddList.Items {
		if err := r.upgradeAgent(dd, image); err != nil {
			return err
		}
	}

	return nil
}

func (r *Reconciler) enableFeatures(ctx context.Context, state bool) error {
	userNamespace := "default"

	ddList := &v2alpha1.DatadogAgentList{}
	if err := r.client.List(context.TODO(), ddList, &kubeclient.ListOptions{Namespace: userNamespace}); err != nil {
		return fmt.Errorf("unable to list DatadogAgent: %w", err)
	}
	if len(ddList.Items) == 0 {
		return errors.New("cannot find any DatadogAgent")
	}

	for _, dd := range ddList.Items {
		if err := r.enableFeature(dd, state); err != nil {
			return err
		}
	}

	return nil
}

func (r *Reconciler) upgradeAgent(dd v2alpha1.DatadogAgent, image string) error {
	if dd.Spec.Override == nil {
		dd.Spec.Override = make(map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride)
	}

	imgName, imgTag := common.SplitImageString(image)

	newDD := dd.DeepCopy()

	if err := common.OverrideComponentImage(&newDD.Spec, v2alpha1.NodeAgentComponentName, imgName, imgTag); err != nil {
		return fmt.Errorf("unable to update %s image, err: %w", v2alpha1.NodeAgentComponentName, err)
	}

	if err := common.OverrideComponentImage(&newDD.Spec, v2alpha1.ClusterChecksRunnerComponentName, imgName, imgTag); err != nil {
		return fmt.Errorf("unable to update %s image, err: %w", v2alpha1.ClusterChecksRunnerComponentName, err)
	}

	if !apiutils.IsEqualStruct(dd.Spec, newDD.Spec) {
		return r.client.Update(context.TODO(), newDD)
	}

	return nil
}

func (r *Reconciler) enableFeature(dd v2alpha1.DatadogAgent, state bool) error {
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

	log.Infof("Enabling CWS in CRD")
	*newDD.Spec.Features.CWS.Enabled = state

	if !apiutils.IsEqualStruct(dd.Spec, newDD.Spec) {
		return r.client.Update(context.TODO(), newDD)
	}

	return nil
}
