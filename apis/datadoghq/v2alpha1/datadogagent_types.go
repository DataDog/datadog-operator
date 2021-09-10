// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v2alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DatadogAgentSpec defines the desired state of DatadogAgent
// +k8s:openapi-gen=true
type DatadogAgentSpec struct {
	// Set a unique cluster name to allow scoping hosts and Cluster Checks Runner easily.
	// +optional
	ClusterName string `json:"clusterName,omitempty"`
}

// DatadogAgentStatus defines the observed state of DatadogAgent
// +k8s:openapi-gen=true
type DatadogAgentStatus struct {

	// DefaultOverride contains attributes that were not configured that the runtime defaulted.
	// +optional
	DefaultOverride *DatadogAgentSpec `json:"defaultOverride,omitempty"`
}

// DatadogAgent Deployment with Datadog Operator.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
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
//+kubebuilder:object:root=true
type DatadogAgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// +listType=atomic
	Items []DatadogAgent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatadogAgent{}, &DatadogAgentList{})
}
