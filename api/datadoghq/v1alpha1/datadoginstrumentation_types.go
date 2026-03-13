// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DatadogInstrumentationSpec defines the desired state of a DatadogInstrumentation.
// +k8s:openapi-gen=true
type DatadogInstrumentationSpec struct {
	// Selector determines which pods this DatadogInstrumentation applies to.
	// At least one of matchLabels or matchAnnotations must be set.
	// When both are specified, a pod must match all criteria to be selected.
	Selector PodSelector `json:"selector"`

	// Config holds the Datadog feature configurations to apply to the selected pods.
	Config InstrumentationConfig `json:"config"`
}

// InstrumentationConfig holds the set of Datadog features to configure for the
// selected pods. Currently supports integration checks; APM instrumentation
// will be added in a future release.
// +k8s:openapi-gen=true
type InstrumentationConfig struct {
	// Checks is the list of Datadog integration checks to run on the selected pods.
	// Each entry defines one check, including what to monitor and how to connect to it.
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +listType=atomic
	Checks []CheckConfig `json:"checks,omitempty"`
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
	// Integration is the Datadog integration name (e.g. "nginx", "http_check", "redis").
	// Must correspond to a valid Datadog integration.
	Integration string `json:"integration"`

	// ContainerImage is a list of container image names (e.g. "nginx", "redis")
	// that the Datadog Agent uses to determine which container in a pod this
	// check should monitor.
	// +listType=atomic
	ContainerImage []string `json:"containerImage,omitempty"`

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

// DatadogInstrumentationStatus defines the observed state of a DatadogInstrumentation.
// +k8s:openapi-gen=true
type DatadogInstrumentationStatus struct {
	// Conditions represents the latest available observations of the state of a DatadogInstrumentation.
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// DatadogInstrumentation defines Datadog features such as integration checks and
// APM instrumentation to apply to pods that match the given selector. This enables
// monitoring without modifying pod annotations or restarting the Datadog Agent.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=datadoginstrumentations,scope=Namespaced,shortName=ddi
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
// +k8s:openapi-gen=true
// +genclient
type DatadogInstrumentation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatadogInstrumentationSpec   `json:"spec,omitempty"`
	Status DatadogInstrumentationStatus `json:"status,omitempty"`
}

// DatadogInstrumentationList contains a list of DatadogInstrumentation resources.
// +kubebuilder:object:root=true
type DatadogInstrumentationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatadogInstrumentation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatadogInstrumentation{}, &DatadogInstrumentationList{})
}
