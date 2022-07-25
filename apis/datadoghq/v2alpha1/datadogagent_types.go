// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v2alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
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

	// LogCollection configuration.
	LogCollection *LogCollectionFeatureConfig `json:"logCollection,omitempty"`
	// LiveProcessCollection configuration.
	LiveProcessCollection *LiveProcessCollectionFeatureConfig `json:"liveProcessCollection,omitempty"`
	// LiveContainerCollection configuration.
	LiveContainerCollection *LiveContainerCollectionFeatureConfig `json:"liveContainerCollection,omitempty"`
	// OOMKill configuration.
	OOMKill *OOMKillFeatureConfig `json:"oomKill,omitempty"`
	// TCPQueueLength configuration.
	TCPQueueLength *TCPQueueLengthFeatureConfig `json:"tcpQueueLength,omitempty"`
	// APM (Application Performance Monitoring) configuration.
	APM *APMFeatureConfig `json:"apm,omitempty"`
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
	// ClusterChecks configuration.
	ClusterChecks *ClusterChecksFeatureConfig `json:"clusterChecks,omitempty"`
	// PrometheusScrape configuration.
	PrometheusScrape *PrometheusScrapeFeatureConfig `json:"prometheusScrape,omitempty"`
	// DatadogMonitor configuration.
	DatadogMonitor *DatadogMonitorFeatureConfig `json:"datadogMonitor,omitempty"`
}

// Configuration structs for each feature in DatadogFeatures. All parameters are optional and have default values when necessary.
// Note: configuration in DatadogAgentSpec.Override takes precedence.

// APMFeatureConfig contains APM (Application Performance Monitoring) configuration.
// APM runs in the Trace Agent.
type APMFeatureConfig struct {
	// Enabled enables Application Performance Monitoring.
	// Default: false
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
}

