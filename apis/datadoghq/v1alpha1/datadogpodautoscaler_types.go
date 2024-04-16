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
	RemoteVersion uint64 `json:"remoteVersion,omitempty"`

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

	// Upscale defines the policy to scale up the target resource.
	Upscale *DatadogPodAutoscalerScalingPolicy `json:"upscale,omitempty"`

	// Downscale defines the policy to scale up the target resource.
	Downscale *DatadogPodAutoscalerScalingPolicy `json:"downscale,omitempty"`
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

//
// Scaling policy is inspired by the HorizontalPodAutoscalerV2
// https://github.com/kubernetes/api/blob/master/autoscaling/v2/types.go
// Copyright 2021 The Kubernetes Authors.
//

// DatadogPodAutoscalerScalingPolicySelect is used to specify which policy should be used while scaling in a certain direction
type DatadogPodAutoscalerScalingPolicySelect string

const (
	// MaxChangePolicySelect selects the policy with the highest possible change.
	MaxChangePolicySelect DatadogPodAutoscalerScalingPolicySelect = "Max"

	// MinChangePolicySelect selects the policy with the lowest possible change.
	MinChangePolicySelect DatadogPodAutoscalerScalingPolicySelect = "Min"

	// DisabledPolicySelect disables the scaling in this direction.
	DisabledPolicySelect DatadogPodAutoscalerScalingPolicySelect = "Disabled"
)

// DatadogPodAutoscalerScalingPolicy defines the policy to scale the target resource.
type DatadogPodAutoscalerScalingPolicy struct {
	// Strategy is used to specify which policy should be used.
	// If not set, the default value Max is used.
	// +optional
	Strategy *DatadogPodAutoscalerScalingPolicySelect `json:"selectPolicy,omitempty"`

	// Rules is a list of potential scaling polices which can be used during scaling.
	// At least one policy must be specified, otherwise the HPAScalingRules will be discarded as invalid
	// +listType=atomic
	// +optional
	Rules []DatadogPodAutoscalerScalingRule `json:"policies,omitempty"`
}

// DatadogPodAutoscalerScalingRuleType is the type of the policy which could be used while making scaling decisions.
type DatadogPodAutoscalerScalingRuleType string

const (
	// PodsScalingPolicy is a policy used to specify a change in absolute number of pods.
	PodsScalingPolicy DatadogPodAutoscalerScalingRuleType = "Pods"

	// PercentScalingPolicy is a policy used to specify a relative amount of change with respect to
	// the current number of pods.
	PercentScalingPolicy DatadogPodAutoscalerScalingRuleType = "Percent"
)

// DatadogPodAutoscalerScalingRule define rules for horizontal that should be true for a certain amount of time.
type DatadogPodAutoscalerScalingRule struct {
	// Type is used to specify the scaling policy.
	Type DatadogPodAutoscalerScalingRuleType `json:"type"`

	// Value contains the amount of change which is permitted by the policy.
	// It must be greater than zero
	Value int32 `json:"value"`

	// PeriodSeconds specifies the window of time for which the policy should hold true.
	// PeriodSeconds must be greater than zero and less than or equal to 1800 (30 min).
	PeriodSeconds int32 `json:"periodSeconds"`
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
	// RecommendationsVersion holds the version of the recommendation
	// +optional
	RecommendationsVersion *uint64 `json:"recommendationsVersion,omitempty"`

	// UpdateTime is the timestamp at which the last recommendation was received
	// +optional
	UpdateTime *metav1.Time `json:"updateTime,omitempty"`

	// Vertical is the status of the vertical scaling, if activated.
	// +optional
	Vertical *DatadogPodAutoscalerVerticalStatus `json:"vertical,omitempty"`

	// Horizontal is the status of the horizontal scaling, if activated.
	// +optional
	Horizontal *DatadogPodAutoscalerHorizontalStatus `json:"horizontal,omitempty"`

	// CurrentReplicas is the current number of PODs for the targetRef
	// +optional
	CurrentReplicas *int32 `json:"currentReplicas,omitempty"`

	// Conditions describe the current state of the DatadogPodAutoscaler operations.
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []DatadogPodAutoscalerCondition `json:"conditions,omitempty"`
}

// DatadogPodAutoscalerHorizontalStatus defines the status of the horizontal scaling
type DatadogPodAutoscalerHorizontalStatus struct {
	// DesiredReplicas is the desired number of replicas for the resource
	DesiredReplicas int32 `json:"desiredReplicas"`

	// LastAction is the last successful action done by the controller
	LastAction *DatadogPodAutoscalerHorizontalAction `json:"lastAction,omitempty"`
}

// DatadogPodAutoscalerHorizontalAction represents an horizontal action done by the controller
type DatadogPodAutoscalerHorizontalAction struct {
	// Time is the timestamp of the action
	Time metav1.Time `json:"time"`

	// FromReplicas is the number of replicas before the action
	FromReplicas int32 `json:"replicas"`

	// ToReplicas is the number of replicas after the action
	ToReplicas int32 `json:"toReplicas"`
}

// DatadogPodAutoscalerVerticalStatus defines the status of the vertical scaling
type DatadogPodAutoscalerVerticalStatus struct {
	// Scaled is the current number of PODs having desired resources
	Scaled *int32 `json:"scaled"`

	// DesiredResources is the desired resources for containers
	DesiredResources []DatadogPodAutoscalerContainerResources `json:"desiredResources"`

	// LastAction is the last successful action done by the controller
	LastAction *DatadogPodAutoscalerVerticalAction `json:"lastAction,omitempty"`
}

// DatadogPodAutoscalerVerticalActionType represents the type of action done by the controller
type DatadogPodAutoscalerVerticalActionType string

const (
	// RolloutTriggered is the action when the controller triggers a rollout of the targetRef
	RolloutTriggered DatadogPodAutoscalerVerticalActionType = "RolloutTriggered"
)

// DatadogPodAutoscalerVerticalAction represents a vertical action done by the controller
type DatadogPodAutoscalerVerticalAction struct {
	// Time is the timestamp of the action
	Time metav1.Time `json:"time"`

	// Type is the type of action
	Type DatadogPodAutoscalerVerticalActionType `json:"type"`
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
	// DatadogPodAutoscalerErrorCondition is true when a global error is encountered by the controller.
	DatadogPodAutoscalerErrorCondition DatadogPodAutoscalerConditionType = "Error"

	// DatadogPodAutoscalerHorizontalAbleToScaleCondition is true when horizontal scaling is working correctly.
	DatadogPodAutoscalerHorizontalAbleToScaleCondition DatadogPodAutoscalerConditionType = "HorizontalAbleToScale"

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
