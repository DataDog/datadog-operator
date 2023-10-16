// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// DatadogAgentProfileSpec defines the desired state of DatadogAgentProfile
type DatadogAgentProfileSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of DatadogAgentProfile. Edit datadogagentprofile_types.go to remove/update
	Foo string `json:"foo,omitempty"`

	// Foo is an example field of DatadogAgentProfile. Edit datadogagentprofile_types.go to remove/update
	DAPAffinity *DAPAffinity                        `json:"dapAffinity,omitempty"`
	Config      *datadoghqv2alpha1.DatadogAgentSpec `json:"config,omitempty"`
}

type DAPAffinity struct {
	DAPNodeAffinity []corev1.NodeSelectorRequirement `json:"dapNodeAffinity,omitempty"`
}

// type DAPNodeAffinity struct {
// 	[]corev1.NodeSelectorRequirement `json:"`
// }

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
