// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DatadogMonitorSpec defines the desired state of DatadogMonitor
type DatadogMonitorSpec struct {
	// Name is the monitor name
	Name string `json:"name,omitempty"`
	// Message is a message to include with notifications for this monitor
	Message string `json:"message,omitempty"`
	// Priority is an integer from 1 (high) to 5 (low) indicating alert severity
	Priority int64 `json:"priority,omitempty"`
	// Query is the Datadog monitor query
	Query string `json:"query,omitempty"`
	// Tags is the monitor tags associated with your monitor
	Tags []string `json:"tags,omitempty"`
	// Type is the monitor type
	Type DatadogMonitorType `json:"type,omitempty"`
	// Options are the optional parameters associated with your monitor
	Options DatadogMonitorOptions `json:"options,omitempty"`
}

// DatadogMonitorType defines the type of monitor
type DatadogMonitorType string

const (
	// DatadogMonitorTypeMetric is the metric alert monitor type
	DatadogMonitorTypeMetric DatadogMonitorType = "metric alert"
	// DatadogMonitorTypeQuery is the query alert monitor type
	DatadogMonitorTypeQuery DatadogMonitorType = "query alert"
	// DatadogMonitorTypeService is the service check monitor type
	DatadogMonitorTypeService DatadogMonitorType = "service check"
	// DatadogMonitorTypeEvent is the event alert monitor type
	DatadogMonitorTypeEvent DatadogMonitorType = "event alert"
	// DatadogMonitorTypeLog is the log alert monitor type
	DatadogMonitorTypeLog DatadogMonitorType = "log alert"
	// DatadogMonitorTypeProcess is the process alert monitor type
	DatadogMonitorTypeProcess DatadogMonitorType = "process alert"
	// DatadogMonitorTypeRUM is the rum alert monitor type
	DatadogMonitorTypeRUM DatadogMonitorType = "rum alert"
	// DatadogMonitorTypeTraceAnalytics is the trace-analytics alert monitor type
	DatadogMonitorTypeTraceAnalytics DatadogMonitorType = "trace-analytics alert"
	// DatadogMonitorTypeSLO is the slo alert monitor type
	DatadogMonitorTypeSLO DatadogMonitorType = "slo alert"
	// DatadogMonitorTypeEventV2 is the event-v2 alert monitor type
	DatadogMonitorTypeEventV2 DatadogMonitorType = "event-v2 alert"
	// DatadogMonitorTypeAudit is the audit alert monitor type
	DatadogMonitorTypeAudit DatadogMonitorType = "audit alert"
	// DatadogMonitorTypeComposite is the composite alert monitor type
	DatadogMonitorTypeComposite DatadogMonitorType = "composite"
)

// DatadogMonitorOptions define the optional parameters of a monitor
type DatadogMonitorOptions struct {
	// A message to include with a re-notification.
	EscalationMessage *string `json:"escalationMessage,omitempty"`
	// Time (in seconds) to delay evaluation, as a non-negative integer. For example, if the value is set to 300 (5min),
	// the timeframe is set to last_5m and the time is 7:00, the monitor evaluates data from 6:50 to 6:55.
	// This is useful for AWS CloudWatch and other backfilled metrics to ensure the monitor always has data during evaluation.
	EvaluationDelay *int64 `json:"evaluationDelay,omitempty"`
	// A Boolean indicating whether notifications from this monitor automatically inserts its triggering tags into the title.
	IncludeTags *bool `json:"includeTags,omitempty"`
	// Whether or not the monitor is locked (only editable by creator and admins).
	Locked *bool `json:"locked,omitempty"`
	// Time (in seconds) to allow a host to boot and applications to fully start before starting the evaluation of
	// monitor results. Should be a non negative integer.
	NewHostDelay *int64 `json:"newHostDelay,omitempty"`
	// The number of minutes before a monitor notifies after data stops reporting. Datadog recommends at least 2x the
	// monitor timeframe for metric alerts or 2 minutes for service checks. If omitted, 2x the evaluation timeframe
	// is used for metric alerts, and 24 hours is used for service checks.
	NoDataTimeframe *int64 `json:"noDataTimeframe,omitempty"`
	// A Boolean indicating whether tagged users are notified on changes to this monitor.
	NotifyAudit *bool `json:"notifyAudit,omitempty"`
	// A Boolean indicating whether this monitor notifies when data stops reporting.
	NotifyNoData *bool `json:"notifyNoData,omitempty"`
	// The number of minutes after the last notification before a monitor re-notifies on the current status.
	// It only re-notifies if it’s not resolved.
	RenotifyInterval *int64 `json:"renotifyInterval,omitempty"`
	// A Boolean indicating whether this monitor needs a full window of data before it’s evaluated. We highly
	// recommend you set this to false for sparse metrics, otherwise some evaluations are skipped. Default is false.
	RequireFullWindow *bool `json:"requireFullWindow,omitempty"`
	// The number of hours of the monitor not reporting data before it automatically resolves from a triggered state.
	TimeoutH *int64 `json:"timeoutH,omitempty"`
	// A struct of the different monitor threshold values.
	Thresholds *DatadogMonitorOptionsThresholds `json:"thresholds,omitempty"`
	// A struct of the alerting time window options.
	ThresholdWindows *DatadogMonitorOptionsThresholdWindows `json:"thresholdWindows,omitempty"`
}

