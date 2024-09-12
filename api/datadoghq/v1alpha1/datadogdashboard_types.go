// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DatadogDashboardSpec defines the desired state of DatadogDashboard
// +k8s:openapi-gen=true
type DatadogDashboardSpec struct {
	// Description is the description of the dashboard.
	// +optional
	Description string `json:"description,omitempty"`
	// LayoutType is the layout type of the dashboard.
	LayoutType datadogV1.DashboardLayoutType `json:"layoutType,omitempty"`
	// NotifyList is the list of handles of users to notify when changes are made to this dashboard.
	// +listType=set
	// +optional
	NotifyList []string `json:"notifyList,omitempty"`
	// Reflowtype is the reflow type for a 'new dashboard layout' dashboard. Set this only when layout type is 'ordered'.
	// If set to 'fixed', the dashboard expects all widgets to have a layout, and if it's set to 'auto',
	// widgets should not have layouts.
	// +optional
	ReflowType *datadogV1.DashboardReflowType `json:"reflowType,omitempty"`
	// Tags is a list of team names representing ownership of a dashboard.
	// +listType=set
	// +optional
	Tags []string `json:"tags,omitempty"`
	// TemplateVariablePresets is an array of template variables saved views.
	// +listType=map
	// +listMapKey=name
	// +optional
	TemplateVariablePresets []DashboardTemplateVariablePreset `json:"templateVariablePresets,omitempty"`
	// TemplateVariables is a list of template variables for this dashboard.
	// +listType=map
	// +listMapKey=name
	// +optional
	TemplateVariables []DashboardTemplateVariable `json:"templateVariables,omitempty"`
	// Title is the title of the dashboard.
	Title string `json:"title,omitempty"`
	// Widgets is a JSON string representation of a list of Datadog API Widgets
	// +optional
	Widgets string `json:"widgets,omitempty"`
}

// DatadogDashboardStatus defines the observed state of DatadogDashboard
// +k8s:openapi-gen=true
type DatadogDashboardStatus struct {
	// Conditions represents the latest available observations of the state of a DatadogDashboard.
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// ID is the dashboard ID generated in Datadog.
	ID string `json:"id,omitempty"`
	// Creator is the identity of the dashboard creator.
	Creator string `json:"creator,omitempty"`
	// Created is the time the dashboard was created.
	Created *metav1.Time `json:"created,omitempty"`
	// SyncStatus shows the health of syncing the dashboard state to Datadog.
	SyncStatus DatadogDashboardSyncStatus `json:"syncStatus,omitempty"`
	// CurrentHash tracks the hash of the current DatadogDashboardSpec to know
	// if the Spec has changed and needs an update.
	CurrentHash string `json:"currentHash,omitempty"`
	// LastForceSyncTime is the last time the API dashboard was last force synced with the DatadogDashboard resource
	LastForceSyncTime *metav1.Time `json:"lastForceSyncTime,omitempty"`
}

type DatadogDashboardSyncStatus string

const (
	// DatadogDashboardSyncStatusOK means syncing is OK.
	DatadogDashboardSyncStatusOK DatadogDashboardSyncStatus = "OK"
	// DatadogDashboardSyncStatusValidateError means there is a dashboard validation error.
	DatadogDashboardSyncStatusValidateError DatadogDashboardSyncStatus = "error validating dashboard"
	// DatadogDashboardSyncStatusUpdateError means there is a dashboard update error.
	DatadogDashboardSyncStatusUpdateError DatadogDashboardSyncStatus = "error updating dashboard"
	// DatadogDashboardSyncStatusCreateError means there is an error getting the dashboard.
	DatadogDashboardSyncStatusCreateError DatadogDashboardSyncStatus = "error creating dashboard"
	// SyncStatusGetError means there is an error getting the monitor
	DatadoggDashboardSyncStatusGetError DatadogDashboardSyncStatus = "error getting dashboard"
)

// DatadogDashboard is the Schema for the datadogdashboards API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=datadogdashboards,scope=Namespaced,shortName=ddd
// +kubebuilder:printcolumn:name="id",type="string",JSONPath=".status.id"
// +kubebuilder:printcolumn:name="sync status",type="string",JSONPath=".status.syncStatus"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
// +k8s:openapi-gen=true
// +genclient
type DatadogDashboard struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatadogDashboardSpec   `json:"spec,omitempty"`
	Status DatadogDashboardStatus `json:"status,omitempty"`
}

// DashboardTemplateVariablePresetValue Template variables saved views.
// +k8s:openapi-gen=true
type DashboardTemplateVariablePresetValue struct {
	// The name of the variable.
	Name *string `json:"name"`
	// One or many template variable values within the saved view, which will be unioned together using `OR` if more than one is specified. Cannot be used in conjunction with `value`.
	// +listType=set
	Values []string `json:"values,omitempty"`
}

// DashboardTemplateVariablePreset Template variables saved views.
// +k8s:openapi-gen=true
type DashboardTemplateVariablePreset struct {
	// The name of the variable.
	Name *string `json:"name"`
	// List of variables.
	// +listType=map
	// +listMapKey=name
	TemplateVariables []DashboardTemplateVariablePresetValue `json:"templateVariables,omitempty"`
}

// DashboardTemplateVariable Template variable.
// +k8s:openapi-gen=true
type DashboardTemplateVariable struct {
	// The list of values that the template variable drop-down is limited to.
	AvailableValues *[]string `json:"availableValues,omitempty"`
	// One or many default values for template variables on load. If more than one default is specified, they will be unioned together with `OR`. Cannot be used in conjunction with `default`.
	// +listType=set
	Defaults []string `json:"defaults,omitempty"`
	// The name of the variable.
	Name string `json:"name"`
	// The tag prefix associated with the variable. Only tags with this prefix appear in the variable drop-down.
	Prefix *string `json:"prefix,omitempty"`
}

// DatadogDashboardList contains a list of DatadogDashboard
// +kubebuilder:object:root=true
type DatadogDashboardList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatadogDashboard `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatadogDashboard{}, &DatadogDashboardList{})
}
