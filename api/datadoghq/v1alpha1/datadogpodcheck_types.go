// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DatadogPodCheckSpec defines the desired state of a DatadogPodCheck.
// +k8s:openapi-gen=true
type DatadogPodCheckSpec struct {
	// Selector provides targeting criteria to narrow which pods these
	// checks apply to. At least one of matchLabels or matchAnnotations must be set.
	// When both are specified, all fields are ANDed together.
	Selector PodSelector `json:"selector"`

	// Checks is the list of integration check configurations to schedule.
	// +kubebuilder:validation:MinItems=1
	// +listType=atomic
	Checks []CheckConfig `json:"checks"`
}

// PodSelector defines criteria for selecting pods by labels and annotations.
// All specified fields are ANDed together.
// +k8s:openapi-gen=true
type PodSelector struct {
	// MatchLabels is a map of key-value pairs that must match a pod's labels.
	// +optional
	MatchLabels map[string]string `json:"matchLabels,omitempty"`

	// MatchAnnotations is a map of key-value pairs that must match a pod's annotations.
	// +optional
	MatchAnnotations map[string]string `json:"matchAnnotations,omitempty"`
}

// CheckConfig defines a Datadog integration check configuration.
// +k8s:openapi-gen=true
type CheckConfig struct {
	// Name is the Datadog integration name (e.g. "nginx", "http_check", "redis").
	Name string `json:"name"`

	// ADIdentifiers is the list of autodiscovery identifiers (e.g. container image names)
	// used for template matching against discovered containers.
	// +optional
	// +listType=atomic
	ADIdentifiers []string `json:"adIdentifiers,omitempty"`

	// InitConfig is the init_config section passed to the integration check.
	// +optional
	InitConfig *apiextensionsv1.JSON `json:"initConfig,omitempty"`

	// Instances is the list of check instance configurations.
	// At least one instance is required.
	// +kubebuilder:validation:MinItems=1
	// +listType=atomic
	Instances []apiextensionsv1.JSON `json:"instances"`

	// Logs defines optional log collection configuration for this check.
	// +optional
	Logs *apiextensionsv1.JSON `json:"logs,omitempty"`
}

// DatadogPodCheckStatus defines the observed state of a DatadogPodCheck.
// +k8s:openapi-gen=true
type DatadogPodCheckStatus struct {
	// Conditions represents the latest available observations of the state of a DatadogPodCheck.
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// DatadogPodCheck allows a user to define Datadog integration checks that are
// scheduled against pods via the autodiscovery system, without requiring pod
// annotation changes or agent restarts.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=datadogpodchecks,scope=Namespaced,shortName=ddpc
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
// +k8s:openapi-gen=true
// +genclient
type DatadogPodCheck struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatadogPodCheckSpec   `json:"spec,omitempty"`
	Status DatadogPodCheckStatus `json:"status,omitempty"`
}

// DatadogPodCheckList contains a list of DatadogPodCheck resources.
// +kubebuilder:object:root=true
type DatadogPodCheckList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatadogPodCheck `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatadogPodCheck{}, &DatadogPodCheckList{})
}
