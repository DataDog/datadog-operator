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
	// Selector determines which pods this DatadogPodCheck applies to.
	// At least one of matchLabels or matchAnnotations must be set.
	// When both are specified, a pod must match all criteria to be selected.
	Selector PodSelector `json:"selector"`

	// Checks is the list of Datadog integration checks to run on the selected pods.
	// Each entry defines one check, including what to monitor and how to connect to it.
	// +kubebuilder:validation:MinItems=1
	// +listType=atomic
	Checks []CheckConfig `json:"checks"`
}

// PodSelector defines criteria for selecting pods by labels and annotations.
// +k8s:openapi-gen=true
type PodSelector struct {
	// MatchLabels is a map of label key-value pairs. A pod must have all
	// these labels with the exact values specified to be selected.
	// +optional
	MatchLabels map[string]string `json:"matchLabels,omitempty"`

	// MatchAnnotations is a map of annotation key-value pairs. A pod must have all
	// these annotations with the exact values specified to be selected.
	// +optional
	MatchAnnotations map[string]string `json:"matchAnnotations,omitempty"`
}

// CheckConfig defines a single Datadog integration check to run on matched pods.
// +k8s:openapi-gen=true
type CheckConfig struct {
	// Name is the Datadog integration name (e.g. "nginx", "http_check", "redis").
	// Must correspond to a valid Datadog integration.
	Name string `json:"name"`

	// ADIdentifiers is a list of container identifiers (typically image names like
	// "nginx" or "redis") that the Datadog Agent uses to determine which container
	// in a pod this check should monitor. When omitted, the check applies to the
	// pod as a whole without targeting a specific container.
	// +optional
	// +listType=atomic
	ADIdentifiers []string `json:"adIdentifiers,omitempty"`

	// InitConfig holds shared configuration that applies across all instances of
	// this check. This corresponds to the init_config section in a Datadog
	// integration YAML file. Most checks do not require this.
	// +optional
	InitConfig *apiextensionsv1.JSON `json:"initConfig,omitempty"`

	// Instances defines how the Agent connects to and collects metrics from the
	// monitored service. Each entry represents one independent check execution.
	// Template variables such as %%host%% and %%port%% can be used and will be
	// resolved at runtime to the pod's IP and exposed port.
	// +kubebuilder:validation:MinItems=1
	// +listType=atomic
	Instances []apiextensionsv1.JSON `json:"instances"`

	// Logs configures log collection from containers matched by this check.
	// When set, the Datadog Agent will tail and forward logs according to
	// the provided rules (e.g. source, service, log processing pipelines).
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

// DatadogPodCheck defines Datadog integration checks to run on pods that match
// the given selector. This enables monitoring without modifying pod annotations
// or restarting the Datadog Agent.
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
