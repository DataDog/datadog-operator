// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// spec:
//   targetRef:
//     apiVersion: apps/v1
//     kind: Deployment
//     name: test
//   owner: local
//   policy:
//     update:
//       mode: Auto|DryRun
//   recommender:
//     name: containerMetrics
//     containerMetrics:
//       cpuUtilizationTarget: 60
//   constraints:
//     minReplicas: 1
//     maxReplicas: 10
//     containers:
//       - name: "*"
//         enabled: true
//         requests:
//           minAllowed:
//           maxAllowed:
//         limits:
//           minAllowed:
//           maxAllowed:

// DatadogPodAutoscalerSpec defines the desired state of DatadogPodAutoscaler
type DatadogPodAutoscalerSpec struct {
	// TargetRef is the reference to the resource to scale.
	TargetRef autoscalingv2.CrossVersionObjectReference `json:"targetRef"`

	// Owner defines the source of truth for this object (local or remote)
	// Value needs to be set when a DatadogPodAutoscaler object is created.
	Owner DatadogPodAutoscalerOwner `json:"owner"`

	// RemoteVersion is the version of the .Spec currently store in this object.
	// Only set if the owner is Remote.
	RemoteVersion int `json:"remoteVersion,omitempty"`

	// Policy defines how recommendations should be applied.
	Policy *DatadogPodAutoscalerPolicy `json:"policy,omitempty"`

	// Recommender defines the recommender to use for the autoscaler and its settings.
	// Default to the `containerMetrics` recommender.
	Recommender *DatadogPodAutoscalerRecommender `json:"recommender,omitempty"`

	// Constraints defines constraints that should always be respected.
	Constraints *DatadogPodAutoscalerConstraints `json:"constraints,omitempty"`
}

// DatadogPodAutoscalerOwner defines the source of truth for this object (local or remote)
// +kubebuilder:validation:Enum:=Local;Remote
type DatadogPodAutoscalerOwner string

const (
	// DatadogPodAutoscalerLocalOwner states that this `DatadogPodAutoscaler` object is created/managed outside of Datadog app.
	DatadogPodAutoscalerLocalOwner DatadogPodAutoscalerOwner = "Local"

	// DatadogPodAutoscalerLocalOwner states that this `DatadogPodAutoscaler` object is created/managed in Datadog app.
	DatadogPodAutoscalerRemoteOwner DatadogPodAutoscalerOwner = "Remote"
)

// DatadogPodAutoscalerPolicy defines how recommendations should be applied.
type DatadogPodAutoscalerPolicy struct {
	// Update defines the policy to update target resource.
	Update *DatadogPodAutoscalerUpdatePolicy `json:"update,omitempty"`
}

// DatadogPodAutoscalerUpdateMode defines the mode of the update policy.
// +kubebuilder:validation:Enum:=Auto;DryRun
type DatadogPodAutoscalerUpdateMode string

const (
	// DatadogPodAutoscalerAutoMode is the default mode.
	DatadogPodAutoscalerAutoMode DatadogPodAutoscalerUpdateMode = "Auto"

	// DatadogPodAutoscalerDrynRunMode will skip the update of the target resource.
	DatadogPodAutoscalerDrynRunMode DatadogPodAutoscalerUpdateMode = "DryRun"
)

// DatadogPodAutoscalerUpdatePolicy defines the policy to update target resource.
type DatadogPodAutoscalerUpdatePolicy struct {
	// Mode defines the mode of the update policy.
	Mode DatadogPodAutoscalerUpdateMode `json:"mode,omitempty"`
}

// DatadogPodAutoscalerUpdateMode defines the mode of the update policy.
// +kubebuilder:validation:Enum:=ContainerMetrics
type DatadogPodAutoscalerRecommenderName string

const (
	// DatadogPodAutoscalerContainerMetricsRecommender uses container resources metrics.
	DatadogPodAutoscalerContainerMetricsRecommender DatadogPodAutoscalerRecommenderName = "ContainerMetrics"
)

// DatadogPodAutoscalerRecommender defines the recommender to use for the autoscaler and its settings.
type DatadogPodAutoscalerRecommender struct {
	// Name is the name of the recommender to use.
	Name DatadogPodAutoscalerRecommenderName `json:"name"`

	// ContainerMetrics is the settings for the ContainerMetrics recommender.
	ContainerMetrics *DatadogPodAutoscalerContainerMetricsRecommenderSettings `json:"containerMetrics,omitempty"`
}

// DatadogPodAutoscalerContainerMetricsRecommenderSettings defines the settings for the ContainerMetrics recommender.
type DatadogPodAutoscalerContainerMetricsRecommenderSettings struct {
	// CPUUtilizationTarget is the target CPU utilization for the containers.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	CPUUtilizationTarget *int32 `json:"cpuUtilizationTarget,omitempty"`
}

// DatadogPodAutoscalerConstraints defines constraints that should always be respected.
type DatadogPodAutoscalerConstraints struct {
	// MinReplicas is the lower limit for the number of POD replicas. Needs to be >= 1. Default to 1.
	// +kubebuilder:validation:Minimum=1
	MinReplicas *int32 `json:"minReplicas,omitempty"`

	// MaxReplicas is the upper limit for the number of POD replicas. Needs to be >= minReplicas.
	MaxReplicas int32 `json:"maxReplicas"`

	// Containers defines constraints for the containers.
	Containers []DatadogPodAutoscalerContainerConstraints `json:"containers,omitempty"`
}

