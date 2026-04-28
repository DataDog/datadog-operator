// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DatadogInstrumentationSpec defines the desired state of DatadogInstrumentation.
type DatadogInstrumentationSpec struct {
	// TargetRef is the reference to the workload resource to instrument.
	// +kubebuilder:validation:XValidation:rule="has(self.apiVersion)",message="targetRef.apiVersion is required"
	TargetRef autoscalingv2.CrossVersionObjectReference `json:"targetRef"`

	// Config defines the Datadog instrumentation configuration to apply to the target workload.
	// +optional
	Config *DatadogInstrumentationConfig `json:"config,omitempty"`
}

// DatadogInstrumentationConfig defines workload-scoped instrumentation configuration.
type DatadogInstrumentationConfig struct {
	// Checks configures Datadog Agent Autodiscovery checks for the target workload.
	// +optional
	// +listType=atomic
	Checks []DatadogInstrumentationCheckConfig `json:"checks,omitempty"`
}

// DatadogInstrumentationCheckConfig defines an Autodiscovery check configuration.
type DatadogInstrumentationCheckConfig struct {
	// Integration is the Datadog integration name, for example redisdb.
	Integration string `json:"integration"`

	// ContainerImage identifies container image names this check applies to.
	// +optional
	// +listType=set
	ContainerImage []string `json:"containerImage,omitempty"`

	// InitConfig is the integration-specific Autodiscovery init_config payload.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	InitConfig runtime.RawExtension `json:"initConfig,omitempty"`

	// Instances contains integration-specific Autodiscovery instances payloads.
	// +optional
	// +listType=atomic
	Instances []runtime.RawExtension `json:"instances,omitempty"`

	// Logs contains log collection configuration payloads for this integration.
	// +optional
	// +listType=atomic
	Logs []DatadogInstrumentationLogConfig `json:"logs,omitempty"`
}

// DatadogInstrumentationLogConfig defines common log collection fields.
type DatadogInstrumentationLogConfig struct {
	// Source sets the log source name.
	// +optional
	Source string `json:"source,omitempty"`

	// Service sets the log service name.
	// +optional
	Service string `json:"service,omitempty"`

	// Tags sets additional tags on collected logs.
	// +optional
	// +listType=set
	Tags []string `json:"tags,omitempty"`

	// ProcessingRules contains Agent log processing rules.
	// +optional
	// +listType=atomic
	ProcessingRules []runtime.RawExtension `json:"processingRules,omitempty"`
}

// DatadogInstrumentationStatus defines the observed state of DatadogInstrumentation.
type DatadogInstrumentationStatus struct {
	// Conditions represent the latest available observations of the instrumentation handlers.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// DatadogInstrumentationConditionType is a DatadogInstrumentation condition type.
type DatadogInstrumentationConditionType string

const (
	// DatadogInstrumentationConditionChecksReady indicates whether check configuration is ready.
	DatadogInstrumentationConditionChecksReady DatadogInstrumentationConditionType = "ChecksReady"
)

// DatadogInstrumentation is the Schema for the datadoginstrumentations API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=datadoginstrumentations,scope=Namespaced,shortName=ddi
// +kubebuilder:printcolumn:name="Target Kind",type="string",JSONPath=".spec.targetRef.kind"
// +kubebuilder:printcolumn:name="Target Name",type="string",JSONPath=".spec.targetRef.name"
// +kubebuilder:printcolumn:name="Checks Ready",type="string",JSONPath=".status.conditions[?(@.type=='ChecksReady')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +k8s:openapi-gen=true
// +genclient
// +genclient:nonNamespaced=false
type DatadogInstrumentation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatadogInstrumentationSpec   `json:"spec,omitempty"`
	Status DatadogInstrumentationStatus `json:"status,omitempty"`
}

// DatadogInstrumentationList contains a list of DatadogInstrumentation.
// +kubebuilder:object:root=true
type DatadogInstrumentationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatadogInstrumentation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatadogInstrumentation{}, &DatadogInstrumentationList{})
}
