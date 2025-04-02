// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DatadogAgentInternalStatus defines the observed state of DatadogAgentInternal
type DatadogAgentInternalStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// +kubebuilder:resource:path=datadogagentinternals,shortName=ddai
// +k8s:openapi-gen=true
// DatadogAgentInternal is the Schema for the datadogagentinternals API
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
