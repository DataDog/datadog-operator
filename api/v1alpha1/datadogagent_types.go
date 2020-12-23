// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package v1alpha1

import (
	edsdatadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// DatadogFeatures are Features running on the Agent and Cluster Agent.
// +k8s:openapi-gen=true
type DatadogFeatures struct {
	// OrchestratorExplorer configuration
	OrchestratorExplorer *OrchestratorExplorerConfig `json:"orchestratorExplorer,omitempty"`
	// KubeStateMetricsCore configuration
	KubeStateMetricsCore *KubeStateMetricsCore `json:"kubeStateMetricsCore,omitempty"`
}

// DatadogAgentSpec defines the desired state of DatadogAgent
// +k8s:openapi-gen=true
type DatadogAgentSpec struct {
	// Configure the credentials needed to run Agents. If not set, then the credentials
	// set in the DatadogOperator will be used
	Credentials AgentCredentials `json:"credentials,omitempty"`

	// Features running on the Agent and Cluster Agent
	// +optional
	Features *DatadogFeatures `json:"features,omitempty"`

	// The desired state of the Agent as an extended daemonset
	// Contains the Node Agent configuration and deployment strategy
	// +optional
	Agent *DatadogAgentSpecAgentSpec `json:"agent,omitempty"`

	// The desired state of the Cluster Agent as a deployment
	// +optional
	ClusterAgent *DatadogAgentSpecClusterAgentSpec `json:"clusterAgent,omitempty"`

	// The desired state of the Cluster Checks Runner as a deployment
	// +optional
	ClusterChecksRunner *DatadogAgentSpecClusterChecksRunnerSpec `json:"clusterChecksRunner,omitempty"`

	// Set a unique cluster name to allow scoping hosts and Cluster Checks Runner easily
	// +optional
	ClusterName string `json:"clusterName,omitempty"`

	// The site of the Datadog intake to send Agent data to.
	// Set to 'datadoghq.eu' to send data to the EU site.
	// +optional
	Site string `json:"site,omitempty"`
}

// AgentCredentials contains credentials values to configure the Agent
// +k8s:openapi-gen=true
type AgentCredentials struct {
	// APIKey Set this to your Datadog API key before the Agent runs.
	// ref: https://app.datadoghq.com/account/settings#agent/kubernetes
	APIKey string `json:"apiKey,omitempty"`

	// APIKeyExistingSecret is DEPRECATED.
	// In order to pass the API key through an existing secret, please consider "apiSecret" instead.
	// If set, this parameter takes precedence over "apiKey".
	// +optional
	// +deprecated
	APIKeyExistingSecret string `json:"apiKeyExistingSecret,omitempty"`

	// APISecret Use existing Secret which stores API key instead of creating a new one.
	// If set, this parameter takes precedence over "apiKey" and "apiKeyExistingSecret".
	// +optional
	APISecret *Secret `json:"apiSecret,omitempty"`

	// If you are using clusterAgent.metricsProvider.enabled = true, you must set
	// a Datadog application key for read access to your metrics.
	// +optional
	AppKey string `json:"appKey,omitempty"`

	// AppKeyExistingSecret is DEPRECATED.
	// In order to pass the APP key through an existing secret, please consider "appSecret" instead.
	// If set, this parameter takes precedence over "appKey".
	// +optional
	// +deprecated
	AppKeyExistingSecret string `json:"appKeyExistingSecret,omitempty"`

	// APPSecret Use existing Secret which stores API key instead of creating a new one.
	// If set, this parameter takes precedence over "apiKey" and "appKeyExistingSecret".
	// +optional
	APPSecret *Secret `json:"appSecret,omitempty"`

	// This needs to be at least 32 characters a-zA-z
	// It is a preshared key between the node agents and the cluster agent
	// +optional
	Token string `json:"token,omitempty"`

	// UseSecretBackend use the Agent secret backend feature for retreiving all credentials needed by
	// the different components: Agent, Cluster, Cluster-Checks.
	// If `useSecretBackend: true`, other credential parameters will be ignored.
	// default value is false.
	UseSecretBackend *bool `json:"useSecretBackend,omitempty"`
}

// Secret contains a secret name and an included key
// +k8s:openapi-gen=true
type Secret struct {
	// SecretName is the name of the secret
	SecretName string `json:"secretName"`

	// KeyName is the key of the secret to use
	// +optional
	KeyName string `json:"keyName,omitempty"`
}

// DatadogAgentSpecAgentSpec defines the desired state of the node Agent
// +k8s:openapi-gen=true
type DatadogAgentSpecAgentSpec struct {
	// UseExtendedDaemonset use ExtendedDaemonset for Agent deployment.
	// default value is false.
	UseExtendedDaemonset *bool `json:"useExtendedDaemonset,omitempty"`

	// The container image of the Datadog Agent
	Image ImageConfig `json:"image"`

	// Name of the Daemonset to create or migrate from
	// +optional
	DaemonsetName string `json:"daemonsetName,omitempty"`

	// Agent configuration
	Config NodeAgentConfig `json:"config,omitempty"`

	// RBAC configuration of the Agent
	Rbac RbacConfig `json:"rbac,omitempty"`

	// Update strategy configuration for the DaemonSet
	DeploymentStrategy *DaemonSetDeploymentStrategy `json:"deploymentStrategy,omitempty"`

	// AdditionalAnnotations provide annotations that will be added to the Agent Pods.
	AdditionalAnnotations map[string]string `json:"additionalAnnotations,omitempty"`

	// AdditionalLabels provide labels that will be added to the cluster checks runner Pods.
	AdditionalLabels map[string]string `json:"additionalLabels,omitempty"`

	// If specified, indicates the pod's priority. "system-node-critical" and "system-cluster-critical"
	// are two special keywords which indicate the highest priorities with the former being the highest priority.
	// Any other name must be defined by creating a PriorityClass object with that name. If not specified,
	// the pod priority will be default or zero if there is no default.
	PriorityClassName string `json:"priorityClassName,omitempty"`

	// Set DNS policy for the pod.
	// Defaults to "ClusterFirst".
	// Valid values are 'ClusterFirstWithHostNet', 'ClusterFirst', 'Default' or 'None'.
	// DNS parameters given in DNSConfig will be merged with the policy selected with DNSPolicy.
	// To have DNS options set along with hostNetwork, you have to specify DNS policy
	// explicitly to 'ClusterFirstWithHostNet'.
	// +optional
	DNSPolicy corev1.DNSPolicy `json:"dnsPolicy,omitempty" protobuf:"bytes,6,opt,name=dnsPolicy,casttype=DNSPolicy"`
	// Specifies the DNS parameters of a pod.
	// Parameters specified here will be merged to the generated DNS
	// configuration based on DNSPolicy.
	// +optional
	DNSConfig *corev1.PodDNSConfig `json:"dnsConfig,omitempty"`

	// Host networking requested for this pod. Use the host's network namespace.
	// If this option is set, the ports that will be used must be specified.
	// Default to false.
	// +k8s:conversion-gen=false
	// +optional
	HostNetwork bool `json:"hostNetwork,omitempty"`
	// Use the host's pid namespace.
	// Optional: Default to false.
	// +k8s:conversion-gen=false
	// +optional
	HostPID bool `json:"hostPID,omitempty"`

	// Environment variables for all Datadog Agents
	// Ref: https://docs.datadoghq.com/agent/docker/?tab=standard#environment-variables
	//
	// +optional
	// +listType=map
	// +listMapKey=name
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Trace Agent configuration
	// +optional
	Apm APMSpec `json:"apm,omitempty"`

	// Log Agent configuration
	// +optional
	Log LogSpec `json:"log,omitempty"`

	// Process Agent configuration
	// +optional
	Process ProcessSpec `json:"process,omitempty"`

	// SystemProbe configuration
	// +optional
	SystemProbe SystemProbeSpec `json:"systemProbe,omitempty"`

	// Security Agent configuration
	// +optional
	Security SecuritySpec `json:"security,omitempty"`

	// Allow to put custom configuration for the agent, corresponding to the datadog.yaml config file
	// See https://docs.datadoghq.com/agent/guide/agent-configuration-files/?tab=agentv6 for more details.
	// +optional
	CustomConfig *CustomConfigSpec `json:"customConfig,omitempty"`

	// Provide Agent Network Policy configuration
	// +optional
	NetworkPolicy NetworkPolicySpec `json:"networkPolicy,omitempty"`
}

// RbacConfig contains RBAC configuration
// +k8s:openapi-gen=true
type RbacConfig struct {
	// Used to configure RBAC resources creation
	Create *bool `json:"create,omitempty"`

	// Used to set up the service account name to use
	// Ignored if the field Create is true
	ServiceAccountName *string `json:"serviceAccountName,omitempty"`
}

// DaemonSetDeploymentStrategy contains the node Agent deployment configuration
// +k8s:openapi-gen=true
type DaemonSetDeploymentStrategy struct {
	// The update strategy used for the DaemonSet
	UpdateStrategyType *appsv1.DaemonSetUpdateStrategyType `json:"updateStrategyType,omitempty"`
	// Configure the rolling updater strategy of the DaemonSet or the ExtendedDaemonSet
	RollingUpdate DaemonSetRollingUpdateSpec `json:"rollingUpdate,omitempty"`
	// Configure the canary deployment configuration using ExtendedDaemonSet
	Canary *edsdatadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanary `json:"canary,omitempty"`
	// The reconcile frequency of the ExtendDaemonSet
	ReconcileFrequency *metav1.Duration `json:"reconcileFrequency,omitempty"`
}

// DaemonSetRollingUpdateSpec contains configuration fields of the rolling update strategy
// The configuration is shared between DaemonSet and ExtendedDaemonSet
// +k8s:openapi-gen=true
type DaemonSetRollingUpdateSpec struct {
	// The maximum number of DaemonSet pods that can be unavailable during the
	// update. Value can be an absolute number (ex: 5) or a percentage of total
	// number of DaemonSet pods at the start of the update (ex: 10%). Absolute
	// number is calculated from percentage by rounding up.
	// This cannot be 0.
	// Default value is 1.
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`
	// MaxPodSchedulerFailure the maxinum number of not scheduled on its Node due to a
	// scheduler failure: resource constraints. Value can be an absolute number (ex: 5) or a percentage of total
	// number of DaemonSet pods at the start of the update (ex: 10%). Absolute
	MaxPodSchedulerFailure *intstr.IntOrString `json:"maxPodSchedulerFailure,omitempty"`
	// The maxium number of pods created in parallel.
	// Default value is 250.
	MaxParallelPodCreation *int32 `json:"maxParallelPodCreation,omitempty"`
	// SlowStartIntervalDuration the duration between to 2
	// Default value is 1min.
	SlowStartIntervalDuration *metav1.Duration `json:"slowStartIntervalDuration,omitempty"`
	// SlowStartAdditiveIncrease
	// Value can be an absolute number (ex: 5) or a percentage of total
	// number of DaemonSet pods at the start of the update (ex: 10%).
	// Default value is 5.
	SlowStartAdditiveIncrease *intstr.IntOrString `json:"slowStartAdditiveIncrease,omitempty"`
}

// APMSpec contains the Trace Agent configuration
// +k8s:openapi-gen=true
type APMSpec struct {
	// Enable this to enable APM and tracing, on port 8126
	// ref: https://github.com/DataDog/docker-dd-agent#tracing-from-the-host
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
	// Number of port to expose on the host.
	// If specified, this must be a valid port number, 0 < x < 65536.
	// If HostNetwork is specified, this must match ContainerPort.
	// Most containers do not need this.
	//
	// +optional
	HostPort *int32 `json:"hostPort,omitempty"`

	// The Datadog Agent supports many environment variables
	// Ref: https://docs.datadoghq.com/agent/docker/?tab=standard#environment-variables
	//
	// +optional
	// +listType=map
	// +listMapKey=name
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Datadog APM Agent resource requests and limits
	// Make sure to keep requests and limits equal to keep the pods in the Guaranteed QoS class
	// Ref: http://kubernetes.io/docs/user-guide/compute-resources/
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
}

// LogSpec contains the Log Agent configuration
// +k8s:openapi-gen=true
type LogSpec struct {
	// Enables this to activate Datadog Agent log collection.
	// ref: https://docs.datadoghq.com/agent/basic_agent_usage/kubernetes/#log-collection-setup
	//
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Enable this to allow log collection for all containers.
	// ref: https://docs.datadoghq.com/agent/basic_agent_usage/kubernetes/#log-collection-setup
	//
	// +optional
	LogsConfigContainerCollectAll *bool `json:"logsConfigContainerCollectAll,omitempty"`

	// Collect logs from files in /var/log/pods instead of using container runtime API.
	// It's usually the most efficient way of collecting logs.
	// ref: https://docs.datadoghq.com/agent/basic_agent_usage/kubernetes/#log-collection-setup
	// Default: true
	//
	// +optional
	ContainerCollectUsingFiles *bool `json:"containerCollectUsingFiles,omitempty"`

	// This to allow log collection from container log path. Set to a different path if not using docker runtime.
	// ref: https://docs.datadoghq.com/agent/kubernetes/daemonset_setup/?tab=k8sfile#create-manifest
	// Default to `/var/lib/docker/containers`
	//
	// +optional
	ContainerLogsPath *string `json:"containerLogsPath,omitempty"`

	// This to allow log collection from pod log path.
	// Default to `/var/log/pods`
	//
	// +optional
	PodLogsPath *string `json:"podLogsPath,omitempty"`

	// This path (always mounted from the host) is used by Datadog Agent to store information about processed log files.
	// If the Datadog Agent is restarted, it allows to start tailing the log files from the right offset
	// Default to `/var/lib/datadog-agent/logs`
	//
	// +optional
	TempStoragePath *string `json:"tempStoragePath,omitempty"`

	// Set the maximum number of logs files that the Datadog Agent will
	// tail up to. Increasing this limit can increase resource consumption
	// of the Agent.
	// ref: https://docs.datadoghq.com/agent/basic_agent_usage/kubernetes/#log-collection-setup
	// Default to 100
	//
	// +optional
	OpenFilesLimit *int32 `json:"openFilesLimit,omitempty"`
}

// ProcessSpec contains the Process Agent configuration
// +k8s:openapi-gen=true
type ProcessSpec struct {
	// Enable this to activate the process-agent to collection live-containers and if activated process information

	// Note: /etc/passwd is automatically mounted to allow username resolution.
	// ref: https://docs.datadoghq.com/graphing/infrastructure/process/#kubernetes-daemonset
	//
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// false (default): Only collect containers if available.
	// true: collect process information as well
	ProcessCollectionEnabled *bool `json:"processCollectionEnabled,omitempty"`

	// The Datadog Agent supports many environment variables
	// Ref: https://docs.datadoghq.com/agent/docker/?tab=standard#environment-variables
	//
	// +optional
	// +listType=map
	// +listMapKey=name
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Datadog Process Agent resource requests and limits
	// Make sure to keep requests and limits equal to keep the pods in the Guaranteed QoS class
	// Ref: http://kubernetes.io/docs/user-guide/compute-resources/
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
}

// KubeStateMetricsCore contains the required parameters to enable and override the configuration
// of the Kubernetes State Metrics Core (aka v2.0.0) of the check.
// +k8s:openapi-gen=true
type KubeStateMetricsCore struct {
	// Enable this to start the Kubernetes State Metrics Core check.
	// Refer to https://github.com/DataDog/datadog-operator/blob/master/docs/kubernetes_state_metrics.md
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// To override the configuration for the default Kubernetes State Metrics Core check.
	// Must point to a ConfigMap containing a valid cluster check configuration.
	Conf *CustomConfigSpec `json:"conf,omitempty"`
}

// OrchestratorExplorerConfig contains the orchestrator explorer configuration.
// The orchestratorExplorer runs in the process-agent and DCA.
// +k8s:openapi-gen=true
type OrchestratorExplorerConfig struct {
	// Enable this to activate live Kubernetes monitoring.
	// ref: https://docs.datadoghq.com/infrastructure/livecontainers/#kubernetes-resources
	//
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// +optional
	// Option to disable scrubbing of sensitive container data (passwords, tokens, etc. ).
	Scrubbing *Scrubbing `json:"scrubbing,omitempty"`

	// +optional
	// Additional endpoints for shipping the collected data as json in the form of {"https://process.agent.datadoghq.com": ["apikey1", ...], ...}'.
	AdditionalEndpoints *string `json:"additionalEndpoints,omitempty"`

	// +optional
	// Set this for the Datadog endpoint for the orchestrator explorer
	DDUrl *string `json:"ddUrl,omitempty"`

	// +optional
	// +listType=set
	// Additional tags for the collected data in the form of `a b c`
	// Difference to DD_TAGS: this is a cluster agent option that is used to define custom cluster tags
	ExtraTags []string `json:"extraTags,omitempty"`
}

// Scrubbing contains configuration to enable or disable scrubbing options
type Scrubbing struct {
	// Deactivate this to stop the scrubbing of sensitive container data (passwords, tokens, etc. ).
	Containers *bool `json:"containers,omitempty"`
}

// SystemProbeSpec contains the SystemProbe Agent configuration
// +k8s:openapi-gen=true
type SystemProbeSpec struct {
	// Enable this to activate live process monitoring.
	// Note: /etc/passwd is automatically mounted to allow username resolution.
	// ref: https://docs.datadoghq.com/graphing/infrastructure/process/#kubernetes-daemonset
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// SecCompRootPath specify the seccomp profile root directory
	// +optional
	SecCompRootPath string `json:"secCompRootPath,omitempty"`

	// SecCompCustomProfileConfigMap specify a pre-existing ConfigMap containing a custom SecComp profile
	// +optional
	SecCompCustomProfileConfigMap string `json:"secCompCustomProfileConfigMap,omitempty"`

	// SecCompProfileName specify a seccomp profile
	// +optional
	SecCompProfileName string `json:"secCompProfileName,omitempty"`

	// AppArmorProfileName specify a apparmor profile
	// +optional
	AppArmorProfileName string `json:"appArmorProfileName,omitempty"`

	// ConntrackEnabled enable the system-probe agent to connect to the netlink/conntrack subsystem to add NAT information to connection data
	// Ref: http://conntrack-tools.netfilter.org/
	ConntrackEnabled *bool `json:"conntrackEnabled,omitempty"`

	// BPFDebugEnabled logging for kernel debug
	BPFDebugEnabled *bool `json:"bpfDebugEnabled,omitempty"`

	// DebugPort Specify the port to expose pprof and expvar for system-probe agent
	DebugPort int32 `json:"debugPort,omitempty"`

	// EnableTCPQueueLength enables the TCP queue length eBPF-based check
	EnableTCPQueueLength *bool `json:"enableTCPQueueLength,omitempty"`

	// EnableOOMKill enables the OOM kill eBPF-based check
	EnableOOMKill *bool `json:"enableOOMKill,omitempty"`

	// CollectDNSStats enables DNS stat collection
	CollectDNSStats *bool `json:"collectDNSStats,omitempty"`

	// The Datadog SystemProbe supports many environment variables
	// Ref: https://docs.datadoghq.com/agent/docker/?tab=standard#environment-variables
	//
	// +optional
	// +listType=map
	// +listMapKey=name
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Datadog SystemProbe resource requests and limits
	// Make sure to keep requests and limits equal to keep the pods in the Guaranteed QoS class
	// Ref: http://kubernetes.io/docs/user-guide/compute-resources/
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// You can modify the security context used to run the containers by
	// modifying the label type
	// +optional
	SecurityContext *corev1.SecurityContext `json:"securityContext,omitempty"`
}

// SecuritySpec contains the Security Agent configuration
// +k8s:openapi-gen=true
type SecuritySpec struct {
	// Compliance configuration
	// +optional
	Compliance ComplianceSpec `json:"compliance,omitempty"`

	// Runtime security configuration
	// +optional
	Runtime RuntimeSecuritySpec `json:"runtime,omitempty"`

	// The Datadog Security Agent supports many environment variables
	// Ref: https://docs.datadoghq.com/agent/docker/?tab=standard#environment-variables
	//
	// +optional
	// +listType=map
	// +listMapKey=name
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Datadog Security Agent resource requests and limits
	// Make sure to keep requests and limits equal to keep the pods in the Guaranteed QoS class
	// Ref: http://kubernetes.io/docs/user-guide/compute-resources/
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
}

// ComplianceSpec contains configuration for continuous compliance
// +k8s:openapi-gen=true
type ComplianceSpec struct {
	// Enables continuous compliance monitoring
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Check interval
	// +optional
	CheckInterval *metav1.Duration `json:"checkInterval,omitempty"`

	// Config dir containing compliance benchmarks
	// +optional
	ConfigDir *ConfigDirSpec `json:"configDir,omitempty"`
}

// RuntimeSecuritySpec contains configuration for runtime security features
// +k8s:openapi-gen=true
type RuntimeSecuritySpec struct {
	// Enables runtime security features
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// ConfigDir containing security policies
	// +optional
	PoliciesDir *ConfigDirSpec `json:"policiesDir,omitempty"`

	// Syscall monitor configuration
	// +optional
	SyscallMonitor *SyscallMonitorSpec `json:"syscallMonitor,omitempty"`
}

// SyscallMonitorSpec contains configuration for syscall monitor
// +k8s:openapi-gen=true
type SyscallMonitorSpec struct {
	// Enabled enables syscall monitor
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// ConfigDirSpec contains config file directory configuration
// +k8s:openapi-gen=true
type ConfigDirSpec struct {
	// ConfigMapName name of a ConfigMap used to mount a directory
	ConfigMapName string `json:"configMapName,omitempty"`
}

// ConfigFileConfigMapSpec contains configMap information used to store a config file
// +k8s:openapi-gen=true
type ConfigFileConfigMapSpec struct {
	// Name the ConfigMap name
	Name string `json:"name,omitempty"`
	// FileKey corresponds to the key used in the ConfigMap.Data to store the configuration file content
	FileKey string `json:"fileKey,omitempty"`
}

// CustomConfigSpec Allow to put custom configuration for the agent, corresponding to the datadog-cluster.yaml or datadog.yaml config file
// the configuration can be provided in the 'configData' field as raw data, or in a configmap thanks to `configMap` field.
// Important: `configData` and `configMap` can't be set together.
// +k8s:openapi-gen=true
type CustomConfigSpec struct {
	// ConfigData corresponds to the configuration file content
	ConfigData *string `json:"configData,omitempty"`
	// ConfigMap name of a ConfigMap used to mount the configuration file
	ConfigMap *ConfigFileConfigMapSpec `json:"configMap,omitempty"`
}

// NodeAgentConfig contains the configuration of the Node Agent
// +k8s:openapi-gen=true
type NodeAgentConfig struct {
	// Pod-level SecurityContext
	// +optional
	SecurityContext *corev1.PodSecurityContext `json:"securityContext,omitempty"`

	// The host of the Datadog intake server to send Agent data to, only set this option
	// if you need the Agent to send data to a custom URL.
	// Overrides the site setting defined in "site".
	// +optional
	DDUrl *string `json:"ddUrl,omitempty"`

	// Set logging verbosity, valid log levels are:
	// trace, debug, info, warn, error, critical, and off
	LogLevel *string `json:"logLevel,omitempty"`

	// Confd configuration allowing to specify config files for custom checks placed under /etc/datadog-agent/conf.d/.
	// See https://docs.datadoghq.com/agent/guide/agent-configuration-files/?tab=agentv6 for more details.
	// +optional
	Confd *ConfigDirSpec `json:"confd,omitempty"`

	// Checksd configuration allowing to specify custom checks placed under /etc/datadog-agent/checks.d/
	// See https://docs.datadoghq.com/agent/guide/agent-configuration-files/?tab=agentv6 for more details.
	// +optional
	Checksd *ConfigDirSpec `json:"checksd,omitempty"`

	// Provide a mapping of Kubernetes Labels to Datadog Tags.
	// <KUBERNETES_LABEL>: <DATADOG_TAG_KEY>
	// +optional
	PodLabelsAsTags map[string]string `json:"podLabelsAsTags,omitempty"`

	// Provide a mapping of Kubernetes Annotations to Datadog Tags.
	// <KUBERNETES_ANNOTATIONS>: <DATADOG_TAG_KEY>
	// +optional
	PodAnnotationsAsTags map[string]string `json:"podAnnotationsAsTags,omitempty"`

	// List of tags to attach to every metric, event and service check collected by this Agent.
	// Learn more about tagging: https://docs.datadoghq.com/tagging/
	// +optional
	// +listType=set
	Tags []string `json:"tags,omitempty"`

	// Enables this to start event collection from the Kubernetes API
	// ref: https://docs.datadoghq.com/agent/kubernetes/event_collection/
	// +optional
	CollectEvents *bool `json:"collectEvents,omitempty"`

	// Enables leader election mechanism for event collection.
	// +optional
	LeaderElection *bool `json:"leaderElection,omitempty"`

	// The Datadog Agent supports many environment variables
	// Ref: https://docs.datadoghq.com/agent/docker/?tab=standard#environment-variables
	//
	// +optional
	// +listType=map
	// +listMapKey=name
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Specify additional volume mounts in the Datadog Agent container
	// +optional
	// +listType=map
	// +listMapKey=name
	// +listMapKey=mountPath
	VolumeMounts []corev1.VolumeMount `json:"volumeMounts,omitempty"`

	// Specify additional volumes in the Datadog Agent container
	// +optional
	// +listType=map
	// +listMapKey=name
	Volumes []corev1.Volume `json:"volumes,omitempty"`

	// Datadog Agent resource requests and limits
	// Make sure to keep requests and limits equal to keep the pods in the Guaranteed QoS class
	// Ref: http://kubernetes.io/docs/user-guide/compute-resources/
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// Configure the CRI Socket
	CriSocket *CRISocketConfig `json:"criSocket,omitempty"`

	// Configure Dogstatsd
	Dogstatsd *DogstatsdConfig `json:"dogstatsd,omitempty"`

	// If specified, the Agent pod's tolerations.
	// +optional
	// +listType=atomic
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Number of port to expose on the host.
	// If specified, this must be a valid port number, 0 < x < 65536.
	// If HostNetwork is specified, this must match ContainerPort.
	// Most containers do not need this.
	//
	// +optional
	HostPort *int32 `json:"hostPort,omitempty"`
}

// CRISocketConfig contains the CRI socket configuration parameters
// +k8s:openapi-gen=true
type CRISocketConfig struct {
	// Path to the docker runtime socket
	// +optional
	DockerSocketPath *string `json:"dockerSocketPath,omitempty"`

	// Path to the container runtime socket (if different from Docker)
	// This is supported starting from agent 6.6.0
	// +optional
	CriSocketPath *string `json:"criSocketPath,omitempty"`
}

// DogstatsdConfig contains the Dogstatsd configuration parameters
// +k8s:openapi-gen=true
type DogstatsdConfig struct {
	// Enable origin detection for container tagging
	// https://docs.datadoghq.com/developers/dogstatsd/unix_socket/#using-origin-detection-for-container-tagging
	// +optional
	DogstatsdOriginDetection *bool `json:"dogstatsdOriginDetection,omitempty"`

	// Enable dogstatsd over Unix Domain Socket
	// ref: https://docs.datadoghq.com/developers/dogstatsd/unix_socket/
	// +optional
	UseDogStatsDSocketVolume *bool `json:"useDogStatsDSocketVolume,omitempty"`
}

// DatadogAgentSpecClusterAgentSpec defines the desired state of the cluster Agent
// +k8s:openapi-gen=true
type DatadogAgentSpecClusterAgentSpec struct {
	// The container image of the Datadog Cluster Agent
	Image ImageConfig `json:"image"`

	// Name of the Cluster Agent Deployment to create or migrate from
	// +optional
	DeploymentName string `json:"deploymentName,omitempty"`

	// Cluster Agent configuration
	Config ClusterAgentConfig `json:"config,omitempty"`

	// Allow to put custom configuration for the agent, corresponding to the datadog-cluster.yaml config file
	// +optional
	CustomConfig *CustomConfigSpec `json:"customConfig,omitempty"`

	// RBAC configuration of the Datadog Cluster Agent
	Rbac RbacConfig `json:"rbac,omitempty"`

	// Number of the Cluster Agent replicas
	Replicas *int32 `json:"replicas,omitempty"`

	// AdditionalAnnotations provide annotations that will be added to the cluster-agent Pods.
	AdditionalAnnotations map[string]string `json:"additionalAnnotations,omitempty"`

	// AdditionalLabels provide labels that will be added to the cluster checks runner Pods.
	AdditionalLabels map[string]string `json:"additionalLabels,omitempty"`

	// If specified, indicates the pod's priority. "system-node-critical" and "system-cluster-critical"
	// are two special keywords which indicate the highest priorities with the former being the highest priority.
	// Any other name must be defined by creating a PriorityClass object with that name. If not specified,
	// the pod priority will be default or zero if there is no default.
	PriorityClassName string `json:"priorityClassName,omitempty"`

	// If specified, the pod's scheduling constraints
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// If specified, the Cluster-Agent pod's tolerations.
	// +optional
	// +listType=atomic
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// NodeSelector is a selector which must be true for the pod to fit on a node.
	// Selector which must match a node's labels for the pod to be scheduled on that node.
	// More info: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Provide Cluster Agent Network Policy configuration
	// +optional
	NetworkPolicy NetworkPolicySpec `json:"networkPolicy,omitempty"`
}

// ClusterAgentConfig contains the configuration of the Cluster Agent
// +k8s:openapi-gen=true
type ClusterAgentConfig struct {
	ExternalMetrics *ExternalMetricsConfig `json:"externalMetrics,omitempty"`

	// Configure the Admission Controller
	AdmissionController *AdmissionControllerConfig `json:"admissionController,omitempty"`

	// Enable the Cluster Checks and Endpoint Checks feature on both the cluster-agents and the daemonset
	// ref:
	// https://docs.datadoghq.com/agent/cluster_agent/clusterchecks/
	// https://docs.datadoghq.com/agent/cluster_agent/endpointschecks/
	// Autodiscovery via Kube Service annotations is automatically enabled
	ClusterChecksEnabled *bool `json:"clusterChecksEnabled,omitempty"`

	// Enable this to start event collection from the kubernetes API
	// ref: https://docs.datadoghq.com/agent/cluster_agent/event_collection/
	// +optional
	CollectEvents *bool `json:"collectEvents,omitempty"`

	// Set logging verbosity, valid log levels are:
	// trace, debug, info, warn, error, critical, and off
	LogLevel *string `json:"logLevel,omitempty"`

	// Datadog cluster-agent resource requests and limits
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// Confd Provide additional cluster check configurations. Each key will become a file in /conf.d
	// see https://docs.datadoghq.com/agent/autodiscovery/ for more details.
	// +optional
	Confd *ConfigDirSpec `json:"confd,omitempty"`

	// The Datadog Agent supports many environment variables
	// Ref: https://docs.datadoghq.com/agent/docker/?tab=standard#environment-variables
	//
	// +optional
	// +listType=map
	// +listMapKey=name
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Specify additional volume mounts in the Datadog Cluster Agent container
	// +optional
	// +listType=map
	// +listMapKey=name
	// +listMapKey=mountPath
	VolumeMounts []corev1.VolumeMount `json:"volumeMounts,omitempty"`

	// Specify additional volumes in the Datadog Cluster Agent container
	// +optional
	// +listType=map
	// +listMapKey=name
	Volumes []corev1.Volume `json:"volumes,omitempty"`
}

// ExternalMetricsConfig contains the configuration of the external metrics provider in Cluster Agent
// +k8s:openapi-gen=true
type ExternalMetricsConfig struct {
	// Enable the metricsProvider to be able to scale based on metrics in Datadog
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Enable informer and controller of the watermark pod autoscaler
	// NOTE: The WatermarkPodAutoscaler controller needs to be installed
	// see https://github.com/DataDog/watermarkpodautoscaler for more details.
	// +optional
	WpaController bool `json:"wpaController,omitempty"`

	// Enable usage of DatadogMetrics CRD (allow to scale on arbitrary queries)
	// +optional
	UseDatadogMetrics bool `json:"useDatadogMetrics,omitempty"`

	// If specified configures the metricsProvider external metrics service port
	// +optional
	Port *int32 `json:"port,omitempty"`

	// Override the API endpoint for the external metrics server. Defaults
	// to .spec.agent.config.ddUrl or "https://app.datadoghq.com" if that's
	// empty.
	// +optional
	Endpoint *string `json:"endpoint,omitempty"`
}

// AdmissionControllerConfig contains the configuration of the admission controller in Cluster Agent
// +k8s:openapi-gen=true
type AdmissionControllerConfig struct {
	// Enable the admission controller to be able to inject APM/Dogstatsd config
	// and standard tags (env, service, version) automatically into your pods
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// MutateUnlabelled enables injecting config without having the pod label 'admission.datadoghq.com/enabled="true"'
	// +optional
	MutateUnlabelled *bool `json:"mutateUnlabelled,omitempty"`

	// ServiceName corresponds to the webhook service name
	// +optional
	ServiceName *string `json:"serviceName,omitempty"`
}

// ClusterChecksRunnerConfig contains the configuration of the Cluster Checks Runner
// +k8s:openapi-gen=true
type ClusterChecksRunnerConfig struct {
	// Datadog Cluster Checks Runner resource requests and limits
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// Set logging verbosity, valid log levels are:
	// trace, debug, info, warn, error, critical, and off
	LogLevel *string `json:"logLevel,omitempty"`

	// The Datadog Agent supports many environment variables
	// Ref: https://docs.datadoghq.com/agent/docker/?tab=standard#environment-variables
	//
	// +optional
	// +listType=map
	// +listMapKey=name
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Specify additional volume mounts in the Datadog Cluster Check Runner container
	// +optional
	// +listType=map
	// +listMapKey=name
	// +listMapKey=mountPath
	VolumeMounts []corev1.VolumeMount `json:"volumeMounts,omitempty"`

	// Specify additional volumes in the Datadog Cluster Check Runner container
	// +optional
	// +listType=map
	// +listMapKey=name
	Volumes []corev1.Volume `json:"volumes,omitempty"`
}

// DatadogAgentSpecClusterChecksRunnerSpec defines the desired state of the Cluster Checks Runner
// +k8s:openapi-gen=true
type DatadogAgentSpecClusterChecksRunnerSpec struct {
	// The container image of the Datadog Cluster Checks Runner
	Image ImageConfig `json:"image"`

	// Name of the cluster checks deployment to create or migrate from
	// +optional
	DeploymentName string `json:"deploymentName,omitempty"`

	// Agent configuration
	Config ClusterChecksRunnerConfig `json:"config,omitempty"`

	// Allow to put custom configuration for the agent, corresponding to the datadog.yaml config file
	// See https://docs.datadoghq.com/agent/guide/agent-configuration-files/?tab=agentv6 for more details.
	// +optional
	CustomConfig *CustomConfigSpec `json:"customConfig,omitempty"`

	// RBAC configuration of the Datadog Cluster Checks Runner
	Rbac RbacConfig `json:"rbac,omitempty"`

	// Number of the Cluster Agent replicas
	Replicas *int32 `json:"replicas,omitempty"`

	// AdditionalAnnotations provide annotations that will be added to the cluster checks runner Pods.
	AdditionalAnnotations map[string]string `json:"additionalAnnotations,omitempty"`

	// AdditionalLabels provide labels that will be added to the cluster checks runner Pods.
	AdditionalLabels map[string]string `json:"additionalLabels,omitempty"`

	// If specified, indicates the pod's priority. "system-node-critical" and "system-cluster-critical"
	// are two special keywords which indicate the highest priorities with the former being the highest priority.
	// Any other name must be defined by creating a PriorityClass object with that name. If not specified,
	// the pod priority will be default or zero if there is no default.
	PriorityClassName string `json:"priorityClassName,omitempty"`

	// If specified, the pod's scheduling constraints
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// If specified, the Cluster-Checks pod's tolerations.
	// +optional
	// +listType=atomic
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// NodeSelector is a selector which must be true for the pod to fit on a node.
	// Selector which must match a node's labels for the pod to be scheduled on that node.
	// More info: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Provide Cluster Checks Runner Network Policy configuration
	// +optional
	NetworkPolicy NetworkPolicySpec `json:"networkPolicy,omitempty"`
}

// ImageConfig Datadog agent container image config
// +k8s:openapi-gen=true
type ImageConfig struct {
	// Define the image to use
	// Use "datadog/agent:latest" for Datadog Agent 6
	// Use "datadog/dogstatsd:latest" for Standalone Datadog Agent DogStatsD6
	// Use "datadog/cluster-agent:latest" for Datadog Cluster Agent
	Name string `json:"name"`

	// The Kubernetes pull policy
	// Use Always, Never or IfNotPresent
	PullPolicy *corev1.PullPolicy `json:"pullPolicy,omitempty"`

	// It is possible to specify docker registry credentials
	// See https://kubernetes.io/docs/concepts/containers/images/#specifying-imagepullsecrets-on-a-pod
	// +optional
	PullSecrets *[]corev1.LocalObjectReference `json:"pullSecrets,omitempty"`
}

// NetworkPolicySpec provides Network Policy configuration for the agents
// +k8s:openapi-gen=true
type NetworkPolicySpec struct {
	// If true, create a NetworkPolicy for the current agent
	// +optional
	Create *bool `json:"create,omitempty"`
}

// DatadogAgentState type representing the deployment state of the different Agent components
type DatadogAgentState string

const (
	// DatadogAgentStateProgressing the deployment is running properly
	DatadogAgentStateProgressing DatadogAgentState = "Progressing"
	// DatadogAgentStateRunning the deployment is running properly
	DatadogAgentStateRunning DatadogAgentState = "Running"
	// DatadogAgentStateUpdating the deployment is currently under a rolling update
	DatadogAgentStateUpdating DatadogAgentState = "Updating"
	// DatadogAgentStateCanary the deployment is currently under a canary testing (EDS only)
	DatadogAgentStateCanary DatadogAgentState = "Canary"
	// DatadogAgentStateFailed the current state of the deployment is considered as Failed
	DatadogAgentStateFailed DatadogAgentState = "Failed"
)

// DatadogAgentStatus defines the observed state of DatadogAgent
// +k8s:openapi-gen=true
type DatadogAgentStatus struct {
	// The actual state of the Agent as an extended daemonset
	// +optional
	Agent *DaemonSetStatus `json:"agent,omitempty"`

	// The actual state of the Cluster Agent as a deployment
	// +optional
	ClusterAgent *DeploymentStatus `json:"clusterAgent,omitempty"`

	// The actual state of the Cluster Checks Runner as a deployment
	// +optional
	ClusterChecksRunner *DeploymentStatus `json:"clusterChecksRunner,omitempty"`

	// Conditions Represents the latest available observations of a DatadogAgent's current state.
	// +listType=map
	// +listMapKey=type
	Conditions []DatadogAgentCondition `json:"conditions,omitempty"`
}

// DaemonSetStatus defines the observed state of Agent running as DaemonSet
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

	// DaemonsetName corresponds to the name of the created DaemonSet
	DaemonsetName string `json:"daemonsetName,omitempty"`
}

// DeploymentStatus type representing the Cluster Agent Deployment status
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
	// enabled
	// +optional
	GeneratedToken string `json:"generatedToken,omitempty"`

	// Status corresponds to the ClusterAgent deployment computed status
	Status string `json:"status,omitempty"`
	// State corresponds to the ClusterAgent deployment state
	State string `json:"state,omitempty"`

	// DeploymentName corresponds to the name of the Cluster Agent Deployment
	DeploymentName string `json:"deploymentName,omitempty"`
}

// DatadogAgentCondition describes the state of a DatadogAgent at a certain point.
// +k8s:openapi-gen=true
type DatadogAgentCondition struct {
	// Type of DatadogAgent condition.
	Type DatadogAgentConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// Last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// Last time the condition was updated.
	// +optional
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
	// The reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty"`
	// A human readable message indicating details about the transition.
	// +optional
	Message string `json:"message,omitempty"`
}

// DatadogAgentConditionType type use to represent a DatadogAgent condition
type DatadogAgentConditionType string

const (
	// DatadogAgentConditionTypeActive DatadogAgent is active
	DatadogAgentConditionTypeActive DatadogAgentConditionType = "Active"
	// DatadogAgentConditionTypeReconcileError the controller wasn't able to run properly the reconcile loop with this DatadogAgent
	DatadogAgentConditionTypeReconcileError DatadogAgentConditionType = "ReconcileError"
	// DatadogAgentConditionTypeSecretError the required Secret doesn't exist.
	DatadogAgentConditionTypeSecretError DatadogAgentConditionType = "SecretError"

	// DatadogAgentConditionTypeActiveDatadogMetrics forwarding metrics and events to Datadog is active
	DatadogAgentConditionTypeActiveDatadogMetrics DatadogAgentConditionType = "ActiveDatadogMetrics"
	// DatadogAgentConditionTypeDatadogMetricsError cannot forward deployment metrics and events to Datadog
	DatadogAgentConditionTypeDatadogMetricsError DatadogAgentConditionType = "DatadogMetricsError"
)

// DatadogAgent Deployment with Datadog Operator
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
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

// DatadogAgentList contains a list of DatadogAgent
// +kubebuilder:object:root=true
type DatadogAgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// +listType=atomic
	Items []DatadogAgent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatadogAgent{}, &DatadogAgentList{})
}
