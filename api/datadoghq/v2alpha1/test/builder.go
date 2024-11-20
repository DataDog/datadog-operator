// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package test

import (
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/api/utils"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	defaulting "github.com/DataDog/datadog-operator/pkg/defaulting"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DatadogAgentBuilder struct {
	datadogAgent v2alpha1.DatadogAgent
}

// NewDatadogAgentBuilder creates DatadogAgent and initializes Global, Features, Override properties
func NewDatadogAgentBuilder() *DatadogAgentBuilder {
	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{},
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

func (builder *DatadogAgentBuilder) WithAnnotations(annotations map[string]string) *DatadogAgentBuilder {
	builder.datadogAgent.ObjectMeta.Annotations = annotations
	return builder
}

// Global environment variable
func (builder *DatadogAgentBuilder) WithEnvVars(envs []corev1.EnvVar) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Global.Env = envs
	return builder
}

// Dogstatsd
func (builder *DatadogAgentBuilder) initDogstatsd() {
	if builder.datadogAgent.Spec.Features.Dogstatsd == nil {
		builder.datadogAgent.Spec.Features.Dogstatsd = &v2alpha1.DogstatsdFeatureConfig{}
		builder.datadogAgent.Spec.Features.Dogstatsd.UnixDomainSocketConfig = &v2alpha1.UnixDomainSocketConfig{}
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
	builder.initDogstatsd()
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

func (builder *DatadogAgentBuilder) WithProcessChecksInCoreAgent(enabled bool) *DatadogAgentBuilder {
	if builder.datadogAgent.Spec.Global == nil {
		builder.datadogAgent.Spec.Global = &v2alpha1.GlobalConfig{}
	}

	builder.datadogAgent.Spec.Global.RunProcessChecksInCoreAgent = apiutils.NewBoolPointer(enabled)
	return builder
}

// Admission Controller
func (builder *DatadogAgentBuilder) initAdmissionController() {
	if builder.datadogAgent.Spec.Features.AdmissionController == nil {
		builder.datadogAgent.Spec.Features.AdmissionController = &v2alpha1.AdmissionControllerFeatureConfig{}
	}
	if builder.datadogAgent.Spec.Features.AdmissionController.CWSInstrumentation == nil {
		builder.datadogAgent.Spec.Features.AdmissionController.CWSInstrumentation = &v2alpha1.CWSInstrumentationConfig{}
	}
}

func (builder *DatadogAgentBuilder) initSidecarInjection() {
	if builder.datadogAgent.Spec.Features.AdmissionController.AgentSidecarInjection == nil {
		builder.datadogAgent.Spec.Features.AdmissionController.AgentSidecarInjection = &v2alpha1.AgentSidecarInjectionConfig{}
	}
	if builder.datadogAgent.Spec.Features.AdmissionController.AgentSidecarInjection.Image == nil {
		builder.datadogAgent.Spec.Features.AdmissionController.AgentSidecarInjection.Image = &v2alpha1.AgentImageConfig{}
	}
}

func (builder *DatadogAgentBuilder) WithAdmissionControllerEnabled(enabled bool) *DatadogAgentBuilder {
	builder.initAdmissionController()
	builder.datadogAgent.Spec.Features.AdmissionController.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithAdmissionControllerMutateUnlabelled(enabled bool) *DatadogAgentBuilder {
	builder.initAdmissionController()
	builder.datadogAgent.Spec.Features.AdmissionController.MutateUnlabelled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithAdmissionControllerServiceName(name string) *DatadogAgentBuilder {
	builder.initAdmissionController()
	builder.datadogAgent.Spec.Features.AdmissionController.ServiceName = apiutils.NewStringPointer(name)
	return builder
}

func (builder *DatadogAgentBuilder) WithAdmissionControllerAgentCommunicationMode(comMode string) *DatadogAgentBuilder {
	builder.initAdmissionController()
	builder.datadogAgent.Spec.Features.AdmissionController.AgentCommunicationMode = apiutils.NewStringPointer(comMode)
	return builder
}

func (builder *DatadogAgentBuilder) WithAdmissionControllerFailurePolicy(policy string) *DatadogAgentBuilder {
	builder.initAdmissionController()
	builder.datadogAgent.Spec.Features.AdmissionController.FailurePolicy = apiutils.NewStringPointer(policy)
	return builder
}

func (builder *DatadogAgentBuilder) WithAdmissionControllerWebhookName(name string) *DatadogAgentBuilder {
	builder.initAdmissionController()
	builder.datadogAgent.Spec.Features.AdmissionController.WebhookName = apiutils.NewStringPointer(name)
	return builder
}

func (builder *DatadogAgentBuilder) WithAdmissionControllerRegistry(name string) *DatadogAgentBuilder {
	builder.initAdmissionController()
	builder.datadogAgent.Spec.Features.AdmissionController.Registry = apiutils.NewStringPointer(name)
	return builder
}

// sidecar Injection
func (builder *DatadogAgentBuilder) WithSidecarInjectionEnabled(enabled bool) *DatadogAgentBuilder {
	// builder.initAdmissionController()
	builder.initSidecarInjection()
	builder.datadogAgent.Spec.Features.AdmissionController.AgentSidecarInjection.Enabled = apiutils.NewBoolPointer(enabled)
	if enabled {
		builder.datadogAgent.Spec.Features.AdmissionController.AgentSidecarInjection.ClusterAgentCommunicationEnabled = apiutils.NewBoolPointer(enabled)
		builder.datadogAgent.Spec.Features.AdmissionController.AgentSidecarInjection.Provider = apiutils.NewStringPointer("fargate")
		builder.datadogAgent.Spec.Features.AdmissionController.AgentSidecarInjection.Image.Name = "agent"
		builder.datadogAgent.Spec.Features.AdmissionController.AgentSidecarInjection.Image.Tag = defaulting.AgentLatestVersion
	}
	return builder
}

func (builder *DatadogAgentBuilder) WithSidecarInjectionClusterAgentCommunicationEnabled(enabled bool) *DatadogAgentBuilder {
	builder.initAdmissionController()
	builder.initSidecarInjection()
	builder.datadogAgent.Spec.Features.AdmissionController.AgentSidecarInjection.ClusterAgentCommunicationEnabled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithSidecarInjectionProvider(provider string) *DatadogAgentBuilder {
	builder.initAdmissionController()
	builder.initSidecarInjection()
	builder.datadogAgent.Spec.Features.AdmissionController.AgentSidecarInjection.Provider = apiutils.NewStringPointer(provider)
	return builder
}

func (builder *DatadogAgentBuilder) WithSidecarInjectionRegistry(registry string) *DatadogAgentBuilder {
	builder.initAdmissionController()
	builder.initSidecarInjection()
	builder.datadogAgent.Spec.Features.AdmissionController.AgentSidecarInjection.Registry = apiutils.NewStringPointer(registry)
	return builder
}

func (builder *DatadogAgentBuilder) WithSidecarInjectionImageName(name string) *DatadogAgentBuilder {
	builder.initAdmissionController()
	builder.initSidecarInjection()
	if name != "" {
		builder.datadogAgent.Spec.Features.AdmissionController.AgentSidecarInjection.Image.Name = name
	} else if builder.datadogAgent.Spec.Override["nodeAgent"] != nil && builder.datadogAgent.Spec.Override["nodeAgent"].Image != nil && builder.datadogAgent.Spec.Override["nodeAgent"].Image.Name != "" {
		builder.datadogAgent.Spec.Features.AdmissionController.AgentSidecarInjection.Image.Name = builder.datadogAgent.Spec.Override["nodeAgent"].Image.Name
	}
	return builder
}

func (builder *DatadogAgentBuilder) WithSidecarInjectionImageTag(tag string) *DatadogAgentBuilder {
	builder.initAdmissionController()
	builder.initSidecarInjection()
	if tag != "" {
		builder.datadogAgent.Spec.Features.AdmissionController.AgentSidecarInjection.Image.Tag = tag
	} else if builder.datadogAgent.Spec.Override["nodeAgent"] != nil && builder.datadogAgent.Spec.Override["nodeAgent"].Image != nil && builder.datadogAgent.Spec.Override["nodeAgent"].Image.Tag != "" {
		builder.datadogAgent.Spec.Features.AdmissionController.AgentSidecarInjection.Image.Tag = builder.datadogAgent.Spec.Override["nodeAgent"].Image.Tag
	}
	return builder
}

func (builder *DatadogAgentBuilder) WithSidecarInjectionSelectors(selectorKey, selectorValue string) *DatadogAgentBuilder {
	builder.initAdmissionController()
	builder.initSidecarInjection()

	selectors := []*v2alpha1.Selector{
		{
			NamespaceSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					selectorKey: selectorValue,
				},
			},
			ObjectSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					selectorKey: selectorValue,
				},
			},
		},
	}

	builder.datadogAgent.Spec.Features.AdmissionController.AgentSidecarInjection.Selectors = selectors
	return builder
}

func (builder *DatadogAgentBuilder) WithSidecarInjectionProfiles(envKey, envValue, resourceCPU, resourceMem string) *DatadogAgentBuilder {
	builder.initAdmissionController()
	builder.initSidecarInjection()

	profiles := []*v2alpha1.Profile{
		{
			EnvVars: []corev1.EnvVar{
				{
					Name:  envKey,
					Value: envValue,
				},
			},
			ResourceRequirements: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse(resourceCPU),
					corev1.ResourceMemory: resource.MustParse(resourceMem),
				},
			},
		},
	}

	builder.datadogAgent.Spec.Features.AdmissionController.AgentSidecarInjection.Profiles = profiles
	return builder
}

