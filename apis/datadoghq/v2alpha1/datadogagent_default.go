// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v2alpha1

import (
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
)

// Default configuration values. These are the recommended settings for monitoring with Datadog in Kubernetes.
// Note: many default values are set in the Datadog Agent and deliberately not set by the Operator.
const (
	defaultSite     string = "datadoghq.com"
	defaultRegistry string = "gcr.io/datadoghq"
	defaultLogLevel string = "info"

	// defaultLogCollectionEnabled          bool   = false
	defaultLogContainerCollectUsingFiles bool   = true
	defaultLogContainerLogsPath          string = "/var/lib/docker/containers"
	defaultLogPodLogsPath                string = "/var/log/pods"
	defaultLogContainerSymlinksPath      string = "/var/log/containers"
	defaultLogTempStoragePath            string = "/var/lib/datadog-agent/logs"

	// defaultLiveProcessCollectionEnabled   bool = false
	defaultLiveContainerCollectionEnabled bool = true

	// defaultOOMKillEnabled        bool = false
	// defaultTCPQueueLengthEnabled bool = false

	// defaultAPMEnabled         bool   = false
	defaultAPMHostPortEnabled bool   = false
	defaultAPMHostPort        int32  = 8126
	defaultAPMSocketEnabled   bool   = true
	defaultAPMSocketPath      string = "/var/run/datadog/apm.sock"

	// defaultCSPMEnabled              bool = false
	// defaultCWSEnabled               bool = false
	defaultCWSSyscallMonitorEnabled bool = false

	// defaultNPMEnabled         bool = false
	defaultNPMEnableConntrack bool = true
	defaultNPMCollectDNSStats bool = true

	// defaultUSMEnabled bool = false

	defaultDogStatsDOriginDetectionEnabled bool   = false
	defaultDogStatsDHostPortEnabled        bool   = false
	defaultDogStatsDPort                   int32  = 8125
	defaultDogStatsDUseSocketVolume        bool   = false
	defaultDogStatsDSocketPath             string = "/var/run/datadog/dsd.socket"

	defaultCollectKubernetesEvents bool = true

	// defaultAdmissionControllerEnabled          bool = false
	defaultAdmissionControllerMutateUnlabelled bool = false

	defaultOrchestratorExplorerEnabled bool   = true
	DefaultOrchestratorExplorerConf    string = "orchestrator-explorer-config"

	// defaultExternalMetricsServerEnabled bool = false
	defaultDatadogMetricsEnabled bool = true
	// Cluster Agent versions < 1.20 should use 443
	defaultMetricsProviderPort int32 = 8443

	defaultKubeStateMetricsCoreEnabled bool   = true
	defaultKubeStateMetricsCoreConf    string = "kube-state-metrics-core-config"

	defaultClusterChecksEnabled    bool = true
	defaultUseClusterChecksRunners bool = true

	// defaultPrometheusScrapeEnabled                bool = false
	defaultPrometheusScrapeEnableServiceEndpoints bool = false

	// defaultDatadogMonitorEnabled bool = false
)

// DefaultDatadogAgent defaults the DatadogAgentSpec GlobalConfig and Features.
func DefaultDatadogAgent(dda *DatadogAgent) {
	defaultGlobalConfig(&dda.Spec)

	defaultFeaturesConfig(&dda.Spec)
}

// defaultGlobalConfig sets default values in DatadogAgentSpec.Global.
func defaultGlobalConfig(ddaSpec *DatadogAgentSpec) {
	if ddaSpec.Global == nil {
		ddaSpec.Global = &GlobalConfig{}
	}

	if ddaSpec.Global.Site == nil {
		ddaSpec.Global.Site = apiutils.NewStringPointer(defaultSite)
	}

	if ddaSpec.Global.Registry == nil {
		ddaSpec.Global.Registry = apiutils.NewStringPointer(defaultRegistry)
	}

	if ddaSpec.Global.LogLevel == nil {
		ddaSpec.Global.LogLevel = apiutils.NewStringPointer(defaultLogLevel)
	}
}

