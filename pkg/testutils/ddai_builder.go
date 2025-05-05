// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutils

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/defaults"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/otelcollector/defaultconfig"
	"github.com/DataDog/datadog-operator/pkg/defaulting"
)

type DatadogAgentInternalBuilder struct {
	datadogAgentInternal v1alpha1.DatadogAgentInternal
}

// NewDatadogAgentInternalBuilder creates DatadogAgent and initializes Global, Features, Override properties
func NewDatadogAgentInternalBuilder() *DatadogAgentInternalBuilder {
	ddai := &v1alpha1.DatadogAgentInternal{
		ObjectMeta: metav1.ObjectMeta{},
		Spec: v2alpha1.DatadogAgentSpec{
			Global:   &v2alpha1.GlobalConfig{},
			Features: &v2alpha1.DatadogFeatures{},
			Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{},
		},
	}

	return &DatadogAgentInternalBuilder{
		datadogAgentInternal: *ddai,
	}
}

// NewDefaultDatadogAgentInternalBuilder created DatadogAgent and applies defaults
func NewDefaultDatadogAgentInternalBuilder() *DatadogAgentInternalBuilder {
	ddai := &v1alpha1.DatadogAgentInternal{}
	defaults.DefaultDatadogAgent(ddai)

	return &DatadogAgentInternalBuilder{
		datadogAgentInternal: *ddai,
	}
}

// NewDefaultDatadogAgentInternalBuilder initialized with name, namespace, creds and metadata
func NewInitializedDatadogAgentInternalBuilder(ns, name string) *DatadogAgentInternalBuilder {
	ddai := NewDatadogAgentInternal(ns, name, nil)
	ddai.Spec.Features = &v2alpha1.DatadogFeatures{}

	return &DatadogAgentInternalBuilder{
		datadogAgentInternal: *ddai,
	}
}

// Build returns DatadogAgent pointer with current properties
func (builder *DatadogAgentInternalBuilder) Build() *v1alpha1.DatadogAgentInternal {
	return &builder.datadogAgentInternal
}

// BuildWithDefaults applies defaults to current properties and returns resulting DatadogAgent
func (builder *DatadogAgentInternalBuilder) BuildWithDefaults() *v1alpha1.DatadogAgentInternal {
	defaults.DefaultDatadogAgent(&builder.datadogAgentInternal)
	return &builder.datadogAgentInternal
}

