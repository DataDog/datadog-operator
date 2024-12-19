// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v2alpha1

import (
	apiutils "github.com/DataDog/datadog-operator/api/utils"
)

// Default configuration values. These are the recommended settings for monitoring with Datadog in Kubernetes.
const (
	defaultSite       string = "datadoghq.com"
	defaultEuropeSite string = "datadoghq.eu"
	defaultAsiaSite   string = "ap1.datadoghq.com"
	defaultAzureSite  string = "us3.datadoghq.com"
	defaultGovSite    string = "ddog-gov.com"
	defaultLogLevel   string = "info"

	defaultLogCollectionEnabled          bool   = false
	defaultLogContainerCollectUsingFiles bool   = true
	defaultLogContainerLogsPath          string = "/var/lib/docker/containers"
	defaultLogPodLogsPath                string = "/var/log/pods"
	defaultLogContainerSymlinksPath      string = "/var/log/containers"

	defaultLiveProcessCollectionEnabled   bool = false
	defaultLiveContainerCollectionEnabled bool = true
	defaultProcessDiscoveryEnabled        bool = true
	defaultRunProcessChecksInCoreAgent    bool = false

	defaultOOMKillEnabled        bool = false
	defaultTCPQueueLengthEnabled bool = false

	defaultEBPFCheckEnabled bool = false

	defaultServiceDiscoveryEnabled bool = false

	defaultAPMEnabled                 bool   = true
	defaultAPMHostPortEnabled         bool   = false
	defaultAPMHostPort                int32  = 8126
	defaultAPMSocketEnabled           bool   = true
	defaultAPMSocketHostPath          string = DogstatsdAPMSocketHostPath + "/" + APMSocketName
	defaultAPMSingleStepInstrEnabled  bool   = false
	defaultLanguageDetectionEnabled   bool   = true
	defaultCSPMEnabled                bool   = false
	defaultCSPMHostBenchmarksEnabled  bool   = true
	defaultCWSEnabled                 bool   = false
	defaultCWSSyscallMonitorEnabled   bool   = false
	defaultCWSNetworkEnabled          bool   = true
	defaultCWSSecurityProfilesEnabled bool   = true

	defaultNPMEnabled         bool = false
	defaultNPMEnableConntrack bool = true
	defaultNPMCollectDNSStats bool = true

	defaultUSMEnabled bool = false

	defaultDogstatsdOriginDetectionEnabled bool   = false
	defaultDogstatsdHostPortEnabled        bool   = false
	defaultDogstatsdSocketEnabled          bool   = true
	defaultDogstatsdHostSocketPath         string = DogstatsdAPMSocketHostPath + "/" + DogstatsdSocketName

	defaultOTLPGRPCEnabled         bool   = false
	defaultOTLPGRPCHostPortEnabled bool   = true
	defaultOTLPGRPCEndpoint        string = "0.0.0.0:4317"
	defaultOTLPHTTPEnabled         bool   = false
	defaultOTLPHTTPHostPortEnabled bool   = true
	defaultOTLPHTTPEndpoint        string = "0.0.0.0:4318"

	defaultRemoteConfigurationEnabled bool = true

	defaultCollectKubernetesEvents bool = true

	defaultAdmissionControllerAgentSidecarClusterAgentEnabled bool   = true
	defaultAdmissionControllerEnabled                         bool   = true
	defaultAdmissionControllerValidationEnabled               bool   = true
	defaultAdmissionControllerMutationEnabled                 bool   = true
	defaultAdmissionControllerMutateUnlabelled                bool   = false
	defaultAdmissionServiceName                               string = "datadog-admission-controller"

	defaultAdmissionControllerKubernetesAdmissionEventsEnabled bool = false

	// DefaultAdmissionControllerCWSInstrumentationEnabled default CWS Instrumentation enabled value
	DefaultAdmissionControllerCWSInstrumentationEnabled bool = false
	// DefaultAdmissionControllerCWSInstrumentationMode default CWS Instrumentation mode
	DefaultAdmissionControllerCWSInstrumentationMode string = "remote_copy"

	defaultAdmissionASMThreatsEnabled bool = false
	defaultAdmissionASMSCAEnabled     bool = false
	defaultAdmissionASMIASTEnabled    bool = false

	defaultOrchestratorExplorerEnabled         bool = true
	defaultOrchestratorExplorerScrubContainers bool = true

	defaultExternalMetricsServerEnabled bool = false
	defaultDatadogMetricsEnabled        bool = true
	defaultRegisterAPIService           bool = true
	// Cluster Agent versions < 1.20 should use 443
	defaultMetricsProviderPort int32 = 8443

	defaultKubeStateMetricsCoreEnabled bool = true

	defaultClusterChecksEnabled    bool = true
	defaultUseClusterChecksRunners bool = false

	defaultPrometheusScrapeEnabled                bool = false
	defaultPrometheusScrapeEnableServiceEndpoints bool = false
	defaultPrometheusScrapeVersion                int  = 2

	// defaultKubeletAgentCAPath            = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	// defaultKubeletAgentCAPathHostPathSet = "/var/run/host-kubelet-ca.crt"

	defaultContainerStrategy = OptimizedContainerStrategy

	defaultHelmCheckEnabled       bool = false
	defaultHelmCheckCollectEvents bool = false

	defaultFIPSEnabled      bool   = false
	defaultFIPSImageName    string = "fips-proxy"
	defaultFIPSImageTag     string = FIPSProxyLatestVersion
	defaultFIPSLocalAddress string = "127.0.0.1"
	defaultFIPSPort         int32  = 9803
	defaultFIPSPortRange    int32  = 15
	defaultFIPSUseHTTPS     bool   = false
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
		switch *ddaSpec.Global.Site {
		case defaultEuropeSite:
			ddaSpec.Global.Registry = apiutils.NewStringPointer(DefaultEuropeImageRegistry)
		case defaultAsiaSite:
			ddaSpec.Global.Registry = apiutils.NewStringPointer(DefaultAsiaImageRegistry)
		case defaultAzureSite:
			ddaSpec.Global.Registry = apiutils.NewStringPointer(DefaultAzureImageRegistry)
		case defaultGovSite:
			ddaSpec.Global.Registry = apiutils.NewStringPointer(DefaultGovImageRegistry)
		default:
			ddaSpec.Global.Registry = apiutils.NewStringPointer(DefaultImageRegistry)
		}
	}

	if ddaSpec.Global.LogLevel == nil {
		ddaSpec.Global.LogLevel = apiutils.NewStringPointer(defaultLogLevel)
	}

	if ddaSpec.Global.ContainerStrategy == nil {
		dcs := defaultContainerStrategy
		ddaSpec.Global.ContainerStrategy = &dcs
	}

	if ddaSpec.Global.FIPS == nil {
		ddaSpec.Global.FIPS = &FIPSConfig{}
	}
	apiutils.DefaultBooleanIfUnset(&ddaSpec.Global.FIPS.Enabled, defaultFIPSEnabled)

	if *ddaSpec.Global.FIPS.Enabled {
		if ddaSpec.Global.FIPS.Image == nil {
			ddaSpec.Global.FIPS.Image = &AgentImageConfig{}
		}
		if ddaSpec.Global.FIPS.Image.Name == "" {
			ddaSpec.Global.FIPS.Image.Name = defaultFIPSImageName
		}
		if ddaSpec.Global.FIPS.Image.Tag == "" {
			ddaSpec.Global.FIPS.Image.Tag = defaultFIPSImageTag
		}
		apiutils.DefaultStringIfUnset(&ddaSpec.Global.FIPS.LocalAddress, defaultFIPSLocalAddress)
		apiutils.DefaultInt32IfUnset(&ddaSpec.Global.FIPS.Port, defaultFIPSPort)
		apiutils.DefaultInt32IfUnset(&ddaSpec.Global.FIPS.PortRange, defaultFIPSPortRange)
		apiutils.DefaultBooleanIfUnset(&ddaSpec.Global.FIPS.UseHTTPS, defaultFIPSUseHTTPS)
	}

	apiutils.DefaultBooleanIfUnset(&ddaSpec.Global.RunProcessChecksInCoreAgent, defaultRunProcessChecksInCoreAgent)
}

