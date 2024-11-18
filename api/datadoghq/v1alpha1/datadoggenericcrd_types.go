// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DatadogGenericCRDSpec defines the desired state of DatadogGenericCRD
// +k8s:openapi-gen=true
type DatadogGenericCRDSpec struct {
	// Type is the type of the API object
	// TODO: Add validation for the type (enum)
	Type string `json:"type"`
	// Spec is the specification of the API object
	// TODO: rename Spec, it's confusing accessing spec within spec
	Spec string `json:"spec"`
}

// DatadogGenericCRDStatus defines the observed state of DatadogGenericCRD
// +k8s:openapi-gen=true
type DatadogGenericCRDStatus struct {
	// Conditions represents the latest available observations of the state of a DatadogGenericCRD.
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// ID is the object ID generated in Datadog.
	ID string `json:"id,omitempty"`
	// Creator is the identity of the creator.
	Creator string `json:"creator,omitempty"`
	// Created is the time the object was created.
	Created *metav1.Time `json:"created,omitempty"`
	// SyncStatus shows the health of syncing the object state to Datadog.
	SyncStatus DatadogSyncStatus `json:"syncStatus,omitempty"`
	// CurrentHash tracks the hash of the current DatadogGenericCRDSpec to know
	// if the Spec has changed and needs an update.
	CurrentHash string `json:"currentHash,omitempty"`
	// LastForceSyncTime is the last time the API object was last force synced with the custom resource
	LastForceSyncTime *metav1.Time `json:"lastForceSyncTime,omitempty"`
}

type DatadogSyncStatus string

const (
	// DatadogSyncStatusOK means syncing is OK.
	DatadogSyncStatusOK DatadogSyncStatus = "OK"
	// DatadogSyncStatusValidateError means there is an object validation error.
	DatadogSyncStatusValidateError DatadogSyncStatus = "error validating object"
	// DatadogSyncStatusUpdateError means there is an object update error.
	DatadogSyncStatusUpdateError DatadogSyncStatus = "error updating object"
	// DatadogSyncStatusCreateError means there is an error getting the object.
	DatadogSyncStatusCreateError DatadogSyncStatus = "error creating object"
	// DatadogSyncStatusGetError means there is an error getting the object.
	DatadogSyncStatusGetError DatadogSyncStatus = "error getting object"
)

// DatadogGenericCRD is the Schema for the datadoggenericcrds API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=datadoggenericcrds,scope=Namespaced,shortName=ddgcrd
// +kubebuilder:printcolumn:name="id",type="string",JSONPath=".status.id"
// +kubebuilder:printcolumn:name="sync status",type="string",JSONPath=".status.syncStatus"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
// +k8s:openapi-gen=true
// +genclient
type DatadogGenericCRD struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatadogGenericCRDSpec   `json:"spec,omitempty"`
	Status DatadogGenericCRDStatus `json:"status,omitempty"`
}

// DatadogGenericCRDList contains a list of DatadogGenericCRD
// +kubebuilder:object:root=true
type DatadogGenericCRDList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatadogGenericCRD `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatadogGenericCRD{}, &DatadogGenericCRDList{})
}
