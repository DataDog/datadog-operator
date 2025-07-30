// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v2alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
)

// ComponentName is the name of a Deployment Component
type ComponentName string

const (
	// NodeAgentComponentName is the name of the Datadog Node Agent
	NodeAgentComponentName ComponentName = "nodeAgent"
	// ClusterAgentComponentName is the name of the Cluster Agent
	ClusterAgentComponentName ComponentName = "clusterAgent"
	// ClusterChecksRunnerComponentName is the name of the Cluster Check Runner
	ClusterChecksRunnerComponentName ComponentName = "clusterChecksRunner"
)

// DatadogAgentSpec defines the desired state of DatadogAgent
type DatadogAgentSpec struct {
	// Features running on the Agent and Cluster Agent
	// +optional
	Features *DatadogFeatures `json:"features,omitempty"`

	// Global settings to configure the agents
	// +optional
	Global *GlobalConfig `json:"global,omitempty"`

	// Override the default configurations of the agents
	// +optional
	Override map[ComponentName]*DatadogAgentComponentOverride `json:"override,omitempty"`
}

// DatadogFeatures are features running on the Agent and Cluster Agent.
// +k8s:openapi-gen=true
type DatadogFeatures struct {
	// Application-level features

	// OtelCollector configuration.
	OtelCollector *OtelCollectorFeatureConfig `json:"otelCollector,omitempty"`
	// LogCollection configuration.
	LogCollection *LogCollectionFeatureConfig `json:"logCollection,omitempty"`
	// LiveProcessCollection configuration.
	LiveProcessCollection *LiveProcessCollectionFeatureConfig `json:"liveProcessCollection,omitempty"`
	// LiveContainerCollection configuration.
	LiveContainerCollection *LiveContainerCollectionFeatureConfig `json:"liveContainerCollection,omitempty"`
	// ProcessDiscovery configuration.
	ProcessDiscovery *ProcessDiscoveryFeatureConfig `json:"processDiscovery,omitempty"`
	// OOMKill configuration.
	OOMKill *OOMKillFeatureConfig `json:"oomKill,omitempty"`
	// TCPQueueLength configuration.
	TCPQueueLength *TCPQueueLengthFeatureConfig `json:"tcpQueueLength,omitempty"`
	// EBPFCheck configuration.
	EBPFCheck *EBPFCheckFeatureConfig `json:"ebpfCheck,omitempty"`
	// APM (Application Performance Monitoring) configuration.
	APM *APMFeatureConfig `json:"apm,omitempty"`
	// ASM (Application Security Management) configuration.
	ASM *ASMFeatureConfig `json:"asm,omitempty"`
	// CSPM (Cloud Security Posture Management) configuration.
	CSPM *CSPMFeatureConfig `json:"cspm,omitempty"`
	// CWS (Cloud Workload Security) configuration.
	CWS *CWSFeatureConfig `json:"cws,omitempty"`
	// NPM (Network Performance Monitoring) configuration.
	NPM *NPMFeatureConfig `json:"npm,omitempty"`
	// USM (Universal Service Monitoring) configuration.
	USM *USMFeatureConfig `json:"usm,omitempty"`
	// Dogstatsd configuration.
	Dogstatsd *DogstatsdFeatureConfig `json:"dogstatsd,omitempty"`
	// OTLP ingest configuration
	OTLP *OTLPFeatureConfig `json:"otlp,omitempty"`
	// Remote Configuration configuration.
	RemoteConfiguration *RemoteConfigurationFeatureConfig `json:"remoteConfiguration,omitempty"`
	// SBOM collection configuration.
	SBOM *SBOMFeatureConfig `json:"sbom,omitempty"`
	// ServiceDiscovery
	ServiceDiscovery *ServiceDiscoveryFeatureConfig `json:"serviceDiscovery,omitempty"`
	// GPU monitoring
	GPU *GPUFeatureConfig `json:"gpu,omitempty"`

	// Cluster-level features

	// EventCollection configuration.
	EventCollection *EventCollectionFeatureConfig `json:"eventCollection,omitempty"`
	// OrchestratorExplorer check configuration.
	OrchestratorExplorer *OrchestratorExplorerFeatureConfig `json:"orchestratorExplorer,omitempty"`
	// KubeStateMetricsCore check configuration.
	KubeStateMetricsCore *KubeStateMetricsCoreFeatureConfig `json:"kubeStateMetricsCore,omitempty"`
	// AdmissionController configuration.
	AdmissionController *AdmissionControllerFeatureConfig `json:"admissionController,omitempty"`
	// ExternalMetricsServer configuration.
	ExternalMetricsServer *ExternalMetricsServerFeatureConfig `json:"externalMetricsServer,omitempty"`
	// Autoscaling configuration.
	Autoscaling *AutoscalingFeatureConfig `json:"autoscaling,omitempty"`
	// ClusterChecks configuration.
	ClusterChecks *ClusterChecksFeatureConfig `json:"clusterChecks,omitempty"`
	// PrometheusScrape configuration.
	PrometheusScrape *PrometheusScrapeFeatureConfig `json:"prometheusScrape,omitempty"`
	// HelmCheck configuration.
	HelmCheck *HelmCheckFeatureConfig `json:"helmCheck,omitempty"`
}

// Configuration structs for each feature in DatadogFeatures. All parameters are optional and have default values when necessary.
// Note: configuration in DatadogAgentSpec.Override takes precedence.

// APMFeatureConfig contains APM (Application Performance Monitoring) configuration.
// APM runs in the Trace Agent.
type APMFeatureConfig struct {
	// Enabled enables Application Performance Monitoring.
	// Default: true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// HostPortConfig contains host port configuration.
	// Enabled Default: false
	// Port Default: 8126
	// +optional
	HostPortConfig *HostPortConfig `json:"hostPortConfig,omitempty"`

	// UnixDomainSocketConfig contains socket configuration.
	// See also: https://docs.datadoghq.com/agent/kubernetes/apm/?tab=helm#agent-environment-variables
	// Enabled Default: true
	// Path Default: `/var/run/datadog/apm.socket`
	// +optional
	UnixDomainSocketConfig *UnixDomainSocketConfig `json:"unixDomainSocketConfig,omitempty"`

	// SingleStepInstrumentation allows the agent to inject the Datadog APM libraries into all pods in the cluster.
	// Feature is in beta.
	// See also: https://docs.datadoghq.com/tracing/trace_collection/single-step-apm
	// Enabled Default: false
	// +optional
	SingleStepInstrumentation *SingleStepInstrumentation `json:"instrumentation,omitempty"`

	// ErrorTrackingStandalone contains the configuration for the Error Tracking standalone feature.
	// Feature is in preview.
	// +optional
	ErrorTrackingStandalone *ErrorTrackingStandalone `json:"errorTrackingStandalone,omitempty"`
}