// defaultFeaturesConfig sets default values in DatadogAgentSpec.Features.
// Note: many default values are set in the Datadog Agent code and are not set here.
func defaultFeaturesConfig(ddaSpec *DatadogAgentSpec) {
	if ddaSpec.Features == nil {
		ddaSpec.Features = &DatadogFeatures{}
	}

	// LogsCollection Feature
	if ddaSpec.Features.LogCollection == nil {
		ddaSpec.Features.LogCollection = &LogCollectionFeatureConfig{}
	}
	apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.LogCollection.Enabled, defaultLogCollectionEnabled)

	if *ddaSpec.Features.LogCollection.Enabled {
		apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.LogCollection.ContainerCollectUsingFiles, defaultLogContainerCollectUsingFiles)

		apiutils.DefaultStringIfUnset(&ddaSpec.Features.LogCollection.ContainerLogsPath, defaultLogContainerLogsPath)

		apiutils.DefaultStringIfUnset(&ddaSpec.Features.LogCollection.PodLogsPath, defaultLogPodLogsPath)

		apiutils.DefaultStringIfUnset(&ddaSpec.Features.LogCollection.ContainerSymlinksPath, defaultLogContainerSymlinksPath)

		apiutils.DefaultStringIfUnset(&ddaSpec.Features.LogCollection.TempStoragePath, DefaultLogTempStoragePath)
	}

	// LiveContainerCollection Feature
	if ddaSpec.Features.LiveContainerCollection == nil {
		ddaSpec.Features.LiveContainerCollection = &LiveContainerCollectionFeatureConfig{}
	}
	apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.LiveContainerCollection.Enabled, defaultLiveContainerCollectionEnabled)

	// LiveProcessCollection Feature
	if ddaSpec.Features.LiveProcessCollection == nil {
		ddaSpec.Features.LiveProcessCollection = &LiveProcessCollectionFeatureConfig{}
	}
	apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.LiveProcessCollection.Enabled, defaultLiveProcessCollectionEnabled)

	// ProcessDiscovery Feature
	if ddaSpec.Features.ProcessDiscovery == nil {
		ddaSpec.Features.ProcessDiscovery = &ProcessDiscoveryFeatureConfig{}
	}
	apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.ProcessDiscovery.Enabled, defaultProcessDiscoveryEnabled)

	// OOMKill Feature
	if ddaSpec.Features.OOMKill == nil {
		ddaSpec.Features.OOMKill = &OOMKillFeatureConfig{}
	}
	apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.OOMKill.Enabled, defaultOOMKillEnabled)

	// TCPQueueLength Feature
	if ddaSpec.Features.TCPQueueLength == nil {
		ddaSpec.Features.TCPQueueLength = &TCPQueueLengthFeatureConfig{}
	}
	apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.TCPQueueLength.Enabled, defaultTCPQueueLengthEnabled)

	// EBPFCheck Feature
	if ddaSpec.Features.EBPFCheck == nil {
		ddaSpec.Features.EBPFCheck = &EBPFCheckFeatureConfig{}
	}
	apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.EBPFCheck.Enabled, defaultEBPFCheckEnabled)

	if ddaSpec.Features.ServiceDiscovery == nil {
		ddaSpec.Features.ServiceDiscovery = &ServiceDiscoveryFeatureConfig{}
	}
	apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.ServiceDiscovery.Enabled, defaultServiceDiscoveryEnabled)

	// APM Feature
	// APM is enabled by default
	if ddaSpec.Features.APM == nil {
		ddaSpec.Features.APM = &APMFeatureConfig{}
	}

	apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.APM.Enabled, defaultAPMEnabled)

	if *ddaSpec.Features.APM.Enabled {
		if ddaSpec.Features.APM.HostPortConfig == nil {
			ddaSpec.Features.APM.HostPortConfig = &HostPortConfig{}
		}

		apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.APM.HostPortConfig.Enabled, defaultAPMHostPortEnabled)

		apiutils.DefaultInt32IfUnset(&ddaSpec.Features.APM.HostPortConfig.Port, defaultAPMHostPort)

		if ddaSpec.Features.APM.UnixDomainSocketConfig == nil {
			ddaSpec.Features.APM.UnixDomainSocketConfig = &UnixDomainSocketConfig{}
		}

		apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.APM.UnixDomainSocketConfig.Enabled, defaultAPMSocketEnabled)

		apiutils.DefaultStringIfUnset(&ddaSpec.Features.APM.UnixDomainSocketConfig.Path, defaultAPMSocketHostPath)

		if ddaSpec.Features.APM.SingleStepInstrumentation == nil {
			ddaSpec.Features.APM.SingleStepInstrumentation = &SingleStepInstrumentation{}
		}

		if ddaSpec.Features.APM.SingleStepInstrumentation.LanguageDetection == nil {
			ddaSpec.Features.APM.SingleStepInstrumentation.LanguageDetection = &LanguageDetectionConfig{}
		}

		apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.APM.SingleStepInstrumentation.Enabled, defaultAPMSingleStepInstrEnabled)
		apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.APM.SingleStepInstrumentation.LanguageDetection.Enabled, defaultLanguageDetectionEnabled)
	}

	// ASM Features
	if ddaSpec.Features.ASM == nil {
		ddaSpec.Features.ASM = &ASMFeatureConfig{}
	}

	if ddaSpec.Features.ASM.Threats == nil {
		ddaSpec.Features.ASM.Threats = &ASMThreatsConfig{}
	}
	apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.ASM.Threats.Enabled, defaultAdmissionASMThreatsEnabled)

	if ddaSpec.Features.ASM.SCA == nil {
		ddaSpec.Features.ASM.SCA = &ASMSCAConfig{}
	}
	apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.ASM.SCA.Enabled, defaultAdmissionASMSCAEnabled)

	if ddaSpec.Features.ASM.IAST == nil {
		ddaSpec.Features.ASM.IAST = &ASMIASTConfig{}
	}
	apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.ASM.IAST.Enabled, defaultAdmissionASMIASTEnabled)

	// CSPM (Cloud Security Posture Management) Feature
	if ddaSpec.Features.CSPM == nil {
		ddaSpec.Features.CSPM = &CSPMFeatureConfig{}
	}
	apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.CSPM.Enabled, defaultCSPMEnabled)

	if *ddaSpec.Features.CSPM.Enabled {
		if ddaSpec.Features.CSPM.HostBenchmarks == nil {
			ddaSpec.Features.CSPM.HostBenchmarks = &CSPMHostBenchmarksConfig{}
		}
		apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.CSPM.HostBenchmarks.Enabled, defaultCSPMHostBenchmarksEnabled)
	}

	// CWS (Cloud Workload Security) Feature
	if ddaSpec.Features.CWS == nil {
		ddaSpec.Features.CWS = &CWSFeatureConfig{}
	}
	apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.CWS.Enabled, defaultCWSEnabled)

	if *ddaSpec.Features.CWS.Enabled {
		if ddaSpec.Features.CWS.Network == nil {
			ddaSpec.Features.CWS.Network = &CWSNetworkConfig{}
		}
		if ddaSpec.Features.CWS.SecurityProfiles == nil {
			ddaSpec.Features.CWS.SecurityProfiles = &CWSSecurityProfilesConfig{}
		}

		apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.CWS.SyscallMonitorEnabled, defaultCWSSyscallMonitorEnabled)
		apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.CWS.Network.Enabled, defaultCWSNetworkEnabled)
		apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.CWS.SecurityProfiles.Enabled, defaultCWSSecurityProfilesEnabled)
	}

	// NPM (Network Performance Monitoring) Feature
	if ddaSpec.Features.NPM == nil {
		ddaSpec.Features.NPM = &NPMFeatureConfig{}
	}
	apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.NPM.Enabled, defaultNPMEnabled)

	if *ddaSpec.Features.NPM.Enabled {
		apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.NPM.EnableConntrack, defaultNPMEnableConntrack)
		apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.NPM.CollectDNSStats, defaultNPMCollectDNSStats)
	}

	// USM (Universal Service Monitoring) Feature
	if ddaSpec.Features.USM == nil {
		ddaSpec.Features.USM = &USMFeatureConfig{}
	}
	apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.USM.Enabled, defaultUSMEnabled)

	// Dogstatsd Feature
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
		apiutils.DefaultInt32IfUnset(&ddaSpec.Features.Dogstatsd.HostPortConfig.Port, DefaultDogstatsdPort)
	}

	if ddaSpec.Features.Dogstatsd.UnixDomainSocketConfig == nil {
		ddaSpec.Features.Dogstatsd.UnixDomainSocketConfig = &UnixDomainSocketConfig{}
	}

	apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.Dogstatsd.UnixDomainSocketConfig.Enabled, defaultDogstatsdSocketEnabled)

	// defaultDogstatsdHostSocketPath matches the default hostPath of the helm chart.
	apiutils.DefaultStringIfUnset(&ddaSpec.Features.Dogstatsd.UnixDomainSocketConfig.Path, defaultDogstatsdHostSocketPath)

	// OTLP ingest feature
	if ddaSpec.Features.OTLP == nil {
		ddaSpec.Features.OTLP = &OTLPFeatureConfig{}
	}

	if ddaSpec.Features.OTLP.Receiver.Protocols.GRPC == nil {
		ddaSpec.Features.OTLP.Receiver.Protocols.GRPC = &OTLPGRPCConfig{}
	}

	apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.OTLP.Receiver.Protocols.GRPC.Enabled, defaultOTLPGRPCEnabled)

	if apiutils.BoolValue(ddaSpec.Features.OTLP.Receiver.Protocols.GRPC.Enabled) {
		if ddaSpec.Features.OTLP.Receiver.Protocols.GRPC.HostPortConfig == nil {
			ddaSpec.Features.OTLP.Receiver.Protocols.GRPC.HostPortConfig = &HostPortConfig{}
		}
		apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.OTLP.Receiver.Protocols.GRPC.HostPortConfig.Enabled, defaultOTLPGRPCHostPortEnabled)
	}

	apiutils.DefaultStringIfUnset(&ddaSpec.Features.OTLP.Receiver.Protocols.GRPC.Endpoint, defaultOTLPGRPCEndpoint)

	if ddaSpec.Features.OTLP.Receiver.Protocols.HTTP == nil {
		ddaSpec.Features.OTLP.Receiver.Protocols.HTTP = &OTLPHTTPConfig{}
	}

	apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.OTLP.Receiver.Protocols.HTTP.Enabled, defaultOTLPHTTPEnabled)

	if apiutils.BoolValue(ddaSpec.Features.OTLP.Receiver.Protocols.HTTP.Enabled) {
		if ddaSpec.Features.OTLP.Receiver.Protocols.HTTP.HostPortConfig == nil {
			ddaSpec.Features.OTLP.Receiver.Protocols.HTTP.HostPortConfig = &HostPortConfig{}
		}
		apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.OTLP.Receiver.Protocols.HTTP.HostPortConfig.Enabled, defaultOTLPHTTPHostPortEnabled)
	}

	apiutils.DefaultStringIfUnset(&ddaSpec.Features.OTLP.Receiver.Protocols.HTTP.Endpoint, defaultOTLPHTTPEndpoint)

	// RemoteConfiguration feature
	if ddaSpec.Features.RemoteConfiguration == nil {
		ddaSpec.Features.RemoteConfiguration = &RemoteConfigurationFeatureConfig{}
	}
	apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.RemoteConfiguration.Enabled, defaultRemoteConfigurationEnabled)

	// Cluster-level features

	// EventCollection Feature
	if ddaSpec.Features.EventCollection == nil {
		ddaSpec.Features.EventCollection = &EventCollectionFeatureConfig{}
	}
	apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.EventCollection.CollectKubernetesEvents, defaultCollectKubernetesEvents)
	if apiutils.BoolValue(ddaSpec.Features.EventCollection.UnbundleEvents) && ddaSpec.Features.EventCollection.CollectedEventTypes == nil {
		ddaSpec.Features.EventCollection.CollectedEventTypes = []EventTypes{
			{
				Kind:    "Pod",
				Reasons: []string{"Failed", "BackOff", "Unhealthy", "FailedScheduling", "FailedMount", "FailedAttachVolume"},
			},
			{
				Kind:    "Node",
				Reasons: []string{"TerminatingEvictedPod", "NodeNotReady", "Rebooted", "HostPortConflict"},
			},
			{
				Kind:    "CronJob",
				Reasons: []string{"SawCompletedJob"},
			},
		}
	}

	// OrchestratorExplorer check Feature
	if ddaSpec.Features.OrchestratorExplorer == nil {
		ddaSpec.Features.OrchestratorExplorer = &OrchestratorExplorerFeatureConfig{}
	}
	apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.OrchestratorExplorer.Enabled, defaultOrchestratorExplorerEnabled)

	if *ddaSpec.Features.OrchestratorExplorer.Enabled {
		apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.OrchestratorExplorer.ScrubContainers, defaultOrchestratorExplorerScrubContainers)
	}

	// KubeStateMetricsCore check Feature
	if ddaSpec.Features.KubeStateMetricsCore == nil {
		ddaSpec.Features.KubeStateMetricsCore = &KubeStateMetricsCoreFeatureConfig{}
	}
	apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.KubeStateMetricsCore.Enabled, defaultKubeStateMetricsCoreEnabled)

	// AdmissionController Feature
	if ddaSpec.Features.AdmissionController == nil {
		ddaSpec.Features.AdmissionController = &AdmissionControllerFeatureConfig{}
	}
	apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.AdmissionController.Enabled, defaultAdmissionControllerEnabled)

	if *ddaSpec.Features.AdmissionController.Enabled {
		apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.AdmissionController.Enabled, defaultAdmissionControllerEnabled)
		apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.AdmissionController.MutateUnlabelled, defaultAdmissionControllerMutateUnlabelled)
		apiutils.DefaultStringIfUnset(&ddaSpec.Features.AdmissionController.ServiceName, defaultAdmissionServiceName)
	}

	// AdmissionControllerValidation Feature
	if ddaSpec.Features.AdmissionController.Validation == nil {
		ddaSpec.Features.AdmissionController.Validation = &AdmissionControllerValidationConfig{}
	}
	apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.AdmissionController.Validation.Enabled, defaultAdmissionControllerValidationEnabled)

	// AdmissionControllerMutation Feature
	if ddaSpec.Features.AdmissionController.Mutation == nil {
		ddaSpec.Features.AdmissionController.Mutation = &AdmissionControllerMutationConfig{}
	}
	apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.AdmissionController.Mutation.Enabled, defaultAdmissionControllerMutationEnabled)

	agentSidecarInjection := ddaSpec.Features.AdmissionController.AgentSidecarInjection
	if agentSidecarInjection != nil && agentSidecarInjection.Enabled != nil && *agentSidecarInjection.Enabled {
		apiutils.DefaultBooleanIfUnset(&agentSidecarInjection.ClusterAgentCommunicationEnabled, defaultAdmissionControllerAgentSidecarClusterAgentEnabled)
	}

	// K8s Admission Events in AdmissonController Feature
	if ddaSpec.Features.AdmissionController.KubernetesAdmissionEvents == nil {
		ddaSpec.Features.AdmissionController.KubernetesAdmissionEvents = &KubernetesAdmissionEventsConfig{}
	}
	apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.AdmissionController.KubernetesAdmissionEvents.Enabled, defaultAdmissionControllerKubernetesAdmissionEventsEnabled)

	// CWS Instrumentation in AdmissionController Feature
	if ddaSpec.Features.AdmissionController.CWSInstrumentation == nil {
		ddaSpec.Features.AdmissionController.CWSInstrumentation = &CWSInstrumentationConfig{}
	}
	apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.AdmissionController.CWSInstrumentation.Enabled, DefaultAdmissionControllerCWSInstrumentationEnabled)

	if *ddaSpec.Features.AdmissionController.CWSInstrumentation.Enabled {
		apiutils.DefaultStringIfUnset(&ddaSpec.Features.AdmissionController.CWSInstrumentation.Mode, DefaultAdmissionControllerCWSInstrumentationMode)
	}

	// ExternalMetricsServer Feature
	if ddaSpec.Features.ExternalMetricsServer == nil {
		ddaSpec.Features.ExternalMetricsServer = &ExternalMetricsServerFeatureConfig{}
	}
	apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.ExternalMetricsServer.Enabled, defaultExternalMetricsServerEnabled)

	if *ddaSpec.Features.ExternalMetricsServer.Enabled {
		apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.ExternalMetricsServer.UseDatadogMetrics, defaultDatadogMetricsEnabled)
		apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.ExternalMetricsServer.RegisterAPIService, defaultRegisterAPIService)
		apiutils.DefaultInt32IfUnset(&ddaSpec.Features.ExternalMetricsServer.Port, defaultMetricsProviderPort)
	}

	// ClusterChecks Feature
	if ddaSpec.Features.ClusterChecks == nil {
		ddaSpec.Features.ClusterChecks = &ClusterChecksFeatureConfig{}
	}
	apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.ClusterChecks.Enabled, defaultClusterChecksEnabled)

	if *ddaSpec.Features.ClusterChecks.Enabled {
		apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.ClusterChecks.UseClusterChecksRunners, defaultUseClusterChecksRunners)
	}

	// PrometheusScrape Feature
	if ddaSpec.Features.PrometheusScrape == nil {
		ddaSpec.Features.PrometheusScrape = &PrometheusScrapeFeatureConfig{}
	}
	apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.PrometheusScrape.Enabled, defaultPrometheusScrapeEnabled)

	if *ddaSpec.Features.PrometheusScrape.Enabled {
		apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.PrometheusScrape.EnableServiceEndpoints, defaultPrometheusScrapeEnableServiceEndpoints)
		apiutils.DefaultIntIfUnset(&ddaSpec.Features.PrometheusScrape.Version, defaultPrometheusScrapeVersion)
	}

	// Helm Check Feature
	if ddaSpec.Features.HelmCheck == nil {
		ddaSpec.Features.HelmCheck = &HelmCheckFeatureConfig{}
	}
	apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.HelmCheck.Enabled, defaultHelmCheckEnabled)

	if *ddaSpec.Features.HelmCheck.Enabled {
		apiutils.DefaultBooleanIfUnset(&ddaSpec.Features.HelmCheck.CollectEvents, defaultHelmCheckCollectEvents)
	}
}
