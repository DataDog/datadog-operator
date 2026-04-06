// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutils

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/defaults"
	otelagentgatewaydefaultconfig "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/otelagentgateway/defaultconfig"
	otelcollectordefaultconfig "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/otelcollector/defaultconfig"
	"github.com/DataDog/datadog-operator/pkg/images"
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
	defaults.DefaultDatadogAgentSpec(&dda.Spec)

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
	defaults.DefaultDatadogAgentSpec(&builder.datadogAgent.Spec)
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
	builder.datadogAgent.Spec.Features.Dogstatsd.HostPortConfig.Enabled = ptr.To(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithDogstatsdHostPortConfig(port int32) *DatadogAgentBuilder {
	builder.initDogstatsd()
	builder.datadogAgent.Spec.Features.Dogstatsd.HostPortConfig.Port = ptr.To(port)
	return builder
}

func (builder *DatadogAgentBuilder) WithDogstatsdOriginDetectionEnabled(enabled bool) *DatadogAgentBuilder {
	builder.initDogstatsd()
	builder.datadogAgent.Spec.Features.Dogstatsd.OriginDetectionEnabled = ptr.To(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithDogstatsdTagCardinality(cardinality string) *DatadogAgentBuilder {
	builder.initDogstatsd()
	builder.datadogAgent.Spec.Features.Dogstatsd.OriginDetectionEnabled = ptr.To(true)
	builder.datadogAgent.Spec.Features.Dogstatsd.TagCardinality = ptr.To(cardinality)
	return builder
}

func (builder *DatadogAgentBuilder) WithDogstatsdUnixDomainSocketConfigEnabled(enabled bool) *DatadogAgentBuilder {
	builder.initDogstatsd()
	builder.datadogAgent.Spec.Features.Dogstatsd.UnixDomainSocketConfig.Enabled = ptr.To(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithDogstatsdUnixDomainSocketConfigPath(customPath string) *DatadogAgentBuilder {
	builder.initDogstatsd()
	builder.datadogAgent.Spec.Features.Dogstatsd.UnixDomainSocketConfig.Path = ptr.To(customPath)
	return builder
}

func (builder *DatadogAgentBuilder) WithDogstatsdMapperProfiles(customMapperProfilesConf string) *DatadogAgentBuilder {
	builder.initDogstatsd()
	builder.datadogAgent.Spec.Features.Dogstatsd.MapperProfiles = &v2alpha1.CustomConfig{ConfigData: ptr.To(customMapperProfilesConf)}
	return builder
}

func (builder *DatadogAgentBuilder) WithDogstatsdNonLocalTraffic(enabled bool) *DatadogAgentBuilder {
	builder.initDogstatsd()
	builder.datadogAgent.Spec.Features.Dogstatsd.NonLocalTraffic = ptr.To(enabled)
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
	builder.datadogAgent.Spec.Features.LiveContainerCollection.Enabled = ptr.To(enabled)
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
	builder.datadogAgent.Spec.Features.LiveProcessCollection.Enabled = ptr.To(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithLiveProcessScrubStrip(scrubEnabled, stripEnabled bool) *DatadogAgentBuilder {
	builder.initLiveProcesses()
	builder.datadogAgent.Spec.Features.LiveProcessCollection.ScrubProcessArguments = ptr.To(scrubEnabled)
	builder.datadogAgent.Spec.Features.LiveProcessCollection.StripProcessArguments = ptr.To(stripEnabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithWorkloadAutoscalerEnabled(enabled bool) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Features.Autoscaling = &v2alpha1.AutoscalingFeatureConfig{
		Workload: &v2alpha1.WorkloadAutoscalingFeatureConfig{
			Enabled: ptr.To(enabled),
		},
	}

	return builder
}

// Admission Controller
func (builder *DatadogAgentBuilder) initAdmissionController() {
	if builder.datadogAgent.Spec.Features.AdmissionController == nil {
		builder.datadogAgent.Spec.Features.AdmissionController = &v2alpha1.AdmissionControllerFeatureConfig{}
	}
	if builder.datadogAgent.Spec.Features.AdmissionController.Validation == nil {
		builder.datadogAgent.Spec.Features.AdmissionController.Validation = &v2alpha1.AdmissionControllerValidationConfig{}
	}
	if builder.datadogAgent.Spec.Features.AdmissionController.Mutation == nil {
		builder.datadogAgent.Spec.Features.AdmissionController.Mutation = &v2alpha1.AdmissionControllerMutationConfig{}
	}
	if builder.datadogAgent.Spec.Features.AdmissionController.CWSInstrumentation == nil {
		builder.datadogAgent.Spec.Features.AdmissionController.CWSInstrumentation = &v2alpha1.CWSInstrumentationConfig{}
	}
	if builder.datadogAgent.Spec.Features.AdmissionController.Probe == nil {
		builder.datadogAgent.Spec.Features.AdmissionController.Probe = &v2alpha1.AdmissionControllerProbeConfig{}
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
	builder.datadogAgent.Spec.Features.AdmissionController.Enabled = ptr.To(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithAdmissionControllerValidationEnabled(enabled bool) *DatadogAgentBuilder {
	builder.initAdmissionController()
	builder.datadogAgent.Spec.Features.AdmissionController.Validation.Enabled = ptr.To(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithAdmissionControllerMutationEnabled(enabled bool) *DatadogAgentBuilder {
	builder.initAdmissionController()
	builder.datadogAgent.Spec.Features.AdmissionController.Mutation.Enabled = ptr.To(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithAdmissionControllerMutateUnlabelled(enabled bool) *DatadogAgentBuilder {
	builder.initAdmissionController()
	builder.datadogAgent.Spec.Features.AdmissionController.MutateUnlabelled = ptr.To(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithAdmissionControllerServiceName(name string) *DatadogAgentBuilder {
	builder.initAdmissionController()
	builder.datadogAgent.Spec.Features.AdmissionController.ServiceName = ptr.To(name)
	return builder
}

func (builder *DatadogAgentBuilder) WithAdmissionControllerAgentCommunicationMode(comMode string) *DatadogAgentBuilder {
	builder.initAdmissionController()
	builder.datadogAgent.Spec.Features.AdmissionController.AgentCommunicationMode = ptr.To(comMode)
	return builder
}

func (builder *DatadogAgentBuilder) WithAdmissionControllerFailurePolicy(policy string) *DatadogAgentBuilder {
	builder.initAdmissionController()
	builder.datadogAgent.Spec.Features.AdmissionController.FailurePolicy = ptr.To(policy)
	return builder
}

func (builder *DatadogAgentBuilder) WithAdmissionControllerWebhookName(name string) *DatadogAgentBuilder {
	builder.initAdmissionController()
	builder.datadogAgent.Spec.Features.AdmissionController.WebhookName = ptr.To(name)
	return builder
}

func (builder *DatadogAgentBuilder) WithAdmissionControllerRegistry(name string) *DatadogAgentBuilder {
	builder.initAdmissionController()
	builder.datadogAgent.Spec.Features.AdmissionController.Registry = ptr.To(name)
	return builder
}

func (builder *DatadogAgentBuilder) WithAdmissionControllerProbeEnabled(enabled bool) *DatadogAgentBuilder {
	builder.initAdmissionController()
	builder.datadogAgent.Spec.Features.AdmissionController.Probe.Enabled = ptr.To(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithAdmissionControllerProbeInterval(interval int32) *DatadogAgentBuilder {
	builder.initAdmissionController()
	builder.datadogAgent.Spec.Features.AdmissionController.Probe.Interval = &interval
	return builder
}

func (builder *DatadogAgentBuilder) WithAdmissionControllerProbeGracePeriod(gracePeriod int32) *DatadogAgentBuilder {
	builder.initAdmissionController()
	builder.datadogAgent.Spec.Features.AdmissionController.Probe.GracePeriod = &gracePeriod
	return builder
}

// sidecar Injection
func (builder *DatadogAgentBuilder) WithSidecarInjectionEnabled(enabled bool) *DatadogAgentBuilder {
	// builder.initAdmissionController()
	builder.initSidecarInjection()
	builder.datadogAgent.Spec.Features.AdmissionController.AgentSidecarInjection.Enabled = ptr.To(enabled)
	if enabled {
		builder.datadogAgent.Spec.Features.AdmissionController.AgentSidecarInjection.ClusterAgentCommunicationEnabled = ptr.To(enabled)
		builder.datadogAgent.Spec.Features.AdmissionController.AgentSidecarInjection.Provider = ptr.To("fargate")
		builder.datadogAgent.Spec.Features.AdmissionController.AgentSidecarInjection.Image.Name = "agent"
		builder.datadogAgent.Spec.Features.AdmissionController.AgentSidecarInjection.Image.Tag = images.AgentLatestVersion
	}
	return builder
}

func (builder *DatadogAgentBuilder) WithSidecarInjectionClusterAgentCommunicationEnabled(enabled bool) *DatadogAgentBuilder {
	builder.initAdmissionController()
	builder.initSidecarInjection()
	builder.datadogAgent.Spec.Features.AdmissionController.AgentSidecarInjection.ClusterAgentCommunicationEnabled = ptr.To(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithSidecarInjectionProvider(provider string) *DatadogAgentBuilder {
	builder.initAdmissionController()
	builder.initSidecarInjection()
	builder.datadogAgent.Spec.Features.AdmissionController.AgentSidecarInjection.Provider = ptr.To(provider)
	return builder
}

func (builder *DatadogAgentBuilder) WithSidecarInjectionRegistry(registry string) *DatadogAgentBuilder {
	builder.initAdmissionController()
	builder.initSidecarInjection()
	builder.datadogAgent.Spec.Features.AdmissionController.AgentSidecarInjection.Registry = ptr.To(registry)
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

func (builder *DatadogAgentBuilder) WithSidecarInjectionProfiles(envKey, envValue, resourceCPU, resourceMem string, securityContext *corev1.SecurityContext) *DatadogAgentBuilder {
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
	builder.datadogAgent.Spec.Features.ProcessDiscovery.Enabled = ptr.To(enabled)
	return builder
}

// OTel Agent
func (builder *DatadogAgentBuilder) initOtelCollector() {
	if builder.datadogAgent.Spec.Features.OtelCollector == nil {
		builder.datadogAgent.Spec.Features.OtelCollector = &v2alpha1.OtelCollectorFeatureConfig{}
	}
}

func (builder *DatadogAgentBuilder) WithOTelCollectorEnabled(enabled bool) *DatadogAgentBuilder {
	builder.initOtelCollector()
	builder.datadogAgent.Spec.Features.OtelCollector.Enabled = ptr.To(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithOTelCollectorConfig() *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Features.OtelCollector.Conf = &v2alpha1.CustomConfig{}
	builder.datadogAgent.Spec.Features.OtelCollector.Conf.ConfigData = ptr.To(otelcollectordefaultconfig.DefaultOtelCollectorConfig)
	return builder
}

func (builder *DatadogAgentBuilder) WithOTelCollectorCoreConfigEnabled(enabled bool) *DatadogAgentBuilder {
	if builder.datadogAgent.Spec.Features.OtelCollector.CoreConfig == nil {
		builder.datadogAgent.Spec.Features.OtelCollector.CoreConfig = &v2alpha1.CoreConfig{}
	}
	builder.datadogAgent.Spec.Features.OtelCollector.CoreConfig.Enabled = ptr.To(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithOTelCollectorCoreConfigExtensionTimeout(timeout int) *DatadogAgentBuilder {
	if builder.datadogAgent.Spec.Features.OtelCollector.CoreConfig == nil {
		builder.datadogAgent.Spec.Features.OtelCollector.CoreConfig = &v2alpha1.CoreConfig{}
	}
	builder.datadogAgent.Spec.Features.OtelCollector.CoreConfig.ExtensionTimeout = ptr.To(timeout)
	return builder
}

func (builder *DatadogAgentBuilder) WithOTelCollectorCoreConfigExtensionURL(url string) *DatadogAgentBuilder {
	if builder.datadogAgent.Spec.Features.OtelCollector.CoreConfig == nil {
		builder.datadogAgent.Spec.Features.OtelCollector.CoreConfig = &v2alpha1.CoreConfig{}
	}
	builder.datadogAgent.Spec.Features.OtelCollector.CoreConfig.ExtensionURL = ptr.To(url)
	return builder
}

func (builder *DatadogAgentBuilder) WithOTelCollectorConfigMap() *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Features.OtelCollector.Conf = &v2alpha1.CustomConfig{}
	builder.datadogAgent.Spec.Features.OtelCollector.Conf.ConfigMap = &v2alpha1.ConfigMapConfig{
		Name: "user-provided-config-map",
	}
	return builder
}

func (builder *DatadogAgentBuilder) WithOTelCollectorConfigMapMultipleItems() *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Features.OtelCollector.Conf = &v2alpha1.CustomConfig{}
	builder.datadogAgent.Spec.Features.OtelCollector.Conf.ConfigMap = &v2alpha1.ConfigMapConfig{
		Name: "user-provided-config-map",
		Items: []corev1.KeyToPath{
			{
				Key:  "otel-config.yaml",
				Path: "otel-config.yaml",
			},
			{
				Key:  "otel-config-two.yaml",
				Path: "otel-config-two.yaml",
			},
		},
	}
	return builder
}

func (builder *DatadogAgentBuilder) WithOTelCollectorPorts(grpcPort int32, httpPort int32) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Features.OtelCollector.Ports = []*corev1.ContainerPort{
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

// OtelAgentGateway
func (builder *DatadogAgentBuilder) initOtelAgentGateway() {
	if builder.datadogAgent.Spec.Features.OtelAgentGateway == nil {
		builder.datadogAgent.Spec.Features.OtelAgentGateway = &v2alpha1.OtelAgentGatewayFeatureConfig{}
	}
}

func (builder *DatadogAgentBuilder) WithOTelAgentGatewayEnabled(enabled bool) *DatadogAgentBuilder {
	builder.initOtelAgentGateway()
	builder.datadogAgent.Spec.Features.OtelAgentGateway.Enabled = ptr.To(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithOTelAgentGatewayConfig() *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Features.OtelAgentGateway.Conf = &v2alpha1.CustomConfig{}
	builder.datadogAgent.Spec.Features.OtelAgentGateway.Conf.ConfigData = ptr.To(otelagentgatewaydefaultconfig.DefaultOtelAgentGatewayConfig)
	return builder
}

func (builder *DatadogAgentBuilder) WithOTelAgentGatewayConfigMap() *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Features.OtelAgentGateway.Conf = &v2alpha1.CustomConfig{}
	builder.datadogAgent.Spec.Features.OtelAgentGateway.Conf.ConfigMap = &v2alpha1.ConfigMapConfig{
		Name: "user-provided-config-map",
	}
	return builder
}

func (builder *DatadogAgentBuilder) WithOTelAgentGatewayConfigMapMultipleItems() *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Features.OtelAgentGateway.Conf = &v2alpha1.CustomConfig{}
	builder.datadogAgent.Spec.Features.OtelAgentGateway.Conf.ConfigMap = &v2alpha1.ConfigMapConfig{
		Name: "user-provided-config-map",
		Items: []corev1.KeyToPath{
			{
				Key:  "otel-gateway-config.yaml",
				Path: "otel-gateway-config.yaml",
			},
			{
				Key:  "otel-config-two.yaml",
				Path: "otel-config-two.yaml",
			},
		},
	}
	return builder
}

func (builder *DatadogAgentBuilder) WithOTelAgentGatewayPorts(grpcPort int32, httpPort int32) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Features.OtelAgentGateway.Ports = []*corev1.ContainerPort{
		{
			Name:          "otel-grpc",
			ContainerPort: grpcPort,
			Protocol:      corev1.ProtocolTCP,
		},
		{
			Name:          "otel-http",
			ContainerPort: httpPort,
			Protocol:      corev1.ProtocolTCP,
		},
	}
	return builder
}

func (builder *DatadogAgentBuilder) WithOTelAgentGatewayFeatureGates(featureGates string) *DatadogAgentBuilder {
	builder.initOtelAgentGateway()
	builder.datadogAgent.Spec.Features.OtelAgentGateway.FeatureGates = &featureGates
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
	builder.datadogAgent.Spec.Features.LogCollection.Enabled = ptr.To(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithLogCollectionCollectAll(enabled bool) *DatadogAgentBuilder {
	builder.initLogCollection()
	builder.datadogAgent.Spec.Features.LogCollection.ContainerCollectAll = ptr.To(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithLogCollectionLogCollectionUsingFiles(enabled bool) *DatadogAgentBuilder {
	builder.initLogCollection()
	builder.datadogAgent.Spec.Features.LogCollection.ContainerCollectUsingFiles = ptr.To(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithLogCollectionOpenFilesLimit(limit int32) *DatadogAgentBuilder {
	builder.initLogCollection()
	builder.datadogAgent.Spec.Features.LogCollection.OpenFilesLimit = ptr.To(limit)
	return builder
}

func (builder *DatadogAgentBuilder) WithLogCollectionAutoMultiLineDetection(enabled bool) *DatadogAgentBuilder {
	builder.initLogCollection()
	builder.datadogAgent.Spec.Features.LogCollection.AutoMultiLineDetection = ptr.To(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithLogCollectionPaths(podLogs, containerLogs, containerSymlinks, tempStorate string) *DatadogAgentBuilder {
	builder.initLogCollection()
	builder.datadogAgent.Spec.Features.LogCollection.PodLogsPath = ptr.To(podLogs)
	builder.datadogAgent.Spec.Features.LogCollection.ContainerLogsPath = ptr.To(containerLogs)
	builder.datadogAgent.Spec.Features.LogCollection.ContainerSymlinksPath = ptr.To(containerSymlinks)
	builder.datadogAgent.Spec.Features.LogCollection.TempStoragePath = ptr.To(tempStorate)
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
	builder.datadogAgent.Spec.Features.EventCollection.CollectKubernetesEvents = ptr.To(enabled)

	return builder
}

func (builder *DatadogAgentBuilder) WithEventCollectionUnbundleEvents(enabled bool, eventTypes []v2alpha1.EventTypes) *DatadogAgentBuilder {
	builder.initEventCollection()
	builder.datadogAgent.Spec.Features.EventCollection.UnbundleEvents = ptr.To(enabled)
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
	builder.datadogAgent.Spec.Features.RemoteConfiguration.Enabled = ptr.To(enabled)
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
	builder.datadogAgent.Spec.Features.KubeStateMetricsCore.Enabled = ptr.To(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithKSMCustomConf(customData string) *DatadogAgentBuilder {
	builder.initKSM()
	builder.datadogAgent.Spec.Features.KubeStateMetricsCore.Conf = &v2alpha1.CustomConfig{
		ConfigData: ptr.To(customData),
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
	builder.datadogAgent.Spec.Features.OrchestratorExplorer.Enabled = ptr.To(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithOrchestratorExplorerScrubContainers(enabled bool) *DatadogAgentBuilder {
	builder.initOE()
	builder.datadogAgent.Spec.Features.OrchestratorExplorer.ScrubContainers = ptr.To(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithOrchestratorExplorerExtraTags(tags []string) *DatadogAgentBuilder {
	builder.initOE()
	builder.datadogAgent.Spec.Features.OrchestratorExplorer.ExtraTags = tags
	return builder
}

func (builder *DatadogAgentBuilder) WithOrchestratorExplorerDDUrl(ddUrl string) *DatadogAgentBuilder {
	builder.initOE()
	builder.datadogAgent.Spec.Features.OrchestratorExplorer.DDUrl = ptr.To(ddUrl)
	return builder
}

func (builder *DatadogAgentBuilder) WithOrchestratorExplorerCustomConfigData(customConfigData string) *DatadogAgentBuilder {
	builder.initOE()
	builder.datadogAgent.Spec.Features.OrchestratorExplorer.Conf = &v2alpha1.CustomConfig{
		ConfigData: &customConfigData,
	}
	return builder
}

func (builder *DatadogAgentBuilder) WithOrchestratorExplorerCustomResources(customResources []string) *DatadogAgentBuilder {
	builder.initOE()
	builder.datadogAgent.Spec.Features.OrchestratorExplorer.CustomResources = customResources
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
	builder.datadogAgent.Spec.Features.ClusterChecks.Enabled = ptr.To(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithClusterChecksUseCLCEnabled(enabled bool) *DatadogAgentBuilder {
	builder.initCC()
	builder.datadogAgent.Spec.Features.ClusterChecks.UseClusterChecksRunners = ptr.To(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithClusterChecks(enabled bool, useRunners bool) *DatadogAgentBuilder {
	builder.initCC()
	builder.datadogAgent.Spec.Features.ClusterChecks.Enabled = ptr.To(enabled)
	builder.datadogAgent.Spec.Features.ClusterChecks.UseClusterChecksRunners = ptr.To(useRunners)
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
	builder.datadogAgent.Spec.Features.PrometheusScrape.Enabled = ptr.To(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithPrometheusScrapeServiceEndpoints(enabled bool) *DatadogAgentBuilder {
	builder.initPrometheusScrape()
	builder.datadogAgent.Spec.Features.PrometheusScrape.EnableServiceEndpoints = ptr.To(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithPrometheusScrapeAdditionalConfigs(additionalConfig string) *DatadogAgentBuilder {
	builder.initPrometheusScrape()
	builder.datadogAgent.Spec.Features.PrometheusScrape.AdditionalConfigs = ptr.To(additionalConfig)
	return builder
}

func (builder *DatadogAgentBuilder) WithPrometheusScrapeVersion(version int) *DatadogAgentBuilder {
	builder.initPrometheusScrape()
	builder.datadogAgent.Spec.Features.PrometheusScrape.Version = ptr.To(version)
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
	builder.datadogAgent.Spec.Features.APM.Enabled = ptr.To(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithErrorTrackingStandalone(enabled bool) *DatadogAgentBuilder {
	builder.initAPM()
	builder.datadogAgent.Spec.Features.APM.ErrorTrackingStandalone = &v2alpha1.ErrorTrackingStandalone{
		Enabled: ptr.To(enabled),
	}
	return builder
}

func (builder *DatadogAgentBuilder) WithAPMHostPortEnabled(enabled bool, port *int32) *DatadogAgentBuilder {
	builder.initAPM()
	builder.datadogAgent.Spec.Features.APM.HostPortConfig = &v2alpha1.HostPortConfig{
		Enabled: ptr.To(enabled),
	}
	if port != nil {
		builder.datadogAgent.Spec.Features.APM.HostPortConfig.Port = port
	}
	return builder
}

func (builder *DatadogAgentBuilder) WithAPMUDSEnabled(enabled bool, apmSocketHostPath string) *DatadogAgentBuilder {
	builder.initAPM()
	builder.datadogAgent.Spec.Features.APM.UnixDomainSocketConfig = &v2alpha1.UnixDomainSocketConfig{
		Enabled: ptr.To(enabled),
		Path:    ptr.To(apmSocketHostPath),
	}
	return builder
}

func (builder *DatadogAgentBuilder) WithClusterAgentTag(tag string) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Override[v2alpha1.ClusterAgentComponentName] = &v2alpha1.DatadogAgentComponentOverride{
		Image: &v2alpha1.AgentImageConfig{Tag: tag},
	}
	return builder
}

func (builder *DatadogAgentBuilder) WithAPMSingleStepInstrumentationEnabled(enabled bool, enabledNamespaces []string, disabledNamespaces []string, libVersion map[string]string, languageDetectionEnabled bool, injectorImageTag string, targets []v2alpha1.SSITarget, injectionMode v2alpha1.InjectionModeType) *DatadogAgentBuilder {
	builder.initAPM()
	builder.datadogAgent.Spec.Features.APM.SingleStepInstrumentation = &v2alpha1.SingleStepInstrumentation{
		Enabled:            ptr.To(enabled),
		EnabledNamespaces:  enabledNamespaces,
		DisabledNamespaces: disabledNamespaces,
		LibVersions:        libVersion,
		LanguageDetection:  &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(languageDetectionEnabled)},
		Injector: &v2alpha1.InjectorConfig{
			ImageTag: injectorImageTag,
		},
		Targets:       targets,
		InjectionMode: injectionMode,
	}
	return builder
}

func (builder *DatadogAgentBuilder) WithASMEnabled(threats, sca, iast bool) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Features.ASM = &v2alpha1.ASMFeatureConfig{
		Threats: &v2alpha1.ASMThreatsConfig{
			Enabled: ptr.To(threats),
		},
		SCA: &v2alpha1.ASMSCAConfig{
			Enabled: ptr.To(sca),
		},
		IAST: &v2alpha1.ASMIASTConfig{
			Enabled: ptr.To(iast),
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
		Enabled: ptr.To(enabled),
		HostPortConfig: &v2alpha1.HostPortConfig{
			Enabled: ptr.To(hostPortEnabled),
			Port:    ptr.To(customHostPort),
		},
		Endpoint: ptr.To(endpoint),
	}
	return builder
}

func (builder *DatadogAgentBuilder) WithOTLPHTTPSettings(enabled bool, hostPortEnabled bool, customHostPort int32, endpoint string) *DatadogAgentBuilder {
	builder.initOTLP()
	builder.datadogAgent.Spec.Features.OTLP.Receiver.Protocols.HTTP = &v2alpha1.OTLPHTTPConfig{
		Enabled: ptr.To(enabled),
		HostPortConfig: &v2alpha1.HostPortConfig{
			Enabled: ptr.To(hostPortEnabled),
			Port:    ptr.To(customHostPort),
		},
		Endpoint: ptr.To(endpoint),
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
	builder.datadogAgent.Spec.Features.NPM.Enabled = ptr.To(enabled)
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
	builder.datadogAgent.Spec.Features.CSPM.Enabled = ptr.To(enabled)
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
	builder.datadogAgent.Spec.Features.CWS.Enabled = ptr.To(enabled)
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
	builder.datadogAgent.Spec.Features.AdmissionController.CWSInstrumentation.Enabled = ptr.To(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithCWSInstrumentationMode(mode string) *DatadogAgentBuilder {
	builder.initCWSInstrumentation()
	builder.datadogAgent.Spec.Features.AdmissionController.CWSInstrumentation.Mode = ptr.To(mode)
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
	builder.datadogAgent.Spec.Features.OOMKill.Enabled = ptr.To(enabled)
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
	builder.datadogAgent.Spec.Features.HelmCheck.Enabled = ptr.To(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithHelmCheckCollectEvents(enabled bool) *DatadogAgentBuilder {
	builder.initHelmCheck()
	builder.datadogAgent.Spec.Features.HelmCheck.CollectEvents = ptr.To(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithHelmCheckValuesAsTags(valuesAsTags map[string]string) *DatadogAgentBuilder {
	builder.initHelmCheck()
	builder.datadogAgent.Spec.Features.HelmCheck.ValuesAsTags = valuesAsTags
	return builder
}

// Global Kubelet

func (builder *DatadogAgentBuilder) WithGlobalKubeletConfig(hostCAPath, agentCAPath string, tlsVerify bool, podResourcesSocketDir string) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Global.Kubelet = &v2alpha1.KubeletConfig{
		TLSVerify:              ptr.To(tlsVerify),
		HostCAPath:             hostCAPath,
		AgentCAPath:            agentCAPath,
		PodResourcesSocketPath: podResourcesSocketDir,
	}
	return builder
}

func (builder *DatadogAgentBuilder) WithGlobalDockerSocketPath(dockerSocketPath string) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Global.DockerSocketPath = ptr.To(dockerSocketPath)
	return builder
}

func (builder *DatadogAgentBuilder) WithGlobalCriSocketPath(criSocketPath string) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Global.DockerSocketPath = ptr.To(criSocketPath)
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
		APIKey: ptr.To(apiKey),
		AppKey: ptr.To(appKey),
	}
	return builder
}

func (builder *DatadogAgentBuilder) WithCredentialsFromSecret(apiSecretName, apiSecretKey, appSecretName, appSecretKey string) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Global.Credentials = &v2alpha1.DatadogCredentials{}
	if apiSecretName != "" && apiSecretKey != "" {
		builder.datadogAgent.Spec.Global.Credentials.APISecret = &v2alpha1.SecretConfig{
			SecretName: apiSecretName,
			KeyName:    apiSecretKey,
		}
	}
	if appSecretName != "" && appSecretKey != "" {
		builder.datadogAgent.Spec.Global.Credentials.AppSecret = &v2alpha1.SecretConfig{
			SecretName: appSecretName,
			KeyName:    appSecretKey,
		}
	}
	return builder
}

// Global DCA Token
func (builder *DatadogAgentBuilder) WithDCAToken(token string) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Global.ClusterAgentToken = ptr.To(token)
	return builder
}

func (builder *DatadogAgentBuilder) WithDCATokenFromSecret(secretName, secretKey string) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Global.ClusterAgentTokenSecret = &v2alpha1.SecretConfig{
		SecretName: secretName,
		KeyName:    secretKey,
	}
	return builder
}

// Global OriginDetectionUnified

func (builder *DatadogAgentBuilder) WithOriginDetectionUnified(enabled bool) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Global.OriginDetectionUnified = &v2alpha1.OriginDetectionUnified{
		Enabled: ptr.To(enabled),
	}
	return builder
}

// Global Registry

func (builder *DatadogAgentBuilder) WithRegistry(registry string) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Global.Registry = ptr.To(registry)

	return builder
}

// Global ChecksTagCardinality

func (builder *DatadogAgentBuilder) WithChecksTagCardinality(cardinality string) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Global.ChecksTagCardinality = ptr.To(cardinality)
	return builder
}

// CSI Activation Config

func (builder *DatadogAgentBuilder) WithCSIActivation(enabled bool) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Global.CSI = &v2alpha1.CSIConfig{Enabled: ptr.To(enabled)}
	return builder
}

// Global SecretBackend

func (builder *DatadogAgentBuilder) WithGlobalSecretBackendGlobalPerms(command string, args string, timeout int32, refreshInterval int32) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Global.SecretBackend = &v2alpha1.SecretBackendConfig{
		Command:                 ptr.To(command),
		Args:                    ptr.To(args),
		Timeout:                 ptr.To(timeout),
		RefreshInterval:         ptr.To(refreshInterval),
		EnableGlobalPermissions: ptr.To(true),
	}
	return builder
}

func (builder *DatadogAgentBuilder) WithGlobalSecretBackendType(backendType string, config map[string]string) *DatadogAgentBuilder {
	if builder.datadogAgent.Spec.Global.SecretBackend == nil {
		builder.datadogAgent.Spec.Global.SecretBackend = &v2alpha1.SecretBackendConfig{}
	}
	builder.datadogAgent.Spec.Global.SecretBackend.Type = ptr.To(backendType)
	builder.datadogAgent.Spec.Global.SecretBackend.Config = config
	return builder
}

func (builder *DatadogAgentBuilder) WithGlobalSecretBackendSpecificRoles(command string, args string, timeout int32, refreshInterval int32, secretNs string, secretNames []string) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Global.SecretBackend = &v2alpha1.SecretBackendConfig{
		Command:                 ptr.To(command),
		Args:                    ptr.To(args),
		Timeout:                 ptr.To(timeout),
		RefreshInterval:         ptr.To(refreshInterval),
		EnableGlobalPermissions: ptr.To(false),
		Roles: []*v2alpha1.SecretBackendRolesConfig{
			{
				Namespace: ptr.To(secretNs),
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

func (builder *DatadogAgentBuilder) WithClusterAgentImage(image string) *DatadogAgentBuilder {
	if builder.datadogAgent.Spec.Override == nil {
		builder.datadogAgent.Spec.Override = map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{}
	}

	if builder.datadogAgent.Spec.Override[v2alpha1.ClusterAgentComponentName] == nil {
		builder.datadogAgent.Spec.Override[v2alpha1.ClusterAgentComponentName] = &v2alpha1.DatadogAgentComponentOverride{}
	}

	builder.datadogAgent.Spec.Override[v2alpha1.ClusterAgentComponentName].Image = &v2alpha1.AgentImageConfig{
		Name: image,
	}
	return builder
}

func (builder *DatadogAgentBuilder) WithClusterAgentDisabled(disabled bool) *DatadogAgentBuilder {
	builder.WithComponentOverride(v2alpha1.ClusterAgentComponentName, v2alpha1.DatadogAgentComponentOverride{
		Disabled: ptr.To(disabled),
	})
	return builder
}

func (builder *DatadogAgentBuilder) WithClusterChecksRunnerImage(image string) *DatadogAgentBuilder {
	if builder.datadogAgent.Spec.Override == nil {
		builder.datadogAgent.Spec.Override = map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{}
	}

	if builder.datadogAgent.Spec.Override[v2alpha1.ClusterChecksRunnerComponentName] == nil {
		builder.datadogAgent.Spec.Override[v2alpha1.ClusterChecksRunnerComponentName] = &v2alpha1.DatadogAgentComponentOverride{}
	}

	builder.datadogAgent.Spec.Override[v2alpha1.ClusterChecksRunnerComponentName].Image = &v2alpha1.AgentImageConfig{
		Name: image,
	}
	return builder
}

// FIPS

func (builder *DatadogAgentBuilder) WithUseFIPSAgent() *DatadogAgentBuilder {
	if builder.datadogAgent.Spec.Global == nil {
		builder.datadogAgent.Spec.Global = &v2alpha1.GlobalConfig{}
	}

	builder.datadogAgent.Spec.Global.UseFIPSAgent = ptr.To(true)
	return builder
}

func (builder *DatadogAgentBuilder) WithFIPS(fipsConfig v2alpha1.FIPSConfig) *DatadogAgentBuilder {
	if builder.datadogAgent.Spec.Global == nil {
		builder.datadogAgent.Spec.Global = &v2alpha1.GlobalConfig{}
	}

	builder.datadogAgent.Spec.Global.FIPS = &fipsConfig
	return builder
}

// GPU

func (builder *DatadogAgentBuilder) initGPUMonitoring() {
	if builder.datadogAgent.Spec.Features.GPU == nil {
		builder.datadogAgent.Spec.Features.GPU = &v2alpha1.GPUFeatureConfig{}
	}
}

func (builder *DatadogAgentBuilder) WithGPUMonitoringEnabled(enabled bool) *DatadogAgentBuilder {
	builder.initGPUMonitoring()
	builder.datadogAgent.Spec.Features.GPU.Enabled = ptr.To(enabled)
	builder.datadogAgent.Spec.Features.GPU.PrivilegedMode = ptr.To(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) initControlPlaneMonitoring() {
	if builder.datadogAgent.Spec.Features.ControlPlaneMonitoring == nil {
		builder.datadogAgent.Spec.Features.ControlPlaneMonitoring = &v2alpha1.ControlPlaneMonitoringFeatureConfig{}
	}
}

func (builder *DatadogAgentBuilder) WithControlPlaneMonitoring(enabled bool) *DatadogAgentBuilder {
	builder.initControlPlaneMonitoring()
	builder.datadogAgent.Spec.Features.ControlPlaneMonitoring.Enabled = ptr.To(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) initDataPlane() {
	if builder.datadogAgent.Spec.Features == nil {
		builder.datadogAgent.Spec.Features = &v2alpha1.DatadogFeatures{}
	}
	if builder.datadogAgent.Spec.Features.DataPlane == nil {
		builder.datadogAgent.Spec.Features.DataPlane = &v2alpha1.DataPlaneFeatureConfig{}
	}
}

func (builder *DatadogAgentBuilder) WithDataPlaneEnabled(enabled bool) *DatadogAgentBuilder {
	builder.initDataPlane()
	builder.datadogAgent.Spec.Features.DataPlane.Enabled = ptr.To(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithDataPlaneDogstatsdEnabled(enabled bool) *DatadogAgentBuilder {
	builder.initDataPlane()
	if builder.datadogAgent.Spec.Features.DataPlane.Dogstatsd == nil {
		builder.datadogAgent.Spec.Features.DataPlane.Dogstatsd = &v2alpha1.DataPlaneDogstatsdConfig{}
	}
	builder.datadogAgent.Spec.Features.DataPlane.Dogstatsd.Enabled = ptr.To(enabled)
	return builder
}

func (builder *DatadogAgentBuilder) WithStatus(status v2alpha1.DatadogAgentStatus) *DatadogAgentBuilder {
	builder.datadogAgent.Status = status
	return builder
}