// LiveContainerCollectionFeatureConfig contains Container Collection configuration.
// Container Collection is run in the Process Agent.
type LiveContainerCollectionFeatureConfig struct {
	// Enables container collection for the Live Container View.
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

	// ConfigMap contains CSPM benchmarks.
	// The content of the ConfigMap will be merged with the benchmarks bundled with the agent.
	// Any benchmarks with the same name as those existing in the agent will take precedence.
	// +optional
	CustomBenchmarks *commonv1.ConfigMapConfig `json:"customBenchmarks,omitempty"`
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

	// ConfigMap contains security policies.
	// The content of the ConfigMap will be merged with the policies bundled with the agent.
	// Any policies with the same name as those existing in the agent will take precedence.
	// +optional
	CustomPolicies *commonv1.ConfigMapConfig `json:"customPolicies,omitempty"`
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

// DogstatsdFeatureConfig contains the Dogstatsd configuration parameters.
// +k8s:openapi-gen=true
type DogstatsdFeatureConfig struct {
	// OriginDetectionEnabled enables origin detection for container tagging.
	// See also: https://docs.datadoghq.com/developers/dogstatsd/unix_socket/#using-origin-detection-for-container-tagging
	// +optional
	OriginDetectionEnabled *bool `json:"originDetectionEnabled,omitempty"`

	// HostPortConfig contains host port configuration.
	// Enabled Default: true
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
}

// EventCollectionFeatureConfig contains the Event Collection configuration.
// +k8s:openapi-gen=true
type EventCollectionFeatureConfig struct {
	// CollectKubernetesEvents enables Kubernetes event collection.
	// Default: true
	CollectKubernetesEvents *bool `json:"collectKubernetesEvents,omitempty"`
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

// AdmissionControllerFeatureConfig contains the Admission Controller feature configuration.
// The Admission Controller runs in the Cluster Agent.
type AdmissionControllerFeatureConfig struct {
	// Enabled enables the Admission Controller.
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// MutateUnlabelled enables config injection without the need of pod label 'admission.datadoghq.com/enabled="true"'.
	// Default: false
	// +optional
	MutateUnlabelled *bool `json:"mutateUnlabelled,omitempty"`

	// ServiceName corresponds to the webhook service name.
	// +optional
	ServiceName *string `json:"serviceName,omitempty"`

	// agentCommunicationMode corresponds to the mode used by the Datadog application libraries to communicate with the Agent.
	// It can be "hostip", "service", or "socket".
	// +optional
	AgentCommunicationMode *string `json:"agentCommunicationMode,omitempty"`
}

// ExternalMetricsServerFeatureConfig contains the External Metrics Server feature configuration.
// The External Metrics Server runs in the Cluster Agent.
type ExternalMetricsServerFeatureConfig struct {
	// Enabled enables the External Metrics Server.
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

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
}

// DatadogMonitorFeatureConfig contains the Datadog Monitor feature configuration.
// DatadogMonitor is run by the Datadog Operator.
type DatadogMonitorFeatureConfig struct {
	// Enabled enables Datadog Monitors.
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// Generic support structs

// HostPortConfig contains host port configuration.
type HostPortConfig struct {
	// Enabled enables host port configuration
	// Default: false
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

// CustomConfig provides a place for custom configuration of the Agent or Cluster Agent, corresponding to datadog.yaml or datadog-cluster.yaml.
// The configuration can be provided in the ConfigData field as raw data, or referenced in a ConfigMap.
// Note: `ConfigData` and `ConfigMap` cannot be set together.
// +k8s:openapi-gen=true
type CustomConfig struct {
	// ConfigData corresponds to the configuration file content.
	ConfigData *string `json:"configData,omitempty"`

	// ConfigMap references an existing ConfigMap with the configuration file content.
	ConfigMap *commonv1.ConfigMapConfig `json:"configMap,omitempty"`
}

// GlobalConfig is a set of parameters that are used to configure all the components of the Datadog Operator.
type GlobalConfig struct {
	// Credentials defines the Datadog credentials used to submit data to/query data from Datadog.
	Credentials *DatadogCredentials `json:"credentials,omitempty"`

	// ClusterAgentToken is the token for communication between the NodeAgent and ClusterAgent
	ClusterAgentToken *string `json:"clusterAgentToken,omitempty"`

	// ClusterName sets a unique cluster name for the deployment to easily scope monitoring data in the Datadog app.
	// +optional
	ClusterName *string `json:"clusterName,omitempty"`

	// Site is the Datadog intake site Agent data are sent to.
	// Set to 'datadoghq.eu' to send data to the EU site.
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

	// Provide a mapping of Kubernetes Labels to Datadog Tags.
	// <KUBERNETES_LABEL>: <DATADOG_TAG_KEY>
	// +optional
	PodLabelsAsTags map[string]string `json:"podLabelsAsTags,omitempty"`

	// Provide a mapping of Kubernetes Annotations to Datadog Tags.
	// <KUBERNETES_ANNOTATIONS>: <DATADOG_TAG_KEY>
	// +optional
	PodAnnotationsAsTags map[string]string `json:"podAnnotationsAsTags,omitempty"`

	// NetworkPolicy contains the network configuration.
	// +optional
	NetworkPolicy *NetworkPolicyConfig `json:"networkPolicy,omitempty"`

	// LocalService contains configuration to customize the internal traffic policy service.
	// +optional
	LocalService *LocalService `json:"localService,omitempty"`

	// Kubelet contains the kubelet configuration parameters.
	// +optional
	Kubelet *commonv1.KubeletConfig `json:"kubelet,omitempty"`

	// Path to the docker runtime socket.
	// +optional
	DockerSocketPath *string `json:"dockerSocketPath,omitempty"`

	// Path to the container runtime socket (if different from Docker).
	// +optional
	CriSocketPath *string `json:"criSocketPath,omitempty"`
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
	APISecret *commonv1.SecretConfig `json:"apiSecret,omitempty"`

	// AppKey configures your Datadog application key.
	// If you are using clusterAgent.metricsProvider.enabled = true, you must set
	// a Datadog application key for read access to your metrics.
	// +optional
	AppKey *string `json:"appKey,omitempty"`

	// AppSecret references an existing Secret which stores the application key instead of creating a new one.
	// If set, this parameter takes precedence over "AppKey".
	// +optional
	AppSecret *commonv1.SecretConfig `json:"appSecret,omitempty"`
}

// SecretBackendConfig provides configuration for the secret backend.
type SecretBackendConfig struct {
	// Command defines the secret backend command to use
	Command *string `json:"command,omitempty"`

	// Args defines the list of arguments to pass to the command
	Args []string `json:"args,omitempty"`
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

// AgentConfigFileName is the list of known Agent config files
type AgentConfigFileName string

const (
	// AgentGeneralConfigFile is the name of the main Agent config file
	AgentGeneralConfigFile AgentConfigFileName = "datadog.yaml"
	// SystemProbeConfigFile is the name of the of System Probe config file
	SystemProbeConfigFile AgentConfigFileName = "system-probe.yaml"
	// SecurityAgentConfigFile is the name of the Security Agent config file
	SecurityAgentConfigFile AgentConfigFileName = "security-agent.yaml"
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

	// Set CreateRbac to false to prevent automatic creation of Role/ClusterRole for this component
	// +optional
	CreateRbac *bool `json:"createRbac,omitempty"`

	// Sets the ServiceAccount used by this component.
	// Ignored if the field CreateRbac is true.
	// +optional
	ServiceAccountName *string `json:"serviceAccountName,omitempty"`

	// The container image of the different components (Datadog Agent, Cluster Agent, Cluster Check Runner).
	// +optional
	Image *commonv1.AgentImageConfig `json:"image,omitempty"`

	// Specify additional environmental variables for all containers in this component
	// Priority is Container > Component
	// See also: https://docs.datadoghq.com/agent/kubernetes/?tab=helm#environment-variables
	//
	// +optional
	// +listType=map
	// +listMapKey=name
	Env []corev1.EnvVar `json:"env,omitempty"`

	// CustomConfiguration allows to specify custom configuration files for any known `AgentConfigFileName`
	// The content will be merged with configuration generated by the Datadog Operator, with priority given to custom configuration.
	// WARNING: It's thus possible to override values set in the `DatadogAgent`.
	// +optional
	CustomConfigurations map[AgentConfigFileName]CustomConfig `json:"customConfigurations,omitempty"`

	// Confd configuration allowing to specify config files for custom checks placed under /etc/datadog-agent/conf.d/.
	// See https://docs.datadoghq.com/agent/guide/agent-configuration-files/?tab=agentv6 for more details.
	// +optional
	ExtraConfd *CustomConfig `json:"extraConfd,omitempty"`

	// Checksd configuration allowing to specify custom checks placed under /etc/datadog-agent/checks.d/
	// See https://docs.datadoghq.com/agent/guide/agent-configuration-files/?tab=agentv6 for more details.
	// +optional
	ExtraChecksd *CustomConfig `json:"extraChecksd,omitempty"`

	// Configure the basic configurations for each agent container
	// +optional
	Containers map[commonv1.AgentContainerName]*DatadogAgentGenericContainer `json:"containers,omitempty"`

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
	// the pod priority will be default or zero if there is no default.
	PriorityClassName *string `json:"priorityClassName,omitempty"`

	// If specified, the pod's scheduling constraints.
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// NodeSelector is a selector which must be true for the pod to fit on a node.
	// Selector which must match a node's labels for the pod to be scheduled on that node.
	// More info: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Configure the component tolerations.
	// +optional
	// +listType=atomic
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Annotations provide annotations that will be added to the different component (Datadog Agent, Cluster Agent, Cluster Check Runner) pods.
	Annotations map[string]string `json:"annotations,omitempty"`

	// AdditionalLabels provide labels that will be added to the different component (Datadog Agent, Cluster Agent, Cluster Check Runner) pods.
	Labels map[string]string `json:"labels,omitempty"`

	// Host networking requested for this pod. Use the host's network namespace.
	// +optional
	HostNetwork *bool `json:"hostNetwork,omitempty"`

	// Use the host's pid namespace.
	// +optional
	HostPID *bool `json:"hostPID,omitempty"`

	// SecCompRootPath specify the seccomp profile root directory.
	// +optional
	SecCompRootPath *string `json:"secCompRootPath,omitempty"`

	// SecCompCustomProfileConfigMap specify a pre-existing ConfigMap containing a custom SecComp profile.
	// +optional
	SecCompCustomProfile *CustomConfig `json:"secCompCustomProfile,omitempty"`

	// SecCompProfileName specify a seccomp profile.
	// +optional
	SecCompProfileName *string `json:"secCompProfileName,omitempty"`

	// Disabled force disables a component.
	// +optional
	Disabled *bool `json:"disabled,omitempty"`
}

// DatadogAgentGenericContainer is the generic structure describing any container's common configuration.
// +k8s:openapi-gen=true
type DatadogAgentGenericContainer struct {
	// Name of the container that is overridden
	//+optional
	Name *string `json:"name,omitempty"`

	// LogLevel sets logging verbosity (overrides global setting)
	// Valid log levels are: trace, debug, info, warn, error, critical, and off.
	// Default: 'info'
	// +optional
	LogLevel *string `json:"logLevel,omitempty"`

	// Specify additional environmental variables in the container
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

	// Container-level SecurityContext.
	// +optional
	SecurityContext *corev1.SecurityContext `json:"securityContext,omitempty"`

	// AppArmorProfileName specify a apparmor profile.
	// +optional
	AppArmorProfileName *string `json:"appArmorProfileName,omitempty"`
}

// DatadogAgentStatus defines the observed state of DatadogAgent.
// +k8s:openapi-gen=true
type DatadogAgentStatus struct {
	// Conditions Represents the latest available observations of a DatadogAgent's current state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions"`
	// The actual state of the Agent as an extended daemonset.
	// +optional
	Agent *DaemonSetStatus `json:"agent,omitempty"`
	// The actual state of the Cluster Agent as a deployment.
	// +optional
	ClusterAgent *DeploymentStatus `json:"clusterAgent,omitempty"`
	// The actual state of the Cluster Checks Runner as a deployment.
	// +optional
	ClusterChecksRunner *DeploymentStatus `json:"clusterChecksRunner,omitempty"`
}

// DaemonSetStatus defines the observed state of Agent running as DaemonSet.
// +k8s:openapi-gen=true
type DaemonSetStatus struct {
	Desired   int32 `json:"desired"`
	Current   int32 `json:"current"`
	Ready     int32 `json:"ready"`
	Available int32 `json:"available"`
	UpToDate  int32 `json:"upToDate"`

	Status      string       `json:"status,omitempty"`
	State       string       `json:"state,omitempty"`
	LastUpdate  *metav1.Time `json:"lastUpdate,omitempty"`
	CurrentHash string       `json:"currentHash,omitempty"`

	// DaemonsetName corresponds to the name of the created DaemonSet.
	DaemonsetName string `json:"daemonsetName,omitempty"`
}

// DeploymentStatus type representing a Deployment status.
// +k8s:openapi-gen=true
type DeploymentStatus struct {
	// Total number of non-terminated pods targeted by this deployment (their labels match the selector).
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// Total number of non-terminated pods targeted by this deployment that have the desired template spec.
	// +optional
	UpdatedReplicas int32 `json:"updatedReplicas,omitempty"`

	// Total number of ready pods targeted by this deployment.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// Total number of available pods (ready for at least minReadySeconds) targeted by this deployment.
	// +optional
	AvailableReplicas int32 `json:"availableReplicas,omitempty"`

	// Total number of unavailable pods targeted by this deployment. This is the total number of
	// pods that are still required for the deployment to have 100% available capacity. They may
	// either be pods that are running but not yet available or pods that still have not been created.
	// +optional
	UnavailableReplicas int32 `json:"unavailableReplicas,omitempty"`

	LastUpdate  *metav1.Time `json:"lastUpdate,omitempty"`
	CurrentHash string       `json:"currentHash,omitempty"`

	// GeneratedToken corresponds to the generated token if any token was provided in the Credential configuration when ClusterAgent is
	// enabled.
	// +optional
	GeneratedToken string `json:"generatedToken,omitempty"`

	// Status corresponds to the ClusterAgent deployment computed status.
	Status string `json:"status,omitempty"`
	// State corresponds to the ClusterAgent deployment state.
	State string `json:"state,omitempty"`

	// DeploymentName corresponds to the name of the Cluster Agent Deployment.
	DeploymentName string `json:"deploymentName,omitempty"`
}

// DatadogAgent Deployment with the Datadog Operator.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:unservedversion
// +kubebuilder:resource:path=datadogagents,shortName=dd
// +kubebuilder:printcolumn:name="active",type="string",JSONPath=".status.conditions[?(@.type=='Active')].status"
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
