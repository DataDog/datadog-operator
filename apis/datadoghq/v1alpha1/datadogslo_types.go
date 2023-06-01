// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2023 Datadog, Inc.

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DatadogSLOSpec struct {
	// Name of Datadog service level objective
	Name string `json:"name"`

	// Description of this service level objective.
	Description *string `json:"description,omitempty"`

	// Groups A list of (up to 100) monitor groups that narrow the scope of a monitor service level objective.
	// Included in service level objective responses if it is not empty.
	// Optional in create/update requests for monitor service level objectives, but may only be used when then length of the monitor_ids field is one.
	Groups []string `json:"groups,omitempty"`

	// MonitorIDs a list of monitor IDs that defines the scope of a monitor service level objective. Required if type is monitor.
	MonitorIDs []int64 `json:"monitor_ids,omitempty"`

	// Tags A list of tags to associate with your service level objective.
	// This can help you categorize and filter service level objectives in the service level objectives page of the UI.
	// Note: it's not currently possible to filter by these tags when querying via the API
	Tags []string `json:"tags,omitempty"`

	// Query The metric query of good / total events
	Query *DatadogSLOQuery `json:"query"`

	// Thresholds A list of thresholds and targets that define the service level objectives from the provided SLIs.
	Thresholds []DatadogSLOThreshold `json:"thresholds"`

	// Type the slo allowed values: monitor | metric
	Type DatadogSLOType `json:"type"`
}

type DatadogSLOQuery struct {
	// Numerator the sum of all the good events.
	Numerator string `json:"numerator"`
	// Denominator the sum of the total events.
	Denominator string `json:"denominator"`
}

type DatadogSLOThreshold struct {
	// The target value for the service level indicator within the corresponding timeframe.
	Target resource.Quantity `json:"target"`

	// TargetDisplay A string representation of the target that indicates its precision. It uses trailing zeros to show significant decimal places (for example 98.00).
	// Always included in service level objective responses. Ignored in create/update requests.
	// +optional
	TargetDisplay string `json:"target_display"`

	// Timeframe The SLO time window options. Allowed enum values: 7d,30d,90d,custom
	Timeframe DatadogSLOTimeFrame `json:"timeframe"`

	// The warning value for the service level objective.
	// +optional
	Warning *resource.Quantity `json:"warning"`

	// A string representation of the warning target (see the description of the target_display field for details).
	// Included in service level objective responses if a warning target exists. Ignored in create/update requests.
	// +optional
	WarningDisplay string `json:"warning_display"`
}

type DatadogSLOTimeFrame string

type DatadogSLOType string

func (t DatadogSLOType) IsValid() bool {
	switch t {
	case DatadogSLOTypeMetric, DatadogSLOTypeMonitor:
		return true
	default:
		return false
	}
}

const (
	DatadogSLOTypeMetric  DatadogSLOType = "metric"
	DatadogSLOTypeMonitor DatadogSLOType = "monitor"

	DatadogSLOTimeFrame7d     DatadogSLOTimeFrame = "7d"
	DatadogSLOTimeFrame30d    DatadogSLOTimeFrame = "30d"
	DatadogSLOTimeFrame90d    DatadogSLOTimeFrame = "90d"
	DatadogSLOTimeFrameCustom DatadogSLOTimeFrame = "custom"
)

// DatadogSLOStatus defines the observed state of DatadogSLO
type DatadogSLOStatus struct {
	// Conditions Represents the latest available observations of a DatadogSLOs current state.
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ID is the SLO ID generated in Datadog
	ID string `json:"id,omitempty"`

	// Creator is the identity of the SLO creator
	Creator string `json:"creator,omitempty"`

	// Created is the time the SLO was created
	Created *metav1.Time `json:"created,omitempty"`

	// SyncStatus shows the health of syncing the SLO state to Datadog
	SyncStatus DatadogSLOSyncStatus `json:"syncStatus,omitempty"`

	// ManagedByDatadogOperator defines whether the SLO is managed by the Kubernetes custom
	// resource (true) or outside Kubernetes (false)
	ManagedByDatadogOperator bool `json:"primary,omitempty"`

	// CurrentHash tracks the hash of the current DatadogMonitorSpec to know
	// if the Spec has changed and needs an update
	CurrentHash string `json:"currentHash,omitempty"`
}

// DatadogSLOSyncStatus is the message reflecting the health of SLO state syncs to Datadog
type DatadogSLOSyncStatus string

const (
	// DatadogSLOSyncStatusOK means syncing is OK
	DatadogSLOSyncStatusOK DatadogSLOSyncStatus = "OK"
	// DatadogSLOSyncStatusValidateError means there is a SLO validation error
	DatadogSLOSyncStatusValidateError DatadogSLOSyncStatus = "error validating SLO"
	// DatadogSLOSyncStatusUpdateError means there is a SLO update error
	DatadogSLOSyncStatusUpdateError DatadogSLOSyncStatus = "error updating SLO"
	// DatadogSLOSyncStatusCreateError means there is an error getting the SLO
	DatadogSLOSyncStatusCreateError DatadogSLOSyncStatus = "error creating SLO"
)

// DatadogSLO allows to define and manage datadog SLOs from kubernetes cluster
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=datadogslos,scope=Namespaced
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

// DatadogSLOList contains a list of DatadogSLOs
// +kubebuilder:object:root=true
type DatadogSLOList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatadogSLO `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatadogSLO{}, &DatadogSLOList{})
}