// DatadogPodAutoscalerContainerConstraints defines constraints that should always be respected for a container.
// If no constraints are set, it enables resources scaling for all containers without any constraints.
type DatadogPodAutoscalerContainerConstraints struct {
	// Name is the name of the container. Can be "*" to apply to all containers.
	Name string `json:"name"`

	// Enabled false allows to disable resources autoscaling for the container. Default to true.
	Enabled *bool `json:"enabled,omitempty"`

	// Requests defines the constraints for the requests of the container.
	Requests *DatadogPodAutoscalerContainerResourceConstraints `json:"requests,omitempty"`

	// Limits defines the constraints for the limits of the container.
	Limits *DatadogPodAutoscalerContainerResourceConstraints `json:"limits,omitempty"`
}

type DatadogPodAutoscalerContainerResourceConstraints struct {
	// MinAllowed is the lower limit for the requests of the container.
	// +optional
	MinAllowed corev1.ResourceList `json:"minAllowed,omitempty"`

	// MaxAllowed is the upper limit for the requests of the container.
	// +optional
	MaxAllowed corev1.ResourceList `json:"maxAllowed,omitempty"`
}

// DatadogPodAutoscalerStatus defines the observed state of DatadogPodAutoscaler
type DatadogPodAutoscalerStatus struct {
	// Vertical is the status of the vertical scaling, if activated.
	// +optional
	Vertical DatadogPodAutoscalerVerticalStatus `json:"vertical,omitempty"`

	// Horizontal is the status of the horizontal scaling, if activated.
	// +optional
	Horizontal DatadogPodAutoscalerHorizontalStatus `json:"horizontal,omitempty"`

	// Conditions describe the current state of the DatadogPodAutoscaler operations.
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []DatadogPodAutoscalerCondition `json:"conditions,omitempty"`
}

type DatadogPodAutoscalerHorizontalStatus struct {
	// CurrentReplicas is the current number of replicas for the resource
	CurrentReplicas int32 `json:"currentReplicas"`

	// DesiredReplicas is the desired number of replicas for the resource
	DesiredReplicas int32 `json:"desiredReplicas"`

	// UpdateTime is the timestamp from last recommendation
	UpdateTime metav1.Time `json:"updateTime"`
}

type DatadogPodAutoscalerVerticalStatus struct {
	// Scaled is the current number of PODs having desired resources
	Scaled int32 `json:"scaled"`

	// Total is the total number of PODs
	Total int32 `json:"total"`

	// DesiredResources is the desired resources for containers
	DesiredResources []DatadogPodAutoscalerContainerResources `json:"desiredResources"`

	// UpdateTime is the timestamp from last recommendation
	UpdateTime metav1.Time `json:"updateTime"`
}

type DatadogPodAutoscalerContainerResources struct {
	// Name is the name of the container
	Name string `json:"name"`

	// Limits describes the maximum amount of compute resources allowed.
	// +optional
	Limits corev1.ResourceList `json:"limits,omitempty"`

	// Requests describes target resources of compute resources allowed.
	// +optional
	Requests corev1.ResourceList `json:"requests,omitempty"`
}

// DatadogPodAutoscalerConditionType type use to represent a DatadogMetric condition
type DatadogPodAutoscalerConditionType string

const (
	// DatadogPodAutoscalerActiveCondition is true when Autoscaling is active (not in dry run).
	DatadogPodAutoscalerActiveCondition DatadogPodAutoscalerConditionType = "Active"

	// DatadogPodAutoscalerHorizontalAbleToScaleCondition is true when horizontal scaling is working correctly.
	DatadogPodAutoscalerHorizontalAbleToScaleCondition DatadogPodAutoscalerConditionType = "HorizontalAbleToScale"

	// DatadogPodAutoscalerVerticalAbleToPatch is true when we can patch the resources of the PODs.
	DatadogPodAutoscalerVerticalAbleToPatch DatadogPodAutoscalerConditionType = "VerticalAbleToPatch"

	// DatadogPodAutoscalerVerticalAbleToRollout is true when we can rollout the targetRef to pick up patched resources.
	DatadogPodAutoscalerVerticalAbleToRollout DatadogPodAutoscalerConditionType = "VerticalAbleToRollout"
)

// DatadogPodAutoscalerCondition describes the state of DatadogPodAutoscaler.
type DatadogPodAutoscalerCondition struct {
	// Type of DatadogMetric condition.
	Type DatadogPodAutoscalerConditionType `json:"type"`

	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`

	// Last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`

	// The reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty"`

	// A human readable message indicating details about the transition.
	// +optional
	Message string `json:"message,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// DatadogPodAutoscaler is the Schema for the datadogpodautoscalers API
type DatadogPodAutoscaler struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatadogPodAutoscalerSpec   `json:"spec,omitempty"`
	Status DatadogPodAutoscalerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DatadogPodAutoscalerList contains a list of DatadogPodAutoscaler
type DatadogPodAutoscalerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatadogPodAutoscaler `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatadogPodAutoscaler{}, &DatadogPodAutoscalerList{})
}