// Common
func (builder *DatadogAgentInternalBuilder) WithName(name string) *DatadogAgentInternalBuilder {
	builder.datadogAgentInternal.Name = name
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithAnnotations(annotations map[string]string) *DatadogAgentInternalBuilder {
	builder.datadogAgentInternal.ObjectMeta.Annotations = annotations
	return builder
}

// Global environment variable
func (builder *DatadogAgentInternalBuilder) WithEnvVars(envs []corev1.EnvVar) *DatadogAgentInternalBuilder {
	builder.datadogAgentInternal.Spec.Global.Env = envs
	return builder
}

// Dogstatsd
func (builder *DatadogAgentInternalBuilder) initDogstatsd() {
	if builder.datadogAgentInternal.Spec.Features.Dogstatsd == nil {
		builder.datadogAgentInternal.Spec.Features.Dogstatsd = &v2alpha1.DogstatsdFeatureConfig{}
		builder.datadogAgentInternal.Spec.Features.Dogstatsd.UnixDomainSocketConfig = &v2alpha1.UnixDomainSocketConfig{}
	}
}

func (builder *DatadogAgentInternalBuilder) WithDogstatsdHostPortEnabled(enabled bool) *DatadogAgentInternalBuilder {
	builder.initDogstatsd()
	builder.datadogAgentInternal.Spec.Features.Dogstatsd.HostPortConfig.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithDogstatsdHostPortConfig(port int32) *DatadogAgentInternalBuilder {
	builder.initDogstatsd()
	builder.datadogAgentInternal.Spec.Features.Dogstatsd.HostPortConfig.Port = apiutils.NewInt32Pointer(port)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithDogstatsdOriginDetectionEnabled(enabled bool) *DatadogAgentInternalBuilder {
	builder.initDogstatsd()
	builder.datadogAgentInternal.Spec.Features.Dogstatsd.OriginDetectionEnabled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithDogstatsdTagCardinality(cardinality string) *DatadogAgentInternalBuilder {
	builder.initDogstatsd()
	builder.datadogAgentInternal.Spec.Features.Dogstatsd.OriginDetectionEnabled = apiutils.NewBoolPointer(true)
	builder.datadogAgentInternal.Spec.Features.Dogstatsd.TagCardinality = apiutils.NewStringPointer(cardinality)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithDogstatsdUnixDomainSocketConfigEnabled(enabled bool) *DatadogAgentInternalBuilder {
	builder.initDogstatsd()
	builder.datadogAgentInternal.Spec.Features.Dogstatsd.UnixDomainSocketConfig.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithDogstatsdUnixDomainSocketConfigPath(customPath string) *DatadogAgentInternalBuilder {
	builder.initDogstatsd()
	builder.datadogAgentInternal.Spec.Features.Dogstatsd.UnixDomainSocketConfig.Path = apiutils.NewStringPointer(customPath)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithDogstatsdMapperProfiles(customMapperProfilesConf string) *DatadogAgentInternalBuilder {
	builder.initDogstatsd()
	builder.datadogAgentInternal.Spec.Features.Dogstatsd.MapperProfiles = &v2alpha1.CustomConfig{ConfigData: apiutils.NewStringPointer(customMapperProfilesConf)}
	return builder
}

// Live ContainerCollection

func (builder *DatadogAgentInternalBuilder) initLiveContainer() {
	if builder.datadogAgentInternal.Spec.Features.LiveContainerCollection == nil {
		builder.datadogAgentInternal.Spec.Features.LiveContainerCollection = &v2alpha1.LiveContainerCollectionFeatureConfig{}
	}
}

func (builder *DatadogAgentInternalBuilder) WithLiveContainerCollectionEnabled(enabled bool) *DatadogAgentInternalBuilder {
	builder.initLiveContainer()
	builder.datadogAgentInternal.Spec.Features.LiveContainerCollection.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

// Live Processes
func (builder *DatadogAgentInternalBuilder) initLiveProcesses() {
	if builder.datadogAgentInternal.Spec.Features.LiveProcessCollection == nil {
		builder.datadogAgentInternal.Spec.Features.LiveProcessCollection = &v2alpha1.LiveProcessCollectionFeatureConfig{}
	}
}

func (builder *DatadogAgentInternalBuilder) WithLiveProcessEnabled(enabled bool) *DatadogAgentInternalBuilder {
	builder.initLiveProcesses()
	builder.datadogAgentInternal.Spec.Features.LiveProcessCollection.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithLiveProcessScrubStrip(scrubEnabled, stripEnabled bool) *DatadogAgentInternalBuilder {
	builder.initLiveProcesses()
	builder.datadogAgentInternal.Spec.Features.LiveProcessCollection.ScrubProcessArguments = apiutils.NewBoolPointer(scrubEnabled)
	builder.datadogAgentInternal.Spec.Features.LiveProcessCollection.StripProcessArguments = apiutils.NewBoolPointer(stripEnabled)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithProcessChecksInCoreAgent(enabled bool) *DatadogAgentInternalBuilder {
	if builder.datadogAgentInternal.Spec.Global == nil {
		builder.datadogAgentInternal.Spec.Global = &v2alpha1.GlobalConfig{}
	}

	builder.datadogAgentInternal.Spec.Global.RunProcessChecksInCoreAgent = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithWorkloadAutoscalerEnabled(enabled bool) *DatadogAgentInternalBuilder {
	builder.datadogAgentInternal.Spec.Features.Autoscaling = &v2alpha1.AutoscalingFeatureConfig{
		Workload: &v2alpha1.WorkloadAutoscalingFeatureConfig{
			Enabled: apiutils.NewBoolPointer(enabled),
		},
	}

	return builder
}

// Admission Controller
func (builder *DatadogAgentInternalBuilder) initAdmissionController() {
	if builder.datadogAgentInternal.Spec.Features.AdmissionController == nil {
		builder.datadogAgentInternal.Spec.Features.AdmissionController = &v2alpha1.AdmissionControllerFeatureConfig{}
	}
	if builder.datadogAgentInternal.Spec.Features.AdmissionController.Validation == nil {
		builder.datadogAgentInternal.Spec.Features.AdmissionController.Validation = &v2alpha1.AdmissionControllerValidationConfig{}
	}
	if builder.datadogAgentInternal.Spec.Features.AdmissionController.Mutation == nil {
		builder.datadogAgentInternal.Spec.Features.AdmissionController.Mutation = &v2alpha1.AdmissionControllerMutationConfig{}
	}
	if builder.datadogAgentInternal.Spec.Features.AdmissionController.CWSInstrumentation == nil {
		builder.datadogAgentInternal.Spec.Features.AdmissionController.CWSInstrumentation = &v2alpha1.CWSInstrumentationConfig{}
	}
}

func (builder *DatadogAgentInternalBuilder) initSidecarInjection() {
	if builder.datadogAgentInternal.Spec.Features.AdmissionController.AgentSidecarInjection == nil {
		builder.datadogAgentInternal.Spec.Features.AdmissionController.AgentSidecarInjection = &v2alpha1.AgentSidecarInjectionConfig{}
	}
	if builder.datadogAgentInternal.Spec.Features.AdmissionController.AgentSidecarInjection.Image == nil {
		builder.datadogAgentInternal.Spec.Features.AdmissionController.AgentSidecarInjection.Image = &v2alpha1.AgentImageConfig{}
	}
}

func (builder *DatadogAgentInternalBuilder) WithAdmissionControllerEnabled(enabled bool) *DatadogAgentInternalBuilder {
	builder.initAdmissionController()
	builder.datadogAgentInternal.Spec.Features.AdmissionController.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithAdmissionControllerValidationEnabled(enabled bool) *DatadogAgentInternalBuilder {
	builder.initAdmissionController()
	builder.datadogAgentInternal.Spec.Features.AdmissionController.Validation.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithAdmissionControllerMutationEnabled(enabled bool) *DatadogAgentInternalBuilder {
	builder.initAdmissionController()
	builder.datadogAgentInternal.Spec.Features.AdmissionController.Mutation.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithAdmissionControllerMutateUnlabelled(enabled bool) *DatadogAgentInternalBuilder {
	builder.initAdmissionController()
	builder.datadogAgentInternal.Spec.Features.AdmissionController.MutateUnlabelled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithAdmissionControllerServiceName(name string) *DatadogAgentInternalBuilder {
	builder.initAdmissionController()
	builder.datadogAgentInternal.Spec.Features.AdmissionController.ServiceName = apiutils.NewStringPointer(name)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithAdmissionControllerAgentCommunicationMode(comMode string) *DatadogAgentInternalBuilder {
	builder.initAdmissionController()
	builder.datadogAgentInternal.Spec.Features.AdmissionController.AgentCommunicationMode = apiutils.NewStringPointer(comMode)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithAdmissionControllerFailurePolicy(policy string) *DatadogAgentInternalBuilder {
	builder.initAdmissionController()
	builder.datadogAgentInternal.Spec.Features.AdmissionController.FailurePolicy = apiutils.NewStringPointer(policy)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithAdmissionControllerWebhookName(name string) *DatadogAgentInternalBuilder {
	builder.initAdmissionController()
	builder.datadogAgentInternal.Spec.Features.AdmissionController.WebhookName = apiutils.NewStringPointer(name)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithAdmissionControllerRegistry(name string) *DatadogAgentInternalBuilder {
	builder.initAdmissionController()
	builder.datadogAgentInternal.Spec.Features.AdmissionController.Registry = apiutils.NewStringPointer(name)
	return builder
}

// sidecar Injection
func (builder *DatadogAgentInternalBuilder) WithSidecarInjectionEnabled(enabled bool) *DatadogAgentInternalBuilder {
	// builder.initAdmissionController()
	builder.initSidecarInjection()
	builder.datadogAgentInternal.Spec.Features.AdmissionController.AgentSidecarInjection.Enabled = apiutils.NewBoolPointer(enabled)
	if enabled {
		builder.datadogAgentInternal.Spec.Features.AdmissionController.AgentSidecarInjection.ClusterAgentCommunicationEnabled = apiutils.NewBoolPointer(enabled)
		builder.datadogAgentInternal.Spec.Features.AdmissionController.AgentSidecarInjection.Provider = apiutils.NewStringPointer("fargate")
		builder.datadogAgentInternal.Spec.Features.AdmissionController.AgentSidecarInjection.Image.Name = "agent"
		builder.datadogAgentInternal.Spec.Features.AdmissionController.AgentSidecarInjection.Image.Tag = defaulting.AgentLatestVersion
	}
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithSidecarInjectionClusterAgentCommunicationEnabled(enabled bool) *DatadogAgentInternalBuilder {
	builder.initAdmissionController()
	builder.initSidecarInjection()
	builder.datadogAgentInternal.Spec.Features.AdmissionController.AgentSidecarInjection.ClusterAgentCommunicationEnabled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithSidecarInjectionProvider(provider string) *DatadogAgentInternalBuilder {
	builder.initAdmissionController()
	builder.initSidecarInjection()
	builder.datadogAgentInternal.Spec.Features.AdmissionController.AgentSidecarInjection.Provider = apiutils.NewStringPointer(provider)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithSidecarInjectionRegistry(registry string) *DatadogAgentInternalBuilder {
	builder.initAdmissionController()
	builder.initSidecarInjection()
	builder.datadogAgentInternal.Spec.Features.AdmissionController.AgentSidecarInjection.Registry = apiutils.NewStringPointer(registry)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithSidecarInjectionImageName(name string) *DatadogAgentInternalBuilder {
	builder.initAdmissionController()
	builder.initSidecarInjection()
	if name != "" {
		builder.datadogAgentInternal.Spec.Features.AdmissionController.AgentSidecarInjection.Image.Name = name
	} else if builder.datadogAgentInternal.Spec.Override["nodeAgent"] != nil && builder.datadogAgentInternal.Spec.Override["nodeAgent"].Image != nil && builder.datadogAgentInternal.Spec.Override["nodeAgent"].Image.Name != "" {
		builder.datadogAgentInternal.Spec.Features.AdmissionController.AgentSidecarInjection.Image.Name = builder.datadogAgentInternal.Spec.Override["nodeAgent"].Image.Name
	}
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithSidecarInjectionImageTag(tag string) *DatadogAgentInternalBuilder {
	builder.initAdmissionController()
	builder.initSidecarInjection()
	if tag != "" {
		builder.datadogAgentInternal.Spec.Features.AdmissionController.AgentSidecarInjection.Image.Tag = tag
	} else if builder.datadogAgentInternal.Spec.Override["nodeAgent"] != nil && builder.datadogAgentInternal.Spec.Override["nodeAgent"].Image != nil && builder.datadogAgentInternal.Spec.Override["nodeAgent"].Image.Tag != "" {
		builder.datadogAgentInternal.Spec.Features.AdmissionController.AgentSidecarInjection.Image.Tag = builder.datadogAgentInternal.Spec.Override["nodeAgent"].Image.Tag
	}
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithSidecarInjectionSelectors(selectorKey, selectorValue string) *DatadogAgentInternalBuilder {
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

	builder.datadogAgentInternal.Spec.Features.AdmissionController.AgentSidecarInjection.Selectors = selectors
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithSidecarInjectionProfiles(envKey, envValue, resourceCPU, resourceMem string, securityContext *corev1.SecurityContext) *DatadogAgentInternalBuilder {
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
			SecurityContext: securityContext,
		},
	}

	builder.datadogAgentInternal.Spec.Features.AdmissionController.AgentSidecarInjection.Profiles = profiles
	return builder
}

// Process Discovery
func (builder *DatadogAgentInternalBuilder) initProcessDiscovery() {
	if builder.datadogAgentInternal.Spec.Features.ProcessDiscovery == nil {
		builder.datadogAgentInternal.Spec.Features.ProcessDiscovery = &v2alpha1.ProcessDiscoveryFeatureConfig{}
	}
}

func (builder *DatadogAgentInternalBuilder) WithProcessDiscoveryEnabled(enabled bool) *DatadogAgentInternalBuilder {
	builder.initProcessDiscovery()
	builder.datadogAgentInternal.Spec.Features.ProcessDiscovery.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

// OTel Agent
func (builder *DatadogAgentInternalBuilder) initOtelCollector() {
	if builder.datadogAgentInternal.Spec.Features.OtelCollector == nil {
		builder.datadogAgentInternal.Spec.Features.OtelCollector = &v2alpha1.OtelCollectorFeatureConfig{}
	}
}

func (builder *DatadogAgentInternalBuilder) WithOTelCollectorEnabled(enabled bool) *DatadogAgentInternalBuilder {
	builder.initOtelCollector()
	builder.datadogAgentInternal.Spec.Features.OtelCollector.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithOTelCollectorConfig() *DatadogAgentInternalBuilder {
	builder.datadogAgentInternal.Spec.Features.OtelCollector.Conf = &v2alpha1.CustomConfig{}
	builder.datadogAgentInternal.Spec.Features.OtelCollector.Conf.ConfigData =
		apiutils.NewStringPointer(defaultconfig.DefaultOtelCollectorConfig)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithOTelCollectorCoreConfigEnabled(enabled bool) *DatadogAgentInternalBuilder {
	if builder.datadogAgentInternal.Spec.Features.OtelCollector.CoreConfig == nil {
		builder.datadogAgentInternal.Spec.Features.OtelCollector.CoreConfig = &v2alpha1.CoreConfig{}
	}
	builder.datadogAgentInternal.Spec.Features.OtelCollector.CoreConfig.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithOTelCollectorCoreConfigExtensionTimeout(timeout int) *DatadogAgentInternalBuilder {
	if builder.datadogAgentInternal.Spec.Features.OtelCollector.CoreConfig == nil {
		builder.datadogAgentInternal.Spec.Features.OtelCollector.CoreConfig = &v2alpha1.CoreConfig{}
	}
	builder.datadogAgentInternal.Spec.Features.OtelCollector.CoreConfig.ExtensionTimeout = apiutils.NewIntPointer(timeout)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithOTelCollectorCoreConfigExtensionURL(url string) *DatadogAgentInternalBuilder {
	if builder.datadogAgentInternal.Spec.Features.OtelCollector.CoreConfig == nil {
		builder.datadogAgentInternal.Spec.Features.OtelCollector.CoreConfig = &v2alpha1.CoreConfig{}
	}
	builder.datadogAgentInternal.Spec.Features.OtelCollector.CoreConfig.ExtensionURL = apiutils.NewStringPointer(url)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithOTelCollectorConfigMap() *DatadogAgentInternalBuilder {
	builder.datadogAgentInternal.Spec.Features.OtelCollector.Conf = &v2alpha1.CustomConfig{}
	builder.datadogAgentInternal.Spec.Features.OtelCollector.Conf.ConfigMap = &v2alpha1.ConfigMapConfig{
		Name: "user-provided-config-map",
	}
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithOTelCollectorPorts(grpcPort int32, httpPort int32) *DatadogAgentInternalBuilder {
	builder.datadogAgentInternal.Spec.Features.OtelCollector.Ports = []*corev1.ContainerPort{
		{
			Name:          "otel-http",
			ContainerPort: httpPort,
			Protocol:      corev1.ProtocolTCP,
		},
		{
			Name:          "otel-grpc",
			ContainerPort: grpcPort,
			Protocol:      corev1.ProtocolTCP,
		},
	}
	return builder
}

// Log Collection
func (builder *DatadogAgentInternalBuilder) initLogCollection() {
	if builder.datadogAgentInternal.Spec.Features.LogCollection == nil {
		builder.datadogAgentInternal.Spec.Features.LogCollection = &v2alpha1.LogCollectionFeatureConfig{}
	}
}

func (builder *DatadogAgentInternalBuilder) WithLogCollectionEnabled(enabled bool) *DatadogAgentInternalBuilder {
	builder.initLogCollection()
	builder.datadogAgentInternal.Spec.Features.LogCollection.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithLogCollectionCollectAll(enabled bool) *DatadogAgentInternalBuilder {
	builder.initLogCollection()
	builder.datadogAgentInternal.Spec.Features.LogCollection.ContainerCollectAll = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithLogCollectionLogCollectionUsingFiles(enabled bool) *DatadogAgentInternalBuilder {
	builder.initLogCollection()
	builder.datadogAgentInternal.Spec.Features.LogCollection.ContainerCollectUsingFiles = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithLogCollectionOpenFilesLimit(limit int32) *DatadogAgentInternalBuilder {
	builder.initLogCollection()
	builder.datadogAgentInternal.Spec.Features.LogCollection.OpenFilesLimit = apiutils.NewInt32Pointer(limit)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithLogCollectionPaths(podLogs, containerLogs, containerSymlinks, tempStorate string) *DatadogAgentInternalBuilder {
	builder.initLogCollection()
	builder.datadogAgentInternal.Spec.Features.LogCollection.PodLogsPath = apiutils.NewStringPointer(podLogs)
	builder.datadogAgentInternal.Spec.Features.LogCollection.ContainerLogsPath = apiutils.NewStringPointer(containerLogs)
	builder.datadogAgentInternal.Spec.Features.LogCollection.ContainerSymlinksPath = apiutils.NewStringPointer(containerSymlinks)
	builder.datadogAgentInternal.Spec.Features.LogCollection.TempStoragePath = apiutils.NewStringPointer(tempStorate)
	return builder
}

// Event Collection
func (builder *DatadogAgentInternalBuilder) initEventCollection() {
	if builder.datadogAgentInternal.Spec.Features.EventCollection == nil {
		builder.datadogAgentInternal.Spec.Features.EventCollection = &v2alpha1.EventCollectionFeatureConfig{}
	}
}

func (builder *DatadogAgentInternalBuilder) WithEventCollectionKubernetesEvents(enabled bool) *DatadogAgentInternalBuilder {
	builder.initEventCollection()
	builder.datadogAgentInternal.Spec.Features.EventCollection.CollectKubernetesEvents = apiutils.NewBoolPointer(enabled)

	return builder
}

func (builder *DatadogAgentInternalBuilder) WithEventCollectionUnbundleEvents(enabled bool, eventTypes []v2alpha1.EventTypes) *DatadogAgentInternalBuilder {
	builder.initEventCollection()
	builder.datadogAgentInternal.Spec.Features.EventCollection.UnbundleEvents = apiutils.NewBoolPointer(enabled)
	builder.datadogAgentInternal.Spec.Features.EventCollection.CollectedEventTypes = eventTypes

	return builder
}

// Remote Config
func (builder *DatadogAgentInternalBuilder) initRemoteConfig() {
	if builder.datadogAgentInternal.Spec.Features.RemoteConfiguration == nil {
		builder.datadogAgentInternal.Spec.Features.RemoteConfiguration = &v2alpha1.RemoteConfigurationFeatureConfig{}
	}
}

func (builder *DatadogAgentInternalBuilder) WithRemoteConfigEnabled(enabled bool) *DatadogAgentInternalBuilder {
	builder.initRemoteConfig()
	builder.datadogAgentInternal.Spec.Features.RemoteConfiguration.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

// KSM

func (builder *DatadogAgentInternalBuilder) initKSM() {
	if builder.datadogAgentInternal.Spec.Features.KubeStateMetricsCore == nil {
		builder.datadogAgentInternal.Spec.Features.KubeStateMetricsCore = &v2alpha1.KubeStateMetricsCoreFeatureConfig{}
	}
}

func (builder *DatadogAgentInternalBuilder) WithKSMEnabled(enabled bool) *DatadogAgentInternalBuilder {
	builder.initKSM()
	builder.datadogAgentInternal.Spec.Features.KubeStateMetricsCore.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithKSMCustomConf(customData string) *DatadogAgentInternalBuilder {
	builder.initKSM()
	builder.datadogAgentInternal.Spec.Features.KubeStateMetricsCore.Conf = &v2alpha1.CustomConfig{
		ConfigData: apiutils.NewStringPointer(customData),
	}
	return builder
}

// Orchestrator Explorer

func (builder *DatadogAgentInternalBuilder) initOE() {
	if builder.datadogAgentInternal.Spec.Features.OrchestratorExplorer == nil {
		builder.datadogAgentInternal.Spec.Features.OrchestratorExplorer = &v2alpha1.OrchestratorExplorerFeatureConfig{}
	}
}

func (builder *DatadogAgentInternalBuilder) WithOrchestratorExplorerEnabled(enabled bool) *DatadogAgentInternalBuilder {
	builder.initOE()
	builder.datadogAgentInternal.Spec.Features.OrchestratorExplorer.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithOrchestratorExplorerScrubContainers(enabled bool) *DatadogAgentInternalBuilder {
	builder.initOE()
	builder.datadogAgentInternal.Spec.Features.OrchestratorExplorer.ScrubContainers = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithOrchestratorExplorerExtraTags(tags []string) *DatadogAgentInternalBuilder {
	builder.initOE()
	builder.datadogAgentInternal.Spec.Features.OrchestratorExplorer.ExtraTags = tags
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithOrchestratorExplorerDDUrl(ddUrl string) *DatadogAgentInternalBuilder {
	builder.initOE()
	builder.datadogAgentInternal.Spec.Features.OrchestratorExplorer.DDUrl = apiutils.NewStringPointer(ddUrl)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithOrchestratorExplorerCustomConfigData(customConfigData string) *DatadogAgentInternalBuilder {
	builder.initOE()
	builder.datadogAgentInternal.Spec.Features.OrchestratorExplorer.Conf = &v2alpha1.CustomConfig{
		ConfigData: &customConfigData,
	}
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithOrchestratorExplorerCustomResources(customResources []string) *DatadogAgentInternalBuilder {
	builder.initOE()
	builder.datadogAgentInternal.Spec.Features.OrchestratorExplorer.CustomResources = customResources
	return builder
}

// Cluster Checks

func (builder *DatadogAgentInternalBuilder) initCC() {
	if builder.datadogAgentInternal.Spec.Features.ClusterChecks == nil {
		builder.datadogAgentInternal.Spec.Features.ClusterChecks = &v2alpha1.ClusterChecksFeatureConfig{}
	}
}

func (builder *DatadogAgentInternalBuilder) WithClusterChecksEnabled(enabled bool) *DatadogAgentInternalBuilder {
	builder.initCC()
	builder.datadogAgentInternal.Spec.Features.ClusterChecks.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithClusterChecksUseCLCEnabled(enabled bool) *DatadogAgentInternalBuilder {
	builder.initCC()
	builder.datadogAgentInternal.Spec.Features.ClusterChecks.UseClusterChecksRunners = apiutils.NewBoolPointer(enabled)
	return builder
}

// Prometheus Scrape

func (builder *DatadogAgentInternalBuilder) initPrometheusScrape() {
	if builder.datadogAgentInternal.Spec.Features.PrometheusScrape == nil {
		builder.datadogAgentInternal.Spec.Features.PrometheusScrape = &v2alpha1.PrometheusScrapeFeatureConfig{}
	}
}

func (builder *DatadogAgentInternalBuilder) WithPrometheusScrapeEnabled(enabled bool) *DatadogAgentInternalBuilder {
	builder.initPrometheusScrape()
	builder.datadogAgentInternal.Spec.Features.PrometheusScrape.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithPrometheusScrapeServiceEndpoints(enabled bool) *DatadogAgentInternalBuilder {
	builder.initPrometheusScrape()
	builder.datadogAgentInternal.Spec.Features.PrometheusScrape.EnableServiceEndpoints = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithPrometheusScrapeAdditionalConfigs(additionalConfig string) *DatadogAgentInternalBuilder {
	builder.initPrometheusScrape()
	builder.datadogAgentInternal.Spec.Features.PrometheusScrape.AdditionalConfigs = apiutils.NewStringPointer(additionalConfig)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithPrometheusScrapeVersion(version int) *DatadogAgentInternalBuilder {
	builder.initPrometheusScrape()
	builder.datadogAgentInternal.Spec.Features.PrometheusScrape.Version = apiutils.NewIntPointer(version)
	return builder
}

// APM

func (builder *DatadogAgentInternalBuilder) initAPM() {
	if builder.datadogAgentInternal.Spec.Features.APM == nil {
		builder.datadogAgentInternal.Spec.Features.APM = &v2alpha1.APMFeatureConfig{}
	}
}

func (builder *DatadogAgentInternalBuilder) WithAPMEnabled(enabled bool) *DatadogAgentInternalBuilder {
	builder.initAPM()
	builder.datadogAgentInternal.Spec.Features.APM.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithErrorTrackingStandalone(enabled bool) *DatadogAgentInternalBuilder {
	builder.initAPM()
	builder.datadogAgentInternal.Spec.Features.APM.ErrorTrackingStandalone = &v2alpha1.ErrorTrackingStandalone{
		Enabled: apiutils.NewBoolPointer(enabled),
	}
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithAPMHostPortEnabled(enabled bool, port *int32) *DatadogAgentInternalBuilder {
	builder.initAPM()
	builder.datadogAgentInternal.Spec.Features.APM.HostPortConfig = &v2alpha1.HostPortConfig{
		Enabled: apiutils.NewBoolPointer(enabled),
	}
	if port != nil {
		builder.datadogAgentInternal.Spec.Features.APM.HostPortConfig.Port = port
	}
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithAPMUDSEnabled(enabled bool, apmSocketHostPath string) *DatadogAgentInternalBuilder {
	builder.initAPM()
	builder.datadogAgentInternal.Spec.Features.APM.UnixDomainSocketConfig = &v2alpha1.UnixDomainSocketConfig{
		Enabled: apiutils.NewBoolPointer(enabled),
		Path:    apiutils.NewStringPointer(apmSocketHostPath),
	}
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithClusterAgentTag(tag string) *DatadogAgentInternalBuilder {
	builder.datadogAgentInternal.Spec.Override[v2alpha1.ClusterAgentComponentName] = &v2alpha1.DatadogAgentComponentOverride{
		Image: &v2alpha1.AgentImageConfig{Tag: tag},
	}
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithAPMSingleStepInstrumentationEnabled(enabled bool, enabledNamespaces []string, disabledNamespaces []string, libVersion map[string]string, languageDetectionEnabled bool, injectorImageTag string, targets []v2alpha1.SSITarget) *DatadogAgentInternalBuilder {
	builder.initAPM()
	builder.datadogAgentInternal.Spec.Features.APM.SingleStepInstrumentation = &v2alpha1.SingleStepInstrumentation{
		Enabled:            apiutils.NewBoolPointer(enabled),
		EnabledNamespaces:  enabledNamespaces,
		DisabledNamespaces: disabledNamespaces,
		LibVersions:        libVersion,
		LanguageDetection:  &v2alpha1.LanguageDetectionConfig{Enabled: apiutils.NewBoolPointer(languageDetectionEnabled)},
		Injector: &v2alpha1.InjectorConfig{
			ImageTag: injectorImageTag,
		},
		Targets: targets,
	}
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithASMEnabled(threats, sca, iast bool) *DatadogAgentInternalBuilder {
	builder.datadogAgentInternal.Spec.Features.ASM = &v2alpha1.ASMFeatureConfig{
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

func (builder *DatadogAgentInternalBuilder) initOTLP() {
	if builder.datadogAgentInternal.Spec.Features.OTLP == nil {
		builder.datadogAgentInternal.Spec.Features.OTLP = &v2alpha1.OTLPFeatureConfig{}
		builder.datadogAgentInternal.Spec.Features.OTLP.Receiver = v2alpha1.OTLPReceiverConfig{
			Protocols: v2alpha1.OTLPProtocolsConfig{},
		}
	}
}

func (builder *DatadogAgentInternalBuilder) WithOTLPGRPCSettings(enabled bool, hostPortEnabled bool, customHostPort int32, endpoint string) *DatadogAgentInternalBuilder {
	builder.initOTLP()
	builder.datadogAgentInternal.Spec.Features.OTLP.Receiver.Protocols.GRPC = &v2alpha1.OTLPGRPCConfig{
		Enabled: apiutils.NewBoolPointer(enabled),
		HostPortConfig: &v2alpha1.HostPortConfig{
			Enabled: apiutils.NewBoolPointer(hostPortEnabled),
			Port:    apiutils.NewInt32Pointer(customHostPort),
		},
		Endpoint: apiutils.NewStringPointer(endpoint),
	}
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithOTLPHTTPSettings(enabled bool, hostPortEnabled bool, customHostPort int32, endpoint string) *DatadogAgentInternalBuilder {
	builder.initOTLP()
	builder.datadogAgentInternal.Spec.Features.OTLP.Receiver.Protocols.HTTP = &v2alpha1.OTLPHTTPConfig{
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

func (builder *DatadogAgentInternalBuilder) initNPM() {
	if builder.datadogAgentInternal.Spec.Features.NPM == nil {
		builder.datadogAgentInternal.Spec.Features.NPM = &v2alpha1.NPMFeatureConfig{}
	}
}

func (builder *DatadogAgentInternalBuilder) WithNPMEnabled(enabled bool) *DatadogAgentInternalBuilder {
	builder.initNPM()
	builder.datadogAgentInternal.Spec.Features.NPM.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

// CSPM

func (builder *DatadogAgentInternalBuilder) initCSPM() {
	if builder.datadogAgentInternal.Spec.Features.CSPM == nil {
		builder.datadogAgentInternal.Spec.Features.CSPM = &v2alpha1.CSPMFeatureConfig{}
	}
}

func (builder *DatadogAgentInternalBuilder) WithCSPMEnabled(enabled bool) *DatadogAgentInternalBuilder {
	builder.initCSPM()
	builder.datadogAgentInternal.Spec.Features.CSPM.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

// CWS

func (builder *DatadogAgentInternalBuilder) initCWS() {
	if builder.datadogAgentInternal.Spec.Features.CWS == nil {
		builder.datadogAgentInternal.Spec.Features.CWS = &v2alpha1.CWSFeatureConfig{}
	}
}

func (builder *DatadogAgentInternalBuilder) WithCWSEnabled(enabled bool) *DatadogAgentInternalBuilder {
	builder.initCWS()
	builder.datadogAgentInternal.Spec.Features.CWS.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

// cwsInstrumentation

func (builder *DatadogAgentInternalBuilder) initCWSInstrumentation() {
	if builder.datadogAgentInternal.Spec.Features.AdmissionController.CWSInstrumentation == nil {
		builder.datadogAgentInternal.Spec.Features.AdmissionController.CWSInstrumentation = &v2alpha1.CWSInstrumentationConfig{}
	}
}

func (builder *DatadogAgentInternalBuilder) WithCWSInstrumentationEnabled(enabled bool) *DatadogAgentInternalBuilder {
	builder.initCWSInstrumentation()
	builder.datadogAgentInternal.Spec.Features.AdmissionController.CWSInstrumentation.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithCWSInstrumentationMode(mode string) *DatadogAgentInternalBuilder {
	builder.initCWSInstrumentation()
	builder.datadogAgentInternal.Spec.Features.AdmissionController.CWSInstrumentation.Mode = apiutils.NewStringPointer(mode)
	return builder
}

// OOMKill

func (builder *DatadogAgentInternalBuilder) initOOMKill() {
	if builder.datadogAgentInternal.Spec.Features.OOMKill == nil {
		builder.datadogAgentInternal.Spec.Features.OOMKill = &v2alpha1.OOMKillFeatureConfig{}
	}
}

func (builder *DatadogAgentInternalBuilder) WithOOMKillEnabled(enabled bool) *DatadogAgentInternalBuilder {
	builder.initOOMKill()
	builder.datadogAgentInternal.Spec.Features.OOMKill.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

// Helm Check

func (builder *DatadogAgentInternalBuilder) initHelmCheck() {
	if builder.datadogAgentInternal.Spec.Features.HelmCheck == nil {
		builder.datadogAgentInternal.Spec.Features.HelmCheck = &v2alpha1.HelmCheckFeatureConfig{}
	}
}

func (builder *DatadogAgentInternalBuilder) WithHelmCheckEnabled(enabled bool) *DatadogAgentInternalBuilder {
	builder.initHelmCheck()
	builder.datadogAgentInternal.Spec.Features.HelmCheck.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithHelmCheckCollectEvents(enabled bool) *DatadogAgentInternalBuilder {
	builder.initHelmCheck()
	builder.datadogAgentInternal.Spec.Features.HelmCheck.CollectEvents = apiutils.NewBoolPointer(enabled)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithHelmCheckValuesAsTags(valuesAsTags map[string]string) *DatadogAgentInternalBuilder {
	builder.initHelmCheck()
	builder.datadogAgentInternal.Spec.Features.HelmCheck.ValuesAsTags = valuesAsTags
	return builder
}

// Global Kubelet

func (builder *DatadogAgentInternalBuilder) WithGlobalKubeletConfig(hostCAPath, agentCAPath string, tlsVerify bool, podResourcesSocketDir string) *DatadogAgentInternalBuilder {
	builder.datadogAgentInternal.Spec.Global.Kubelet = &v2alpha1.KubeletConfig{
		TLSVerify:              apiutils.NewBoolPointer(tlsVerify),
		HostCAPath:             hostCAPath,
		AgentCAPath:            agentCAPath,
		PodResourcesSocketPath: podResourcesSocketDir,
	}
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithGlobalDockerSocketPath(dockerSocketPath string) *DatadogAgentInternalBuilder {
	builder.datadogAgentInternal.Spec.Global.DockerSocketPath = apiutils.NewStringPointer(dockerSocketPath)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithGlobalCriSocketPath(criSocketPath string) *DatadogAgentInternalBuilder {
	builder.datadogAgentInternal.Spec.Global.DockerSocketPath = apiutils.NewStringPointer(criSocketPath)
	return builder
}

// Global ContainerStrategy

func (builder *DatadogAgentInternalBuilder) WithSingleContainerStrategy(enabled bool) *DatadogAgentInternalBuilder {
	if enabled {
		scs := v2alpha1.SingleContainerStrategy
		builder.datadogAgentInternal.Spec.Global.ContainerStrategy = &scs
	} else {
		ocs := v2alpha1.OptimizedContainerStrategy
		builder.datadogAgentInternal.Spec.Global.ContainerStrategy = &ocs
	}
	return builder
}

// Global Credentials

func (builder *DatadogAgentInternalBuilder) WithCredentials(apiKey, appKey string) *DatadogAgentInternalBuilder {
	builder.datadogAgentInternal.Spec.Global.Credentials = &v2alpha1.DatadogCredentials{
		APIKey: apiutils.NewStringPointer(apiKey),
		AppKey: apiutils.NewStringPointer(appKey),
	}
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithCredentialsFromSecret(apiSecretName, apiSecretKey, appSecretName, appSecretKey string) *DatadogAgentInternalBuilder {
	builder.datadogAgentInternal.Spec.Global.Credentials = &v2alpha1.DatadogCredentials{}
	if apiSecretName != "" && apiSecretKey != "" {
		builder.datadogAgentInternal.Spec.Global.Credentials.APISecret = &v2alpha1.SecretConfig{
			SecretName: apiSecretName,
			KeyName:    apiSecretKey,
		}
	}
	if appSecretName != "" && appSecretKey != "" {
		builder.datadogAgentInternal.Spec.Global.Credentials.AppSecret = &v2alpha1.SecretConfig{
			SecretName: appSecretName,
			KeyName:    appSecretKey,
		}
	}
	return builder
}

// Global DCA Token
func (builder *DatadogAgentInternalBuilder) WithDCAToken(token string) *DatadogAgentInternalBuilder {
	builder.datadogAgentInternal.Spec.Global.ClusterAgentToken = apiutils.NewStringPointer(token)
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithDCATokenFromSecret(secretName, secretKey string) *DatadogAgentInternalBuilder {
	builder.datadogAgentInternal.Spec.Global.ClusterAgentTokenSecret = &v2alpha1.SecretConfig{
		SecretName: secretName,
		KeyName:    secretKey,
	}
	return builder
}

// Global OriginDetectionUnified

func (builder *DatadogAgentInternalBuilder) WithOriginDetectionUnified(enabled bool) *DatadogAgentInternalBuilder {
	builder.datadogAgentInternal.Spec.Global.OriginDetectionUnified = &v2alpha1.OriginDetectionUnified{
		Enabled: apiutils.NewBoolPointer(enabled),
	}
	return builder
}

// Global Registry

func (builder *DatadogAgentInternalBuilder) WithRegistry(registry string) *DatadogAgentInternalBuilder {
	builder.datadogAgentInternal.Spec.Global.Registry = apiutils.NewStringPointer(registry)

	return builder
}

// Global ChecksTagCardinality

func (builder *DatadogAgentInternalBuilder) WithChecksTagCardinality(cardinality string) *DatadogAgentInternalBuilder {
	builder.datadogAgentInternal.Spec.Global.ChecksTagCardinality = apiutils.NewStringPointer(cardinality)
	return builder
}

// Global SecretBackend

func (builder *DatadogAgentInternalBuilder) WithGlobalSecretBackendGlobalPerms(command string, args string, timeout int32) *DatadogAgentInternalBuilder {
	builder.datadogAgentInternal.Spec.Global.SecretBackend = &v2alpha1.SecretBackendConfig{
		Command:                 apiutils.NewStringPointer(command),
		Args:                    apiutils.NewStringPointer(args),
		Timeout:                 apiutils.NewInt32Pointer(timeout),
		EnableGlobalPermissions: apiutils.NewBoolPointer(true),
	}
	return builder
}

func (builder *DatadogAgentInternalBuilder) WithGlobalSecretBackendSpecificRoles(command string, args string, timeout int32, secretNs string, secretNames []string) *DatadogAgentInternalBuilder {
	builder.datadogAgentInternal.Spec.Global.SecretBackend = &v2alpha1.SecretBackendConfig{
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

func (builder *DatadogAgentInternalBuilder) WithComponentOverride(componentName v2alpha1.ComponentName, override v2alpha1.DatadogAgentComponentOverride) *DatadogAgentInternalBuilder {
	if builder.datadogAgentInternal.Spec.Override == nil {
		builder.datadogAgentInternal.Spec.Override = map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{}
	}

	builder.datadogAgentInternal.Spec.Override[componentName] = &override
	return builder
}

// FIPS

func (builder *DatadogAgentInternalBuilder) WithFIPS(fipsConfig v2alpha1.FIPSConfig) *DatadogAgentInternalBuilder {
	if builder.datadogAgentInternal.Spec.Global == nil {
		builder.datadogAgentInternal.Spec.Global = &v2alpha1.GlobalConfig{}
	}

	builder.datadogAgentInternal.Spec.Global.FIPS = &fipsConfig
	return builder
}

// GPU

func (builder *DatadogAgentInternalBuilder) initGPUMonitoring() {
	if builder.datadogAgentInternal.Spec.Features.GPU == nil {
		builder.datadogAgentInternal.Spec.Features.GPU = &v2alpha1.GPUFeatureConfig{}
	}
}

func (builder *DatadogAgentInternalBuilder) WithGPUMonitoringEnabled(enabled bool) *DatadogAgentInternalBuilder {
	builder.initGPUMonitoring()
	builder.datadogAgentInternal.Spec.Features.GPU.Enabled = apiutils.NewBoolPointer(enabled)
	return builder
}
