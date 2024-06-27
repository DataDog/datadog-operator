// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"os"
	"strings"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/cache"
)

const (
	// AgentWatchNamespaceEnvVar is a comma separated list of namespaces relevant to Agent controller.
	AgentWatchNamespaceEnvVar = "AGENT_WATCH_NAMESPACE"
	// SLOWatchNamespaceEnvVar is a comma separated list of namespaces relevant to SLO controller.
	SLOWatchNamespaceEnvVar = "SLO_WATCH_NAMESPACE"
	// MonitorWatchNamespaceEnvVar is a comma separated list of namespaces relevant to Monitor controller.
	MonitorWatchNamespaceEnvVar = "MONITOR_WATCH_NAMESPACE"
	// ProfilesWatchNamespaceEnvVar is a comma separated list of namespaces relevant to Agent Profile controller.
	ProfilesWatchNamespaceEnvVar = "PROFILE_WATCH_NAMESPACE"
	// WatchNamespaceEnvVar is a comma separated list of namespaces watched by all controllers unless controller specific configuration is provided.
	// An empty value means the operator is running with cluster scope.
	WatchNamespaceEnvVar = "WATCH_NAMESPACE"
	// DDAPIKeyEnvVar is the constant for the env variable DD_API_KEY which is the fallback
	// API key to use if a resource does not have it defined in its spec.
	DDAPIKeyEnvVar = "DD_API_KEY"
	// DDAppKeyEnvVar is the constant for the env variable DD_APP_KEY which is the fallback
	// App key to use if a resource does not have it defined in its spec.
	DDAppKeyEnvVar = "DD_APP_KEY"
	// DDURLEnvVar is the constant for the env variable DD_URL which is the
	// host of the Datadog intake server to send data to.
	DDURLEnvVar = "DD_URL"
	// TODO consider moving DDSite here as well
)

// GetWatchNamespaces returns default namespace map with empty cache config.
func GetWatchNamespaces(logger logr.Logger) map[string]cache.Config {
	return getWatchNamespacesFromEnv(logger, WatchNamespaceEnvVar)
}

// GetMonitorWatchNamespaces returns namespace map for Agents with empty cache config.
func GetAgentWatchNamespaces(logger logr.Logger) map[string]cache.Config {
	return getWatchNamespacesFromEnv(logger, AgentWatchNamespaceEnvVar)
}

// GetMonitorWatchNamespaces returns namespace map for Monitors with empty cache config.
func GetMonitorWatchNamespaces(logger logr.Logger) map[string]cache.Config {
	return getWatchNamespacesFromEnv(logger, MonitorWatchNamespaceEnvVar)
}

// GetSLOWatchNamespaces returns namespace map for SLOs with empty cache config.
func GetSLOWatchNamespaces(logger logr.Logger) map[string]cache.Config {
	return getWatchNamespacesFromEnv(logger, SLOWatchNamespaceEnvVar)
}

// GetProfileWatchNamespaces returns namespace map for DAPs with empty cache config.
func GetProfileWatchNamespaces(logger logr.Logger) map[string]cache.Config {
	return getWatchNamespacesFromEnv(logger, ProfilesWatchNamespaceEnvVar)
}

func getWatchNamespacesFromEnv(logger logr.Logger, envVar string) map[string]cache.Config {
	cacheConfig := cache.Config{}

	nsEnvValue, found := os.LookupEnv(envVar)
	if !found {
		logger.Info("CRD specific cache namespaces config not found, will be using common config")
		nsEnvValue, found = os.LookupEnv(WatchNamespaceEnvVar)
		if !found {
			logger.Info("Common namespace config not found, will be watching all namespaces")
			return map[string]cache.Config{cache.AllNamespaces: cacheConfig}
		}
	}

	var namespaces []string
	if strings.Contains(nsEnvValue, ",") {
		namespaces = strings.Split(nsEnvValue, ",")
	} else {
		namespaces = []string{nsEnvValue}
	}
	nsConfigs := make(map[string]cache.Config)
	for _, ns := range namespaces {
		nsConfigs[ns] = cache.Config{}
	}
	return nsConfigs
}
