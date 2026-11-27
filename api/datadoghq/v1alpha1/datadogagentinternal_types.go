// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// DatadogAgentInternalStatus defines the observed state of DatadogAgentInternal
// +k8s:openapi-gen=true
type DatadogAgentInternalStatus struct {
	// Conditions Represents the latest available observations of a DatadogAgent's current state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions"`
	// The actual state of the Agent as a DaemonSet.
	// +optional
	Agent *v2alpha1.DaemonSetStatus `json:"agent,omitempty"`
	// The actual state of the Cluster Agent as a deployment.
	// +optional
	ClusterAgent *v2alpha1.DeploymentStatus `json:"clusterAgent,omitempty"`
	// The actual state of the Cluster Checks Runner as a deployment.
	// +optional
	ClusterChecksRunner *v2alpha1.DeploymentStatus `json:"clusterChecksRunner,omitempty"`
	// The actual state of the OTel Agent Gateway as a deployment.
	// +optional
	OtelAgentGateway *v2alpha1.DeploymentStatus `json:"otelAgentGateway,omitempty"`
	// RemoteConfigConfiguration stores the configuration received from RemoteConfig.
	// +optional
	RemoteConfigConfiguration *v2alpha1.RemoteConfigConfiguration `json:"remoteConfigConfiguration,omitempty"`
	// ClusterChecksRunnerGroups lists the runner groups that waited for an available runner before taking their
	// checks, once they have had one.
	// +optional
	// +listType=map
	// +listMapKey=name
	ClusterChecksRunnerGroups []ClusterChecksRunnerGroupStatus `json:"clusterChecksRunnerGroups,omitempty"`
}

// ClusterChecksRunnerGroupStatus records when a runner group started taking its checks.
// +k8s:openapi-gen=true
type ClusterChecksRunnerGroupStatus struct {
	// Name is the runner group's name.
	Name string `json:"name"`
	// ActivatedAt is when the group first had an available runner. From then on the Cluster Agent sends the group its
	// checks, even while its runners are unavailable.
	ActivatedAt metav1.Time `json:"activatedAt"`
}

// DatadogAgentInternal is the Schema for the datadogagentinternals API
// +kubebuilder:resource:path=datadogagentinternals,shortName=ddai
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="agent",type="string",JSONPath=".status.agent.status"
// +kubebuilder:printcolumn:name="cluster-agent",type="string",JSONPath=".status.clusterAgent.status"
// +kubebuilder:printcolumn:name="cluster-checks-runner",type="string",JSONPath=".status.clusterChecksRunner.status"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
// +k8s:openapi-gen=true
type DatadogAgentInternal struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   v2alpha1.DatadogAgentSpec  `json:"spec,omitempty"`
	Status DatadogAgentInternalStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DatadogAgentInternalList contains a list of DatadogAgentInternal
type DatadogAgentInternalList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatadogAgentInternal `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatadogAgentInternal{}, &DatadogAgentInternalList{})
}
