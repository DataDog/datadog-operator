// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package v1alpha1

import (
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ExtendedDaemonsetSettingSpec is the Schema for the extendeddaemonsetsetting API
// +k8s:openapi-gen=true
type ExtendedDaemonsetSettingSpec struct {
	// Reference contains enough information to let you identify the referred resource.
	Reference *autoscalingv1.CrossVersionObjectReference `json:"reference"`
	// NodeSelector lists labels that must be present on nodes to trigger the usage of this resource.
	NodeSelector metav1.LabelSelector `json:"nodeSelector"`
	// Containers contains a list of container spec override.
	// +listType=map
	// +listMapKey=name
	Containers []ExtendedDaemonsetSettingContainerSpec `json:"containers,omitempty"`
}

// ExtendedDaemonsetSettingContainerSpec defines the resources override for a container identified by its name
// +k8s:openapi-gen=true
type ExtendedDaemonsetSettingContainerSpec struct {
	Name      string                      `json:"name"`
	Resources corev1.ResourceRequirements `json:"resources"`
}

// ExtendedDaemonsetSettingStatusStatus defines the readable status in ExtendedDaemonsetSettingStatus.
type ExtendedDaemonsetSettingStatusStatus string

const (
	// ExtendedDaemonsetSettingStatusValid status when ExtendedDaemonsetSetting is valide.
	ExtendedDaemonsetSettingStatusValid ExtendedDaemonsetSettingStatusStatus = "valid"
	// ExtendedDaemonsetSettingStatusError status when ExtendedDaemonsetSetting is in error state.
	ExtendedDaemonsetSettingStatusError ExtendedDaemonsetSettingStatusStatus = "error"
)

// ExtendedDaemonsetSettingStatus defines the observed state of ExtendedDaemonsetSetting.
// +k8s:openapi-gen=true
type ExtendedDaemonsetSettingStatus struct {
	Status ExtendedDaemonsetSettingStatusStatus `json:"status"`
	Error  string                               `json:"error,omitempty"`
}

// ExtendedDaemonsetSetting is the Schema for the extendeddaemonsetsettings API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=extendeddaemonsetsettings,scope=Namespaced
// +kubebuilder:printcolumn:name="status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="node selector",type="string",JSONPath=".spec.nodeSelector"
// +kubebuilder:printcolumn:name="error",type="string",JSONPath=".status.error"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
// +k8s:openapi-gen=true
// +genclient
type ExtendedDaemonsetSetting struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ExtendedDaemonsetSettingSpec   `json:"spec,omitempty"`
	Status ExtendedDaemonsetSettingStatus `json:"status,omitempty"`
}

// ExtendedDaemonsetSettingList contains a list of ExtendedDaemonsetSetting
// +kubebuilder:object:root=true
type ExtendedDaemonsetSettingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ExtendedDaemonsetSetting `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ExtendedDaemonsetSetting{}, &ExtendedDaemonsetSettingList{})
}