// defaultFeaturesConfig sets default values in DatadogAgentSpec.Features.
// Note: many default values are set in the Datadog Agent code and are not set here.
func defaultFeaturesConfig(ddaSpec *DatadogAgentSpec) {
	if ddaSpec.Features == nil {
		ddaSpec.Features = &DatadogFeatures{}
	}

	// LogsCollection Feature
	if ddaSpec.Features.LogCollection != nil && *ddaSpec.Features.LogCollection.Enabled {
		if ddaSpec.Features.LogCollection.ContainerCollectUsingFiles == nil {
			ddaSpec.Features.LogCollection.ContainerCollectUsingFiles = apiutils.NewBoolPointer(defaultLogContainerCollectUsingFiles)
		}
		if ddaSpec.Features.LogCollection.ContainerLogsPath == nil {
			ddaSpec.Features.LogCollection.ContainerLogsPath = apiutils.NewStringPointer(defaultLogContainerLogsPath)
		}
		if ddaSpec.Features.LogCollection.PodLogsPath == nil {
			ddaSpec.Features.LogCollection.PodLogsPath = apiutils.NewStringPointer(defaultLogPodLogsPath)
		}
		if ddaSpec.Features.LogCollection.ContainerSymlinksPath == nil {
			ddaSpec.Features.LogCollection.ContainerSymlinksPath = apiutils.NewStringPointer(defaultLogContainerSymlinksPath)
		}
		if ddaSpec.Features.LogCollection.TempStoragePath == nil {
			ddaSpec.Features.LogCollection.TempStoragePath = apiutils.NewStringPointer(defaultLogTempStoragePath)
		}
	}

	// ContainerCollection Feature
	if ddaSpec.Features.LiveContainerCollection == nil {
		ddaSpec.Features.LiveContainerCollection = &LiveContainerCollectionFeatureConfig{
			Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
		}
	}

	// APM Feature
	if ddaSpec.Features.APM != nil && *ddaSpec.Features.APM.Enabled {
		if ddaSpec.Features.APM.HostPortConfig == nil {
			ddaSpec.Features.APM.HostPortConfig = &HostPortConfig{}
		}

		if ddaSpec.Features.APM.HostPortConfig.Enabled == nil {
			ddaSpec.Features.APM.HostPortConfig.Enabled = apiutils.NewBoolPointer(defaultAPMHostPortEnabled)
		}

		if ddaSpec.Features.APM.HostPortConfig.Port == nil {
			ddaSpec.Features.APM.HostPortConfig.Port = apiutils.NewInt32Pointer(defaultAPMHostPort)
		}

		if ddaSpec.Features.APM.UnixDomainSocketConfig == nil {
			ddaSpec.Features.APM.UnixDomainSocketConfig = &UnixDomainSocketConfig{}
		}

		if ddaSpec.Features.APM.UnixDomainSocketConfig.Enabled == nil {
			ddaSpec.Features.APM.UnixDomainSocketConfig.Enabled = apiutils.NewBoolPointer(defaultAPMSocketEnabled)
		}

		if ddaSpec.Features.APM.UnixDomainSocketConfig.Path == nil {
			ddaSpec.Features.APM.UnixDomainSocketConfig.Path = apiutils.NewStringPointer(defaultAPMSocketPath)
		}
	}

	// CWS (Cloud Workload Security) Feature
	if ddaSpec.Features.CWS != nil && *ddaSpec.Features.CWS.Enabled {
		ddaSpec.Features.CWS.SyscallMonitorEnabled = apiutils.NewBoolPointer(defaultCWSSyscallMonitorEnabled)
	}

	// NPM (Network Performance Monitoring) Feature
	if ddaSpec.Features.NPM != nil && *ddaSpec.Features.NPM.Enabled {
		if ddaSpec.Features.NPM.EnableConntrack == nil {
			ddaSpec.Features.NPM.EnableConntrack = apiutils.NewBoolPointer(defaultNPMEnableConntrack)
		}
		if ddaSpec.Features.NPM.CollectDNSStats == nil {
			ddaSpec.Features.NPM.CollectDNSStats = apiutils.NewBoolPointer(defaultNPMCollectDNSStats)
		}
	}

	// Dogstatd Feature
	if ddaSpec.Features.DogStatsD == nil {
		ddaSpec.Features.DogStatsD = &DogStatsDFeatureConfig{}
	}

	if ddaSpec.Features.DogStatsD.OriginDetectionEnabled == nil {
		ddaSpec.Features.DogStatsD.OriginDetectionEnabled = apiutils.NewBoolPointer(defaultDogStatsDOriginDetectionEnabled)
	}

	if ddaSpec.Features.DogStatsD.HostPortConfig == nil {
		ddaSpec.Features.DogStatsD.HostPortConfig = &HostPortConfig{
			Enabled: apiutils.NewBoolPointer(defaultDogStatsDHostPortEnabled),
		}
	}

	if *ddaSpec.Features.DogStatsD.HostPortConfig.Enabled {
		ddaSpec.Features.DogStatsD.HostPortConfig.Port = apiutils.NewInt32Pointer(defaultDogStatsDPort)
	}

	if ddaSpec.Features.DogStatsD.UnixDomainSocketConfig == nil {
		ddaSpec.Features.DogStatsD.UnixDomainSocketConfig = &UnixDomainSocketConfig{}
	}

	if ddaSpec.Features.DogStatsD.UnixDomainSocketConfig.Enabled == nil {
		ddaSpec.Features.DogStatsD.UnixDomainSocketConfig.Enabled = apiutils.NewBoolPointer(defaultDogStatsDUseSocketVolume)
	}

	if ddaSpec.Features.DogStatsD.UnixDomainSocketConfig.Path == nil {
		ddaSpec.Features.DogStatsD.UnixDomainSocketConfig.Path = apiutils.NewStringPointer(defaultDogStatsDSocketPath)
	}

	// Cluster-level features

	// EventCollection Feature
	if ddaSpec.Features.EventCollection == nil {
		ddaSpec.Features.EventCollection = &EventCollectionFeatureConfig{
			CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
		}
	}

	// OrchestratorExplorer check Feature
	if ddaSpec.Features.OrchestratorExplorer == nil {
		ddaSpec.Features.OrchestratorExplorer = &OrchestratorExplorerFeatureConfig{
			Enabled: apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
		}
	}

	if *ddaSpec.Features.OrchestratorExplorer.Enabled {
		if ddaSpec.Features.OrchestratorExplorer.Conf == nil {
			ddaSpec.Features.OrchestratorExplorer.Conf = &CustomConfig{
				ConfigData: apiutils.NewStringPointer(DefaultOrchestratorExplorerConf),
			}
		}
	}

	// KubeStateMetricsCore check Feature
	if ddaSpec.Features.KubeStateMetricsCore == nil {
		ddaSpec.Features.KubeStateMetricsCore = &KubeStateMetricsCoreFeatureConfig{
			Enabled: apiutils.NewBoolPointer(defaultKubeStateMetricsCoreEnabled),
		}
	}

	if *ddaSpec.Features.KubeStateMetricsCore.Enabled {
		if ddaSpec.Features.KubeStateMetricsCore.Conf == nil {
			ddaSpec.Features.KubeStateMetricsCore.Conf = &CustomConfig{
				ConfigData: apiutils.NewStringPointer(defaultKubeStateMetricsCoreConf),
			}
		}
	}

	// AdmissionController Feature
	if ddaSpec.Features.AdmissionController != nil && *ddaSpec.Features.AdmissionController.Enabled {
		ddaSpec.Features.AdmissionController.MutateUnlabelled = apiutils.NewBoolPointer(defaultAdmissionControllerMutateUnlabelled)
	}

	// ExternalMetricsServer Feature
	if ddaSpec.Features.ExternalMetricsServer != nil && *ddaSpec.Features.ExternalMetricsServer.Enabled {
		if ddaSpec.Features.ExternalMetricsServer.UseDatadogMetrics == nil {
			ddaSpec.Features.ExternalMetricsServer.UseDatadogMetrics = apiutils.NewBoolPointer(defaultDatadogMetricsEnabled)
		}
		if ddaSpec.Features.ExternalMetricsServer.Port == nil {
			ddaSpec.Features.ExternalMetricsServer.Port = apiutils.NewInt32Pointer(defaultMetricsProviderPort)
		}
	}

	// ClusterChecks Feature
	if ddaSpec.Features.ClusterChecks == nil {
		ddaSpec.Features.ClusterChecks = &ClusterChecksFeatureConfig{
			Enabled: apiutils.NewBoolPointer(defaultClusterChecksEnabled),
		}
	}

	if *ddaSpec.Features.ClusterChecks.Enabled {
		ddaSpec.Features.ClusterChecks.UseClusterChecksRunners = apiutils.NewBoolPointer(defaultUseClusterChecksRunners)
	}

	// PrometheusScrape Feature
	if ddaSpec.Features.PrometheusScrape != nil && *ddaSpec.Features.PrometheusScrape.Enabled {
		ddaSpec.Features.PrometheusScrape.EnableServiceEndpoints = apiutils.NewBoolPointer(defaultPrometheusScrapeEnableServiceEndpoints)
	}
}
