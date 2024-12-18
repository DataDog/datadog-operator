// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type SupportedResourcesType string

const (
	Notebook              SupportedResourcesType = "notebook"
	SyntheticsAPITest     SupportedResourcesType = "synthetics_api_test"
	SyntheticsBrowserTest SupportedResourcesType = "synthetics_browser_test"
)

// DatadogGenericCRSpec defines the desired state of DatadogGenericCR
// +k8s:openapi-gen=true
type DatadogGenericCRSpec struct {
	// Type is the type of the API object
	// TODO: Add validation for the type (enum)
	Type SupportedResourcesType `json:"type"`
	// JsonSpec is the specification of the API object
	JsonSpec string `json:"jsonSpec"`
}

// DatadogGenericCRStatus defines the observed state of DatadogGenericCR
// +k8s:openapi-gen=true
type DatadogGenericCRStatus struct {
	// Conditions represents the latest available observations of the state of a DatadogGenericCR.
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// Id is the object unique identifier generated in Datadog.
	Id string `json:"id,omitempty"`
	// Creator is the identity of the creator.
	Creator string `json:"creator,omitempty"`
	// Created is the time the object was created.
	Created *metav1.Time `json:"created,omitempty"`
	// SyncStatus shows the health of syncing the object state to Datadog.
	SyncStatus DatadogSyncStatus `json:"syncStatus,omitempty"`
	// CurrentHash tracks the hash of the current DatadogGenericCRSpec to know
	// if the JsonSpec has changed and needs an update.
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

// DatadogGenericCR is the Schema for the datadoggenericcrs API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=datadoggenericcrs,scope=Namespaced,shortName=ddgcr
// +kubebuilder:printcolumn:name="id",type="string",JSONPath=".status.id"
// +kubebuilder:printcolumn:name="sync status",type="string",JSONPath=".status.syncStatus"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
// +k8s:openapi-gen=true
// +genclient
type DatadogGenericCR struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatadogGenericCRSpec   `json:"spec,omitempty"`
	Status DatadogGenericCRStatus `json:"status,omitempty"`
}

// DatadogGenericCRList contains a list of DatadogGenericCR
// +kubebuilder:object:root=true
type DatadogGenericCRList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatadogGenericCR `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatadogGenericCR{}, &DatadogGenericCRList{})
}