// Process Discovery
func (builder *DatadogAgentBuilder) initProcessDiscovery() {
	if builder.datadogAgent.Spec.Features.ProcessDiscovery == nil {
		builder.datadogAgent.Spec.Features.ProcessDiscovery = &v2alpha1.ProcessDiscoveryFeatureConfig{}
	}
}

func (builder *DatadogAgentBuilder) WithProcessDiscoveryEnabled(enabled bool) *DatadogAgentBuilder {
	builder.initProcessDiscovery()
	builder.datadogAgent.Spec.Features.ProcessDiscovery.Enabled = apiutils.NewBoolPointer(enabled)
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

func (builder *DatadogAgentBuilder) WithEventCollectionUnbundleEvents(enabled bool, eventTypes []v2alpha1.EventTypes) *DatadogAgentBuilder {
	builder.initEventCollection()
	builder.datadogAgent.Spec.Features.EventCollection.UnbundleEvents = apiutils.NewBoolPointer(enabled)
	builder.datadogAgent.Spec.Features.EventCollection.CollectedEventTypes = eventTypes

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

func (builder *DatadogAgentBuilder) WithAPMSingleStepInstrumentationEnabled(enabled bool, enabledNamespaces []string, disabledNamespaces []string, libVersion map[string]string, languageDetectionEnabled bool) *DatadogAgentBuilder {
	builder.initAPM()
	builder.datadogAgent.Spec.Features.APM.SingleStepInstrumentation = &v2alpha1.SingleStepInstrumentation{
		Enabled:            apiutils.NewBoolPointer(enabled),
		EnabledNamespaces:  enabledNamespaces,
		DisabledNamespaces: disabledNamespaces,
		LibVersions:        libVersion,
		LanguageDetection:  &v2alpha1.LanguageDetectionConfig{Enabled: apiutils.NewBoolPointer(languageDetectionEnabled)},
	}
	return builder
}

func (builder *DatadogAgentBuilder) WithASMEnabled(threats, sca, iast bool) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Features.ASM = &v2alpha1.ASMFeatureConfig{
		Threats: &v2alpha1.ASMThreatsConfig{
			Enabled: apiutils.NewBoolPointer(threats),
		},
		SCA: &v2alpha1.ASMSCAConfig{
			Enabled: apiutils.NewBoolPointer(sca),
		},
		IAST: &v2alpha1.ASMIASTConfig{
			Enabled: apiutils.NewBoolPointer(iast),
		},
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

func (builder *DatadogAgentBuilder) WithOTLPGRPCSettings(enabled bool, hostPortEnabled bool, customHostPort int32, endpoint string) *DatadogAgentBuilder {
	builder.initOTLP()
	builder.datadogAgent.Spec.Features.OTLP.Receiver.Protocols.GRPC = &v2alpha1.OTLPGRPCConfig{
		Enabled: apiutils.NewBoolPointer(enabled),
		HostPortConfig: &v2alpha1.HostPortConfig{
			Enabled: apiutils.NewBoolPointer(hostPortEnabled),
			Port:    apiutils.NewInt32Pointer(customHostPort),
		},
		Endpoint: apiutils.NewStringPointer(endpoint),
	}
	return builder
}

func (builder *DatadogAgentBuilder) WithOTLPHTTPSettings(enabled bool, hostPortEnabled bool, customHostPort int32, endpoint string) *DatadogAgentBuilder {
	builder.initOTLP()
	builder.datadogAgent.Spec.Features.OTLP.Receiver.Protocols.HTTP = &v2alpha1.OTLPHTTPConfig{
		Enabled: apiutils.NewBoolPointer(enabled),
		HostPortConfig: &v2alpha1.HostPortConfig{
			Enabled: apiutils.NewBoolPointer(hostPortEnabled),
			Port:    apiutils.NewInt32Pointer(customHostPort),
		},
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

// cwsInstrumentation

func (builder *DatadogAgentBuilder) initCWSInstrumentation() {
	if builder.datadogAgent.Spec.Features.AdmissionController.CWSInstrumentation == nil {
		builder.datadogAgent.Spec.Features.AdmissionController.CWSInstrumentation = &v2alpha1.CWSInstrumentationConfig{}
	}
}

func (builder *DatadogAgentBuilder) WithCWSInstrumentationEnabled(enabled bool) *DatadogAgentBuilder {
	builder.initCWSInstrumentation()
	builder.datadogAgent.Spec.Features.AdmissionController.CWSInstrumentation.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithCWSInstrumentationMode(mode string) *DatadogAgentBuilder {
	builder.initCWSInstrumentation()
	builder.datadogAgent.Spec.Features.AdmissionController.CWSInstrumentation.Mode = apiutils.NewStringPointer(mode)
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

// Helm Check

func (builder *DatadogAgentBuilder) initHelmCheck() {
	if builder.datadogAgent.Spec.Features.HelmCheck == nil {
		builder.datadogAgent.Spec.Features.HelmCheck = &v2alpha1.HelmCheckFeatureConfig{}
	}
}

func (builder *DatadogAgentBuilder) WithHelmCheckEnabled(enabled bool) *DatadogAgentBuilder {
	builder.initHelmCheck()
	builder.datadogAgent.Spec.Features.HelmCheck.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithHelmCheckCollectEvents(enabled bool) *DatadogAgentBuilder {
	builder.initHelmCheck()
	builder.datadogAgent.Spec.Features.HelmCheck.CollectEvents = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithHelmCheckValuesAsTags(valuesAsTags map[string]string) *DatadogAgentBuilder {
	builder.initHelmCheck()
	builder.datadogAgent.Spec.Features.HelmCheck.ValuesAsTags = valuesAsTags
	return builder
}

// Global Kubelet

func (builder *DatadogAgentBuilder) WithGlobalKubeletConfig(hostCAPath, agentCAPath string, tlsVerify bool) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Global.Kubelet = &v2alpha1.KubeletConfig{
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

// Global OriginDetectionUnified

func (builder *DatadogAgentBuilder) WithOriginDetectionUnified(enabled bool) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Global.OriginDetectionUnified = &v2alpha1.OriginDetectionUnified{
		Enabled: apiutils.NewBoolPointer(enabled),
	}
	return builder
}

// Global OriginDetectionUnified

func (builder *DatadogAgentBuilder) WithRegistry(registry string) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Global.Registry = apiutils.NewStringPointer(registry)

	return builder
}

// Global SecretBackend

func (builder *DatadogAgentBuilder) WithGlobalSecretBackendGlobalPerms(command string, args string, timeout int32) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Global.SecretBackend = &v2alpha1.SecretBackendConfig{
		Command:                 apiutils.NewStringPointer(command),
		Args:                    apiutils.NewStringPointer(args),
		Timeout:                 apiutils.NewInt32Pointer(timeout),
		EnableGlobalPermissions: apiutils.NewBoolPointer(true),
	}
	return builder
}

func (builder *DatadogAgentBuilder) WithGlobalSecretBackendSpecificRoles(command string, args string, timeout int32, secretNs string, secretNames []string) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Global.SecretBackend = &v2alpha1.SecretBackendConfig{
		Command:                 apiutils.NewStringPointer(command),
		Args:                    apiutils.NewStringPointer(args),
		Timeout:                 apiutils.NewInt32Pointer(timeout),
		EnableGlobalPermissions: apiutils.NewBoolPointer(false),
		Roles: []*v2alpha1.SecretBackendRolesConfig{
			{
				Namespace: apiutils.NewStringPointer(secretNs),
				Secrets:   secretNames,
			},
		},
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
