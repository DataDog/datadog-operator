// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DatadogInstrumentationSpec defines the desired state of DatadogInstrumentation.
type DatadogInstrumentationSpec struct {
	// TargetRef is the reference to the workload resource to instrument.
	TargetRef autoscalingv2.CrossVersionObjectReference `json:"targetRef"`

	// Config defines the Datadog instrumentation configuration to apply to the target workload.
	Config DatadogInstrumentationConfig `json:"config"`
}

// DatadogInstrumentationConfig defines workload-scoped instrumentation configuration.
type DatadogInstrumentationConfig struct {
	// Checks configures Datadog Agent Autodiscovery checks for the target workload.
	// +optional
	// +listType=atomic
	Checks []DatadogInstrumentationCheckConfig `json:"checks,omitempty"`

	// APM configures the APM product through Single Step Instrumentation for the target workload.
	// +optional
	APM *DatadogInstrumentationAPMConfig `json:"apm,omitempty"`
}

// DatadogInstrumentationAPMConfig defines workload-scoped APM configuration.
type DatadogInstrumentationAPMConfig struct {
	// Enabled turns on APM via Single Step Instrumentation to automatically install the Datadog SDKs for supported
	// languages with no additional configuration required.
	Enabled bool `json:"enabled,omitempty"`
	// TracerVersions is a map of SDK versions to install for target workload. The key is the language name and the
	// value is the version to use. If omitted, all default supported SDKs will be added to the application runtime.
	// +optional
	TracerVersions map[string]string `json:"ddTraceVersions,omitempty"`
	// TracerConfigs is a list of configuration options to use for the installed SDKs. These options will be added
	// as environment variables to the workload in addition to the configured SDKs.
	// +optional
	// +listType=map
	// +listMapKey=name
	TracerConfigs []corev1.EnvVar `json:"ddTraceConfigs,omitempty"`
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

// DatadogInstrumentationLogConfig defines Agent log collection configuration fields.
// +kubebuilder:pruning:PreserveUnknownFields
type DatadogInstrumentationLogConfig struct {
	// Type is the type of log input source. Common values include tcp, udp, file, windows_event, docker, and journald.
	// +optional
	Type string `json:"type,omitempty"`

	// Port is the port for listening to logs when type is tcp or udp.
	// +optional
	Port *int32 `json:"port,omitempty"`

	// Path is the file path for gathering logs when type is file or journald.
	// +optional
	Path string `json:"path,omitempty"`

	// ChannelPath is the Windows event channel path when type is windows_event.
	// +optional
	ChannelPath string `json:"channel_path,omitempty"`

	// Service sets the log service name.
	// +optional
	Service string `json:"service,omitempty"`

	// Source sets the log source name.
	// +optional
	Source string `json:"source,omitempty"`

	// IncludeUnits lists journald units to include when type is journald.
	// +optional
	// +listType=set
	IncludeUnits []string `json:"include_units,omitempty"`

	// ExcludePaths lists matching files to exclude when type is file and path contains a wildcard.
	// +optional
	// +listType=set
	ExcludePaths []string `json:"exclude_paths,omitempty"`

	// ExcludeUnits lists journald units to exclude when type is journald.
	// +optional
	// +listType=set
	ExcludeUnits []string `json:"exclude_units,omitempty"`

	// SourceCategory sets the source category attribute.
	// +optional
	SourceCategory string `json:"sourcecategory,omitempty"`

	// StartPosition sets where the Agent starts reading for file and journald tailers.
	// Common values include beginning, end, forceBeginning, and forceEnd.
	// +optional
	StartPosition string `json:"start_position,omitempty"`

	// Encoding sets the file encoding when type is file.
	// Common values include utf-16-le, utf-16-be, and shift-jis.
	// +optional
	Encoding string `json:"encoding,omitempty"`

	// Tags sets additional tags on collected logs.
	// +optional
	// +listType=set
	Tags []string `json:"tags,omitempty"`

	// LogProcessingRules contains Agent log processing rules for this log source.
	// +optional
	// +listType=atomic
	LogProcessingRules []runtime.RawExtension `json:"log_processing_rules,omitempty"`
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
