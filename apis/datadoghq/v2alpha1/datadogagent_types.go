// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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

// DatadogFeatures are Features running on the Agent and Cluster Agent.
// TODO add the features (APM, Kubernetes State Metrics Core...)
// +k8s:openapi-gen=true
type DatadogFeatures struct{}

// GlobalConfig is a set of parameters that are used to configure all the components of the Datadog Operator
type GlobalConfig struct {
	// Set a unique cluster name to allow scoping hosts and Cluster Checks Runner easily.
	// +optional
	ClusterName string `json:"clusterName,omitempty"`

	// The site of the Datadog intake to send Agent data to.
	// Set to 'datadoghq.eu' to send data to the EU site.
	// +optional
	Site string `json:"site,omitempty"`

	// Registry to use for all Agent images (default gcr.io/datadoghq).
	// Use public.ecr.aws/datadog for AWS
	// Use docker.io/datadog for DockerHub
	// +optional
	Registry *string `json:"registry,omitempty"`
}

// DatadogAgentResourceOverride is the generic description of a component (Cluster Agent Deployment, Node Agent Daemonset...)
// TODO: Add the Resource type and name to allow overriding the kind of the Resource (e.g. ExtendedDaemonset)
type DatadogAgentResourceOverride struct {
	// DatadogAgentPodTemplateOverride is used to configure the components at the pod level in an agnostic way
	DatadogAgentPodTemplateOverride *DatadogAgentPodTemplateOverride `json:"podOverride,omitempty"`

	// Name overrides the default name for the resource
	Name string `json:"name,omitempty"`
}

// DatadogAgentPodTemplateOverride is the generic description equivalent to a subset of the PodTemplate for a component
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

// DatadogAgentStatus defines the observed state of DatadogAgent
// +k8s:openapi-gen=true
type DatadogAgentStatus struct {

	// DefaultOverride contains attributes that were not configured that the runtime defaulted.
	// +optional
	DefaultOverride *DatadogAgentSpec `json:"defaultOverride,omitempty"`
}

// DatadogAgent Deployment with the Datadog Operator.
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

//+kubebuilder:object:root=true

// DatadogAgentList contains a list of DatadogAgent
type DatadogAgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatadogAgent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatadogAgent{}, &DatadogAgentList{})
}
