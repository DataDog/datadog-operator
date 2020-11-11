// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DatadogMonitorSpec defines the desired state of DatadogMonitor
type DatadogMonitorSpec struct {
	// Query is the Datadog query
	Query string `json:"query,omitempty"`
	// Type is the monitor type
	Type DatadogMonitorType `json:"type,omitempty"`
	// Title is the monitor title
	Title string `json:"title,omitempty"`
	// Message is the message to include in a monitor notification
	Message string `json:"message,omitempty"`
	// Tags is the monitor tags used to organize monitors
	Tags []string `json:"tags,omitempty"`

	// Options are the optional parameters of a monitor
	Options DatadogMonitorOptions `json:"options,omitempty"`
}

// DatadogMonitorType defines the type of monitor
type DatadogMonitorType string

const (
	// DatadogMonitorTypeMetric is the metric alert monitor
	DatadogMonitorTypeMetric DatadogMonitorType = "metric"
)

// DatadogMonitorOptions define the optional parameters of a monitor
type DatadogMonitorOptions struct {
	// Time (in seconds) to delay evaluation, as a non-negative integer. For example, if the value is set to 300 (5min),
	// the timeframe is set to last_5m and the time is 7:00, the monitor evaluates data from 6:50 to 6:55.
	// This is useful for AWS CloudWatch and other backfilled metrics to ensure the monitor always has data during evaluation.
	EvaluationDelay int `json:"evaluationDelay,omitempty"`
	// Whether or not the monitor is locked (only editable by creator and admins).
	Locked bool `json:"locked,omitempty"`
	// Time (in seconds) to allow a host to boot and applications to fully start before starting the evaluation of
	// monitor results. Should be a non negative integer.
	NewHostDelay int `json:"newHostDelay,omitempty"`
	// The number of minutes before a monitor notifies after data stops reporting. Datadog recommends at least 2x the
	// monitor timeframe for metric alerts or 2 minutes for service checks. If omitted, 2x the evaluation timeframe
	// is used for metric alerts, and 24 hours is used for service checks.
	NoDataTimeframe int `json:"noDataTimeframe,omitempty"`
	// A Boolean indicating whether this monitor notifies when data stops reporting.
	NotifyNoData bool `json:"notifyNoData,omitempty"`
	// The number of minutes after the last notification before a monitor re-notifies on the current status.
	// It only re-notifies if it’s not resolved.
	RenotifyInterval int `json:"renotifyInterval,omitempty"`
	// A Boolean indicating whether this monitor needs a full window of data before it’s evaluated. We highly
	// recommend you set this to false for sparse metrics, otherwise some evaluations are skipped. Default is false.
	RequireFullWindow bool `json:"requireFullWindow,omitempty"`
}

// DatadogMonitorStatus defines the observed state of DatadogMonitor
type DatadogMonitorStatus struct {
	// Conditions Represents the latest available observations of a DatadogMonitor's current state.
	// +listType=map
	// +listMapKey=type
	Conditions []DatadogMonitorCondition `json:"conditions,omitempty"`

	// ID is the monitor ID generated in Datadog
	ID int `json:"id,omitempty"`
	// MonitorState is the overall state of monitor
	MonitorState DatadogMonitorState `json:"monitorState,omitempty"`
	// TriggeredState only includes details for monitor groups that are triggering
	TriggeredState []DatadogMonitorTriggeredState `json:"triggeredState,omitempty"`
	// DowntimeStatus defines whether the monitor is downtimed
	DowntimeStatus DatadogMonitorDowntimeStatus `json:"downtimeStatus,omitempty"`
	// Creator is the identify of the monitor creator
	Creator string `json:"creator,omitempty"`
	// Created is the time the monitor was created
	Created metav1.Time `json:"created,omitempty"`
}

