// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package test

import (
	"github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/apis/utils"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
)

type DatadogAgentBuilder struct {
	datadogAgent v2alpha1.DatadogAgent
}

// NewDatadogAgentBuilder creates DatadogAgent and initializes Global, Features, Override properties
func NewDatadogAgentBuilder() *DatadogAgentBuilder {
	dda := &v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Global:   &v2alpha1.GlobalConfig{},
			Features: &v2alpha1.DatadogFeatures{},
			Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{},
		},
	}

	return &DatadogAgentBuilder{
		datadogAgent: *dda,
	}
}

// NewDefaultDatadogAgentBuilder created DatadogAgent and applies defaults
func NewDefaultDatadogAgentBuilder() *DatadogAgentBuilder {
	dda := &v2alpha1.DatadogAgent{}
	v2alpha1.DefaultDatadogAgent(dda)

	return &DatadogAgentBuilder{
		datadogAgent: *dda,
	}
}

// NewDefaultDatadogAgentBuilder initialized with name, namespace, creds and metadata
func NewInitializedDatadogAgentBuilder(ns, name string) *DatadogAgentBuilder {
	dda := NewDatadogAgent(ns, name, nil)
	dda.Spec.Features = &v2alpha1.DatadogFeatures{}

	return &DatadogAgentBuilder{
		datadogAgent: *dda,
	}
}

// Build returns DatadogAgent pointer with current properties
func (builder *DatadogAgentBuilder) Build() *v2alpha1.DatadogAgent {
	return &builder.datadogAgent
}

// BuildWithDefaults applies defaults to current properties and returns resulting DatadogAgent
func (builder *DatadogAgentBuilder) BuildWithDefaults() *v2alpha1.DatadogAgent {
	v2alpha1.DefaultDatadogAgent(&builder.datadogAgent)
	return &builder.datadogAgent
}

// Common
func (builder *DatadogAgentBuilder) WithName(name string) *DatadogAgentBuilder {
	builder.datadogAgent.Name = name
	return builder
}

// Dogstatsd
func (builder *DatadogAgentBuilder) initDogstatsd() {
	if builder.datadogAgent.Spec.Features.Dogstatsd == nil {
		builder.datadogAgent.Spec.Features.Dogstatsd = &v2alpha1.DogstatsdFeatureConfig{}
	}
}

