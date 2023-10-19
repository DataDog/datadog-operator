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
	ProfileAffinity *ProfileAffinity `json:"dapAffinity,omitempty"`
	Config          *Config          `json:"config,omitempty"`
}

type ProfileAffinity struct {
	ProfileNodeAffinity []corev1.NodeSelectorRequirement `json:"dapNodeAffinity,omitempty"`
}

type Config struct {
	Override map[ComponentName]*Override `json:"override,omitempty"`
}

type Override struct {
	Containers map[commonv1.AgentContainerName]*Container `json:"containers,omitempty"`
}

type Container struct {
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
}

// DatadogAgentProfileStatus defines the observed state of DatadogAgentProfile
type DatadogAgentProfileStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:path=datadogagentprofiles,shortName=dap

// DatadogAgentProfile is the Schema for the datadogagentprofiles API
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