// DatadogMonitorOptionsThresholds is a struct of the different monitor threshold values
type DatadogMonitorOptionsThresholds struct {
	// The monitor CRITICAL threshold.
	Critical *string `json:"critical,omitempty"`
	// The monitor CRITICAL recovery threshold.
	CriticalRecovery *string `json:"criticalRecovery,omitempty"`
	// The monitor OK threshold.
	OK *string `json:"ok,omitempty"`
	// The monitor UNKNOWN threshold.
	Unknown *string `json:"unknown,omitempty"`
	// The monitor WARNING threshold.
	Warning *string `json:"warning,omitempty"`
	// The monitor WARNING recovery threshold.
	WarningRecovery *string `json:"warningRecovery,omitempty"`
}

// DatadogMonitorOptionsThresholdWindows is a struct of the alerting time window options
type DatadogMonitorOptionsThresholdWindows struct {
	// Describes how long an anomalous metric must be normal before the alert recovers.
	RecoveryWindow *string `json:"recoveryWindow,omitempty"`
	// Describes how long a metric must be anomalous before an alert triggers.
	TriggerWindow *string `json:"triggerWindow,omitempty"`
}

// DatadogMonitorStatus defines the observed state of DatadogMonitor
type DatadogMonitorStatus struct {
	// Conditions Represents the latest available observations of a DatadogMonitor's current state.
	// +listType=map
	// +listMapKey=type
	Conditions []DatadogMonitorCondition `json:"conditions,omitempty"`

	// ID is the monitor ID generated in Datadog
	ID int `json:"id,omitempty"`
	// Creator is the identify of the monitor creator
	Creator string `json:"creator,omitempty"`
	// Created is the time the monitor was created
	Created *metav1.Time `json:"created,omitempty"`
	// MonitorState is the overall state of monitor
	MonitorState DatadogMonitorState `json:"monitorState,omitempty"`
	// MonitorStateLastUpdateTime is the last time the monitor state updated
	MonitorStateLastUpdateTime *metav1.Time `json:"monitorStateLastUpdateTime,omitempty"`
	// MonitorStateLastTransitionTime is the last time the monitor state changed
	MonitorStateLastTransitionTime *metav1.Time `json:"monitorStateLastTransitionTime,omitempty"`
	// SyncStatus shows the health of syncing the monitor state to Datadog
	SyncStatus SyncStatusMessage `json:"syncStatus,omitempty"`
	// TriggeredState only includes details for monitor groups that are triggering
	TriggeredState []DatadogMonitorTriggeredState `json:"triggeredState,omitempty"`
	// DowntimeStatus defines whether the monitor is downtimed
	DowntimeStatus DatadogMonitorDowntimeStatus `json:"downtimeStatus,omitempty"`

	// Primary defines whether the monitor is managed by the Kubernetes custom
	// resource (true) or outside Kubernetes (false)
	Primary bool `json:"primary,omitempty"`

	// CurrentHash tracks the hash of the current DatadogMonitorSpec to know
	// if the Spec has changed and needs an update
	CurrentHash string `json:"currentHash,omitempty"`
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
	// DatadogMonitorConditionTypeActive means the DatadogMonitor is active
	DatadogMonitorConditionTypeActive DatadogMonitorConditionType = "Active"
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

// SyncStatusMessage is the message reflecting the health of monitor state syncs to Datadog
type SyncStatusMessage string

const (
	// SyncStatusOK means syncing is OK
	SyncStatusOK SyncStatusMessage = "OK"
	// SyncStatusValidateError means there is a monitor validation error
	SyncStatusValidateError SyncStatusMessage = "error validating monitor"
	// SyncStatusUpdateError means there is a monitor update error
	SyncStatusUpdateError SyncStatusMessage = "error updating monitor"
	// SyncStatusGetError means there is an error getting the monitor
	SyncStatusGetError SyncStatusMessage = "error getting monitor"
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

// DatadogMonitor allows to define and manage Monitors from your Kubernetes Cluster
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=datadogmonitors,scope=Namespaced
// +kubebuilder:printcolumn:name="id",type="string",JSONPath=".status.id"
// +kubebuilder:printcolumn:name="monitor state",type="string",JSONPath=".status.monitorState"
// +kubebuilder:printcolumn:name="last transition",type="string",JSONPath=".status.monitorStateLastTransitionTime"
// +kubebuilder:printcolumn:name="last sync",type="string",format="date",JSONPath=".status.monitorStateLastUpdateTime"
// +kubebuilder:printcolumn:name="sync status",type="string",JSONPath=".status.syncStatus"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
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
