// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	"github.com/DataDog/datadog-operator/api/datadoghq/common"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ComponentName string
type CreateStrategyStatus string

const (
	// NodeAgentComponentName is the name of the Datadog Node Agent
	NodeAgentComponentName ComponentName = "nodeAgent"

	CompletedStatus  CreateStrategyStatus = "Completed"
	WaitingStatus    CreateStrategyStatus = "Waiting"
	InProgressStatus CreateStrategyStatus = "In Progress"
)

// DatadogAgentProfileSpec defines the desired state of DatadogAgentProfile
type DatadogAgentProfileSpec struct {
	ProfileAffinity *ProfileAffinity `json:"profileAffinity,omitempty"`
	Config          *Config          `json:"config,omitempty"`
}

type ProfileAffinity struct {
	ProfileNodeAffinity []corev1.NodeSelectorRequirement `json:"profileNodeAffinity,omitempty"`
}

type Config struct {
	// Override the default configurations of the node agent.
	Override map[ComponentName]*Override `json:"override,omitempty"`
}

type Override struct {
	// Configure the basic configurations for an Agent container
	// Valid Agent container names are: `agent`
	// +optional
	Containers map[common.AgentContainerName]*Container `json:"containers,omitempty"`

	// If specified, indicates the pod's priority. "system-node-critical" and
	// "system-cluster-critical" are two special keywords which indicate the
	// highest priorities with the former being the highest priority. Any other
	// name must be defined by creating a PriorityClass object with that name.
	// If not specified, the pod priority will be default or zero if there is no
	// default.
	// +optional
	PriorityClassName *string `json:"priorityClassName,omitempty"`

	// If specified, indicates the pod's RuntimeClass kubelet should use to run the pod.
	// If the named RuntimeClass does not exist, or the CRI cannot run the corresponding handler, the pod enters the Failed terminal phase.
	// If no runtimeClassName is specified, the default RuntimeHandler is used, which is equivalent to the behavior when the RuntimeClass feature is disabled.
	// +optional
	RuntimeClassName *string `json:"runtimeClassName,omitempty"`

	// The deployment strategy to use to replace existing pods with new ones.
	// Valid types are `RollingUpdate` or `OnDelete` for DaemonSets
	// +optional
	UpdateStrategy *common.UpdateStrategy `json:"updateStrategy,omitempty"`

	// Labels provide labels that are added to the Datadog Agent pods.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

type Container struct {
	// Specify additional environment variables in the container.
	// See also: https://docs.datadoghq.com/agent/guide/environment-variables/
	//
	// +optional
	// +listType=map
	// +listMapKey=name
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Specify the Request and Limits of the pods.
	// To get guaranteed QoS class, specify requests and limits equal.
	// See also: http://kubernetes.io/docs/user-guide/compute-resources/
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
}

// DatadogAgentProfileStatus defines the observed state of DatadogAgentProfile
// +k8s:openapi-gen=true
type DatadogAgentProfileStatus struct {
	// LastUpdate is the last time the status was updated.
	// +optional
	LastUpdate *metav1.Time `json:"lastUpdate,omitempty"`

	// CurrentHash is the stored hash of the DatadogAgentProfile.
	// +optional
	CurrentHash string `json:"currentHash,omitempty"`

	// Conditions represents the latest available observations of a DatadogAgentProfile's current state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions"`

	// Valid shows if the DatadogAgentProfile has a valid config spec.
	// +optional
	Valid metav1.ConditionStatus `json:"valid,omitempty"`

	// Applied shows whether the DatadogAgentProfile conflicts with an existing DatadogAgentProfile.
	// +optional
	Applied metav1.ConditionStatus `json:"applied,omitempty"`

	// CreateStrategy is the state of the create strategy feature.
	// +optional
	CreateStrategy *CreateStrategy `json:"createStrategy,omitempty"`
}

// CreateStrategy defines the observed state of the create strategy feature based on the agent deployment.
// +k8s:openapi-gen=true
// +kubebuilder:object:generate=true
type CreateStrategy struct {
	// Status shows the current state of the feature.
	// +optional
	Status CreateStrategyStatus `json:"status,omitempty"`

	// NodesLabeled shows the number of nodes currently labeled.
	// +optional
	NodesLabeled int32 `json:"nodesLabeled"`

	// PodsReady shows the number of pods in the ready state.
	// +optional
	PodsReady int32 `json:"podsReady"`

	// MaxUnavailable shows the number of pods that can be in an unready state.
	// +optional
	MaxUnavailable int32 `json:"maxUnavailable"`

	// LastTransition is the last time the status was updated.
	// +optional
	LastTransition *metav1.Time `json:"lastTransition,omitempty"`
}

// DatadogAgentProfile is the Schema for the datadogagentprofiles API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=datadogagentprofiles,shortName=dap
// +kubebuilder:printcolumn:name="valid",type="string",JSONPath=".status.valid"
// +kubebuilder:printcolumn:name="applied",type="string",JSONPath=".status.applied"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
// +k8s:openapi-gen=true
type DatadogAgentProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatadogAgentProfileSpec   `json:"spec,omitempty"`
	Status DatadogAgentProfileStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DatadogAgentProfileList contains a list of DatadogAgentProfile
type DatadogAgentProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatadogAgentProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatadogAgentProfile{}, &DatadogAgentProfileList{})
}