func (builder *DatadogAgentBuilder) WithDogstatsdHostPortEnabled(enabled bool) *DatadogAgentBuilder {
	builder.initDogstatsd()
	builder.datadogAgent.Spec.Features.Dogstatsd.HostPortConfig.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithDogstatsdHostPortConfig(port int32) *DatadogAgentBuilder {
	builder.initDogstatsd()
	builder.datadogAgent.Spec.Features.Dogstatsd.HostPortConfig.Port = apiutils.NewInt32Pointer(port)
	return builder
}

func (builder *DatadogAgentBuilder) WithDogstatsdOriginDetectionEnabled(enabled bool) *DatadogAgentBuilder {
	builder.initDogstatsd()
	builder.datadogAgent.Spec.Features.Dogstatsd.OriginDetectionEnabled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithDogstatsdTagCardinality(cardinality string) *DatadogAgentBuilder {
	builder.initDogstatsd()
	builder.datadogAgent.Spec.Features.Dogstatsd.OriginDetectionEnabled = apiutils.NewBoolPointer(true)
	builder.datadogAgent.Spec.Features.Dogstatsd.TagCardinality = apiutils.NewStringPointer(cardinality)
	return builder
}

func (builder *DatadogAgentBuilder) WithDogstatsdUnixDomainSocketConfigEnabled(enabled bool) *DatadogAgentBuilder {
	builder.initDogstatsd()
	builder.datadogAgent.Spec.Features.Dogstatsd.UnixDomainSocketConfig.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithDogstatsdUnixDomainSocketConfigPath(customPath string) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Features.Dogstatsd.UnixDomainSocketConfig.Path = apiutils.NewStringPointer(customPath)
	return builder
}

func (builder *DatadogAgentBuilder) WithDogstatsdMapperProfiles(customMapperProfilesConf string) *DatadogAgentBuilder {
	builder.initDogstatsd()
	builder.datadogAgent.Spec.Features.Dogstatsd.MapperProfiles = &v2alpha1.CustomConfig{ConfigData: apiutils.NewStringPointer(customMapperProfilesConf)}
	return builder
}

// Live ContainerCollection

func (builder *DatadogAgentBuilder) initLiveContainer() {
	if builder.datadogAgent.Spec.Features.LiveContainerCollection == nil {
		builder.datadogAgent.Spec.Features.LiveContainerCollection = &v2alpha1.LiveContainerCollectionFeatureConfig{}
	}
}

func (builder *DatadogAgentBuilder) WithLiveContainerCollectionEnabled(enabled bool) *DatadogAgentBuilder {
	builder.initLiveContainer()
	builder.datadogAgent.Spec.Features.LiveContainerCollection.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

// Live Processes
func (builder *DatadogAgentBuilder) initLiveProcesses() {
	if builder.datadogAgent.Spec.Features.LiveProcessCollection == nil {
		builder.datadogAgent.Spec.Features.LiveProcessCollection = &v2alpha1.LiveProcessCollectionFeatureConfig{}
	}
}

func (builder *DatadogAgentBuilder) WithLiveProcessEnabled(enabled bool) *DatadogAgentBuilder {
	builder.initLiveProcesses()
	builder.datadogAgent.Spec.Features.LiveProcessCollection.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithLiveProcessScrubStrip(scrubEnabled, stripEnabled bool) *DatadogAgentBuilder {
	builder.initLiveProcesses()
	builder.datadogAgent.Spec.Features.LiveProcessCollection.ScrubProcessArguments = apiutils.NewBoolPointer(scrubEnabled)
	builder.datadogAgent.Spec.Features.LiveProcessCollection.StripProcessArguments = apiutils.NewBoolPointer(stripEnabled)
	return builder
}

// Log Collection
func (builder *DatadogAgentBuilder) initLogCollection() {
	if builder.datadogAgent.Spec.Features.LogCollection == nil {
		builder.datadogAgent.Spec.Features.LogCollection = &v2alpha1.LogCollectionFeatureConfig{}
	}
}

func (builder *DatadogAgentBuilder) WithLogCollectionEnabled(enabled bool) *DatadogAgentBuilder {
	builder.initLogCollection()
	builder.datadogAgent.Spec.Features.LogCollection.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithLogCollectionCollectAll(enabled bool) *DatadogAgentBuilder {
	builder.initLogCollection()
	builder.datadogAgent.Spec.Features.LogCollection.ContainerCollectAll = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithLogCollectionLogCollectionUsingFiles(enabled bool) *DatadogAgentBuilder {
	builder.initLogCollection()
	builder.datadogAgent.Spec.Features.LogCollection.ContainerCollectUsingFiles = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithLogCollectionOpenFilesLimit(limit int32) *DatadogAgentBuilder {
	builder.initLogCollection()
	builder.datadogAgent.Spec.Features.LogCollection.OpenFilesLimit = apiutils.NewInt32Pointer(limit)
	return builder
}

func (builder *DatadogAgentBuilder) WithLogCollectionPaths(podLogs, containerLogs, containerSymlinks, tempStorate string) *DatadogAgentBuilder {
	builder.initLogCollection()
	builder.datadogAgent.Spec.Features.LogCollection.PodLogsPath = apiutils.NewStringPointer(podLogs)
	builder.datadogAgent.Spec.Features.LogCollection.ContainerLogsPath = apiutils.NewStringPointer(containerLogs)
	builder.datadogAgent.Spec.Features.LogCollection.ContainerSymlinksPath = apiutils.NewStringPointer(containerSymlinks)
	builder.datadogAgent.Spec.Features.LogCollection.TempStoragePath = apiutils.NewStringPointer(tempStorate)
	return builder
}

// Event Collection
func (builder *DatadogAgentBuilder) initEventCollection() {
	if builder.datadogAgent.Spec.Features.EventCollection == nil {
		builder.datadogAgent.Spec.Features.EventCollection = &v2alpha1.EventCollectionFeatureConfig{}
	}
}

func (builder *DatadogAgentBuilder) WithEventCollectionKubernetesEvents(enabled bool) *DatadogAgentBuilder {
	builder.initEventCollection()
	builder.datadogAgent.Spec.Features.EventCollection.CollectKubernetesEvents = apiutils.NewBoolPointer(enabled)

	return builder
}

// Remote Config
func (builder *DatadogAgentBuilder) initRemoteConfig() {
	if builder.datadogAgent.Spec.Features.RemoteConfiguration == nil {
		builder.datadogAgent.Spec.Features.RemoteConfiguration = &v2alpha1.RemoteConfigurationFeatureConfig{}
	}
}

func (builder *DatadogAgentBuilder) WithRemoteConfigEnabled(enabled bool) *DatadogAgentBuilder {
	builder.initRemoteConfig()
	builder.datadogAgent.Spec.Features.RemoteConfiguration.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

// KSM

func (builder *DatadogAgentBuilder) initKSM() {
	if builder.datadogAgent.Spec.Features.KubeStateMetricsCore == nil {
		builder.datadogAgent.Spec.Features.KubeStateMetricsCore = &v2alpha1.KubeStateMetricsCoreFeatureConfig{}
	}
}

func (builder *DatadogAgentBuilder) WithKSMEnabled(enabled bool) *DatadogAgentBuilder {
	builder.initKSM()
	builder.datadogAgent.Spec.Features.KubeStateMetricsCore.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithKSMCustomConf(customData string) *DatadogAgentBuilder {
	builder.initKSM()
	builder.datadogAgent.Spec.Features.KubeStateMetricsCore.Conf = &v2alpha1.CustomConfig{
		ConfigData: apiutils.NewStringPointer(customData),
	}
	return builder
}

// Orchestrator Explorer

func (builder *DatadogAgentBuilder) initOE() {
	if builder.datadogAgent.Spec.Features.OrchestratorExplorer == nil {
		builder.datadogAgent.Spec.Features.OrchestratorExplorer = &v2alpha1.OrchestratorExplorerFeatureConfig{}
	}
}

func (builder *DatadogAgentBuilder) WithOrchestratorExplorerEnabled(enabled bool) *DatadogAgentBuilder {
	builder.initOE()
	builder.datadogAgent.Spec.Features.OrchestratorExplorer.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithOrchestratorExplorerScrubContainers(enabled bool) *DatadogAgentBuilder {
	builder.initOE()
	builder.datadogAgent.Spec.Features.OrchestratorExplorer.ScrubContainers = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithOrchestratorExplorerExtraTags(tags []string) *DatadogAgentBuilder {
	builder.initOE()
	builder.datadogAgent.Spec.Features.OrchestratorExplorer.ExtraTags = tags
	return builder
}

func (builder *DatadogAgentBuilder) WithOrchestratorExplorerDDUrl(ddUrl string) *DatadogAgentBuilder {
	builder.initOE()
	builder.datadogAgent.Spec.Features.OrchestratorExplorer.DDUrl = apiutils.NewStringPointer(ddUrl)
	return builder
}

func (builder *DatadogAgentBuilder) WithOrchestratorExplorerCustomConfigData(customConfigData string) *DatadogAgentBuilder {
	builder.initOE()
	builder.datadogAgent.Spec.Features.OrchestratorExplorer.Conf = &v2alpha1.CustomConfig{
		ConfigData: &customConfigData,
	}
	return builder
}

// Cluster Checks

func (builder *DatadogAgentBuilder) initCC() {
	if builder.datadogAgent.Spec.Features.ClusterChecks == nil {
		builder.datadogAgent.Spec.Features.ClusterChecks = &v2alpha1.ClusterChecksFeatureConfig{}
	}
}

func (builder *DatadogAgentBuilder) WithClusterChecksEnabled(enabled bool) *DatadogAgentBuilder {
	builder.initCC()
	builder.datadogAgent.Spec.Features.ClusterChecks.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithClusterChecksUseCLCEnabled(enabled bool) *DatadogAgentBuilder {
	builder.initCC()
	builder.datadogAgent.Spec.Features.ClusterChecks.UseClusterChecksRunners = apiutils.NewBoolPointer(enabled)
	return builder
}

// Prometheus Scrape

func (builder *DatadogAgentBuilder) initPrometheusScrape() {
	if builder.datadogAgent.Spec.Features.PrometheusScrape == nil {
		builder.datadogAgent.Spec.Features.PrometheusScrape = &v2alpha1.PrometheusScrapeFeatureConfig{}
	}
}

func (builder *DatadogAgentBuilder) WithPrometheusScrapeEnabled(enabled bool) *DatadogAgentBuilder {
	builder.initPrometheusScrape()
	builder.datadogAgent.Spec.Features.PrometheusScrape.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithPrometheusScrapeServiceEndpoints(enabled bool) *DatadogAgentBuilder {
	builder.initPrometheusScrape()
	builder.datadogAgent.Spec.Features.PrometheusScrape.EnableServiceEndpoints = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithPrometheusScrapeAdditionalConfigs(additionalConfig string) *DatadogAgentBuilder {
	builder.initPrometheusScrape()
	builder.datadogAgent.Spec.Features.PrometheusScrape.AdditionalConfigs = apiutils.NewStringPointer(additionalConfig)
	return builder
}

func (builder *DatadogAgentBuilder) WithPrometheusScrapeVersion(version int) *DatadogAgentBuilder {
	builder.initPrometheusScrape()
	builder.datadogAgent.Spec.Features.PrometheusScrape.Version = apiutils.NewIntPointer(version)
	return builder
}

// APM

func (builder *DatadogAgentBuilder) initAPM() {
	if builder.datadogAgent.Spec.Features.APM == nil {
		builder.datadogAgent.Spec.Features.APM = &v2alpha1.APMFeatureConfig{}
	}
}

func (builder *DatadogAgentBuilder) WithAPMEnabled(enabled bool) *DatadogAgentBuilder {
	builder.initAPM()
	builder.datadogAgent.Spec.Features.APM.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithAPMHostPortEnabled(enabled bool, port *int32) *DatadogAgentBuilder {
	builder.initAPM()
	builder.datadogAgent.Spec.Features.APM.HostPortConfig = &v2alpha1.HostPortConfig{
		Enabled: apiutils.NewBoolPointer(enabled),
	}
	if port != nil {
		builder.datadogAgent.Spec.Features.APM.HostPortConfig.Port = port
	}
	return builder
}

func (builder *DatadogAgentBuilder) WithAPMUDSEnabled(enabled bool, apmSocketHostPath string) *DatadogAgentBuilder {
	builder.initAPM()
	builder.datadogAgent.Spec.Features.APM.UnixDomainSocketConfig = &v2alpha1.UnixDomainSocketConfig{
		Enabled: apiutils.NewBoolPointer(enabled),
		Path:    apiutils.NewStringPointer(apmSocketHostPath),
	}
	return builder
}

// OTLP

func (builder *DatadogAgentBuilder) initOTLP() {
	if builder.datadogAgent.Spec.Features.OTLP == nil {
		builder.datadogAgent.Spec.Features.OTLP = &v2alpha1.OTLPFeatureConfig{}
		builder.datadogAgent.Spec.Features.OTLP.Receiver = v2alpha1.OTLPReceiverConfig{
			Protocols: v2alpha1.OTLPProtocolsConfig{},
		}
	}
}

func (builder *DatadogAgentBuilder) WithOTLPGRPCSettings(enabled bool, endpoint string) *DatadogAgentBuilder {
	builder.initOTLP()
	builder.datadogAgent.Spec.Features.OTLP.Receiver.Protocols.GRPC = &v2alpha1.OTLPGRPCConfig{
		Enabled:  apiutils.NewBoolPointer(enabled),
		Endpoint: apiutils.NewStringPointer(endpoint),
	}
	return builder
}

func (builder *DatadogAgentBuilder) WithOTLPHTTPSettings(enabled bool, endpoint string) *DatadogAgentBuilder {
	builder.initOTLP()
	builder.datadogAgent.Spec.Features.OTLP.Receiver.Protocols.HTTP = &v2alpha1.OTLPHTTPConfig{
		Enabled:  apiutils.NewBoolPointer(enabled),
		Endpoint: apiutils.NewStringPointer(endpoint),
	}
	return builder
}

// NPM

func (builder *DatadogAgentBuilder) initNPM() {
	if builder.datadogAgent.Spec.Features.NPM == nil {
		builder.datadogAgent.Spec.Features.NPM = &v2alpha1.NPMFeatureConfig{}
	}
}

func (builder *DatadogAgentBuilder) WithNPMEnabled(enabled bool) *DatadogAgentBuilder {
	builder.initNPM()
	builder.datadogAgent.Spec.Features.NPM.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

// CSPM

func (builder *DatadogAgentBuilder) initCSPM() {
	if builder.datadogAgent.Spec.Features.CSPM == nil {
		builder.datadogAgent.Spec.Features.CSPM = &v2alpha1.CSPMFeatureConfig{}
	}
}

func (builder *DatadogAgentBuilder) WithCSPMEnabled(enabled bool) *DatadogAgentBuilder {
	builder.initCSPM()
	builder.datadogAgent.Spec.Features.CSPM.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

// CWS

func (builder *DatadogAgentBuilder) initCWS() {
	if builder.datadogAgent.Spec.Features.CWS == nil {
		builder.datadogAgent.Spec.Features.CWS = &v2alpha1.CWSFeatureConfig{}
	}
}

func (builder *DatadogAgentBuilder) WithCWSEnabled(enabled bool) *DatadogAgentBuilder {
	builder.initCWS()
	builder.datadogAgent.Spec.Features.CWS.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

// OOMKill

func (builder *DatadogAgentBuilder) initOOMKill() {
	if builder.datadogAgent.Spec.Features.OOMKill == nil {
		builder.datadogAgent.Spec.Features.OOMKill = &v2alpha1.OOMKillFeatureConfig{}
	}
}

func (builder *DatadogAgentBuilder) WithOOMKillEnabled(enabled bool) *DatadogAgentBuilder {
	builder.initOOMKill()
	builder.datadogAgent.Spec.Features.OOMKill.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

// Global Kubelet

func (builder *DatadogAgentBuilder) WithGlobalKubeletConfig(hostCAPath, agentCAPath string, tlsVerify bool) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Global.Kubelet = &common.KubeletConfig{
		TLSVerify:   apiutils.NewBoolPointer(tlsVerify),
		HostCAPath:  hostCAPath,
		AgentCAPath: agentCAPath,
	}
	return builder
}

func (builder *DatadogAgentBuilder) WithGlobalDockerSocketPath(dockerSocketPath string) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Global.DockerSocketPath = apiutils.NewStringPointer(dockerSocketPath)
	return builder
}

func (builder *DatadogAgentBuilder) WithGlobalCriSocketPath(criSocketPath string) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Global.DockerSocketPath = apiutils.NewStringPointer(criSocketPath)
	return builder
}

// Global ContainerStrategy

func (builder *DatadogAgentBuilder) WithSingleContainerStrategy(enabled bool) *DatadogAgentBuilder {
	if enabled {
		scs := v2alpha1.SingleContainerStrategy
		builder.datadogAgent.Spec.Global.ContainerStrategy = &scs
	} else {
		ocs := v2alpha1.OptimizedContainerStrategy
		builder.datadogAgent.Spec.Global.ContainerStrategy = &ocs
	}
	return builder
}

// Global Credentials

func (builder *DatadogAgentBuilder) WithCredentials(apiKey, appKey string) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Global.Credentials = &v2alpha1.DatadogCredentials{
		APIKey: utils.NewStringPointer(apiKey),
		AppKey: utils.NewStringPointer(appKey),
	}
	return builder
}

// Override

func (builder *DatadogAgentBuilder) WithComponentOverride(componentName v2alpha1.ComponentName, override v2alpha1.DatadogAgentComponentOverride) *DatadogAgentBuilder {
	if builder.datadogAgent.Spec.Override == nil {
		builder.datadogAgent.Spec.Override = map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{}
	}

	builder.datadogAgent.Spec.Override[componentName] = &override
	return builder
}

// FIPS

func (builder *DatadogAgentBuilder) WithFIPS(fipsConfig v2alpha1.FIPSConfig) *DatadogAgentBuilder {
	if builder.datadogAgent.Spec.Global == nil {
		builder.datadogAgent.Spec.Global = &v2alpha1.GlobalConfig{}
	}

	builder.datadogAgent.Spec.Global.FIPS = &fipsConfig
	return builder
}
