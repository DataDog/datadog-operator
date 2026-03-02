// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
)

type DatadogPodAutoscalerProfileConditionType string

const (
	DatadogPodAutoscalerProfileReadyCondition DatadogPodAutoscalerProfileConditionType = "Ready"
)

type DatadogPodAutoscalerProfileSpec struct {
	Template common.DatadogPodAutoscalerTemplate `json:"template"`
}

type DatadogPodAutoscalerProfileStatus struct {
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	ControlledAutoscalers int32 `json:"controlledAutoscalers"`

	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=dpap
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Apply Mode",type="string",JSONPath=".spec.template.applyPolicy.mode"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Controlled Autoscalers",type="integer",JSONPath=".status.controlledAutoscalers"
// +kubebuilder:printcolumn:name="Min Replicas",type="integer",JSONPath=".spec.template.constraints.minReplicas"
// +kubebuilder:printcolumn:name="Max Replicas",type="integer",JSONPath=".spec.template.constraints.maxReplicas"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type DatadogPodAutoscalerProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatadogPodAutoscalerProfileSpec   `json:"spec,omitempty"`
	Status DatadogPodAutoscalerProfileStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

type DatadogPodAutoscalerProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatadogPodAutoscalerProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatadogPodAutoscalerProfile{}, &DatadogPodAutoscalerProfileList{})
}
