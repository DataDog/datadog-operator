// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DatadogMetricSpec defines the desired state of DatadogMetric
type DatadogMetricSpec struct {
	// Query is the raw datadog query
	Query string `json:"query,omitempty"`
	// ExternalMetricName is reversed for internal use
	ExternalMetricName string `json:"externalMetricName,omitempty"`
}

// DatadogMetricStatus defines the observed state of DatadogMetric
type DatadogMetricStatus struct {
	// Conditions Represents the latest available observations of a DatadogMetric's current state.
	// +listType=set
	Conditions []DatadogMetricCondition `json:"conditions,omitempty"`
	// Value is the latest value of the metric
	Value float64 `json:"currentValue"`
}

// DatadogMetricCondition describes the state of a DatadogMetric at a certain point.
// +k8s:openapi-gen=true
type DatadogMetricCondition struct {
	// Type of DatadogMetric condition.
	Type DatadogMetricConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// Last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// Last time the condition was updated.
	// +optional
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
	// The reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty"`
	// A human readable message indicating details about the transition.
	// +optional
	Message string `json:"message,omitempty"`
}

// DatadogMetricConditionType type use to represent a DatadogMetric condition
type DatadogMetricConditionType string

const (
	// DatadogMetricConditionTypeActive DatadogMetric is active (referenced by an HPA), Datadog will only be queried for active metrics
	DatadogMetricConditionTypeActive DatadogMetricConditionType = "Active"
	// DatadogMetricConditionTypeUpdated DatadogMetric is updated
	DatadogMetricConditionTypeUpdated DatadogMetricConditionType = "Updated"
	// DatadogMetricConditionTypeValid DatadogMetric.spec.query is invalid
	DatadogMetricConditionTypeValid DatadogMetricConditionType = "Valid"
	// DatadogMetricConditionTypeError the controller wasn't able to handle this DatadogMetric
	DatadogMetricConditionTypeError DatadogMetricConditionType = "Error"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DatadogMetric is the Schema for the datadogmetrics API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=datadogmetrics,scope=Namespaced
// +kubebuilder:printcolumn:name="active",type="string",JSONPath=".status.conditions[?(@.type=='Active')].status"
// +kubebuilder:printcolumn:name="valid",type="string",JSONPath=".status.conditions[?(@.type=='Valid')].status"
// +kubebuilder:printcolumn:name="value",type="string",JSONPath=".status.currentValue"
// +kubebuilder:printcolumn:name="update time",type="date",JSONPath=".status.conditions[?(@.type=='Updated')].lastUpdateTime"
type DatadogMetric struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatadogMetricSpec   `json:"spec,omitempty"`
	Status DatadogMetricStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DatadogMetricList contains a list of DatadogMetric
type DatadogMetricList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatadogMetric `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatadogMetric{}, &DatadogMetricList{})
}
