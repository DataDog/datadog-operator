// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v2alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DatadogAgentSpec defines the desired state of DatadogAgent
type DatadogAgentSpec struct {
	// Set a unique cluster name to allow scoping hosts and Cluster Checks Runner easily.
	// +optional
	ClusterName string `json:"clusterName,omitempty"`
}

// DatadogAgentStatus defines the observed state of DatadogAgent
type DatadogAgentStatus struct {

	// DefaultOverride contains attributes that were not configured that the runtime defaulted.
	// +optional
	DefaultOverride *DatadogAgentSpec `json:"defaultOverride,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// DatadogAgent is the Schema for the datadogagents API
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
