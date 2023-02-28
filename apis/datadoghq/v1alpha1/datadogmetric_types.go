// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DatadogMetricSpec defines the desired state of DatadogMetric
type DatadogMetricSpec struct {
	// Query is the raw datadog query
	Query string `json:"query,omitempty"`
	// ExternalMetricName is reserved for internal use
	ExternalMetricName string `json:"externalMetricName,omitempty"`
	// MaxAge provides the max age for the metric query (overrides the default setting
	// `external_metrics_provider.max_age`)
	// +optional
	MaxAge metav1.Duration `json:"maxAge,omitempty"`
	// TimeWindow provides the time window for the metric query, defaults to MaxAge.
	// +optional
	TimeWindow metav1.Duration `json:"timeWindow,omitempty"`
}

// DatadogMetricStatus defines the observed state of DatadogMetric
type DatadogMetricStatus struct {
	// Conditions Represents the latest available observations of a DatadogMetric's current state.
	// +listType=map
	// +listMapKey=type
	Conditions []DatadogMetricCondition `json:"conditions,omitempty"`
	// Value is the latest value of the metric
	Value string `json:"currentValue"`
	// List of autoscalers currently using this DatadogMetric
	AutoscalerReferences string `json:"autoscalerReferences,omitempty"`
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

// DatadogMetric allows autoscaling on arbitrary Datadog query
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=datadogmetrics,scope=Namespaced
// +kubebuilder:printcolumn:name="active",type="string",JSONPath=".status.conditions[?(@.type=='Active')].status"
// +kubebuilder:printcolumn:name="valid",type="string",JSONPath=".status.conditions[?(@.type=='Valid')].status"
// +kubebuilder:printcolumn:name="value",type="string",JSONPath=".status.currentValue"
// +kubebuilder:printcolumn:name="references",type="string",JSONPath=".status.autoscalerReferences"
// +kubebuilder:printcolumn:name="update time",type="date",JSONPath=".status.conditions[?(@.type=='Updated')].lastUpdateTime"
// +k8s:openapi-gen=true
// +genclient
type DatadogMetric struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatadogMetricSpec   `json:"spec,omitempty"`
	Status DatadogMetricStatus `json:"status,omitempty"`
}

// DatadogMetricList contains a list of DatadogMetric
// +kubebuilder:object:root=true
type DatadogMetricList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatadogMetric `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatadogMetric{}, &DatadogMetricList{})
}
