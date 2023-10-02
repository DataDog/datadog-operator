package test

import (
	"github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
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
	builder.datadogAgent.Spec.Features.Dogstatsd.HostPortConfig.Port = apiutils.NewInt32Pointer(1234)
	return builder
}

func (builder *DatadogAgentBuilder) WithDogstatsdOriginDetectionEnabled(enabled bool) *DatadogAgentBuilder {
	builder.initDogstatsd()
	builder.datadogAgent.Spec.Features.Dogstatsd.OriginDetectionEnabled = apiutils.NewBoolPointer(enabled)
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
