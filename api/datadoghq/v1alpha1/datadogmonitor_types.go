// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DatadogMonitorSpec defines the desired state of DatadogMonitor
// +k8s:openapi-gen=true
type DatadogMonitorSpec struct {
	// Name is the monitor name
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`
	// Message is a message to include with notifications for this monitor
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Message string `json:"message,omitempty"`
	// Priority is an integer from 1 (high) to 5 (low) indicating alert severity
	Priority int64 `json:"priority,omitempty"`
	// Query is the Datadog monitor query
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Query string `json:"query,omitempty"`
	// RestrictedRoles is a list of unique role identifiers to define which roles are allowed to edit the monitor.
	// `restricted_roles` is the successor of `locked`. For more information about `locked` and `restricted_roles`,
	// see the [monitor options docs](https://docs.datadoghq.com/monitors/guide/monitor_api_options/#permissions-options).
	// +listType=set
	RestrictedRoles []string `json:"restrictedRoles,omitempty"`
	// Tags is the monitor tags associated with your monitor
	// +listType=set
	Tags []string `json:"tags,omitempty"`
	// Type is the monitor type
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=metric alert;query alert;service check;event alert;log alert;process alert;rum alert;trace-analytics alert;slo alert;event-v2 alert;audit alert;composite
	Type DatadogMonitorType `json:"type,omitempty"`
	// Options are the optional parameters associated with your monitor
	Options DatadogMonitorOptions `json:"options,omitempty"`

	// ControllerOptions are the optional parameters in the DatadogMonitor controller
	ControllerOptions DatadogMonitorControllerOptions `json:"controllerOptions,omitempty"`
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

// DatadogMonitorOptionsNotificationPreset toggles the display of additional content sent in the monitor notification.
type DatadogMonitorOptionsNotificationPreset string

const (
	DatadogMonitorOptionsNotificationPresetShowAll     DatadogMonitorOptionsNotificationPreset = "show_all"
	DatadogMonitorOptionsNotificationPresetHideQuery   DatadogMonitorOptionsNotificationPreset = "hide_query"
	DatadogMonitorOptionsNotificationPresetHideHandles DatadogMonitorOptionsNotificationPreset = "hide_handles"
	DatadogMonitorOptionsNotificationPresetHideAll     DatadogMonitorOptionsNotificationPreset = "hide_all"
)

// DatadogMonitorOptions define the optional parameters of a monitor
// +k8s:openapi-gen=true
type DatadogMonitorOptions struct {
	// A Boolean indicating whether to send a log sample when the log monitor triggers.
	EnableLogsSample *bool `json:"enableLogsSample,omitempty"`
	// A message to include with a re-notification.
	EscalationMessage *string `json:"escalationMessage,omitempty"`
	// Time (in seconds) to delay evaluation, as a non-negative integer. For example, if the value is set to 300 (5min),
	// the timeframe is set to last_5m and the time is 7:00, the monitor evaluates data from 6:50 to 6:55.
	// This is useful for AWS CloudWatch and other backfilled metrics to ensure the monitor always has data during evaluation.
	EvaluationDelay *int64 `json:"evaluationDelay,omitempty"`
	// A Boolean indicating whether notifications from this monitor automatically inserts its triggering tags into the title.
	IncludeTags *bool `json:"includeTags,omitempty"`
	// The time span after which groups with missing data are dropped from the monitor state.
	// The minimum value is one hour, and the maximum value is 72 hours.
	// Example values are: "60m", "1h", and "2d".
	// This option is only available for APM Trace Analytics, Audit Trail, CI, Error Tracking, Event, Logs, and RUM monitors.
	GroupRetentionDuration *string `json:"groupRetentionDuration,omitempty"`
	// A Boolean indicating whether the log alert monitor triggers a single alert or multiple alerts when any group breaches a threshold.
	GroupbySimpleMonitor *bool `json:"groupbySimpleMonitor,omitempty"`
	// DEPRECATED: Whether or not the monitor is locked (only editable by creator and admins). Use `restricted_roles` instead.
	// +deprecated
	Locked *bool `json:"locked,omitempty"`
	// Time (in seconds) to allow a host to boot and applications to fully start before starting the evaluation of
	// monitor results. Should be a non negative integer.
	NewGroupDelay *int64 `json:"newGroupDelay,omitempty"`
	// The number of minutes before a monitor notifies after data stops reporting. Datadog recommends at least 2x the
	// monitor timeframe for metric alerts or 2 minutes for service checks. If omitted, 2x the evaluation timeframe
	// is used for metric alerts, and 24 hours is used for service checks.
	NoDataTimeframe *int64 `json:"noDataTimeframe,omitempty"`
	// An enum that toggles the display of additional content sent in the monitor notification.
	NotificationPresetName DatadogMonitorOptionsNotificationPreset `json:"notificationPresetName,omitempty"`
	// A Boolean indicating whether tagged users are notified on changes to this monitor.
	NotifyAudit *bool `json:"notifyAudit,omitempty"`
	// A string indicating the granularity a monitor alerts on. Only available for monitors with groupings.
	// For instance, a monitor grouped by cluster, namespace, and pod can be configured to only notify on each new
	// cluster violating the alert conditions by setting notify_by to ["cluster"]. Tags mentioned in notify_by must
	// be a subset of the grouping tags in the query. For example, a query grouped by cluster and namespace cannot
	// notify on region. Setting notify_by to [*] configures the monitor to notify as a simple-alert.
	// +listType=set
	NotifyBy []string `json:"notifyBy,omitempty"`
	// A Boolean indicating whether this monitor notifies when data stops reporting.
	NotifyNoData *bool `json:"notifyNoData,omitempty"`
	// An enum that controls how groups or monitors are treated if an evaluation does not return data points.
	// The default option results in different behavior depending on the monitor query type.
	// For monitors using Count queries, an empty monitor evaluation is treated as 0 and is compared to the threshold conditions.
	// For monitors using any query type other than Count, for example Gauge, Measure, or Rate, the monitor shows the last known status.
	// This option is only available for APM Trace Analytics, Audit Trail, CI, Error Tracking, Event, Logs, and RUM monitors
	OnMissingData DatadogMonitorOptionsOnMissingData `json:"onMissingData,omitempty"`
	// The number of minutes after the last notification before a monitor re-notifies on the current status.
	// It only re-notifies if it’s not resolved.
	RenotifyInterval *int64 `json:"renotifyInterval,omitempty"`
	// The number of times re-notification messages should be sent on the current status at the provided re-notification interval.
	RenotifyOccurrences *int64 `json:"renotifyOccurrences,omitempty"`
	// The types of statuses for which re-notification messages should be sent. Valid values are alert, warn, no data.
	// +listType=set
	RenotifyStatuses []datadogV1.MonitorRenotifyStatusType `json:"renotifyStatuses,omitempty"`
	// A Boolean indicating whether this monitor needs a full window of data before it’s evaluated. We highly
	// recommend you set this to false for sparse metrics, otherwise some evaluations are skipped. Default is false.
	RequireFullWindow *bool `json:"requireFullWindow,omitempty"`
	// Configuration options for scheduling.
	SchedulingOptions *DatadogMonitorOptionsSchedulingOptions `json:"schedulingOptions,omitempty"`
	// The number of hours of the monitor not reporting data before it automatically resolves from a triggered state.
	TimeoutH *int64 `json:"timeoutH,omitempty"`
	// A struct of the different monitor threshold values.
	Thresholds *DatadogMonitorOptionsThresholds `json:"thresholds,omitempty"`
	// A struct of the alerting time window options.
	ThresholdWindows *DatadogMonitorOptionsThresholdWindows `json:"thresholdWindows,omitempty"`
}

// DatadogMonitorOptionsSchedulingOptions is a struct of the different scheduling options
// +k8s:openapi-gen=true
type DatadogMonitorOptionsSchedulingOptions struct {
	// Configuration options for the custom schedule. If start is omitted, the monitor creation time will be used.
	CustomSchedule *DatadogMonitorOptionsSchedulingOptionsCustomSchedule `json:"customSchedule,omitempty"`

	// Configuration options for the evaluation window. If hour_starts is set, no other fields may be set.
	// Otherwise, day_starts and month_starts must be set together.
	EvaluationWindow *DatadogMonitorOptionsSchedulingOptionsEvaluationWindow `json:"evaluationWindow,omitempty"`
}

// DatadogMonitorOptionsSchedulingOptionsCustomSchedule is a struct of the custom schedule options
// +k8s:openapi-gen=true
type DatadogMonitorOptionsSchedulingOptionsCustomSchedule struct {
	Recurrence DatadogMonitorOptionsSchedulingOptionsCustomScheduleRecurrence `json:"recurrence,omitempty"`
}

// DatadogMonitorOptionsSchedulingOptionsCustomScheduleRecurrence is a struct of the recurrence definition
// +k8s:openapi-gen=true
type DatadogMonitorOptionsSchedulingOptionsCustomScheduleRecurrence struct {
	// The recurrence rule in iCalendar format. For example, `FREQ=MONTHLY;BYMONTHDAY=28,29,30,31;BYSETPOS=-1`.
	Rrule *string `json:"rrule,omitempty"`
	// The timezone in `tz database` format, in which the recurrence rule is defined. For example, `America/New_York` or `UTC`.
	Timezone *string `json:"timezone,omitempty"`
	// The start date of the recurrence rule defined in `YYYY-MM-DDThh:mm:ss` format.
	// If omitted, the monitor creation time will be used.
	Start *string `json:"start,omitempty"`
}

// DatadogMonitorOptionsSchedulingOptionsEvaluationWindow is a struct of the evaluation window options
// +k8s:openapi-gen=true
type DatadogMonitorOptionsSchedulingOptionsEvaluationWindow struct {
	// The time of the day at which a one day cumulative evaluation window starts. Must be defined in UTC time in HH:mm format.
	DayStarts *string `json:"dayStarts,omitempty"`
	// The minute of the hour at which a one hour cumulative evaluation window starts.
	HourStarts *int32 `json:"hourStarts,omitempty"`
	// The day of the month at which a one month cumulative evaluation window starts.
	MonthStarts *int32 `json:"monthStarts,omitempty"`
}

// DatadogMonitorOptionsThresholds is a struct of the different monitor threshold values
// +k8s:openapi-gen=true
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
// +k8s:openapi-gen=true
type DatadogMonitorOptionsThresholdWindows struct {
	// Describes how long an anomalous metric must be normal before the alert recovers.
	RecoveryWindow *string `json:"recoveryWindow,omitempty"`
	// Describes how long a metric must be anomalous before an alert triggers.
	TriggerWindow *string `json:"triggerWindow,omitempty"`
}

// DatadogMonitorControllerOptions defines options in the DatadogMonitor controller
// +k8s:openapi-gen=true
type DatadogMonitorControllerOptions struct {
	// DisableRequiredTags disables the automatic addition of required tags to monitors.
	DisableRequiredTags *bool `json:"disableRequiredTags,omitempty"`
}

// DatadogMonitorStatus defines the observed state of DatadogMonitor
// +k8s:openapi-gen=true
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
	// MonitorLastForceSyncTime is the last time the API monitor was last force synced with the DatadogMonitor resource
	MonitorLastForceSyncTime *metav1.Time `json:"monitorLastForceSyncTime,omitempty"`
	// MonitorStateLastUpdateTime is the last time the monitor state updated
	MonitorStateLastUpdateTime *metav1.Time `json:"monitorStateLastUpdateTime,omitempty"`
	// MonitorStateLastTransitionTime is the last time the monitor state changed
	MonitorStateLastTransitionTime *metav1.Time `json:"monitorStateLastTransitionTime,omitempty"`
	// MonitorStateSyncStatus shows the health of syncing the monitor state to Datadog
	MonitorStateSyncStatus MonitorStateSyncStatusMessage `json:"monitorStateSyncStatus,omitempty"`
	// TriggeredState only includes details for monitor groups that are triggering
	// +listType=map
	// +listMapKey=monitorGroup
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
	// DatadogMonitorConditionTypeDriftDetected means drift was detected between the resource and Datadog
	DatadogMonitorConditionTypeDriftDetected DatadogMonitorConditionType = "DriftDetected"
	// DatadogMonitorConditionTypeRecreated means the DatadogMonitor was recreated due to drift
	DatadogMonitorConditionTypeRecreated DatadogMonitorConditionType = "Recreated"
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

// DatadogMonitorOptionsOnMissingData controls how groups or monitors are treated if an evaluation does not return any data points
type DatadogMonitorOptionsOnMissingData string

const (
	DatadogMonitorOptionsOnMissingDataShowNoData          DatadogMonitorOptionsOnMissingData = "show_no_data"
	DatadogMonitorOptionsOnMissingDataShowAndNotifyNoData DatadogMonitorOptionsOnMissingData = "show_and_notify_no_data"
	DatadogMonitorOptionsOnMissingDataResolve             DatadogMonitorOptionsOnMissingData = "resolve"
	DatadogMonitorOptionsOnMissingDataDefault             DatadogMonitorOptionsOnMissingData = "default"
)

// MonitorStateSyncStatusMessage is the message reflecting the health of monitor state syncs to Datadog
type MonitorStateSyncStatusMessage string

const (
	// MonitorStateSyncStatusOK means syncing is OK
	MonitorStateSyncStatusOK MonitorStateSyncStatusMessage = "OK"
	// MonitorStateSyncStatusValidateError means there is a monitor validation error
	MonitorStateSyncStatusValidateError MonitorStateSyncStatusMessage = "error validating monitor"
	// MonitorStateSyncStatusUpdateError means there is a monitor update error
	MonitorStateSyncStatusUpdateError MonitorStateSyncStatusMessage = "error updating monitor"
	// SyncStatusGetError means there is an error getting the monitor
	MonitorStateSyncStatusGetError MonitorStateSyncStatusMessage = "error getting monitor"
)

// DatadogMonitorTriggeredState represents the details of a triggering DatadogMonitor
// The DatadogMonitor is triggering if one of its groups is in Alert, Warn, or No Data
// +k8s:openapi-gen=true
type DatadogMonitorTriggeredState struct {
	// MonitorGroup is the name of the triggering group
	MonitorGroup       string              `json:"monitorGroup"`
	State              DatadogMonitorState `json:"state,omitempty"`
	LastTransitionTime metav1.Time         `json:"lastTransitionTime,omitempty"`
}

// DatadogMonitorDowntimeStatus represents the downtime status of a DatadogMonitor
// +k8s:openapi-gen=true
type DatadogMonitorDowntimeStatus struct {
	// IsDowntimed shows the downtime status of the monitor.
	IsDowntimed bool `json:"isDowntimed,omitempty"`
	// DowntimeID is the downtime ID.
	DowntimeID int `json:"downtimeID,omitempty"`
}

// DatadogMonitor allows to define and manage Monitors from your Kubernetes Cluster
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=datadogmonitors,scope=Namespaced
// +kubebuilder:printcolumn:name="id",type="string",JSONPath=".status.id"
// +kubebuilder:printcolumn:name="monitor state",type="string",JSONPath=".status.monitorState"
// +kubebuilder:printcolumn:name="last state transition",type="string",JSONPath=".status.monitorStateLastTransitionTime"
// +kubebuilder:printcolumn:name="last state sync",type="string",format="date",JSONPath=".status.monitorStateLastUpdateTime"
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
