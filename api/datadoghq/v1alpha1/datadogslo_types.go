// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:openapi-gen=true
type DatadogSLOSpec struct {
	// Name is the name of the service level objective.
	Name string `json:"name"`

	// Description is a user-defined description of the service level objective.
	// Always included in service level objective responses (but may be null). Optional in create/update requests.
	Description *string `json:"description,omitempty"`

	// Groups is a list of (up to 100) monitor groups that narrow the scope of a monitor service level objective.
	// Included in service level objective responses if it is not empty.
	// Optional in create/update requests for monitor service level objectives, but may only be used when the length of the monitor_ids field is one.
	// +listType=set
	Groups []string `json:"groups,omitempty"`

	// MonitorIDs is a list of monitor IDs that defines the scope of a monitor service level objective. Required if type is monitor.
	// +listType=set
	MonitorIDs []int64 `json:"monitorIDs,omitempty"`

	// Tags is a list of tags to associate with your service level objective.
	// This can help you categorize and filter service level objectives in the service level objectives page of the UI.
	// Note: it's not currently possible to filter by these tags when querying via the API.
	// +listType=set
	Tags []string `json:"tags,omitempty"`

	// Query is the query for a metric-based SLO. Required if type is metric.
	// Note that only the `sum by` aggregator is allowed, which sums all request counts. `Average`, `max`, nor `min` request aggregators are not supported.
	Query *DatadogSLOQuery `json:"query,omitempty"`

	// Type is the type of the service level objective.
	Type DatadogSLOType `json:"type"`

	// The SLO time window options.
	Timeframe DatadogSLOTimeFrame `json:"timeframe"`

	// TargetThreshold is the target threshold such that when the service level indicator is above this threshold over the given timeframe, the objective is being met.
	TargetThreshold resource.Quantity `json:"targetThreshold"`

	// WarningThreshold is a optional warning threshold such that when the service level indicator is below this value for the given threshold, but above the target threshold, the objective appears in a "warning" state. This value must be greater than the target threshold.
	WarningThreshold *resource.Quantity `json:"warningThreshold,omitempty"`

	// ControllerOptions are the optional parameters in the DatadogSLO controller
	ControllerOptions *DatadogSLOControllerOptions `json:"controllerOptions,omitempty"`
}

// +k8s:openapi-gen=true
type DatadogSLOQuery struct {
	// Numerator is a Datadog metric query for good events.
	Numerator string `json:"numerator"`
	// Denominator is a Datadog metric query for total (valid) events.
	Denominator string `json:"denominator"`
}

type DatadogSLOType string

const (
	DatadogSLOTypeMetric  DatadogSLOType = "metric"
	DatadogSLOTypeMonitor DatadogSLOType = "monitor"
)

func (t DatadogSLOType) IsValid() bool {
	switch t {
	case DatadogSLOTypeMetric, DatadogSLOTypeMonitor:
		return true
	default:
		return false
	}
}

type DatadogSLOTimeFrame string

const (
	DatadogSLOTimeFrame7d  DatadogSLOTimeFrame = "7d"
	DatadogSLOTimeFrame30d DatadogSLOTimeFrame = "30d"
	DatadogSLOTimeFrame90d DatadogSLOTimeFrame = "90d"
)

// DatadogSLOControllerOptions defines options in the DatadogSLO controller.
// +k8s:openapi-gen=true
type DatadogSLOControllerOptions struct {
	// DisableRequiredTags disables the automatic addition of required tags to SLOs.
	DisableRequiredTags *bool `json:"disableRequiredTags,omitempty"`
}

// DatadogSLOStatus defines the observed state of a DatadogSLO.
// +k8s:openapi-gen=true
type DatadogSLOStatus struct {
	// Conditions represents the latest available observations of the state of a DatadogSLO.
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ID is the SLO ID generated in Datadog.
	ID string `json:"id,omitempty"`

	// Creator is the identity of the SLO creator.
	Creator string `json:"creator,omitempty"`

	// Created is the time the SLO was created.
	Created *metav1.Time `json:"created,omitempty"`

	// SyncStatus shows the health of syncing the SLO state to Datadog.
	SyncStatus DatadogSLOSyncStatus `json:"syncStatus,omitempty"`

	// LastForceSyncTime is the last time the API SLO was last force synced with the DatadogSLO resource.
	LastForceSyncTime *metav1.Time `json:"lastForceSyncTime,omitempty"`

	// CurrentHash tracks the hash of the current DatadogSLOSpec to know
	// if the Spec has changed and needs an update.
	CurrentHash string `json:"currentHash,omitempty"`
}

// DatadogSLOSyncStatus is the message reflecting the health of SLO state syncs to Datadog.
type DatadogSLOSyncStatus string

const (
	// DatadogSLOSyncStatusOK means syncing is OK.
	DatadogSLOSyncStatusOK DatadogSLOSyncStatus = "OK"
	// DatadogSLOSyncStatusValidateError means there is a SLO validation error.
	DatadogSLOSyncStatusValidateError DatadogSLOSyncStatus = "error validating SLO"
	// DatadogSLOSyncStatusUpdateError means there is a SLO update error.
	DatadogSLOSyncStatusUpdateError DatadogSLOSyncStatus = "error updating SLO"
	// DatadogSLOSyncStatusCreateError means there is an error getting the SLO.
	DatadogSLOSyncStatusCreateError DatadogSLOSyncStatus = "error creating SLO"
)

// DatadogSLO allows a user to define and manage datadog SLOs from Kubernetes cluster.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=datadogslos,scope=Namespaced,shortName=ddslo
// +kubebuilder:printcolumn:name="id",type="string",JSONPath=".status.id"
// +kubebuilder:printcolumn:name="sync status",type="string",JSONPath=".status.syncStatus"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
// +k8s:openapi-gen=true
// +genclient
type DatadogSLO struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatadogSLOSpec   `json:"spec,omitempty"`
	Status DatadogSLOStatus `json:"status,omitempty"`
}

// DatadogSLOList contains a list of DatadogSLOs.
// +kubebuilder:object:root=true
type DatadogSLOList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatadogSLO `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatadogSLO{}, &DatadogSLOList{})
}
