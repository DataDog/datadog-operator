// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// nolint
// +kubebuilder:skip
package v2alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResourceName is the name of a Deployment Component
type ResourceName string

const (
	// NodeAgentResourceName is the name of the Datadog Node Agent
	NodeAgentResourceName ResourceName = "nodeAgent"
	// ClusterAgentResourceName is the name of the Cluster Agent
	ClusterAgentResourceName ResourceName = "clusterAgent"
	// ClusterChecksRunnerResourceName is the name of the Cluster Check Runner
	ClusterChecksRunnerResourceName ResourceName = "clusterChecksRunner"
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
	Override map[ResourceName]DatadogAgentResourceOverride `json:"override,omitempty"`
}

// DatadogFeatures are features running on the Agent and Cluster Agent.
// +k8s:openapi-gen=true
type DatadogFeatures struct {
	// Application-level features

	// LogCollection configuration.
	LogCollection *LogCollectionFeatureConfig `json:"logCollection,omitempty"`
	// ProcessCollection configuration.
	ProcessCollection *ProcessCollectionFeatureConfig `json:"processCollection,omitempty"`
	// ContainerCollection configuration.
	ContainerCollection *ContainerCollectionFeatureConfig `json:"containerCollection,omitempty"`
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

	// Cluster-level features

	// OrchestratorExplorer check configuration.
	OrchestratorExplorer *OrchestratorExplorerFeatureConfig `json:"orchestratorExplorer,omitempty"`
	// KubeStateMetricsCore check configuration.
	KubeStateMetricsCore *KubeStateMetricsCoreFeatureConfig `json:"kubeStateMetricsCore,omitempty"`
	// AdmissionController configuration.
	AdmissionController *AdmissionControllerFeatureConfig `json:"admissionController,omitempty"`
	// ExternalMetricsServer configuration.
	ExternalMetricsServer *ExternalMetricsServerFeatureConfig `json:"externalMetricsServer,omitempty"`
	// ClusterChecksRunner configuration.
	ClusterChecksRunner *ClusterChecksRunnerFeatureConfig `json:"clusterChecksRunner,omitempty"`
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
	UnixDomainSocketConfig *UnixDomainSocketConfig `json:"unixDomainSocket,omitempty"`
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

// ProcessCollectionFeatureConfig contains Process Collection configuration.
// Process Collection is run in the Process Agent.
type ProcessCollectionFeatureConfig struct {
	// Enabled enables Process monitoring.
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// ContainerCollectionFeatureConfig contains Container Collection configuration.
// Container Collection is run in the Process Agent.
type ContainerCollectionFeatureConfig struct {
	// Enabled enables Process monitoring.
	// Default: true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// CSPMFeatureConfig contains CSPM (Cloud Security Posture Management) configuration.
// CSPM runs in the Security Agent.
type CSPMFeatureConfig struct {
	// Enabled enables Cloud Security Posture Management.
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// CheckInterval defines the check interval.
	// +optional
	CheckInterval *metav1.Duration `json:"checkInterval,omitempty"`

	// ConfigMap contains compliance benchmarks.
	// The content of the ConfigMap will be merged with the benchmarks bundled with the agent.
	// Any benchmarks with the same name as those existing in the agent will take precedence.
	// +optional
	ConfigMap *ConfigMapConfig `json:"configDir,omitempty"`
}

// CWSFeatureConfig contains CWS (Cloud Workload Security) configuration.
// CWS runs in the Security Agent.
type CWSFeatureConfig struct {
	// Enabled enables Cloud Workload Security.
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// ConfigMap contains security policies.
	// The content of the ConfigMap will be merged with the policies bundled with the agent.
	// Any policies with the same name as those existing in the agent will take precedence.
	// +optional
	ConfigMap *ConfigMapConfig `json:"configDir,omitempty"`

	// EnableSyscallMonitor enables Syscall Monitoring (recommended for troubleshooting only).
	// Default: false
	// +optional
	EnableSyscallMonitor *bool `json:"enableSyscallMonitor"`
}

// NPMFeatureConfig contains NPM (Network Performance Monitoring) feature configuration.
// Network Performance Monitoring runs in the System Probe and Process Agent.
type NPMFeatureConfig struct {
	// Enabled enables Network Performance Monitoring.
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// USMFeatureConfig contains USM (Universal Service Monitoring) feature configuration.
// Universal Service Monitoring runs in the System Probe.
type USMFeatureConfig struct {
	// Enabled enables Universal Service Monitoring.
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
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
	Endpoint *Endpoint `json:"endpoint,omitempty"`
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
	WPAController bool `json:"wpaController,omitempty"`

	// UseDatadogMetrics enables usage of the DatadogMetrics CRD (allowing one to scale on arbitrary Datadog metric queries).
	// Default: true
	// +optional
	UseDatadogMetrics bool `json:"useDatadogMetrics,omitempty"`

	// Port specifies the metricsProvider External Metrics Server service port.
	// Default: 8443
	// +optional
	Port *int32 `json:"port,omitempty"`

	// Override the API endpoint for the External Metrics Server.
	// URL Default: "https://app.datadoghq.com".
	// +optional
	Endpoint *Endpoint `json:"endpoint,omitempty"`
}

// ClusterChecksRunnerFeatureConfig contains the Cluster Checks Runner feature configuration.
// Cluster Checks Runners are Agents dedicated to running Cluster Checks dispatched by the Cluster Agent.
type ClusterChecksRunnerFeatureConfig struct {
	// Enabled enables Cluster Checks Runners to run all Cluster Checks.
	// Default: false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
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
	ConfigMap *ConfigMapConfig `json:"configMap,omitempty"`
}

// ConfigMapConfig contains ConfigMap information used to store a configuration file.
type ConfigMapConfig struct {
	// Name is the name of the ConfigMap.
	Name string `json:"name,omitempty"`

	// Items maps a ConfigMap data key to a file path mount.
	// +listType=map
	// +listMapKey=key
	// +optional
	Items []corev1.KeyToPath `json:"items,omitempty"`
}

// GlobalConfig is a set of parameters that are used to configure all the components of the Datadog Operator.
type GlobalConfig struct {
	// Credentials defines the Datadog credentials used to submit data to/query data from Datadog.
	Credentials *DatadogCredentials `json:"credentials,omitempty"`

	// ClusterName sets a unique cluster name for the deployment to easily scope monitoring data in the Datadog app.
	// +optional
	ClusterName string `json:"clusterName,omitempty"`

	// Site is the Datadog intake site Agent data are sent to.
	// Set to 'datadoghq.eu' to send data to the EU site.
	// Default: 'datadoghq.com'
	// +optional
	Site string `json:"site,omitempty"`

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

	// NetworkPolicy contains the network configuration.
	// +optional
	NetworkPolicy *NetworkPolicyConfig `json:"networkPolicy,omitempty"`

	// LocalService contains configuration to customize the internal traffic policy service.
	// +optional
	LocalService *LocalService `json:"localService,omitempty"`
}

// DatadogCredentials is a generic structure that holds credentials to access Datadog.
// +k8s:openapi-gen=true
type DatadogCredentials struct {
	// APIKey configures your Datadog API key.
	// See also: https://app.datadoghq.com/account/settings#agent/kubernetes
	APIKey string `json:"apiKey,omitempty"`

	// APISecret references an existing Secret which stores the API key instead of creating a new one.
	// If set, this parameter takes precedence over "APIKey".
	// +optional
	APISecret *Secret `json:"apiSecret,omitempty"`

	// AppKey configures your Datadog application key.
	// If you are using clusterAgent.metricsProvider.enabled = true, you must set
	// a Datadog application key for read access to your metrics.
	// +optional
	AppKey string `json:"appKey,omitempty"`

	// AppSecret references an existing Secret which stores the application key instead of creating a new one.
	// If set, this parameter takes precedence over "AppKey".
	// +optional
	AppSecret *Secret `json:"appSecret,omitempty"`
}

// Secret contains a secret name and an included key.
// +k8s:openapi-gen=true
type Secret struct {
	// SecretName is the name of the secret.
	SecretName string `json:"secretName"`

	// KeyName is the key of the secret.
	// +optional
	KeyName string `json:"keyName,omitempty"`
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
	NameOverride string `json:"nameOverride,omitempty"`

	// ForceEnableLocalService forces the creation of the internal traffic policy service to target the agent running on the local node.
	// This parameter only applies to Kubernetes 1.21, where the feature is in alpha and is disabled by default.
	// (On Kubernetes 1.22+, the feature entered beta and the internal traffic service is created by default, so this parameter is ignored.)
	// Default: false
	// +optional
	ForceEnableLocalService *bool `json:"forceEnableLocalService,omitempty"`
}

// DatadogAgentResourceOverride is the generic description of a component (Cluster Agent Deployment, Node Agent Daemonset, etc.).
// TODO: Add the Resource type and name to allow overriding the kind of the Resource (e.g. ExtendedDaemonset)
type DatadogAgentResourceOverride struct {
	// DatadogAgentPodTemplateOverride is used to configure the components at the pod level in an agnostic way
	DatadogAgentPodTemplateOverride *DatadogAgentPodTemplateOverride `json:"podOverride,omitempty"`

	// Name overrides the default name for the resource
	Name string `json:"name,omitempty"`
}

// DatadogAgentPodTemplateOverride is the generic description equivalent to a subset of the PodTemplate for a component.
type DatadogAgentPodTemplateOverride struct {
	// Configure the basic configurations for each agent container
	Containers []DatadogAgentGenericContainer `json:"containers,omitempty"`

	// Specify additional volumes in the different components (Datadog Agent, Cluster Agent, Cluster Check Runner).
	// +optional
	// +listType=map
	// +listMapKey=name
	Volumes []corev1.Volume `json:"volumes,omitempty"`

	// The container image of the different components (Datadog Agent, Cluster Agent, Cluster Check Runner).
	Image *ImageConfig `json:"image,omitempty"`

	// Configure the different component (Datadog Agent, Cluster Agent, Cluster Check Runner) tolerations.
	// +optional
	// +listType=atomic
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Pod-level SecurityContext.
	// +optional
	SecurityContext *corev1.PodSecurityContext `json:"securityContext,omitempty"`

	// If specified, indicates the pod's priority. "system-node-critical" and "system-cluster-critical"
	// are two special keywords which indicate the highest priorities with the former being the highest priority.
	// Any other name must be defined by creating a PriorityClass object with that name. If not specified,
	// the pod priority will be default or zero if there is no default.
	PriorityClassName string `json:"priorityClassName,omitempty"`

	// If specified, the pod's scheduling constraints.
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// Annotations provide annotations that will be added to the different component (Datadog Agent, Cluster Agent, Cluster Check Runner) pods.
	Annotations map[string]string `json:"annotations,omitempty"`

	// AdditionalLabels provide labels that will be added to the different component (Datadog Agent, Cluster Agent, Cluster Check Runner) pods.
	Labels map[string]string `json:"labels,omitempty"`

	// Kubelet contains the kubelet configuration parameters.
	// +optional
	Kubelet *KubeletConfig `json:"kubelet,omitempty"`
}

// KubeletConfig contains the kubelet configuration parameters.
// +k8s:openapi-gen=true
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
}

// DatadogAgentGenericContainer is the generic structure describing any container's common configuration.
// +k8s:openapi-gen=true
type DatadogAgentGenericContainer struct {
	// Name of the container that is overridden
	//+optional
	Name string `json:"name,omitempty"`

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
}

// ImageConfig Datadog agent container image config.
// +k8s:openapi-gen=true
type ImageConfig struct {
	// Define the image to use:
	// Use "gcr.io/datadoghq/agent:latest" for Datadog Agent 7
	// Use "datadog/dogstatsd:latest" for standalone Datadog Agent DogStatsD 7
	// Use "gcr.io/datadoghq/cluster-agent:latest" for Datadog Cluster Agent
	// Use "agent" with the registry and tag configurations for <registry>/agent:<tag>
	// Use "cluster-agent" with the registry and tag configurations for <registry>/cluster-agent:<tag>
	Name string `json:"name,omitempty"`

	// Define the image tag to use.
	// To be used if the Name field does not correspond to a full image string.
	// +optional
	Tag string `json:"tag,omitempty"`

	// Define whether the Agent image should support JMX.
	// +optional
	JMXEnabled bool `json:"jmxEnabled,omitempty"`

	// The Kubernetes pull policy:
	// Use Always, Never or IfNotPresent.
	PullPolicy *corev1.PullPolicy `json:"pullPolicy,omitempty"`

	// It is possible to specify Docker registry credentials.
	// See https://kubernetes.io/docs/concepts/containers/images/#specifying-imagepullsecrets-on-a-pod
	// +optional
	PullSecrets *[]corev1.LocalObjectReference `json:"pullSecrets,omitempty"`
}

// DatadogAgentStatus defines the observed state of DatadogAgent.
// +k8s:openapi-gen=true
type DatadogAgentStatus struct {
	// DefaultOverride contains attributes that were not configured that the runtime defaulted.
	// +optional
	DefaultOverride *DatadogAgentSpec `json:"defaultOverride,omitempty"`
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
