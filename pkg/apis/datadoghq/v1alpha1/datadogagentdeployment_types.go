// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package v1alpha1

import (
	edsdatadoghqv1alpha1 "github.com/datadog/extendeddaemonset/pkg/apis/datadoghq/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// DatadogAgentDeploymentSpec defines the desired state of DatadogAgentDeployment
// +k8s:openapi-gen=true
type DatadogAgentDeploymentSpec struct {
	// Configure the credentials required to run Agents
	Credentials AgentCredentials `json:"credentials"`

	// The desired state of the Agent as an extended daemonset
	// Contains the Node Agent configuration and deployment strategy
	// +optional
	Agent *DatadogAgentDeploymentSpecAgentSpec `json:"agent,omitempty"`

	// The desired state of the Cluster Agent as a deployment
	// +optional
	ClusterAgent *DatadogAgentDeploymentSpecClusterAgentSpec `json:"clusterAgent,omitempty"`

	// The desired state of the Cluster Checks Runner as a deployment
	// +optional
	ClusterChecksRunner *DatadogAgentDeploymentSpecClusterChecksRunnerSpec `json:"clusterChecksRunner,omitempty"`

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

	// APIKeyExistingSecret Use existing Secret which stores API key instead of creating a new one.
	// If set, this parameter takes precedence over "apiKey".
	// +optional
	APIKeyExistingSecret string `json:"apiKeyExistingSecret,omitempty"`

	// If you are using clusterAgent.metricsProvider.enabled = true, you must set
	// a Datadog application key for read access to your metrics.
	// +optional
	AppKey string `json:"appKey,omitempty"`

	// Use existing Secret which stores APP key instead of creating a new one
	// If set, this parameter takes precedence over "appKey".
	// +optional
	AppKeyExistingSecret string `json:"appKeyExistingSecret,omitempty"`

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

// DatadogAgentDeploymentSpecAgentSpec defines the desired state of the node Agent
// +k8s:openapi-gen=true
type DatadogAgentDeploymentSpecAgentSpec struct {
	// UseExtendedDaemonset use ExtendedDaemonset for Agent deployment.
	// default value is false.
	UseExtendedDaemonset *bool `json:"useExtendedDaemonset,omitempty"`

	// The container image of the Datadog Agent
	Image ImageConfig `json:"image"`

	// Agent configuration
	Config NodeAgentConfig `json:"config,omitempty"`

	// RBAC configuration of the Agent
	Rbac RbacConfig `json:"rbac,omitempty"`

	// Update strategy configuration for the DaemonSet
	DeploymentStrategy *DaemonSetDeploymentcStrategy `json:"deploymentStrategy,omitempty"`

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

	// Confd configuration allowing to specify config files for custom checks placed under /etc/datadog-agent/conf.d/.
	// See https://docs.datadoghq.com/agent/guide/agent-configuration-files/?tab=agentv6 for more details.
	// +optional
	Confd *ConfigDirSpec `json:"confd,omitempty"`

	// Checksd configuration allowing to specify custom checks placed under /etc/datadog-agent/checks.d/
	// See https://docs.datadoghq.com/agent/guide/agent-configuration-files/?tab=agentv6 for more details.
	// +optional
	Checksd *ConfigDirSpec `json:"checksd,omitempty"`
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

// DaemonSetDeploymentcStrategy contains the node Agent deployment configuration
// +k8s:openapi-gen=true
type DaemonSetDeploymentcStrategy struct {
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
	// +listType=set
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
	// +optional
	LogsConfigContainerCollectAll *bool `json:"logsConfigContainerCollectAll,omitempty"`

	// This to allow log collection from container log path. Set to a different path if not using docker runtime.
	// ref: https://docs.datadoghq.com/agent/kubernetes/daemonset_setup/?tab=k8sfile#create-manifest
	//
	// +optional
	ContainerLogsPath *string `json:"containerLogsPath,omitempty"`
}

// ProcessSpec contains the Process Agent configuration
// +k8s:openapi-gen=true
type ProcessSpec struct {
	// Enable this to activate live process monitoring.
	// Note: /etc/passwd is automatically mounted to allow username resolution.
	// ref: https://docs.datadoghq.com/graphing/infrastructure/process/#kubernetes-daemonset
	//
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
	// The Datadog Agent supports many environment variables
	// Ref: https://docs.datadoghq.com/agent/docker/?tab=standard#environment-variables
	//
	// +optional
	// +listType=set
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Datadog Process Agent resource requests and limits
	// Make sure to keep requests and limits equal to keep the pods in the Guaranteed QoS class
	// Ref: http://kubernetes.io/docs/user-guide/compute-resources/
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
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

	// The Datadog SystemProbe supports many environment variables
	// Ref: https://docs.datadoghq.com/agent/docker/?tab=standard#environment-variables
	//
	// +optional
	// +listType=set
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Datadog SystemProbe resource requests and limits
	// Make sure to keep requests and limits equal to keep the pods in the Guaranteed QoS class
	// Ref: http://kubernetes.io/docs/user-guide/compute-resources/
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
}

// ConfigDirSpec contains config file directory configuration
// +k8s:openapi-gen=true
type ConfigDirSpec struct {
	// ConfigMapName name of a ConfigMap used to mount a directory
	ConfigMapName string `json:"configMapName,omitempty"`
}

// NodeAgentConfig contains the configuration of the Node Agent
// +k8s:openapi-gen=true
type NodeAgentConfig struct {
	// You can modify the security context used to run the containers by
	// modifying the label type
	// +optional
	SecurityContext *corev1.SecurityContext `json:"securityContext,omitempty"`

	// The host of the Datadog intake server to send Agent data to, only set this option
	// if you need the Agent to send data to a custom URL.
	// Overrides the site setting defined in "site".
	// +optional
	DDUrl *string `json:"ddUrl,omitempty"`

	// Set logging verbosity, valid log levels are:
	// trace, debug, info, warn, error, critical, and off
	LogLevel *string `json:"logLevel,omitempty"`

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

	// nables this to start event collection from the kubernetes API
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
	// +listType=set
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Specify additional volumes to mount in the Datadog Agent container
	// +optional
	// +listType=set
	VolumeMounts []corev1.VolumeMount `json:"volumeMounts,omitempty"`

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
	// +listType=set
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
}

// CRISocketConfig contains the CRI socket configuration parameters
// +k8s:openapi-gen=true
type CRISocketConfig struct {
	// Enable container runtime socket volume mounting
	UseCriSocketVolume *bool `json:"useCriSocketVolume,omitempty"`

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

// DatadogAgentDeploymentSpecClusterAgentSpec defines the desired state of the cluster Agent
// +k8s:openapi-gen=true
type DatadogAgentDeploymentSpecClusterAgentSpec struct {
	// The container image of the Datadog Cluster Agent
	Image ImageConfig `json:"image"`

	// Cluster Agent configuration
	Config ClusterAgentConfig `json:"config,omitempty"`

	// RBAC configuration of the Datadog Cluster Agent
	Rbac RbacConfig `json:"rbac,omitempty"`

	// Number of the Cluster Agent replicas
	Replicas *int32 `json:"replicas,omitempty"`

	// If specified, the pod's scheduling constraints
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// If specified, the Cluster-Agent pod's tolerations.
	// +optional
	// +listType=set
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// NodeSelector is a selector which must be true for the pod to fit on a node.
	// Selector which must match a node's labels for the pod to be scheduled on that node.
	// More info: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
}

// ClusterAgentConfig contains the configuration of the Cluster Agent
// +k8s:openapi-gen=true
type ClusterAgentConfig struct {
	// Enable the metricsProvider to be able to scale based on metrics in Datadog
	MetricsProviderEnabled *bool `json:"metricsProviderEnabled,omitempty"`

	// Enable the Cluster Checks Runner feature on both the cluster-agents and the daemonset
	// ref: https://docs.datadoghq.com/agent/autodiscovery/ClusterChecksRunner/
	// Autodiscovery via Kube Service annotations is automatically enabled
	ClusterChecksRunnerEnabled *bool `json:"clusterChecksRunnerEnabled,omitempty"`

	// Set logging verbosity, valid log levels are:
	// trace, debug, info, warn, error, critical, and off
	LogLevel *string `json:"logLevel,omitempty"`

	// Datadog cluster-agent resource requests and limits
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
}

// ClusterChecksRunnerConfig contains the configuration of the Cluster Checks Runner
// +k8s:openapi-gen=true
type ClusterChecksRunnerConfig struct {
	// Datadog Cluster Checks Runner resource requests and limits
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
	// Set logging verbosity, valid log levels are:
	// trace, debug, info, warn, error, critical, and off
	LogLevel *string `json:"logLevel,omitempty"`
}

// DatadogAgentDeploymentSpecClusterChecksRunnerSpec defines the desired state of the Cluster Checks Runner
// +k8s:openapi-gen=true
type DatadogAgentDeploymentSpecClusterChecksRunnerSpec struct {
	// The container image of the Datadog Cluster Agent
	Image ImageConfig `json:"image"`

	// Agent configuration
	Config ClusterChecksRunnerConfig `json:"config,omitempty"`

	// RBAC configuration of the Datadog Cluster Checks Runner
	Rbac RbacConfig `json:"rbac,omitempty"`

	// Number of the Cluster Agent replicas
	Replicas *int32 `json:"replicas,omitempty"`

	// If specified, the pod's scheduling constraints
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// If specified, the Cluster-Checks pod's tolerations.
	// +optional
	// +listType=set
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// NodeSelector is a selector which must be true for the pod to fit on a node.
	// Selector which must match a node's labels for the pod to be scheduled on that node.
	// More info: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
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

// DatadogAgentDeploymentState type representing the ClusterAgent as a DaemonSet deployment state
type DatadogAgentDeploymentState string

const (
	// DatadogAgentDeploymentStateProgressing the deployment is running properly
	DatadogAgentDeploymentStateProgressing DatadogAgentDeploymentState = "Progressing"
	// DatadogAgentDeploymentStateRunning the deployment is running properly
	DatadogAgentDeploymentStateRunning DatadogAgentDeploymentState = "Running"
	// DatadogAgentDeploymentStateUpdating the deployment is currently under a rolling update
	DatadogAgentDeploymentStateUpdating DatadogAgentDeploymentState = "Updating"
	// DatadogAgentDeploymentStateCanary the deployment is currently under a canary testing (EDS only)
	DatadogAgentDeploymentStateCanary DatadogAgentDeploymentState = "Canary"
	// DatadogAgentDeploymentStateFailed the current state of the deployment is considered as Failed
	DatadogAgentDeploymentStateFailed DatadogAgentDeploymentState = "Failed"
)

// DatadogAgentDeploymentStatus defines the observed state of DatadogAgentDeployment
// +k8s:openapi-gen=true
type DatadogAgentDeploymentStatus struct {
	// The actual state of the Agent as an extended daemonset
	// +optional
	Agent *DatadogAgentDeploymentAgentStatus `json:"agent,omitempty"`

	// The actual state of the Cluster Agent as a deployment
	// +optional
	ClusterAgent *DatadogAgentDeploymentDeploymentStatus `json:"clusterAgent,omitempty"`

	// The actual state of the Cluster Checks Runner as a deployment
	// +optional
	ClusterChecksRunner *DatadogAgentDeploymentDeploymentStatus `json:"clusterChecksRunner,omitempty"`

	// Conditions Represents the latest available observations of a DatadogAgentDeployment's current state.
	// +listType=set
	Conditions []DatadogAgentDeploymentCondition `json:"conditions,omitempty"`
}

// DatadogAgentDeploymentAgentStatus defines the observed state of Agent running as DaemonSet
// +k8s:openapi-gen=true
type DatadogAgentDeploymentAgentStatus struct {
	Desired   int32 `json:"desired"`
	Current   int32 `json:"current"`
	Ready     int32 `json:"ready"`
	Available int32 `json:"available"`
	UpToDate  int32 `json:"upToDate"`

	State       string       `json:"state,omitempty"`
	LastUpdate  *metav1.Time `json:"lastUpdate,omitempty"`
	CurrentHash string       `json:"currentHash,omitempty"`
}

// DatadogAgentDeploymentDeploymentStatus type representing the Cluster Agent Deployment status
// +k8s:openapi-gen=true
type DatadogAgentDeploymentDeploymentStatus struct {
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

	// State corresponds to the ClusterAgent deployment state
	State string `json:"state,omitempty"`
}

// DatadogAgentDeploymentCondition describes the state of a DatadogAgentDeployment at a certain point.
// +k8s:openapi-gen=true
type DatadogAgentDeploymentCondition struct {
	// Type of DatadogAgentDeployment condition.
	Type DatadogAgentDeploymentConditionType `json:"type"`
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

// DatadogAgentDeploymentConditionType type use to represent a DatadogAgentDeployment condition
type DatadogAgentDeploymentConditionType string

const (
	// ConditionTypeActive DatadogAgentDeployment is active
	ConditionTypeActive DatadogAgentDeploymentConditionType = "Active"
	// ConditionTypeReconcileError the controller wasn't able to run properly the reconcile loop with this DatadogAgentDeployment
	ConditionTypeReconcileError DatadogAgentDeploymentConditionType = "ReconcileError"
	// ConditionTypeSecretError the required Secret doesn't exist.
	ConditionTypeSecretError DatadogAgentDeploymentConditionType = "SecretError"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DatadogAgentDeployment is the Schema for the agentdeployments API
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=datadogagentdeployments,shortName=dad
// +kubebuilder:printcolumn:name="active",type="string",JSONPath=".status.conditions[?(@.type=='Active')].status"
// +kubebuilder:printcolumn:name="agent",type="string",JSONPath=".status.agent.state"
// +kubebuilder:printcolumn:name="cluster-agent",type="string",JSONPath=".status.clusterAgent.state"
// +kubebuilder:printcolumn:name="cluster-checks-runner",type="string",JSONPath=".status.clusterChecksRunner.state"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
type DatadogAgentDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatadogAgentDeploymentSpec   `json:"spec,omitempty"`
	Status DatadogAgentDeploymentStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DatadogAgentDeploymentList contains a list of DatadogAgentDeployment
// +k8s:openapi-gen=true
type DatadogAgentDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// +listType=set
	Items []DatadogAgentDeployment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatadogAgentDeployment{}, &DatadogAgentDeploymentList{})
}