// ErrorTrackingStandalone contains the configuration for the Error Tracking standalone feature.
// +k8s:openapi-gen=true
type ErrorTrackingStandalone struct {
	// Enables Error Tracking for backend services.
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// SingleStepInstrumentation contains the config for the namespaces to target and the library to inject.
type SingleStepInstrumentation struct {
	// Enabled enables injecting the Datadog APM libraries into all pods in the cluster.
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// EnabledNamespaces enables injecting the Datadog APM libraries into pods in specific namespaces.
	// +optional
	// +listType=set
	EnabledNamespaces []string `json:"enabledNamespaces,omitempty"`

	// DisabledNamespaces disables injecting the Datadog APM libraries into pods in specific namespaces.
	// +optional
	// +listType=set
	DisabledNamespaces []string `json:"disabledNamespaces,omitempty"`

	// LibVersions configures injection of specific tracing library versions with Single Step Instrumentation.
	// <Library>: <Version>
	// ex: "java": "v1.18.0"
	// +optional
	LibVersions map[string]string `json:"libVersions,omitempty"`

	// LanguageDetection detects languages and adds them as annotations on Deployments, but does not use these languages for injecting libraries to workload pods.
	// (Requires Agent 7.52.0+ and Cluster Agent 7.52.0+)
	// +optional
	LanguageDetection *LanguageDetectionConfig `json:"languageDetection,omitempty"`

	// Injector configures the APM Injector.
	// +optional
	Injector *InjectorConfig `json:"injector,omitempty"`

	// Targets is a list of targets to apply the auto instrumentation to. The first target that matches the pod will be
	// used. If no target matches, the auto instrumentation will not be applied.
	// (Requires Cluster Agent 7.64.0+)
	// +optional
	Targets []SSITarget `json:"targets,omitempty"`
}

// SSITarget is a rule to apply the auto instrumentation to a specific workload using the pod and namespace selectors.
type SSITarget struct {
	// Name is the name of the target. It will be appended to the pod annotations to identify the target that was used.
	// +optional
	Name string `json:"name,omitempty"`
	// PodSelector is the pod selector to match the pods to apply the auto instrumentation to. It will be used in
	// conjunction with the NamespaceSelector to match the pods.
	// +optional
	PodSelector *metav1.LabelSelector `json:"podSelector,omitempty"`
	// NamespaceSelector is the namespace selector to match the namespaces to apply the auto instrumentation to. It will
	// be used in conjunction with the Selector to match the pods.
	// +optional
	NamespaceSelector *NamespaceSelector `json:"namespaceSelector,omitempty"`
	// TracerVersions is a map of tracer versions to inject for workloads that match the target. The key is the tracer
	// name and the value is the version to inject.
	// +optional
	TracerVersions map[string]string `json:"ddTraceVersions,omitempty"`
	// TracerConfigs is a list of configuration options to use for the installed tracers. These options will be added
	// as environment variables in addition to the injected tracer.
	// +optional
	// +listType=map
	// +listMapKey=name
	TracerConfigs []corev1.EnvVar `json:"ddTraceConfigs,omitempty"`
}

// NamespaceSelector is a struct to store the configuration for the namespace selector. It can be used to match the
// namespaces to apply the auto instrumentation to.
type NamespaceSelector struct {
	// MatchNames is a list of namespace names to match. If empty, all namespaces are matched.
	// +optional
	MatchNames []string `json:"matchNames,omitempty"`
	// MatchLabels is a map of key-value pairs to match the labels of the namespace. The labels and expressions are
	// ANDed. This cannot be used with MatchNames.
	// +optional
	MatchLabels map[string]string `json:"matchLabels,omitempty"`
	// MatchExpressions is a list of label selector requirements to match the labels of the namespace. The labels and
	// expressions are ANDed. This cannot be used with MatchNames.
	// +optional
	MatchExpressions []metav1.LabelSelectorRequirement `json:"matchExpressions,omitempty"`
}

// LanguageDetectionConfig contains the config for Language Detection.
type LanguageDetectionConfig struct {
	// Enabled enables Language Detection to automatically detect languages of user workloads (beta).
	// Requires SingleStepInstrumentation.Enabled to be true.
	// Default: true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// CSIConfig contains the config for Datadog CSI driver.
type CSIConfig struct {
	// Enables the usage of CSI driver in Datadog Agent.
	// Requires installation of Datadog CSI Driver https://github.com/DataDog/helm-charts/tree/main/charts/datadog-csi-driver
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// InjectorConfig contains the configuration for the APM Injector.
type InjectorConfig struct {
	// Set the image tag to use for the APM Injector.
	// (Requires Cluster Agent 7.57.0+)
	// +optional
	ImageTag string `json:"imageTag,omitempty"`
}

// ASMFeatureConfig contains Application Security Management (ASM) configuration.
// Note that this will only affect pods where the Datadog client libraries are installed or APM Single Step Instrumentation is enabled.
type ASMFeatureConfig struct {
	// Threats configures ASM App & API Protection.
	// Enabled Default: false
	// +optional
	Threats *ASMThreatsConfig `json:"threats,omitempty"`

	// SCA configures Software Composition Analysis.
	// Enabled Default: false
	// +optional
	SCA *ASMSCAConfig `json:"sca,omitempty"`

	// IAST configures Interactive Application Security Testing.
	// Enabled Default: false
	// +optional
	IAST *ASMIASTConfig `json:"iast,omitempty"`
}

type ASMThreatsConfig struct {
	// Enabled enables ASM App & API Protection.
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

type ASMSCAConfig struct {
	// Enabled enables Software Composition Analysis (SCA).
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

type ASMIASTConfig struct {
	// Enabled enables Interactive Application Security Testing (IAST).
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// LogCollectionFeatureConfig contains Logs configuration.
// Logs collection is run in the Agent.
type LogCollectionFeatureConfig struct {
	// Enabled enables Log collection.
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// ContainerCollectAll enables Log collection from all containers.
	// Default: false
	// +optional
	ContainerCollectAll *bool `json:"containerCollectAll,omitempty"`

	// ContainerCollectUsingFiles enables log collection from files in `/var/log/pods instead` of using the container runtime API.
	// Collecting logs from files is usually the most efficient way of collecting logs.
	// See also: https://docs.datadoghq.com/agent/basic_agent_usage/kubernetes/#log-collection-setup
	// Default: true
	// +optional
	ContainerCollectUsingFiles *bool `json:"containerCollectUsingFiles,omitempty"`

	// ContainerLogsPath allows log collection from the container log path.
	// Set to a different path if you are not using the Docker runtime.
	// See also: https://docs.datadoghq.com/agent/kubernetes/daemonset_setup/?tab=k8sfile#create-manifest
	// Default: `/var/lib/docker/containers`
	// +optional
	ContainerLogsPath *string `json:"containerLogsPath,omitempty"`

	// PodLogsPath allows log collection from a pod log path.
	// Default: `/var/log/pods`
	// +optional
	PodLogsPath *string `json:"podLogsPath,omitempty"`

	// ContainerSymlinksPath allows log collection to use symbolic links in this directory to validate container ID -> pod.
	// Default: `/var/log/containers`
	// +optional
	ContainerSymlinksPath *string `json:"containerSymlinksPath,omitempty"`

	// TempStoragePath (always mounted from the host) is used by the Agent to store information about processed log files.
	// If the Agent is restarted, it starts tailing the log files immediately.
	// Default: `/var/lib/datadog-agent/logs`
	// +optional
	TempStoragePath *string `json:"tempStoragePath,omitempty"`

	// OpenFilesLimit sets the maximum number of log files that the Datadog Agent tails.
	// Increasing this limit can increase resource consumption of the Agent.
	// See also: https://docs.datadoghq.com/agent/basic_agent_usage/kubernetes/#log-collection-setup
	// Default: 100
	// +optional
	OpenFilesLimit *int32 `json:"openFilesLimit,omitempty"`
}

// LiveProcessCollectionFeatureConfig contains Process Collection configuration.
// Process Collection is run in the Process Agent.
type LiveProcessCollectionFeatureConfig struct {
	// Enabled enables Process monitoring.
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// ScrubProcessArguments enables scrubbing of sensitive data in process command-lines (passwords, tokens, etc. ).
	// Default: true
	// +optional
	ScrubProcessArguments *bool `json:"scrubProcessArguments,omitempty"`

	// StripProcessArguments enables stripping of all process arguments.
	// Default: false
	// +optional
	StripProcessArguments *bool `json:"stripProcessArguments,omitempty"`
}

// LiveContainerCollectionFeatureConfig contains Container Collection configuration.
// Container Collection is run in the Process Agent.
type LiveContainerCollectionFeatureConfig struct {
	// Enables container collection for the Live Container View.
	// Default: true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// ProcessDiscoveryFeatureConfig contains the configuration for the process discovery check
// ProcessDiscovery is run in the ProcessAgent
type ProcessDiscoveryFeatureConfig struct {
	// Enabled enables the Process Discovery check in the Agent.
	// Default: true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// OOMKillFeatureConfig configures the OOM Kill monitoring feature.
type OOMKillFeatureConfig struct {
	// Enables the OOMKill eBPF-based check.
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// TCPQueueLengthFeatureConfig configures the TCP queue length monitoring feature.
type TCPQueueLengthFeatureConfig struct {
	// Enables the TCP queue length eBPF-based check.
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// EBPFCheckFeatureConfig configures the eBPF check feature.
type EBPFCheckFeatureConfig struct {
	// Enables the eBPF check.
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// CSPMFeatureConfig contains CSPM (Cloud Security Posture Management) configuration.
// CSPM runs in the Security Agent and Cluster Agent.
type CSPMFeatureConfig struct {
	// Enabled enables Cloud Security Posture Management.
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// CheckInterval defines the check interval.
	// +optional
	CheckInterval *metav1.Duration `json:"checkInterval,omitempty"`

	// CustomBenchmarks contains CSPM benchmarks.
	// The content of the ConfigMap will be merged with the benchmarks bundled with the agent.
	// Any benchmarks with the same name as those existing in the agent will take precedence.
	// +optional
	CustomBenchmarks *CustomConfig `json:"customBenchmarks,omitempty"`

	// HostBenchmarks contains configuration for host benchmarks.
	// +optional
	HostBenchmarks *CSPMHostBenchmarksConfig `json:"hostBenchmarks,omitempty"`
}

// CSPMHostBenchmarksConfig contains configuration for host benchmarks.
// +k8s:openapi-gen=true
type CSPMHostBenchmarksConfig struct {
	// Enabled enables host benchmarks.
	// Default: true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// CWSFeatureConfig contains CWS (Cloud Workload Security) configuration.
// CWS runs in the Security Agent.
type CWSFeatureConfig struct {
	// Enabled enables Cloud Workload Security.
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// SyscallMonitorEnabled enables Syscall Monitoring (recommended for troubleshooting only).
	// Default: false
	// +optional
	SyscallMonitorEnabled *bool `json:"syscallMonitorEnabled,omitempty"`

	Network             *CWSNetworkConfig             `json:"network,omitempty"`
	SecurityProfiles    *CWSSecurityProfilesConfig    `json:"securityProfiles,omitempty"`
	RemoteConfiguration *CWSRemoteConfigurationConfig `json:"remoteConfiguration,omitempty"`

	// CustomPolicies contains security policies.
	// The content of the ConfigMap will be merged with the policies bundled with the agent.
	// Any policies with the same name as those existing in the agent will take precedence.
	// +optional
	CustomPolicies *CustomConfig `json:"customPolicies,omitempty"`
}

type CWSNetworkConfig struct {
	// Enabled enables Cloud Workload Security Network detections.
	// Default: true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

type CWSSecurityProfilesConfig struct {
	// Enabled enables Security Profiles collection for Cloud Workload Security.
	// Default: true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

type CWSRemoteConfigurationConfig struct {
	// Enabled enables Remote Configuration for Cloud Workload Security.
	// Default: true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// RemoteConfigurationFeatureConfig contains RC (Remote Configuration) configuration.
// RC runs in the Agent.
type RemoteConfigurationFeatureConfig struct {
	// Enable this option to activate Remote Configuration.
	// Default: true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// SBOMFeatureConfig contains SBOM (Software Bill of Materials) collection configuration.
// SBOM runs in the Agent.
type SBOMFeatureConfig struct {
	// Enable this option to activate SBOM collection.
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	ContainerImage *SBOMContainerImageConfig `json:"containerImage,omitempty"`
	Host           *SBOMHostConfig           `json:"host,omitempty"`
}

// SBOMTypeConfig contains configuration for a SBOM collection type.
type SBOMHostConfig struct {
	// Enable this option to activate SBOM collection.
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Analyzers to use for SBOM collection.
	// +optional
	// +listType=set
	Analyzers []string `json:"analyzers,omitempty"`
}

// SBOMTypeConfig contains configuration for a SBOM collection type.
type SBOMContainerImageConfig struct {
	// Enable this option to activate SBOM collection.
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Analyzers to use for SBOM collection.
	// +optional
	// +listType=set
	Analyzers []string `json:"analyzers,omitempty"`

	// Enable this option to enable support for uncompressed layers.
	// Default: false
	// +optional
	UncompressedLayersSupport bool `json:"uncompressedLayersSupport,omitempty"`

	// Enable this option to enable experimental overlayFS direct scan.
	// Default: false
	// +optional
	OverlayFSDirectScan bool `json:"overlayFSDirectScan,omitempty"`
}

// NPMFeatureConfig contains NPM (Network Performance Monitoring) feature configuration.
// Network Performance Monitoring runs in the System Probe and Process Agent.
type NPMFeatureConfig struct {
	// Enabled enables Network Performance Monitoring.
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// EnableConntrack enables the system-probe agent to connect to the netlink/conntrack subsystem to add NAT information to connection data.
	// See also: http://conntrack-tools.netfilter.org/
	// Default: false
	// +optional
	EnableConntrack *bool `json:"enableConntrack,omitempty"`

	// CollectDNSStats enables DNS stat collection.
	// Default: false
	// +optional
	CollectDNSStats *bool `json:"collectDNSStats,omitempty"`
}

// USMFeatureConfig contains USM (Universal Service Monitoring) feature configuration.
// Universal Service Monitoring runs in the Process Agent and System Probe.
type USMFeatureConfig struct {
	// Enabled enables Universal Service Monitoring.
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// ServiceDiscoveryFeatureConfig configures the service discovery check feature.
type ServiceDiscoveryFeatureConfig struct {
	// Enables the service discovery check.
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Enables the service discovery network stats collection.
	// Default: true
	// +optional
	NetworkStats *ServiceDiscoveryNetworkStatsConfig `json:"networkStats,omitempty"`
}

// ServiceDiscoveryNetworkStatsConfig configures Service Discovery's network stats
// collection feature.
type ServiceDiscoveryNetworkStatsConfig struct {
	// Enables the Service Discovery Network Stats feature.
	// Default: true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// GPUFeatureConfig contains the GPU monitoring configuration.
type GPUFeatureConfig struct {
	// Enabled enables GPU monitoring.
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// PodRuntimeClassName specifies the runtime class name required for the GPU monitoring feature.
	// If the value is an empty string, the runtime class is not set.
	// Default: nvidia
	// +optional
	PodRuntimeClassName *string `json:"requiredRuntimeClassName"`
}

// DogstatsdFeatureConfig contains the Dogstatsd configuration parameters.
// +k8s:openapi-gen=true
type DogstatsdFeatureConfig struct {
	// OriginDetectionEnabled enables origin detection for container tagging.
	// See also: https://docs.datadoghq.com/developers/dogstatsd/unix_socket/#using-origin-detection-for-container-tagging
	// +optional
	OriginDetectionEnabled *bool `json:"originDetectionEnabled,omitempty"`

	// TagCardinality configures tag cardinality for the metrics collected using origin detection (`low`, `orchestrator` or `high`).
	// See also: https://docs.datadoghq.com/getting_started/tagging/assigning_tags/?tab=containerizedenvironments#environment-variables
	// Cardinality default: low
	// +optional
	TagCardinality *string `json:"tagCardinality,omitempty"`

	// HostPortConfig contains host port configuration.
	// Enabled Default: false
	// Port Default: 8125
	// +optional
	HostPortConfig *HostPortConfig `json:"hostPortConfig,omitempty"`

	// UnixDomainSocketConfig contains socket configuration.
	// See also: https://docs.datadoghq.com/agent/kubernetes/apm/?tab=helm#agent-environment-variables
	// Enabled Default: true
	// Path Default: `/var/run/datadog/dsd.socket`
	// +optional
	UnixDomainSocketConfig *UnixDomainSocketConfig `json:"unixDomainSocketConfig,omitempty"`

	// Configure the Dogstasd Mapper Profiles.
	// Can be passed as raw data or via a json encoded string in a config map.
	// See also: https://docs.datadoghq.com/developers/dogstatsd/dogstatsd_mapper/
	// +optional
	MapperProfiles *CustomConfig `json:"mapperProfiles,omitempty"`

	// NonLocalTraffic enables non-local traffic for Dogstatsd.
	// Default: true
	// +optional
	NonLocalTraffic *bool `json:"nonLocalTraffic,omitempty"`
}

// OTLPFeatureConfig contains configuration for OTLP ingest.
// +k8s:openapi-gen=true
type OTLPFeatureConfig struct {
	// Receiver contains configuration for the OTLP ingest receiver.
	Receiver OTLPReceiverConfig `json:"receiver,omitempty"`
}

// OTLPReceiverConfig contains configuration for the OTLP ingest receiver.
// +k8s:openapi-gen=true
type OTLPReceiverConfig struct {
	// Protocols contains configuration for the OTLP ingest receiver protocols.
	Protocols OTLPProtocolsConfig `json:"protocols,omitempty"`
}

// OTLPProtocolsConfig contains configuration for the OTLP ingest receiver protocols.
// +k8s:openapi-gen=true
type OTLPProtocolsConfig struct {
	// GRPC contains configuration for the OTLP ingest OTLP/gRPC receiver.
	// +optional
	GRPC *OTLPGRPCConfig `json:"grpc,omitempty"`

	// HTTP contains configuration for the OTLP ingest OTLP/HTTP receiver.
	// +optional
	HTTP *OTLPHTTPConfig `json:"http,omitempty"`
}

// OTLPGRPCConfig contains configuration for the OTLP ingest OTLP/gRPC receiver.
// +k8s:openapi-gen=true
type OTLPGRPCConfig struct {
	// Enable the OTLP/gRPC endpoint. Host port is enabled by default and can be disabled.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Enable hostPort for OTLP/gRPC
	// Default: true
	// +optional
	HostPortConfig *HostPortConfig `json:"hostPortConfig,omitempty"`

	// Endpoint for OTLP/gRPC.
	// gRPC supports several naming schemes: https://github.com/grpc/grpc/blob/master/doc/naming.md
	// The Datadog Operator supports only 'host:port' (usually `0.0.0.0:port`).
	// Default: `0.0.0.0:4317`.
	// +optional
	Endpoint *string `json:"endpoint,omitempty"`
}

// OTLPHTTPConfig contains configuration for the OTLP ingest OTLP/HTTP receiver.
// +k8s:openapi-gen=true
type OTLPHTTPConfig struct {
	// Enable the OTLP/HTTP endpoint. Host port is enabled by default and can be disabled.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Enable hostPorts for OTLP/HTTP
	// Default: true
	// +optional
	HostPortConfig *HostPortConfig `json:"hostPortConfig,omitempty"`

	// Endpoint for OTLP/HTTP.
	// Default: '0.0.0.0:4318'.
	// +optional
	Endpoint *string `json:"endpoint,omitempty"`
}

// EventCollectionFeatureConfig contains the Event Collection configuration.
// +k8s:openapi-gen=true
type EventCollectionFeatureConfig struct {
	// CollectKubernetesEvents enables Kubernetes event collection.
	// Default: true
	CollectKubernetesEvents *bool `json:"collectKubernetesEvents,omitempty"`

	// UnbundleEvents enables collection of Kubernetes events as individual events.
	// Default: false
	// +optional
	UnbundleEvents *bool `json:"unbundleEvents,omitempty"`

	// CollectedEventTypes defines the list of events to collect when UnbundleEvents is enabled.
	// Default:
	// [
	// {"kind":"Pod","reasons":["Failed","BackOff","Unhealthy","FailedScheduling","FailedMount","FailedAttachVolume"]},
	// {"kind":"Node","reasons":["TerminatingEvictedPod","NodeNotReady","Rebooted","HostPortConflict"]},
	// {"kind":"CronJob","reasons":["SawCompletedJob"]}
	// ]
	// +optional
	// +listType=atomic
	CollectedEventTypes []EventTypes `json:"collectedEventTypes,omitempty"`
}

// EventTypes defines the kind and reasons of events to collect.
type EventTypes struct {
	// Kind is the kind of event to collect. (ex: Pod, Node, CronJob)
	Kind string `json:"kind"`

	// Reasons is a list of event reasons to collect. (ex: Failed, BackOff, Unhealthy)
	// +listType=atomic
	Reasons []string `json:"reasons"`
}

// OrchestratorExplorerFeatureConfig contains the Orchestrator Explorer check feature configuration.
// The Orchestrator Explorer check runs in the Process and Cluster Agents (or Cluster Check Runners).
// See also: https://docs.datadoghq.com/infrastructure/livecontainers/#kubernetes-resources
// +k8s:openapi-gen=true
type OrchestratorExplorerFeatureConfig struct {
	// Enabled enables the Orchestrator Explorer.
	// Default: true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Conf overrides the configuration for the default Orchestrator Explorer check.
	// This must point to a ConfigMap containing a valid cluster check configuration.
	// +optional
	Conf *CustomConfig `json:"conf,omitempty"`

	// ScrubContainers enables scrubbing of sensitive container data (passwords, tokens, etc. ).
	// Default: true
	// +optional
	ScrubContainers *bool `json:"scrubContainers,omitempty"`

	// `CustomResources` defines custom resources for the orchestrator explorer to collect.
	// Each item should follow the convention `group/version/kind`. For example, `datadoghq.com/v1alpha1/datadogmetrics`.
	// +optional
	// +listType=set
	CustomResources []string `json:"customResources,omitempty"`

	// Additional tags to associate with the collected data in the form of `a b c`.
	// This is a Cluster Agent option distinct from DD_TAGS that is used in the Orchestrator Explorer.
	// +optional
	// +listType=set
	ExtraTags []string `json:"extraTags,omitempty"`

	// Override the API endpoint for the Orchestrator Explorer.
	// URL Default: "https://orchestrator.datadoghq.com".
	// +optional
	DDUrl *string `json:"ddUrl,omitempty"`
}

// KubeStateMetricsCoreFeatureConfig contains the Kube State Metrics Core check feature configuration.
// The Kube State Metrics Core check runs in the Cluster Agent (or Cluster Check Runners).
// See also: https://docs.datadoghq.com/integrations/kubernetes_state_core
// +k8s:openapi-gen=true
type KubeStateMetricsCoreFeatureConfig struct {
	// Enabled enables Kube State Metrics Core.
	// Default: true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Conf overrides the configuration for the default Kubernetes State Metrics Core check.
	// This must point to a ConfigMap containing a valid cluster check configuration.
	// +optional
	Conf *CustomConfig `json:"conf,omitempty"`
}

// OtelCollectorFeatureConfig contains the configuration for the otel-agent.
// +k8s:openapi-gen=true
type OtelCollectorFeatureConfig struct {
	// Enabled enables the OTel Agent.
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Conf overrides the configuration for the default Kubernetes State Metrics Core check.
	// This must point to a ConfigMap containing a valid cluster check configuration.
	// When passing a configmap, file name *must* be otel-config.yaml.
	// +optional
	Conf *CustomConfig `json:"conf,omitempty"`

	// Ports contains the ports for the otel-agent.
	// Defaults: otel-grpc:4317 / otel-http:4318. Note: setting 4317
	// or 4318 manually is *only* supported if name match default names (otel-grpc, otel-http).
	// If not, this will lead to a port conflict.
	// This limitation will be lifted once annotations support is removed.
	// +optional
	// +listType=atomic
	Ports []*corev1.ContainerPort `json:"ports,omitempty"`

	// OTelCollector Config Relevant to the Core agent
	// +optional
	CoreConfig *CoreConfig `json:"coreConfig,omitempty"`
}

// CoreConfig exposes the otel collector configs relevant to the core agent.
// +k8s:openapi-gen=true
type CoreConfig struct {
	// Enabled marks otelcollector as enabled in core agent.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Extension URL provides the URL of the ddflareextension to
	// the core agent.
	// +optional
	ExtensionURL *string `json:"extensionURL,omitempty"`

	// Extension URL provides the timout of the ddflareextension to
	// the core agent.
	// +optional
	ExtensionTimeout *int `json:"extensionTimeout,omitempty"`
}

// AdmissionControllerFeatureConfig contains the Admission Controller feature configuration.
// The Admission Controller runs in the Cluster Agent.
type AdmissionControllerFeatureConfig struct {
	// Enabled enables the Admission Controller.
	// Default: true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Validation contains Admission Controller validation configurations.
	// +optional
	Validation *AdmissionControllerValidationConfig `json:"validation,omitempty"`

	// Mutation contains Admission Controller mutation configurations.
	// +optional
	Mutation *AdmissionControllerMutationConfig `json:"mutation,omitempty"`

	// MutateUnlabelled enables config injection without the need of pod label 'admission.datadoghq.com/enabled="true"'.
	// Default: false
	// +optional
	MutateUnlabelled *bool `json:"mutateUnlabelled,omitempty"`

	// ServiceName corresponds to the webhook service name.
	// +optional
	ServiceName *string `json:"serviceName,omitempty"`

	// AgentCommunicationMode corresponds to the mode used by the Datadog application libraries to communicate with the Agent.
	// It can be "hostip", "service", or "socket".
	// +optional
	AgentCommunicationMode *string `json:"agentCommunicationMode,omitempty"`

	// FailurePolicy determines how unrecognized and timeout errors are handled.
	// +optional
	FailurePolicy *string `json:"failurePolicy,omitempty"`

	// WebhookName is a custom name for the MutatingWebhookConfiguration.
	// Default: "datadog-webhook"
	// +optional
	WebhookName *string `json:"webhookName,omitempty"`

	// AgentSidecarInjection contains Agent sidecar injection configurations.
	// +optional
	AgentSidecarInjection *AgentSidecarInjectionConfig `json:"agentSidecarInjection,omitempty"`

	// Registry defines an image registry for the admission controller.
	// +optional
	Registry *string `json:"registry,omitempty"`

	// KubernetesAdmissionEvents holds the Kubernetes Admission Events configuration.
	// +optional
	KubernetesAdmissionEvents *KubernetesAdmissionEventsConfig `json:"kubernetesAdmissionEvents,omitempty"`

	// CWSInstrumentation holds the CWS Instrumentation endpoint configuration
	// +optional
	CWSInstrumentation *CWSInstrumentationConfig `json:"cwsInstrumentation,omitempty"`
}

type AdmissionControllerValidationConfig struct {
	// Enabled enables the Admission Controller validation webhook.
	// Default: true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

type AdmissionControllerMutationConfig struct {
	// Enabled enables the Admission Controller mutation webhook.
	// Default: true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

type AgentSidecarInjectionConfig struct {
	// Enabled enables Sidecar injections.
	// Default: false
	// +optional
	Enabled *bool `json:"enabled"`

	// ClusterAgentCommunicationEnabled enables communication between Agent sidecars and the Cluster Agent.
	// Default : true
	// +optional
	ClusterAgentCommunicationEnabled *bool `json:"clusterAgentCommunicationEnabled,omitempty"`

	// Provider is used to add infrastructure provider-specific configurations to the Agent sidecar.
	// Currently only "fargate" is supported.
	// To use the feature in other environments (including local testing) omit the config.
	// See also: https://docs.datadoghq.com/integrations/eks_fargate
	// +optional
	Provider *string `json:"provider,omitempty"`

	// Registry overrides the default registry for the sidecar Agent.
	// +optional
	Registry *string `json:"registry,omitempty"`

	// Image overrides the default Agent image name and tag for the Agent sidecar.
	// +optional
	Image *AgentImageConfig `json:"image,omitempty"`

	// Selectors define the pod selector for sidecar injection. Only one rule is supported.
	// +optional
	// +listType=atomic
	Selectors []*Selector `json:"selectors,omitempty"`

	// Profiles define the sidecar configuration override. Only one profile is supported.
	// +optional
	// +listType=atomic
	Profiles []*Profile `json:"profiles,omitempty"`
}

// Selectors define a pod selector for sidecar injection.
type Selector struct {
	// NamespaceSelector specifies the label selector for namespaces.
	// +optional
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`

	// ObjectSelector specifies the label selector for objects.
	// +optional
	ObjectSelector *metav1.LabelSelector `json:"objectSelector,omitempty"`
}

// Profile defines a sidecar configuration override.
type Profile struct {
	// EnvVars specifies the environment variables for the profile.
	// +optional
	// +listType=map
	// +listMapKey=name
	EnvVars []corev1.EnvVar `json:"env,omitempty"`

	// ResourceRequirements specifies the resource requirements for the profile.
	// +optional
	ResourceRequirements *corev1.ResourceRequirements `json:"resources,omitempty"`

	// SecurityContext specifies the security context for the profile.
	// +optional
	SecurityContext *corev1.SecurityContext `json:"securityContext,omitempty"`
}

type KubernetesAdmissionEventsConfig struct {
	// Enable the Kubernetes Admission Events feature.
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// CWSInstrumentationConfig contains the configuration of the CWS Instrumentation admission controller endpoint.
type CWSInstrumentationConfig struct {
	// Enable the CWS Instrumentation admission controller endpoint.
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Mode defines the behavior of the CWS Instrumentation endpoint, and can be either "init_container" or "remote_copy".
	// Default: "remote_copy"
	// +optional
	Mode *string `json:"mode,omitempty"`
}

// ExternalMetricsServerFeatureConfig contains the External Metrics Server feature configuration.
// The External Metrics Server runs in the Cluster Agent.
type ExternalMetricsServerFeatureConfig struct {
	// Enabled enables the External Metrics Server.
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// RegisterAPIService registers the External Metrics endpoint as an APIService
	// Default: true
	// +optional
	RegisterAPIService *bool `json:"registerAPIService,omitempty"`

	// WPAController enables the informer and controller of the Watermark Pod Autoscaler.
	// NOTE: The Watermark Pod Autoscaler controller needs to be installed.
	// See also: https://github.com/DataDog/watermarkpodautoscaler.
	// Default: false
	// +optional
	WPAController *bool `json:"wpaController,omitempty"`

	// UseDatadogMetrics enables usage of the DatadogMetrics CRD (allowing one to scale on arbitrary Datadog metric queries).
	// Default: true
	// +optional
	UseDatadogMetrics *bool `json:"useDatadogMetrics,omitempty"`

	// Port specifies the metricsProvider External Metrics Server service port.
	// Default: 8443
	// +optional
	Port *int32 `json:"port,omitempty"`

	// Override the API endpoint for the External Metrics Server.
	// URL Default: "https://app.datadoghq.com".
	// +optional
	Endpoint *Endpoint `json:"endpoint,omitempty"`
}

// AutoscalingFeatureConfig contains the Autoscaling product configuration.
type AutoscalingFeatureConfig struct {
	// Workload contains the configuration for the workload autoscaling product.
	Workload *WorkloadAutoscalingFeatureConfig `json:"workload,omitempty"`
}

// WorkloadAutoscalingFeatureConfig contains the configuration for the workload autoscaling product.
type WorkloadAutoscalingFeatureConfig struct {
	// Enabled enables the workload autoscaling product.
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// ClusterChecksFeatureConfig contains the Cluster Checks feature configuration.
// Cluster Checks are picked up and scheduled by the Cluster Agent.
// Cluster Checks Runners are Agents dedicated to running Cluster Checks dispatched by the Cluster Agent.
// (If Cluster Checks Runners are not activated, checks are dispatched to Node Agents).
type ClusterChecksFeatureConfig struct {
	// Enables Cluster Checks scheduling in the Cluster Agent.
	// Default: true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Enabled enables Cluster Checks Runners to run all Cluster Checks.
	// Default: false
	// +optional
	UseClusterChecksRunners *bool `json:"useClusterChecksRunners,omitempty"`
}

// PrometheusScrapeFeatureConfig allows configuration of the Prometheus Autodiscovery feature.
// +k8s:openapi-gen=true
type PrometheusScrapeFeatureConfig struct {
	// Enable autodiscovery of pods and services exposing Prometheus metrics.
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// EnableServiceEndpoints enables generating dedicated checks for service endpoints.
	// Default: false
	// +optional
	EnableServiceEndpoints *bool `json:"enableServiceEndpoints,omitempty"`

	// AdditionalConfigs allows adding advanced Prometheus check configurations with custom discovery rules.
	// +optional
	AdditionalConfigs *string `json:"additionalConfigs,omitempty"`

	// Version specifies the version of the OpenMetrics check.
	// Default: 2
	// +optional
	Version *int `json:"version,omitempty"`
}

// HelmCheckFeatureConfig allows configuration of the Helm check feature.
// +k8s:openapi-gen=true
type HelmCheckFeatureConfig struct {
	// Enabled enables the Helm check.
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// CollectEvents set to `true` enables event collection in the Helm check
	// (Requires Agent 7.36.0+ and Cluster Agent 1.20.0+)
	// Default: false
	// +optional
	CollectEvents *bool `json:"collectEvents,omitempty"`

	// ValuesAsTags collects Helm values from a release and uses them as tags
	// (Requires Agent and Cluster Agent 7.40.0+).
	// Default: {}
	// +optional
	ValuesAsTags map[string]string `json:"valuesAsTags,omitempty"`
}

// Generic support structs

// SecretConfig contains a secret name and an included key.
// +kubebuilder:object:generate=true
type SecretConfig struct {
	// SecretName is the name of the secret.
	SecretName string `json:"secretName"`

	// KeyName is the key of the secret to use.
	// +optional
	KeyName string `json:"keyName,omitempty"`
}

// ConfigMapConfig contains ConfigMap information used to store a configuration file.
// +kubebuilder:object:generate=true
type ConfigMapConfig struct {
	// Name is the name of the ConfigMap.
	Name string `json:"name,omitempty"`

	// Items maps a ConfigMap data `key` to a file `path` mount.
	// +listType=map
	// +listMapKey=key
	// +optional
	Items []corev1.KeyToPath `json:"items,omitempty"`
}

// CustomConfig provides a place for custom configuration of the Agent or Cluster Agent, corresponding to datadog.yaml,
// system-probe.yaml, security-agent.yaml or datadog-cluster.yaml.
// The configuration can be provided in the ConfigData field as raw data, or referenced in a ConfigMap.
// Note: `ConfigData` and `ConfigMap` cannot be set together.
// +k8s:openapi-gen=true
type CustomConfig struct {
	// ConfigData corresponds to the configuration file content.
	ConfigData *string `json:"configData,omitempty"`

	// ConfigMap references an existing ConfigMap with the configuration file content.
	ConfigMap *ConfigMapConfig `json:"configMap,omitempty"`
}

// MultiCustomConfig provides a place for custom configuration of the Agent or Cluster Agent, corresponding to /confd/*.yaml.
// The configuration can be provided in the ConfigDataMap field as raw data, or referenced in a single ConfigMap.
// Note: `ConfigDataMap` and `ConfigMap` cannot be set together.
// +k8s:openapi-gen=true
type MultiCustomConfig struct {
	// ConfigDataMap corresponds to the content of the configuration files.
	// The key should be the filename the contents get mounted to; for instance check.py or check.yaml.
	ConfigDataMap map[string]string `json:"configDataMap,omitempty"`

	// ConfigMap references an existing ConfigMap with the content of the configuration files.
	ConfigMap *ConfigMapConfig `json:"configMap,omitempty"`
}

// KubeletConfig contains the kubelet configuration parameters.
// +kubebuilder:object:generate=true
type KubeletConfig struct {
	// Host overrides the host used to contact kubelet API (default to status.hostIP).
	// +optional
	Host *corev1.EnvVarSource `json:"host,omitempty"`

	// TLSVerify toggles kubelet TLS verification.
	// Default: true
	// +optional
	TLSVerify *bool `json:"tlsVerify,omitempty"`

	// HostCAPath is the host path where the kubelet CA certificate is stored.
	// +optional
	HostCAPath string `json:"hostCAPath,omitempty"`

	// AgentCAPath is the container path where the kubelet CA certificate is stored.
	// Default: '/var/run/host-kubelet-ca.crt' if hostCAPath is set, else '/var/run/secrets/kubernetes.io/serviceaccount/ca.crt'
	// +optional
	AgentCAPath string `json:"agentCAPath,omitempty"`

	// PodResourcesSocketPath is the host path where the pod resources socket is stored.
	// Default: `/var/lib/kubelet/pod-resources/`
	// +optional
	PodResourcesSocketPath string `json:"podResourcesSocketPath,omitempty"`
}

// HostPortConfig contains host port configuration.
type HostPortConfig struct {
	// Enabled enables host port configuration
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Port takes a port number (0 < x < 65536) to expose on the host. (Most containers do not need this.)
	// If HostNetwork is enabled, this value must match the ContainerPort.
	// +optional
	Port *int32 `json:"hostPort,omitempty"`
}

// UnixDomainSocketConfig contains the Unix Domain Socket configuration.
// +k8s:openapi-gen=true
type UnixDomainSocketConfig struct {
	// Enabled enables Unix Domain Socket.
	// Default: true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Path defines the socket path used when enabled.
	// +optional
	Path *string `json:"path,omitempty"`
}

// Endpoint configures an endpoint and its associated Datadog credentials.
type Endpoint struct {
	// URL defines the endpoint URL.
	URL *string `json:"url,omitempty"`

	// Credentials defines the Datadog credentials used to submit data to/query data from Datadog.
	Credentials *DatadogCredentials `json:"credentials,omitempty"`
}

// OriginDetectionUnified defines the origin detection unified mechanism behavior.
type OriginDetectionUnified struct {
	// Enabled enables unified mechanism for origin detection.
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// AgentImageConfig defines the agent container image config.
// +kubebuilder:object:generate=true
type AgentImageConfig struct {
	// Defines the Agent image name for the pod. You can provide this as:
	// * `<NAME>` - Use `agent` for the Datadog Agent, `cluster-agent` for the Datadog Cluster Agent, or `dogstatsd`
	// for DogStatsD. The full image string is derived from `global.registry`, `[key].image.tag`, and `[key].image.jmxEnabled`.
	// * `<NAME>:<TAG>` - For example, `agent:latest`. The registry is derived from `global.registry`. `[key].image.tag`
	// and `[key].image.jmxEnabled` are ignored.
	// * `<REGISTRY>/<NAME>:<TAG>` - For example, `gcr.io/datadoghq/agent:latest`. If the full image string is specified
	//   like this, then `global.registry`, `[key].image.tag`, and `[key].image.jmxEnabled` are ignored.
	Name string `json:"name,omitempty"`

	// Define the image tag to use.
	// To be used if the `Name` field does not correspond to a full image string.
	// +optional
	Tag string `json:"tag,omitempty"`

	// Define whether the Agent image should support JMX.
	// To be used if the `Name` field does not correspond to a full image string.
	// +optional
	JMXEnabled bool `json:"jmxEnabled,omitempty"`

	// The Kubernetes pull policy:
	// Use `Always`, `Never`, or `IfNotPresent`.
	PullPolicy *corev1.PullPolicy `json:"pullPolicy,omitempty"`

	// It is possible to specify Docker registry credentials.
	// See https://kubernetes.io/docs/concepts/containers/images/#specifying-imagepullsecrets-on-a-pod
	// +optional
	PullSecrets *[]corev1.LocalObjectReference `json:"pullSecrets,omitempty"`
}

// DaemonSetStatus defines the observed state of Agent running as DaemonSet.
// +k8s:openapi-gen=true
// +kubebuilder:object:generate=true
type DaemonSetStatus struct {
	// Number of desired pods in the DaemonSet.
	Desired int32 `json:"desired"`

	// Number of current pods in the DaemonSet.
	Current int32 `json:"current"`

	// Number of ready pods in the DaemonSet.
	Ready int32 `json:"ready"`

	// Number of available pods in the DaemonSet.
	Available int32 `json:"available"`

	// Number of up to date pods in the DaemonSet.
	UpToDate int32 `json:"upToDate"`

	// LastUpdate is the last time the status was updated.
	LastUpdate *metav1.Time `json:"lastUpdate,omitempty"`

	// CurrentHash is the stored hash of the DaemonSet.
	CurrentHash string `json:"currentHash,omitempty"`

	// Status corresponds to the DaemonSet computed status.
	Status string `json:"status,omitempty"`

	// State corresponds to the DaemonSet state.
	State string `json:"state,omitempty"`

	// DaemonsetName corresponds to the name of the created DaemonSet.
	DaemonsetName string `json:"daemonsetName,omitempty"`
}

// DeploymentStatus type representing a Deployment status.
// +k8s:openapi-gen=true
// +kubebuilder:object:generate=true
type DeploymentStatus struct {
	// Total number of non-terminated pods targeted by this Deployment (their labels match the selector).
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// Total number of non-terminated pods targeted by this Deployment that have the desired template spec.
	// +optional
	UpdatedReplicas int32 `json:"updatedReplicas,omitempty"`

	// Total number of ready pods targeted by this Deployment.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// Total number of available pods (ready for at least minReadySeconds) targeted by this Deployment.
	// +optional
	AvailableReplicas int32 `json:"availableReplicas,omitempty"`

	// Total number of unavailable pods targeted by this Deployment. This is the total number of
	// pods that are still required for the Deployment to have 100% available capacity. They may
	// either be pods that are running but not yet available or pods that still have not been created.
	// +optional
	UnavailableReplicas int32 `json:"unavailableReplicas,omitempty"`

	// LastUpdate is the last time the status was updated.
	LastUpdate *metav1.Time `json:"lastUpdate,omitempty"`

	// CurrentHash is the stored hash of the Deployment.
	CurrentHash string `json:"currentHash,omitempty"`

	// GeneratedToken corresponds to the generated token if any token was provided in the Credential configuration when ClusterAgent is
	// enabled.
	// +optional
	GeneratedToken string `json:"generatedToken,omitempty"`

	// Status corresponds to the Deployment computed status.
	Status string `json:"status,omitempty"`

	// State corresponds to the Deployment state.
	State string `json:"state,omitempty"`

	// DeploymentName corresponds to the name of the Deployment.
	DeploymentName string `json:"deploymentName,omitempty"`
}

// GlobalConfig is a set of parameters that are used to configure all the components of the Datadog Operator.
type GlobalConfig struct {
	// Credentials defines the Datadog credentials used to submit data to/query data from Datadog.
	Credentials *DatadogCredentials `json:"credentials,omitempty"`

	// ClusterAgentToken is the token for communication between the NodeAgent and ClusterAgent.
	ClusterAgentToken *string `json:"clusterAgentToken,omitempty"`

	// ClusterAgentTokenSecret is the secret containing the Cluster Agent token.
	ClusterAgentTokenSecret *SecretConfig `json:"clusterAgentTokenSecret,omitempty"`

	// ClusterName sets a unique cluster name for the deployment to easily scope monitoring data in the Datadog app.
	// +optional
	ClusterName *string `json:"clusterName,omitempty"`

	// Site is the Datadog intake site Agent data are sent to.
	// Set to 'datadoghq.com' to send data to the US1 site (default).
	// Set to 'datadoghq.eu' to send data to the EU site.
	// Set to 'us3.datadoghq.com' to send data to the US3 site.
	// Set to 'us5.datadoghq.com' to send data to the US5 site.
	// Set to 'ddog-gov.com' to send data to the US1-FED site.
	// Set to 'ap1.datadoghq.com' to send data to the AP1 site.
	// Default: 'datadoghq.com'
	// +optional
	Site *string `json:"site,omitempty"`

	// Endpoint is the Datadog intake URL the Agent data are sent to.
	// Only set this option if you need the Agent to send data to a custom URL.
	// Overrides the site setting defined in `Site`.
	// +optional
	Endpoint *Endpoint `json:"endpoint,omitempty"`

	// Registry is the image registry to use for all Agent images.
	// Use 'public.ecr.aws/datadog' for AWS ECR.
	// Use 'datadoghq.azurecr.io' for Azure Container Registry.
	// Use 'gcr.io/datadoghq' for Google Container Registry.
	// Use 'eu.gcr.io/datadoghq' for Google Container Registry in the EU region.
	// Use 'asia.gcr.io/datadoghq' for Google Container Registry in the Asia region.
	// Use 'docker.io/datadog' for DockerHub.
	// Default: 'gcr.io/datadoghq'
	// +optional
	Registry *string `json:"registry,omitempty"`

	// LogLevel sets logging verbosity. This can be overridden by container.
	// Valid log levels are: trace, debug, info, warn, error, critical, and off.
	// Default: 'info'
	LogLevel *string `json:"logLevel,omitempty"`

	// Tags contains a list of tags to attach to every metric, event and service check collected.
	// Learn more about tagging: https://docs.datadoghq.com/tagging/
	// +optional
	// +listType=set
	Tags []string `json:"tags,omitempty"`

	// Env contains a list of environment variables that are set for all Agents.
	// +optional
	// +listType=map
	// +listMapKey=name
	Env []corev1.EnvVar `json:"env,omitempty"`

	// ChecksTagCardinality configures tag cardinality for the metrics collected by integrations (`low`, `orchestrator` or `high`).
	// See also: https://docs.datadoghq.com/getting_started/tagging/assigning_tags/?tab=containerizedenvironments#tags-cardinality.
	// Not set by default to avoid overriding existing DD_CHECKS_TAG_CARDINALITY configurations, the default value in the Agent is low.
	// Ref: https://github.com/DataDog/datadog-agent/blob/856cf4a66142ce91fd4f8a278149436eb971184a/pkg/config/setup/config.go#L625.
	// +optional
	ChecksTagCardinality *string `json:"checksTagCardinality,omitempty"`

	// OriginDetectionUnified defines the origin detection unified mechanism behavior.
	// +optional
	OriginDetectionUnified *OriginDetectionUnified `json:"originDetectionUnified,omitempty"`

	// Provide a mapping of Kubernetes Labels to Datadog Tags.
	// <KUBERNETES_LABEL>: <DATADOG_TAG_KEY>
	// +optional
	PodLabelsAsTags map[string]string `json:"podLabelsAsTags,omitempty"`

	// Provide a mapping of Kubernetes Annotations to Datadog Tags.
	// <KUBERNETES_ANNOTATIONS>: <DATADOG_TAG_KEY>
	// +optional
	PodAnnotationsAsTags map[string]string `json:"podAnnotationsAsTags,omitempty"`

	// Provide a mapping of Kubernetes Node Labels to Datadog Tags.
	// <KUBERNETES_NODE_LABEL>: <DATADOG_TAG_KEY>
	// +optional
	NodeLabelsAsTags map[string]string `json:"nodeLabelsAsTags,omitempty"`

	// Provide a mapping of Kubernetes Namespace Labels to Datadog Tags.
	// <KUBERNETES_NAMESPACE_LABEL>: <DATADOG_TAG_KEY>
	// +optional
	NamespaceLabelsAsTags map[string]string `json:"namespaceLabelsAsTags,omitempty"`

	// Provide a mapping of Kubernetes Namespace Annotations to Datadog Tags.
	// <KUBERNETES_LABEL>: <DATADOG_TAG_KEY>
	// +optional
	NamespaceAnnotationsAsTags map[string]string `json:"namespaceAnnotationsAsTags,omitempty"`

	// Provide a mapping of Kubernetes Resource Groups to labels mapping to Datadog Tags.
	// <KUBERNETES_RESOURCE_GROUP>:
	//		<KUBERNETES_LABEL>: <DATADOG_TAG_KEY>
	// KUBERNETES_RESOURCE_GROUP should be in the form `{resource}.{group}` or `{resource}` (example: deployments.apps, pods)
	// +optional
	KubernetesResourcesLabelsAsTags map[string]map[string]string `json:"kubernetesResourcesLabelsAsTags,omitempty"`

	// Provide a mapping of Kubernetes Resource Groups to annotations mapping to Datadog Tags.
	// <KUBERNETES_RESOURCE_GROUP>:
	//		<KUBERNETES_ANNOTATION>: <DATADOG_TAG_KEY>
	// KUBERNETES_RESOURCE_GROUP should be in the form `{resource}.{group}` or `{resource}` (example: deployments.apps, pods)
	// +optional
	KubernetesResourcesAnnotationsAsTags map[string]map[string]string `json:"kubernetesResourcesAnnotationsAsTags,omitempty"`

	// CSI contains configuration for Datadog CSI Driver
	CSI *CSIConfig `json:"csi,omitempty"`

	// NetworkPolicy contains the network configuration.
	// +optional
	NetworkPolicy *NetworkPolicyConfig `json:"networkPolicy,omitempty"`

	// LocalService contains configuration to customize the internal traffic policy service.
	// +optional
	LocalService *LocalService `json:"localService,omitempty"`

	// Kubelet contains the kubelet configuration parameters.
	// +optional
	Kubelet *KubeletConfig `json:"kubelet,omitempty"`

	// Path to the docker runtime socket.
	// +optional
	DockerSocketPath *string `json:"dockerSocketPath,omitempty"`

	// Path to the container runtime socket (if different from Docker).
	// +optional
	CriSocketPath *string `json:"criSocketPath,omitempty"`

	// Set DisableNonResourceRules to exclude NonResourceURLs from default ClusterRoles.
	// Required 'true' for Google Cloud Marketplace.
	// +optional
	DisableNonResourceRules *bool `json:"disableNonResourceRules,omitempty"`

	// ContainerStrategy determines whether agents run in a single or multiple containers.
	// Default: 'optimized'
	// +optional
	ContainerStrategy *ContainerStrategyType `json:"containerStrategy,omitempty"`

	// UseFIPSAgent enables the FIPS flavor of the Agent. If 'true', the FIPS proxy will always be disabled.
	// Default: 'false'
	// +optional
	UseFIPSAgent *bool `json:"useFIPSAgent,omitempty"`

	// FIPS contains configuration used to customize the FIPS proxy sidecar.
	FIPS *FIPSConfig `json:"fips,omitempty"`

	// Configure the secret backend feature https://docs.datadoghq.com/agent/guide/secrets-management
	// See also: https://github.com/DataDog/datadog-operator/blob/main/docs/secret_management.md
	SecretBackend *SecretBackendConfig `json:"secretBackend,omitempty"`

	// Configure whether the Process Agent or core Agent collects process and/or container information (Linux only).
	// If no other checks are running, the Process Agent container will not initialize.
	// (Requires Agent 7.60.0+)
	// Default: 'true'
	// +optional
	RunProcessChecksInCoreAgent *bool `json:"runProcessChecksInCoreAgent,omitempty"`
}

// DatadogCredentials is a generic structure that holds credentials to access Datadog.
// +k8s:openapi-gen=true
type DatadogCredentials struct {
	// APIKey configures your Datadog API key.
	// See also: https://app.datadoghq.com/account/settings#agent/kubernetes
	APIKey *string `json:"apiKey,omitempty"`

	// APISecret references an existing Secret which stores the API key instead of creating a new one.
	// If set, this parameter takes precedence over "APIKey".
	// +optional
	APISecret *SecretConfig `json:"apiSecret,omitempty"`

	// AppKey configures your Datadog application key.
	// If you are using features.externalMetricsServer.enabled = true, you must set
	// a Datadog application key for read access to your metrics.
	// +optional
	AppKey *string `json:"appKey,omitempty"`

	// AppSecret references an existing Secret which stores the application key instead of creating a new one.
	// If set, this parameter takes precedence over "AppKey".
	// +optional
	AppSecret *SecretConfig `json:"appSecret,omitempty"`
}

// SecretBackendRolesConfig provides configuration of the secrets Datadog agents can read for the SecretBackend feature
// +k8s:openapi-gen=true
type SecretBackendRolesConfig struct {
	// Namespace defines the namespace in which the secrets reside.
	// +required
	Namespace *string `json:"namespace,omitempty"`

	// Secrets defines the list of secrets for which a role should be created.
	// +required
	// +listType=set
	Secrets []string `json:"secrets,omitempty"`
}

// SecretBackendConfig provides configuration for the secret backend.
// +k8s:openapi-gen=true
type SecretBackendConfig struct {
	// The secret backend command to use. Datadog provides a pre-defined binary `/readsecret_multiple_providers.sh`.
	// Read more about `/readsecret_multiple_providers.sh` at https://docs.datadoghq.com/agent/configuration/secrets-management/?tab=linux#script-for-reading-from-multiple-secret-providers.
	Command *string `json:"command,omitempty"`

	// List of arguments to pass to the command (space-separated strings).
	// +optional
	Args *string `json:"args,omitempty"`

	// The command timeout in seconds.
	// Default: `30`.
	// +optional
	Timeout *int32 `json:"timeout,omitempty"`

	// The refresh interval for secrets (0 disables refreshing).
	// Default: `0`.
	// +optional
	RefreshInterval *int32 `json:"refreshInterval,omitempty"`

	// Whether to create a global permission allowing Datadog agents to read all Kubernetes secrets.
	// Default: `false`.
	// +optional
	EnableGlobalPermissions *bool `json:"enableGlobalPermissions,omitempty"`

	// Roles for Datadog to read the specified secrets, replacing `enableGlobalPermissions`.
	// They are defined as a list of namespace/secrets.
	// Each defined namespace needs to be present in the DatadogAgent controller using `WATCH_NAMESPACE` or `DD_AGENT_WATCH_NAMESPACE`.
	// See also: https://github.com/DataDog/datadog-operator/blob/main/docs/secret_management.md#how-to-deploy-the-agent-components-using-the-secret-backend-feature-with-datadogagent.
	// +optional
	// +listType=atomic
	Roles []*SecretBackendRolesConfig `json:"roles,omitempty"`
}

// NetworkPolicyFlavor specifies which flavor of Network Policy to use.
type NetworkPolicyFlavor string

const (
	// NetworkPolicyFlavorKubernetes refers to  `networking.k8s.io/v1/NetworkPolicy`
	NetworkPolicyFlavorKubernetes NetworkPolicyFlavor = "kubernetes"

	// NetworkPolicyFlavorCilium refers to `cilium.io/v2/CiliumNetworkPolicy`
	NetworkPolicyFlavorCilium NetworkPolicyFlavor = "cilium"
)

// NetworkPolicyConfig provides Network Policy configuration for the agents.
// +k8s:openapi-gen=true
type NetworkPolicyConfig struct {
	// Create defines whether to create a NetworkPolicy for the current deployment.
	// +optional
	Create *bool `json:"create,omitempty"`

	// Flavor defines Which network policy to use.
	// +optional
	Flavor NetworkPolicyFlavor `json:"flavor,omitempty"`

	// DNSSelectorEndpoints defines the cilium selector of the DNS server entity.
	// +optional
	// +listType=atomic
	DNSSelectorEndpoints []metav1.LabelSelector `json:"dnsSelectorEndpoints,omitempty"`
}

// LocalService provides the internal traffic policy service configuration.
// +k8s:openapi-gen=true
type LocalService struct {
	// NameOverride defines the name of the internal traffic service to target the agent running on the local node.
	// +optional
	NameOverride *string `json:"nameOverride,omitempty"`

	// ForceEnableLocalService forces the creation of the internal traffic policy service to target the agent running on the local node.
	// This parameter only applies to Kubernetes 1.21, where the feature is in alpha and is disabled by default.
	// (On Kubernetes 1.22+, the feature entered beta and the internal traffic service is created by default, so this parameter is ignored.)
	// Default: false
	// +optional
	ForceEnableLocalService *bool `json:"forceEnableLocalService,omitempty"`
}

// SeccompConfig is used to override default values for Seccomp Profile configurations.
// +k8s:openapi-gen=true
type SeccompConfig struct {
	// CustomRootPath specifies a custom Seccomp Profile root location.
	// +optional
	CustomRootPath *string `json:"customRootPath,omitempty"`

	// CustomProfile specifies a ConfigMap containing a custom Seccomp Profile.
	// ConfigMap data must either have the key `system-probe-seccomp.json` or CustomProfile.Items
	// must include a corev1.KeytoPath that maps the key to the path `system-probe-seccomp.json`.
	// +optional
	CustomProfile *CustomConfig `json:"customProfile,omitempty"`
}

// AgentConfigFileName is the list of known Agent config files
type AgentConfigFileName string

const (
	// AgentGeneralConfigFile is the name of the main Agent config file
	AgentGeneralConfigFile AgentConfigFileName = "datadog.yaml"
	// SystemProbeConfigFile is the name of the of System Probe config file
	SystemProbeConfigFile AgentConfigFileName = "system-probe.yaml"
	// SecurityAgentConfigFile is the name of the Security Agent config file
	SecurityAgentConfigFile AgentConfigFileName = "security-agent.yaml"
	// ClusterAgentConfigFile is the name of the Cluster Agent config file
	ClusterAgentConfigFile AgentConfigFileName = "datadog-cluster.yaml"
)

// DatadogAgentComponentOverride is the generic description equivalent to a subset of the PodTemplate for a component.
type DatadogAgentComponentOverride struct {
	// Name overrides the default name for the resource
	// +optional
	Name *string `json:"name,omitempty"`

	// Number of the replicas.
	// Not applicable for a DaemonSet/ExtendedDaemonSet deployment
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Set CreatePodDisruptionBudget to true to create a PodDisruptionBudget for this component.
	// Not applicable for the Node Agent. A Cluster Agent PDB is set with 1 minimum available pod, and a Cluster Checks Runner PDB is set with 1 maximum unavailable pod.
	// +optional
	CreatePodDisruptionBudget *bool `json:"createPodDisruptionBudget,omitempty"`

	// Set CreateRbac to false to prevent automatic creation of Role/ClusterRole for this component
	// +optional
	CreateRbac *bool `json:"createRbac,omitempty"`

	// Sets the ServiceAccount used by this component.
	// Ignored if the field CreateRbac is true.
	// +optional
	ServiceAccountName *string `json:"serviceAccountName,omitempty"`

	// Sets the ServiceAccountAnnotations used by this component.
	// +optional
	ServiceAccountAnnotations map[string]string `json:"serviceAccountAnnotations,omitempty"`

	// The container image of the different components (Datadog Agent, Cluster Agent, Cluster Check Runner).
	// +optional
	Image *AgentImageConfig `json:"image,omitempty"`

	// Specify additional environment variables for all containers in this component
	// Priority is Container > Component.
	// See also: https://docs.datadoghq.com/agent/kubernetes/?tab=helm#environment-variables
	//
	// +optional
	// +listType=map
	// +listMapKey=name
	Env []corev1.EnvVar `json:"env,omitempty" patchMergeKey:"name" patchStrategy:"merge"`

	// EnvFrom specifies the ConfigMaps and Secrets to expose as environment variables.
	// Priority is env > envFrom.
	// +optional
	EnvFrom []corev1.EnvFromSource `json:"envFrom,omitempty"`

	// CustomConfiguration allows to specify custom configuration files for `datadog.yaml`, `datadog-cluster.yaml`, `security-agent.yaml`, and `system-probe.yaml`.
	// The content is merged with configuration generated by the Datadog Operator, with priority given to custom configuration.
	// WARNING: It is possible to override values set in the `DatadogAgent`.
	// +optional
	CustomConfigurations map[AgentConfigFileName]CustomConfig `json:"customConfigurations,omitempty"`

	// Confd configuration allowing to specify config files for custom checks placed under /etc/datadog-agent/conf.d/.
	// See https://docs.datadoghq.com/agent/guide/agent-configuration-files/?tab=agentv6 for more details.
	// +optional
	ExtraConfd *MultiCustomConfig `json:"extraConfd,omitempty"`

	// Checksd configuration allowing to specify custom checks placed under /etc/datadog-agent/checks.d/
	// See https://docs.datadoghq.com/agent/guide/agent-configuration-files/?tab=agentv6 for more details.
	// +optional
	ExtraChecksd *MultiCustomConfig `json:"extraChecksd,omitempty"`

	// Configure the basic configurations for each Agent container. Valid Agent container names are:
	// `agent`, `cluster-agent`, `init-config`, `init-volume`, `process-agent`, `seccomp-setup`,
	// `security-agent`, `system-probe`, and `trace-agent`.
	// +optional
	Containers map[common.AgentContainerName]*DatadogAgentGenericContainer `json:"containers,omitempty"`

	// Specify additional volumes in the different components (Datadog Agent, Cluster Agent, Cluster Check Runner).
	// +optional
	// +listType=map
	// +listMapKey=name
	Volumes []corev1.Volume `json:"volumes,omitempty"`

	// Pod-level SecurityContext.
	// +optional
	SecurityContext *corev1.PodSecurityContext `json:"securityContext,omitempty"`

	// If specified, indicates the pod's priority. "system-node-critical" and "system-cluster-critical"
	// are two special keywords which indicate the highest priorities with the former being the highest priority.
	// Any other name must be defined by creating a PriorityClass object with that name. If not specified,
	// the pod priority is default, or zero if there is no default.
	PriorityClassName *string `json:"priorityClassName,omitempty"`

	// If specified, indicates the pod's RuntimeClass kubelet should use to run the pod.
	// If the named RuntimeClass does not exist, or the CRI cannot run the corresponding handler, the pod enters the Failed terminal phase.
	// If no runtimeClassName is specified, the default RuntimeHandler is used, which is equivalent to the behavior when the RuntimeClass feature is disabled.
	// +optional
	RuntimeClassName *string `json:"runtimeClassName,omitempty"`

	// If specified, the pod's scheduling constraints.
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// Set DNS policy for the pod.
	// Defaults to "ClusterFirst".
	// Valid values are 'ClusterFirstWithHostNet', 'ClusterFirst', 'Default' or 'None'.
	// DNS parameters given in DNSConfig will be merged with the policy selected with DNSPolicy.
	// To have DNS options set along with hostNetwork, you have to specify DNS policy
	// explicitly to 'ClusterFirstWithHostNet'.
	// +optional
	DNSPolicy *corev1.DNSPolicy `json:"dnsPolicy,omitempty"`

	// Specifies the DNS parameters of a pod.
	// Parameters specified here will be merged to the generated DNS
	// configuration based on DNSPolicy.
	// +optional
	DNSConfig *corev1.PodDNSConfig `json:"dnsConfig,omitempty"`

	// A map of key-value pairs. For this pod to run on a specific node, the node must have these key-value pairs as labels.
	// See https://kubernetes.io/docs/concepts/configuration/assign-pod-node/
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// The deployment strategy to use to replace existing pods with new ones.
	// +optional
	UpdateStrategy *common.UpdateStrategy `json:"updateStrategy,omitempty"`

	// Configure the component tolerations.
	// +optional
	// +listType=atomic
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Annotations provide annotations that are added to the different component (Datadog Agent, Cluster Agent, Cluster Check Runner) pods.
	Annotations map[string]string `json:"annotations,omitempty"`

	// AdditionalLabels provide labels that are added to the different component (Datadog Agent, Cluster Agent, Cluster Check Runner) pods.
	//+mapType=granular
	Labels map[string]string `json:"labels,omitempty"`

	// Host networking requested for this pod. Use the host's network namespace.
	// +optional
	HostNetwork *bool `json:"hostNetwork,omitempty"`

	// Use the host's PID namespace.
	// +optional
	HostPID *bool `json:"hostPID,omitempty"`

	// Disabled force disables a component.
	// +optional
	Disabled *bool `json:"disabled,omitempty"`

	// TopologySpreadConstraints describes how a group of pods ought to spread across topology
	// domains. Scheduler will schedule pods in a way which abides by the constraints.
	// All topologySpreadConstraints are ANDed.
	// +optional
	// +listType=map
	// +listMapKey=topologyKey
	// +listMapKey=whenUnsatisfiable
	TopologySpreadConstraints []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`
}

// DatadogAgentGenericContainer is the generic structure describing any container's common configuration.
// +k8s:openapi-gen=true
type DatadogAgentGenericContainer struct {
	// Name of the container that is overridden
	//+optional
	Name *string `json:"name,omitempty"`

	// LogLevel sets logging verbosity (overrides global setting).
	// Valid log levels are: trace, debug, info, warn, error, critical, and off.
	// Default: 'info'
	// +optional
	LogLevel *string `json:"logLevel,omitempty"`

	// Specify additional ports to be exposed by the container. Not specifying a port here
	// DOES NOT prevent that port from being exposed.
	// See https://pkg.go.dev/k8s.io/api/core/v1#Container documentation for more details.
	// +optional
	// +listType=atomic
	Ports []corev1.ContainerPort `json:"ports,omitempty"`

	// Specify additional environment variables in the container.
	// See also: https://docs.datadoghq.com/agent/kubernetes/?tab=helm#environment-variables
	//
	// +optional
	// +listType=map
	// +listMapKey=name
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Specify additional volume mounts in the container.
	// +optional
	// +listType=map
	// +listMapKey=name
	// +listMapKey=mountPath
	VolumeMounts []corev1.VolumeMount `json:"volumeMounts,omitempty"`

	// Specify the Request and Limits of the pods
	// To get guaranteed QoS class, specify requests and limits equal.
	// See also: http://kubernetes.io/docs/user-guide/compute-resources/
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// Command allows the specification of a custom entrypoint for container
	// +listType=atomic
	Command []string `json:"command,omitempty"`

	// Args allows the specification of extra args to the `Command` parameter
	// +listType=atomic
	Args []string `json:"args,omitempty"`

	// HealthPort of the container for the internal liveness probe.
	// Must be the same as the Liveness/Readiness probes.
	// +optional
	HealthPort *int32 `json:"healthPort,omitempty"`

	// Configure the Readiness Probe of the container
	// +optional
	ReadinessProbe *corev1.Probe `json:"readinessProbe,omitempty"`

	// Configure the Liveness Probe of the container
	// +optional
	LivenessProbe *corev1.Probe `json:"livenessProbe,omitempty"`

	// Configure the Startup Probe of the container
	// +optional
	StartupProbe *corev1.Probe `json:"startupProbe,omitempty"`

	// Container-level SecurityContext.
	// +optional
	SecurityContext *corev1.SecurityContext `json:"securityContext,omitempty"`

	// Seccomp configurations to override Operator actions. For all other Seccomp Profile manipulation,
	// use SecurityContext.
	// +optional
	SeccompConfig *SeccompConfig `json:"seccompConfig,omitempty"`

	// AppArmorProfileName specifies an apparmor profile.
	// +optional
	AppArmorProfileName *string `json:"appArmorProfileName,omitempty"`
}

type ContainerStrategyType string

const (
	// OptimizedContainerStrategy indicates multiple Agent containers with one process per
	// container (default)
	OptimizedContainerStrategy ContainerStrategyType = "optimized"
	// SingleContainerStrategy indicates a single Agent container with multiple (unprivileged)
	// processes in one container
	SingleContainerStrategy ContainerStrategyType = "single"
)

// FIPSConfig contains the FIPS configuration.
// +k8s:openapi-gen=true
type FIPSConfig struct {
	// Enable FIPS sidecar.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// The container image of the FIPS sidecar.
	// +optional
	Image *AgentImageConfig `json:"image,omitempty"`
	// Set the local IP address.
	// Default: `127.0.0.1`
	// +optional
	LocalAddress *string `json:"localAddress,omitempty"`
	// Port specifies which port is used by the containers to communicate to the FIPS sidecar.
	// Default: 9803
	// +optional
	Port *int32 `json:"port,omitempty"`
	// PortRange specifies the number of ports used.
	// Default: 15
	// +optional
	PortRange *int32 `json:"portRange,omitempty"`
	// Resources is the requests and limits for the FIPS sidecar container.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
	// UseHTTPS enables HTTPS.
	// Default: false
	// +optional
	UseHTTPS *bool `json:"useHTTPS,omitempty"`
	// CustomFIPSConfig configures a custom configMap to provide the FIPS configuration.
	// Specify custom contents for the FIPS proxy sidecar container config
	// (/etc/datadog-fips-proxy/datadog-fips-proxy.cfg). If empty, the default FIPS
	// proxy sidecar container config is used.
	// +optional
	CustomFIPSConfig *CustomConfig `json:"customFIPSConfig,omitempty"`
}

// RemoteConfigConfiguration stores the configuration received from RemoteConfig.
// +k8s:openapi-gen=true
type RemoteConfigConfiguration struct {
	Features *DatadogFeatures `json:"features,omitempty"`
}

// DatadogAgentStatus defines the observed state of DatadogAgent.
// +k8s:openapi-gen=true
type DatadogAgentStatus struct {
	// Conditions Represents the latest available observations of a DatadogAgent's current state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions"`
	// The actual state of the Agent as a daemonset or an extended daemonset.
	// +optional
	// +listType=atomic
	AgentList []*DaemonSetStatus `json:"agentList,omitempty"`
	// The combined actual state of all Agents as daemonsets or extended daemonsets.
	// +optional
	Agent *DaemonSetStatus `json:"agent,omitempty"`
	// The actual state of the Cluster Agent as a deployment.
	// +optional
	ClusterAgent *DeploymentStatus `json:"clusterAgent,omitempty"`
	// The actual state of the Cluster Checks Runner as a deployment.
	// +optional
	ClusterChecksRunner *DeploymentStatus `json:"clusterChecksRunner,omitempty"`
	// RemoteConfigConfiguration stores the configuration received from RemoteConfig.
	// +optional
	RemoteConfigConfiguration *RemoteConfigConfiguration `json:"remoteConfigConfiguration,omitempty"`
}

// DatadogAgent Deployment with the Datadog Operator.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:path=datadogagents,shortName=dd
// +kubebuilder:printcolumn:name="agent",type="string",JSONPath=".status.agent.status"
// +kubebuilder:printcolumn:name="cluster-agent",type="string",JSONPath=".status.clusterAgent.status"
// +kubebuilder:printcolumn:name="cluster-checks-runner",type="string",JSONPath=".status.clusterChecksRunner.status"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
// +k8s:openapi-gen=true
// +genclient
type DatadogAgent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatadogAgentSpec   `json:"spec,omitempty"`
	Status DatadogAgentStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DatadogAgentList contains a list of DatadogAgent.
type DatadogAgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatadogAgent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatadogAgent{}, &DatadogAgentList{})
}
