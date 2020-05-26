// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package v1alpha1

import (
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ExtendedNodeSpec is the Schema for the extendednode API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
type ExtendedNodeSpec struct {
	// Reference contains enough information to let you identify the referred resource.
	Reference *autoscalingv1.CrossVersionObjectReference `json:"reference"`
	// NodeSelector lists labels that must be present on nodes to trigger the usage of this resource.
	NodeSelector metav1.LabelSelector `json:"nodeSelector"`
	// Containers contains a list of container spec overwrite.
	// +listType=map
	// +listMapKey=name
	Containers []ExtendedNodeContainerSpec `json:"containers,omitempty"`
}

// ExtendedNodeContainerSpec defines the resources overwrite for a container identified by its name
type ExtendedNodeContainerSpec struct {
	Name      string                      `json:"name"`
	Resources corev1.ResourceRequirements `json:"resources"`
}

// ExtendedNodeStatusStatus defines the readable status in ExtendedNodeStatus
type ExtendedNodeStatusStatus string

const (
	// ExtendedNodeStatusValid status when ExtendedNode is valide
	ExtendedNodeStatusValid ExtendedNodeStatusStatus = "valid"
	// ExtendedNodeStatusError status when ExtendedNode is in error state
	ExtendedNodeStatusError ExtendedNodeStatusStatus = "error"
)

// ExtendedNodeStatus defines the observed state of ExtendedNode
type ExtendedNodeStatus struct {
	Status ExtendedNodeStatusStatus `json:"status"`
	Error  string                   `json:"error,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ExtendedNode is the Schema for the extendednodes API
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=extendednodes,scope=Namespaced
// +kubebuilder:printcolumn:name="status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="node selector",type="string",JSONPath=".spec.nodeSelector"
// +kubebuilder:printcolumn:name="error",type="string",JSONPath=".status.error"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
type ExtendedNode struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ExtendedNodeSpec   `json:"spec,omitempty"`
	Status ExtendedNodeStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ExtendedNodeList contains a list of ExtendedNode
type ExtendedNodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ExtendedNode `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ExtendedNode{}, &ExtendedNodeList{})
}
