// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v2alpha1

import (
	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	commonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
)

// Default configuration values. These are the recommended settings for monitoring with Datadog in Kubernetes.
// Note: many default values are set in the Datadog Agent and deliberately not set by the Operator.
const (
	defaultSite     string = "datadoghq.com"
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

	defaultDogstatsdOriginDetectionEnabled bool   = false
	defaultDogstatsdHostPortEnabled        bool   = false
	defaultDogstatsdPort                   int32  = 8125
	defaultDogstatsdSocketEnabled          bool   = true
	defaultDogstatsdSocketPath             string = "/var/run/datadog/dsd.socket"

	defaultCollectKubernetesEvents bool = true

	// defaultAdmissionControllerEnabled          bool = false
	defaultAdmissionControllerMutateUnlabelled bool = false

	defaultOrchestratorExplorerEnabled         bool = true
	defaultOrchestratorExplorerScrubContainers bool = true

	// defaultExternalMetricsServerEnabled bool = false
	defaultDatadogMetricsEnabled bool = true
	// Cluster Agent versions < 1.20 should use 443
	defaultMetricsProviderPort int32 = 8443

	defaultKubeStateMetricsCoreEnabled bool = true

	defaultClusterChecksEnabled    bool = true
	defaultUseClusterChecksRunners bool = true

	// defaultPrometheusScrapeEnabled                bool = false
	defaultPrometheusScrapeEnableServiceEndpoints bool = false

	defaultKubeletAgentCAPath            = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	defaultKubeletAgentCAPathHostPathSet = "/var/run/host-kubelet-ca.crt"
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
		ddaSpec.Global.Registry = apiutils.NewStringPointer(apicommon.DefaultImageRegistry)
	}

	if ddaSpec.Global.LogLevel == nil {
		ddaSpec.Global.LogLevel = apiutils.NewStringPointer(defaultLogLevel)
	}

	if ddaSpec.Global.Kubelet == nil {
		ddaSpec.Global.Kubelet = &commonv1.KubeletConfig{
			AgentCAPath: defaultKubeletAgentCAPath,
		}
	} else if ddaSpec.Global.Kubelet.AgentCAPath == "" {
		if ddaSpec.Global.Kubelet.HostCAPath != "" {
			ddaSpec.Global.Kubelet.AgentCAPath = defaultKubeletAgentCAPathHostPathSet
		} else {
			ddaSpec.Global.Kubelet.AgentCAPath = defaultKubeletAgentCAPath
		}
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
		apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.LogCollection.ContainerCollectUsingFiles, defaultLogContainerCollectUsingFiles)

		apiutils.DefaultStringIfUnset(&ddaSpec.Features.LogCollection.ContainerLogsPath, defaultLogContainerLogsPath)

		apiutils.DefaultStringIfUnset(&ddaSpec.Features.LogCollection.PodLogsPath, defaultLogPodLogsPath)

		apiutils.DefaultStringIfUnset(&ddaSpec.Features.LogCollection.ContainerSymlinksPath, defaultLogContainerSymlinksPath)

		apiutils.DefaultStringIfUnset(&ddaSpec.Features.LogCollection.TempStoragePath, defaultLogTempStoragePath)
	}

	// LiveContainerCollection Feature
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

		apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.APM.HostPortConfig.Enabled, defaultAPMHostPortEnabled)

		apiutils.DefaultInt32IfUnset(&ddaSpec.Features.APM.HostPortConfig.Port, defaultAPMHostPort)

		if ddaSpec.Features.APM.UnixDomainSocketConfig == nil {
			ddaSpec.Features.APM.UnixDomainSocketConfig = &UnixDomainSocketConfig{}
		}

		apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.APM.UnixDomainSocketConfig.Enabled, defaultAPMSocketEnabled)

		apiutils.DefaultStringIfUnset(&ddaSpec.Features.APM.UnixDomainSocketConfig.Path, defaultAPMSocketPath)
	}

	// CWS (Cloud Workload Security) Feature
	if ddaSpec.Features.CWS != nil && *ddaSpec.Features.CWS.Enabled {
		apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.CWS.SyscallMonitorEnabled, defaultCWSSyscallMonitorEnabled)
	}

	// NPM (Network Performance Monitoring) Feature
	if ddaSpec.Features.NPM != nil && *ddaSpec.Features.NPM.Enabled {
		apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.NPM.EnableConntrack, defaultNPMEnableConntrack)

		apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.NPM.CollectDNSStats, defaultNPMCollectDNSStats)
	}

	// Dogstatd Feature
	if ddaSpec.Features.Dogstatsd == nil {
		ddaSpec.Features.Dogstatsd = &DogstatsdFeatureConfig{}
	}

	apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.Dogstatsd.OriginDetectionEnabled, defaultDogstatsdOriginDetectionEnabled)

	if ddaSpec.Features.Dogstatsd.HostPortConfig == nil {
		ddaSpec.Features.Dogstatsd.HostPortConfig = &HostPortConfig{
			Enabled: apiutils.NewBoolPointer(defaultDogstatsdHostPortEnabled),
		}
	}

	if *ddaSpec.Features.Dogstatsd.HostPortConfig.Enabled {
		apiutils.DefaultInt32IfUnset(&ddaSpec.Features.Dogstatsd.HostPortConfig.Port, defaultDogstatsdPort)
	}

	if ddaSpec.Features.Dogstatsd.UnixDomainSocketConfig == nil {
		ddaSpec.Features.Dogstatsd.UnixDomainSocketConfig = &UnixDomainSocketConfig{}
	}

	apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.Dogstatsd.UnixDomainSocketConfig.Enabled, defaultDogstatsdSocketEnabled)

	apiutils.DefaultStringIfUnset(&ddaSpec.Features.Dogstatsd.UnixDomainSocketConfig.Path, defaultDogstatsdSocketPath)

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
		apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.OrchestratorExplorer.ScrubContainers, defaultOrchestratorExplorerScrubContainers)
	}

	// KubeStateMetricsCore check Feature
	if ddaSpec.Features.KubeStateMetricsCore == nil {
		ddaSpec.Features.KubeStateMetricsCore = &KubeStateMetricsCoreFeatureConfig{
			Enabled: apiutils.NewBoolPointer(defaultKubeStateMetricsCoreEnabled),
		}
	}

	// AdmissionController Feature
	if ddaSpec.Features.AdmissionController != nil && *ddaSpec.Features.AdmissionController.Enabled {
		apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.AdmissionController.MutateUnlabelled, defaultAdmissionControllerMutateUnlabelled)
	}

	// ExternalMetricsServer Feature
	if ddaSpec.Features.ExternalMetricsServer != nil && *ddaSpec.Features.ExternalMetricsServer.Enabled {
		apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.ExternalMetricsServer.UseDatadogMetrics, defaultDatadogMetricsEnabled)

		apiutils.DefaultInt32IfUnset(&ddaSpec.Features.ExternalMetricsServer.Port, defaultMetricsProviderPort)
	}

	// ClusterChecks Feature
	if ddaSpec.Features.ClusterChecks == nil {
		ddaSpec.Features.ClusterChecks = &ClusterChecksFeatureConfig{
			Enabled: apiutils.NewBoolPointer(defaultClusterChecksEnabled),
		}
	}

	if *ddaSpec.Features.ClusterChecks.Enabled {
		apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.ClusterChecks.UseClusterChecksRunners, defaultUseClusterChecksRunners)
	}

	// PrometheusScrape Feature
	if ddaSpec.Features.PrometheusScrape != nil && *ddaSpec.Features.PrometheusScrape.Enabled {
		apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.PrometheusScrape.EnableServiceEndpoints, defaultPrometheusScrapeEnableServiceEndpoints)
	}
}