// DatadogMonitorCondition describes the current state of a DatadogMonitor
// +k8s:openapi-gen=true
type DatadogMonitorCondition struct {
	// Type of DatadogMonitor condition
	Type DatadogMonitorConditionType `json:"type"`
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

// DatadogMonitorConditionType represents a DatadogMonitor condition
type DatadogMonitorConditionType string

const (
	// DatadogMonitorConditionTypeCreated means the DatadogMonitor is created successfully
	DatadogMonitorConditionTypeCreated DatadogMonitorConditionType = "Created"
	// DatadogMonitorConditionTypePending means the DatadogMonitor is pending
	DatadogMonitorConditionTypePending DatadogMonitorConditionType = "Pending"
	// DatadogMonitorConditionTypeUpdated means the DatadogMonitor is updated
	DatadogMonitorConditionTypeUpdated DatadogMonitorConditionType = "Updated"
	// DatadogMonitorConditionTypeError means the DatadogMonitor has an error
	DatadogMonitorConditionTypeError DatadogMonitorConditionType = "Error"
)

// DatadogMonitorState represents the overall DatadogMonitor state
type DatadogMonitorState string

const (
	// DatadogMonitorStateOK means the DatadogMonitor is OK
	DatadogMonitorStateOK DatadogMonitorState = "OK"
	// DatadogMonitorStateAlert means the DatadogMonitor triggered an alert
	DatadogMonitorStateAlert DatadogMonitorState = "Alert"
	// DatadogMonitorStateWarn means the DatadogMonitor triggered a warning
	DatadogMonitorStateWarn DatadogMonitorState = "Warn"
	// DatadogMonitorStateNoData means the DatadogMonitor triggered a no data alert
	DatadogMonitorStateNoData DatadogMonitorState = "No Data"
	// DatadogMonitorStateSkipped means the DatadogMonitor is skipped
	DatadogMonitorStateSkipped DatadogMonitorState = "Skipped"
	// DatadogMonitorStateIgnored means the DatadogMonitor is ignored
	DatadogMonitorStateIgnored DatadogMonitorState = "Ignored"
	// DatadogMonitorStateUnknown means the DatadogMonitor is in an unknown state
	DatadogMonitorStateUnknown DatadogMonitorState = "Unknown"
)

// DatadogMonitorTriggeredState represents the details of a triggering DatadogMonitor
// The DatadogMonitor is triggering if one of its groups is in Alert, Warn, or No Data
type DatadogMonitorTriggeredState struct {
	// MonitorGroup is the name of the triggering group
	MonitorGroup       string              `json:"monitorGroup,omitempty"`
	State              DatadogMonitorState `json:"state,omitempty"`
	LastTransitionTime metav1.Time         `json:"lastTransitionTime,omitempty"`
}

// DatadogMonitorDowntimeStatus represents the downtime status of a DatadogMonitor
type DatadogMonitorDowntimeStatus struct {
	IsDowntimed bool `json:"isDowntimed,omitempty"`
	DowntimeID  int  `json:"downtimeId,omitempty"`
}

// DatadogMonitor is the Schema for the datadogmonitor API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=datadogmonitors,scope=Namespaced
// +kubebuilder:printcolumn:name="id",type="string",JSONPath=".status.id"
// +kubebuilder:printcolumn:name="created",type="string",JSONPath=".status.conditions[?(@.type=='Created')].status"
// +kubebuilder:printcolumn:name="monitor state",type="string",JSONPath=".status.monitorState"
// +kubebuilder:printcolumn:name="last updated",type="date",JSONPath=".status.conditions[?(@.type=='Updated')].lastUpdateTime"
// +k8s:openapi-gen=true
// +genclient
type DatadogMonitor struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatadogMonitorSpec   `json:"spec,omitempty"`
	Status DatadogMonitorStatus `json:"status,omitempty"`
}

// DatadogMonitorList contains a list of DatadogMonitors
// +kubebuilder:object:root=true
type DatadogMonitorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatadogMonitor `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatadogMonitor{}, &DatadogMonitorList{})
}
