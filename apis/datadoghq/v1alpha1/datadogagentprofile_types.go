// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	commonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ComponentName string

const (
	// NodeAgentComponentName is the name of the Datadog Node Agent
	NodeAgentComponentName ComponentName = "nodeAgent"
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
	Override map[ComponentName]*Override `json:"override,omitempty"`
}

type Override struct {
	// Configure the basic configurations for an Agent container
	// Valid Agent container names are: `agent`
	// +optional
	Containers map[commonv1.AgentContainerName]*Container `json:"containers,omitempty"`

	// If specified, indicates the pod's priority. "system-node-critical" and
	// "system-cluster-critical" are two special keywords which indicate the
	// highest priorities with the former being the highest priority. Any other
	// name must be defined by creating a PriorityClass object with that name.
	// If not specified, the pod priority will be default or zero if there is no
	// default.
	// +optional
	PriorityClassName *string `json:"priorityClassName,omitempty"`

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
