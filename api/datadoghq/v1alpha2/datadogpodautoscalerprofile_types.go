// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
)

// DatadogPodAutoscalerTemplate contains the autoscaling behavior configuration
// that can be shared across multiple autoscalers via DatadogPodAutoscalerProfile.
type DatadogPodAutoscalerTemplate struct {
	// ApplyPolicy defines how recommendations should be applied.
	// +optional
	// +kubebuilder:default={}
	ApplyPolicy *DatadogPodAutoscalerApplyPolicy `json:"applyPolicy,omitempty"`

	// Objectives are the objectives to reach and maintain for the target resource.
	// Default to a single objective to maintain 80% POD CPU utilization.
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	Objectives []common.DatadogPodAutoscalerObjective `json:"objectives,omitempty"`

	// Fallback defines how recommendations should be applied when in fallback mode.
	// +optional
	// +kubebuilder:default={}
	Fallback *DatadogFallbackPolicy `json:"fallback,omitempty"`

	// Constraints defines constraints that should always be respected.
	Constraints *common.DatadogPodAutoscalerConstraints `json:"constraints,omitempty"`

	// Options defines optional behavior modifications for the autoscaler.
	// +optional
	Options *common.DatadogPodAutoscalerOptions `json:"options,omitempty"`
}

// DatadogPodAutoscalerProfileSpec defines the desired state of DatadogPodAutoscalerProfile.
type DatadogPodAutoscalerProfileSpec struct {
	// Template contains the autoscaling behavior configuration to apply to managed DatadogPodAutoscalers.
	Template DatadogPodAutoscalerTemplate `json:"template"`
}

// DatadogPodAutoscalerProfileConditionType represents a condition type for DatadogPodAutoscalerProfile.
type DatadogPodAutoscalerProfileConditionType string

const (
	// DatadogPodAutoscalerProfilValidCondition indicates whether the profile is valid and can be used to autoscale.
	DatadogPodAutoscalerProfileValidCondition DatadogPodAutoscalerProfileConditionType = "Valid"
)

// DatadogPodAutoscalerProfileStatus defines the observed state of DatadogPodAutoscalerProfile.
type DatadogPodAutoscalerProfileStatus struct {
	// Conditions represents the latest available observations of the profile's current state.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// TemplateHash is the stored hash of the DatadogPodAutoscalerProfile template.
	// +optional
	TemplateHash string `json:"templateHash,omitempty"`

	// ControlledAutoscalers is the number of DatadogPodAutoscaler objects managed by this profile.
	// +optional
	ControlledAutoscalers int32 `json:"controlledAutoscalers"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=dpacp,scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Valid",type="string",JSONPath=".status.conditions[?(@.type=='Valid')].status"
// +kubebuilder:printcolumn:name="Controlled Autoscalers",type="integer",JSONPath=".status.controlledAutoscalers"
// +kubebuilder:printcolumn:name="Apply Mode",type="string",JSONPath=".spec.template.applyPolicy.mode"
// +kubebuilder:printcolumn:name="Min Replicas",type="integer",JSONPath=".spec.template.constraints.minReplicas"
// +kubebuilder:printcolumn:name="Max Replicas",type="integer",JSONPath=".spec.template.constraints.maxReplicas"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// DatadogPodAutoscalerClusterProfile is the Schema for the datadogpodautoscalerclusterprofiles API
type DatadogPodAutoscalerClusterProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatadogPodAutoscalerProfileSpec   `json:"spec,omitempty"`
	Status DatadogPodAutoscalerProfileStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// DatadogPodAutoscalerClusterProfileList contains a list of DatadogPodAutoscalerClusterProfiles
type DatadogPodAutoscalerClusterProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatadogPodAutoscalerClusterProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatadogPodAutoscalerClusterProfile{}, &DatadogPodAutoscalerClusterProfileList{})
}
