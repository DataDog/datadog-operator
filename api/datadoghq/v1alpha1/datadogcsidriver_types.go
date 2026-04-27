// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// CSI driver container name constants
const (
	CSINodeDriverContainerName          = "csi-node-driver"
	CSINodeDriverRegistrarContainerName = "csi-node-driver-registrar"
)

// DatadogCSIDriverSpec defines the desired state of DatadogCSIDriver
// +k8s:openapi-gen=true
type DatadogCSIDriverSpec struct {
	// CSIDriverImage is the image configuration for the main CSI node driver container.
	// +optional
	CSIDriverImage *v2alpha1.AgentImageConfig `json:"csiDriverImage,omitempty"`

	// RegistrarImage is the image configuration for the CSI node driver registrar sidecar.
	// +optional
	RegistrarImage *v2alpha1.AgentImageConfig `json:"registrarImage,omitempty"`

	// APMSocketPath is the host path to the APM socket.
	// Default: /var/run/datadog/apm.socket
	// +optional
	APMSocketPath *string `json:"apmSocketPath,omitempty"`

	// DSDSocketPath is the host path to the DogStatsD socket.
	// Default: /var/run/datadog/dsd.socket
	// +optional
	DSDSocketPath *string `json:"dsdSocketPath,omitempty"`

	// Override allows customization of the CSI driver DaemonSet pod template.
	// +optional
	Override *DatadogCSIDriverOverride `json:"override,omitempty"`
}

// DatadogCSIDriverOverride provides override capabilities for the CSI driver DaemonSet.
// +k8s:openapi-gen=true
type DatadogCSIDriverOverride struct {
	// AdditionalLabels provides labels that are added to the CSI driver DaemonSet pods.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations provides annotations that are added to the CSI driver DaemonSet pods.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// NodeSelector is a map of key-value pairs. For the CSI driver pod to run on a
	// specific node, the node must have these key-value pairs as labels.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Tolerations configure the CSI driver DaemonSet pod tolerations.
	// +optional
	// +listType=atomic
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Affinity specifies the pod's scheduling constraints.
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// PriorityClassName indicates the pod's priority.
	// +optional
	PriorityClassName *string `json:"priorityClassName,omitempty"`

	// SecurityContext holds pod-level security attributes.
	// +optional
	SecurityContext *corev1.PodSecurityContext `json:"securityContext,omitempty"`

	// UpdateStrategy is the DaemonSet update strategy configuration.
	// +optional
	UpdateStrategy *common.UpdateStrategy `json:"updateStrategy,omitempty"`

	// Containers allows overriding container-level configuration for the CSI driver containers.
	// Valid keys are: "csi-node-driver" and "csi-node-driver-registrar".
	// +optional
	Containers map[string]*v2alpha1.DatadogAgentGenericContainer `json:"containers,omitempty"`

	// Volumes specifies additional volumes to add to the CSI driver DaemonSet pods.
	// +optional
	// +listType=map
	// +listMapKey=name
	Volumes []corev1.Volume `json:"volumes,omitempty"`

	// Env specifies additional environment variables for all containers.
	// +optional
	// +listType=map
	// +listMapKey=name
	Env []corev1.EnvVar `json:"env,omitempty"`

	// ServiceAccountName sets the ServiceAccount used by the CSI driver DaemonSet.
	// +optional
	ServiceAccountName *string `json:"serviceAccountName,omitempty"`
}

// DatadogCSIDriverStatus defines the observed state of DatadogCSIDriver
// +k8s:openapi-gen=true
type DatadogCSIDriverStatus struct {
	// ObservedGeneration is the most recent generation observed for this resource.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represents the latest available observations of the DatadogCSIDriver's current state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// DaemonSet tracks the status of the CSI driver DaemonSet.
	// +optional
	DaemonSet *v2alpha1.DaemonSetStatus `json:"daemonSet,omitempty"`

	// CSIDriverName is the name of the managed CSIDriver Kubernetes object.
	// +optional
	CSIDriverName string `json:"csiDriverName,omitempty"`
}

// DatadogCSIDriver is the Schema for the datadogcsidrivers API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=datadogcsidrivers,shortName=ddcsi
// +kubebuilder:printcolumn:name="status",type="string",JSONPath=".status.daemonSet.status"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
// +k8s:openapi-gen=true
type DatadogCSIDriver struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatadogCSIDriverSpec   `json:"spec,omitempty"`
	Status DatadogCSIDriverStatus `json:"status,omitempty"`
}

// DatadogCSIDriverList contains a list of DatadogCSIDriver
// +kubebuilder:object:root=true
type DatadogCSIDriverList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatadogCSIDriver `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatadogCSIDriver{}, &DatadogCSIDriverList{})
}
