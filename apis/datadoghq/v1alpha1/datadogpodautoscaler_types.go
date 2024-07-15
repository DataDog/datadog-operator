// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// spec:
//   targetRef:
//     apiVersion: apps/v1
//     kind: Deployment
//     name: test
//   owner: local
//   remoteVersion: 1
//   policy:
//     applyMode: All|Manual|None
//     update:
//       strategy: Auto|Disabled
//     upscale:
//       strategy: Max|Min|Disabled
//       rules:
//         - type: Pods|Percent
//           value: 1
//           periodSeconds: 60
//     downscale:
//       strategy: Max|Min|Disabled
//       rules:
//         - type: Pods|Percent
//           value: 1
//           periodSeconds: 60
//   targets:
//     - type: PodResource
//       resource:
//         name: cpu
//         value:
//           type: Absolute|Utilization
//           absolute: 500m
//           utilization: 80
//     - type: ContainerResource
//       containerResource:
//         name: cpu
//         value:
//           type: Absolute|Utilization
//           absolute: 500m
//           utilization: 80
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

// DatadogPodAutoscalerOwner defines the source of truth for this object (local or remote)
// +kubebuilder:validation:Enum:=Local;Remote
type DatadogPodAutoscalerOwner string

const (
	// DatadogPodAutoscalerLocalOwner states that this `DatadogPodAutoscaler` object is created/managed outside of Datadog app.
	DatadogPodAutoscalerLocalOwner DatadogPodAutoscalerOwner = "Local"

	// DatadogPodAutoscalerLocalOwner states that this `DatadogPodAutoscaler` object is created/managed in Datadog app.
	DatadogPodAutoscalerRemoteOwner DatadogPodAutoscalerOwner = "Remote"
)

// DatadogPodAutoscalerSpec defines the desired state of DatadogPodAutoscaler
type DatadogPodAutoscalerSpec struct {
	// TargetRef is the reference to the resource to scale.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Modifying the targetRef is not allowed. Please delete and re-create the DatadogPodAutoscaler object."
	TargetRef autoscalingv2.CrossVersionObjectReference `json:"targetRef"`

	// Owner defines the source of truth for this object (local or remote)
	// Value needs to be set when a DatadogPodAutoscaler object is created.
	Owner DatadogPodAutoscalerOwner `json:"owner"`

	// RemoteVersion is the version of the .Spec currently store in this object.
	// Only set if the owner is Remote.
	RemoteVersion *uint64 `json:"remoteVersion,omitempty"`

	// Policy defines how recommendations should be applied.
	// +optional
	// +kubebuilder:default={}
	Policy *DatadogPodAutoscalerPolicy `json:"policy,omitempty"`

	// Targets are objectives to reach and maintain for the target resource.
	// Default to a single target to maintain 80% POD CPU utilization.
	// +listType=atomic
	// +optional
	Targets []DatadogPodAutoscalerTarget `json:"targets,omitempty"`

	// Constraints defines constraints that should always be respected.
	Constraints *DatadogPodAutoscalerConstraints `json:"constraints,omitempty"`
}

// DatadogPodAutoscalerOwner defines the source of truth for this object (local or remote)
// +kubebuilder:validation:Enum:=All;Manual;None
type DatadogPodAutoscalerApplyMode string

const (
	// DatadogPodAutoscalerAllApplyMode allows the controller to apply all recommendations (regular and manual)
	DatadogPodAutoscalerAllApplyMode DatadogPodAutoscalerApplyMode = "All"

	// DatadogPodAutoscalerManualApplyMode allows the controller to only apply manual recommendations (recommendations manually validated by user in the Datadog app)
	DatadogPodAutoscalerManualApplyMode DatadogPodAutoscalerApplyMode = "Manual"

	// DatadogPodAutoscalerNoneApplyMode prevent the controller to apply any recommendations. Datadog will still produce and display recommendations
	// but the controller will not apply them, even when they are manually validated. Similar to "DryRun" mode.
	DatadogPodAutoscalerNoneApplyMode DatadogPodAutoscalerApplyMode = "None"
)

// DatadogPodAutoscalerPolicy defines how recommendations should be applied.
type DatadogPodAutoscalerPolicy struct {
	// ApplyMode determines recommendations that should be applied by the controller:
	// - All: Apply all recommendations (regular and manual).
	// - Manual: Apply only manual recommendations (recommendations manually validated by user in the Datadog app).
	// - None: Prevent the controller to apply any recommendations.
	// It's also possible to selectively deactivate upscale, downscale or update actions thanks to the `Upscale`, `Downscale` and `Update` fields.
	// +optional
	// +kubebuilder:default=All
	ApplyMode DatadogPodAutoscalerApplyMode `json:"applyMode"`

	// Update defines the policy to update target resource.
	Update *DatadogPodAutoscalerUpdatePolicy `json:"update,omitempty"`

	// Upscale defines the policy to scale up the target resource.
	Upscale *DatadogPodAutoscalerScalingPolicy `json:"upscale,omitempty"`

	// Downscale defines the policy to scale down the target resource.
	Downscale *DatadogPodAutoscalerScalingPolicy `json:"downscale,omitempty"`
}

// DatadogPodAutoscalerUpdateStrategy defines the mode of the update policy.
// +kubebuilder:validation:Enum:=Auto;Disabled
type DatadogPodAutoscalerUpdateStrategy string

const (
	// DatadogPodAutoscalerAutoUpdateStrategy is the default mode.
	DatadogPodAutoscalerAutoUpdateStrategy DatadogPodAutoscalerUpdateStrategy = "Auto"

	// DatadogPodAutoscalerDisabledUpdateStrategy will disable the update of the target resource.
	DatadogPodAutoscalerDisabledUpdateStrategy DatadogPodAutoscalerUpdateStrategy = "Disabled"
)

// DatadogPodAutoscalerUpdatePolicy defines the policy to update target resource.
type DatadogPodAutoscalerUpdatePolicy struct {
	// Mode defines the mode of the update policy.
	Strategy DatadogPodAutoscalerUpdateStrategy `json:"strategy,omitempty"`
}

//
// Scaling policy is inspired by the HorizontalPodAutoscalerV2
// https://github.com/kubernetes/api/blob/master/autoscaling/v2/types.go
// Copyright 2021 The Kubernetes Authors.
//

// DatadogPodAutoscalerScalingStrategySelect is used to specify which policy should be used while scaling in a certain direction
// +kubebuilder:validation:Enum:=Max;Min;Disabled
type DatadogPodAutoscalerScalingStrategySelect string

const (
	// DatadogPodAutoscalerMaxChangeStrategySelect selects the policy with the highest possible change.
	DatadogPodAutoscalerMaxChangeStrategySelect DatadogPodAutoscalerScalingStrategySelect = "Max"

	// DatadogPodAutoscalerMinChangeStrategySelect selects the policy with the lowest possible change.
	DatadogPodAutoscalerMinChangeStrategySelect DatadogPodAutoscalerScalingStrategySelect = "Min"

	// DatadogPodAutoscalerDisabledStrategySelect disables the scaling in this direction.
	DatadogPodAutoscalerDisabledStrategySelect DatadogPodAutoscalerScalingStrategySelect = "Disabled"
)

// DatadogPodAutoscalerScalingPolicy defines the policy to scale the target resource.
type DatadogPodAutoscalerScalingPolicy struct {
	// Strategy is used to specify which policy should be used.
	// If not set, the default value Max is used.
	// +optional
	Strategy *DatadogPodAutoscalerScalingStrategySelect `json:"strategy,omitempty"`

	// Rules is a list of potential scaling polices which can be used during scaling.
	// At least one policy must be specified, otherwise the DatadogPodAutoscalerScalingPolicy will be discarded as invalid
	// +listType=atomic
	// +optional
	Rules []DatadogPodAutoscalerScalingRule `json:"rules,omitempty"`
}

// DatadogPodAutoscalerScalingRuleType defines how scaling rule value should be interpreted.
// +kubebuilder:validation:Enum:=Pods;Percent
type DatadogPodAutoscalerScalingRuleType string

const (
	// DatadogPodAutoscalerPodsScalingRuleType specifies a change in absolute number of pods compared to the starting number of PODs.
	DatadogPodAutoscalerPodsScalingRuleType DatadogPodAutoscalerScalingRuleType = "Pods"

	// DatadogPodAutoscalerPercentScalingRuleType specifies a relative amount of change compared to the starting number of PODs.
	DatadogPodAutoscalerPercentScalingRuleType DatadogPodAutoscalerScalingRuleType = "Percent"
)

// DatadogPodAutoscalerScalingRuleMatch
// +kubebuilder:validation:Enum:=Always;IfScalingEvent
type DatadogPodAutoscalerScalingRuleMatch string

const (
	// DatadogPodAutoscalerAlwaysScalingRuleMatch defines that the rule should always be considered in the calculation.
	DatadogPodAutoscalerAlwaysScalingRuleMatch DatadogPodAutoscalerScalingRuleMatch = "Always"

	// DatadogPodAutoscalerIfScalingEventRuleMatch defines that rule should only be considered if at least one scaling event occurred.
	// It allows to define behaviors such as forbidden windows (e.g. allow 0 PODs (Value) to be created in the next 5m (PeriodSeconds) after a scaling events).
	DatadogPodAutoscalerIfScalingEventRuleMatch DatadogPodAutoscalerScalingRuleMatch = "IfScalingEvent"
)

// DatadogPodAutoscalerScalingRule define rules for horizontal that should be true for a certain amount of time.
type DatadogPodAutoscalerScalingRule struct {
	// Type is used to specify the scaling policy.
	Type DatadogPodAutoscalerScalingRuleType `json:"type"`

	// Value contains the amount of change which is permitted by the policy.
	// Setting it to 0 will prevent any scaling in this direction and should not be used unless Match is set to IfScalingEvent.
	// +kubebuilder:validation:Minimum=0
	Value int32 `json:"value"`

	// PeriodSeconds specifies the window of time for which the policy should hold true.
	// PeriodSeconds must be greater than zero and less than or equal to 1800 (30 min).
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=1800
	PeriodSeconds int32 `json:"periodSeconds"`

	// Match defines if the rule should be considered or not in the calculation.
	// Default to Always if not set.
	Match *DatadogPodAutoscalerScalingRuleMatch `json:"match,omitempty"`
}

// DatadogPodAutoscalerTargetType defines the type of the target.
// +kubebuilder:validation:Enum:=PodResource;ContainerResource
type DatadogPodAutoscalerTargetType string

const (
	// DatadogPodAutoscalerResourceTargetType allows to set POD-level resources targets.
	DatadogPodAutoscalerResourceTargetType DatadogPodAutoscalerTargetType = "PodResource"

	// DatadogPodAutoscalerContainerResourceTargetType allows to set container-level resources targets.
	DatadogPodAutoscalerContainerResourceTargetType DatadogPodAutoscalerTargetType = "ContainerResource"
)

// DatadogPodAutoscalerTarget defines the objectives to reach and maintain for the target resource.
type DatadogPodAutoscalerTarget struct {
	// Type sets the type of the target.
	Type DatadogPodAutoscalerTargetType `json:"type"`

	// PodResource allows to set a POD-level resource target.
	PodResource *DatadogPodAutoscalerResourceTarget `json:"podResource,omitempty"`

	// ContainerResource allows to set a container-level resource target.
	ContainerResource *DatadogPodAutoscalerContainerResourceTarget `json:"containerResource,omitempty"`
}

// DatadogPodAutoscalerResourceTarget defines a POD-level resource target (for instance, CPU Utilization at 80%)
// For POD-level targets, resources are the sum of all containers resources.
// Utilization is computed from sum(usage) / sum(requests).
type DatadogPodAutoscalerResourceTarget struct {
	// Name is the name of the resource.
	// +kubebuilder:validation:Enum:=cpu
	Name corev1.ResourceName `json:"name"`

	// Value is the value of the target.
	Value DatadogPodAutoscalerTargetValue `json:"value"`
}

// DatadogPodAutoscalerContainerResourceTarget defines a Container level resource target (for instance, CPU Utilization for container named "foo" at 80%)
type DatadogPodAutoscalerContainerResourceTarget struct {
	// Name is the name of the resource.
	// +kubebuilder:validation:Enum:=cpu
	Name corev1.ResourceName `json:"name"`

	// Value is the value of the target.
	Value DatadogPodAutoscalerTargetValue `json:"value"`

	// Container is the name of the container.
	Container string `json:"container"`
}

// DatadogPodAutoscalerTargetValue defines the value of the target.
type DatadogPodAutoscalerTargetValue struct {
	// Type specifies how the value is expressed (Absolute or Utilization).
	Type DatadogPodAutoscalerTargetValueType `json:"type"`

	// Absolute defines the absolute value of the target (for instance 500 millicores).
	Absolute *resource.Quantity `json:"absolute,omitempty"`

	// Utilization defines a percentage of the target compared to requested resource
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	Utilization *int32 `json:"utilization,omitempty"`
}

// DatadogPodAutoscalerTargetValueType specifies the type of metric being targeted, and should be either
// kubebuilder:validation:Enum:=Absolute;Utilization
type DatadogPodAutoscalerTargetValueType string

const (
	// DatadogPodAutoscalerAbsoluteTargetValueType is the target type for absolute values
	DatadogPodAutoscalerAbsoluteTargetValueType DatadogPodAutoscalerTargetValueType = "Absolute"

	// DatadogPodAutoscalerUtilizationTargetValueType declares a MetricTarget is an AverageUtilization value
	DatadogPodAutoscalerUtilizationTargetValueType DatadogPodAutoscalerTargetValueType = "Utilization"
)

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
	Vertical *DatadogPodAutoscalerVerticalStatus `json:"vertical,omitempty"`

	// Horizontal is the status of the horizontal scaling, if activated.
	// +optional
	Horizontal *DatadogPodAutoscalerHorizontalStatus `json:"horizontal,omitempty"`

	// CurrentReplicas is the current number of PODs for the targetRef observed by the controller.
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

// DatadogPodAutoscalerValueSource defines the source of the value used to scale the target resource.
type DatadogPodAutoscalerValueSource string

const (
	// DatadogPodAutoscalerAutoscalingValueSource is a recommendation that comes from active autoscaling.
	DatadogPodAutoscalerAutoscalingValueSource DatadogPodAutoscalerValueSource = "Autoscaling"

	// DatadogPodAutoscalerManualValueSource is a recommendation that comes from manually applying a recommendation.
	DatadogPodAutoscalerManualValueSource DatadogPodAutoscalerValueSource = "Manual"
)

// DatadogPodAutoscalerHorizontalStatus defines the status of the horizontal scaling
type DatadogPodAutoscalerHorizontalStatus struct {
	// Target is the current target of the horizontal scaling
	Target *DatadogPodAutoscalerHorizontalTargetStatus `json:"target,omitempty"`

	// LastActions are the last successful actions done by the controller
	LastActions []DatadogPodAutoscalerHorizontalAction `json:"lastActions,omitempty"`
}

// DatadogPodAutoscalerHorizontalTargetStatus defines the current target of the horizontal scaling
type DatadogPodAutoscalerHorizontalTargetStatus struct {
	// Source is the source of the value used to scale the target resource
	Source DatadogPodAutoscalerValueSource `json:"source"`

	// GeneratedAt is the timestamp at which the recommendation was generated
	GeneratedAt metav1.Time `json:"generatedAt,omitempty"`

	// Replicas is the desired number of replicas for the resource
	Replicas int32 `json:"desiredReplicas"`
}

// DatadogPodAutoscalerHorizontalAction represents an horizontal action done by the controller
type DatadogPodAutoscalerHorizontalAction struct {
	// Time is the timestamp of the action
	Time metav1.Time `json:"time"`

	// FromReplicas is the number of replicas before the action
	FromReplicas int32 `json:"replicas"`

	// ToReplicas is the effective number of replicas after the action
	ToReplicas int32 `json:"toReplicas"`

	// RecommendedReplicas is the original number of replicas recommended by Datadog
	RecommendedReplicas *int32 `json:"recommendedReplicas,omitempty"`

	// LimitedReason is the reason why the action was limited (ToReplicas != RecommendedReplicas)
	LimitedReason *string `json:"limitedReason,omitempty"`
}

// DatadogPodAutoscalerVerticalStatus defines the status of the vertical scaling
type DatadogPodAutoscalerVerticalStatus struct {
	// Target is the current target of the vertical scaling
	Target *DatadogPodAutoscalerVerticalTargetStatus `json:"target,omitempty"`

	// LastAction is the last successful action done by the controller
	LastAction *DatadogPodAutoscalerVerticalAction `json:"lastAction,omitempty"`
}

// DatadogPodAutoscalerVerticalTargetStatus defines the current target of the vertical scaling
type DatadogPodAutoscalerVerticalTargetStatus struct {
	// Source is the source of the value used to scale the target resource
	Source DatadogPodAutoscalerValueSource `json:"source"`

	// GeneratedAt is the timestamp at which the recommendation was generated
	GeneratedAt metav1.Time `json:"generatedAt,omitempty"`

	// Version is the current version of the received recommendation
	Version string `json:"version"`

	// Scaled is the current number of PODs having desired resources
	Scaled *int32 `json:"scaled,omitempty"`

	// DesiredResources is the desired resources for containers
	DesiredResources []DatadogPodAutoscalerContainerResources `json:"desiredResources"`

	// PODCPURequest is the sum of CPU requests for all containers (used for display)
	PODCPURequest resource.Quantity `json:"podCPURequest"`

	// PODMemoryRequest is the sum of memory requests for all containers (used for display)
	PODMemoryRequest resource.Quantity `json:"podMemoryRequest"`
}

// DatadogPodAutoscalerVerticalActionType represents the type of action done by the controller
type DatadogPodAutoscalerVerticalActionType string

const (
	// DatadogPodAutoscalerRolloutTriggeredVerticalActionType is the action when the controller triggers a rollout of the targetRef
	DatadogPodAutoscalerRolloutTriggeredVerticalActionType DatadogPodAutoscalerVerticalActionType = "RolloutTriggered"
)

// DatadogPodAutoscalerVerticalAction represents a vertical action done by the controller
type DatadogPodAutoscalerVerticalAction struct {
	// Time is the timestamp of the action
	Time metav1.Time `json:"time"`

	// Version is the recommendation version used for the action
	Version string `json:"version"`

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
	// DatadogPodAutoscalerErrorCondition is true when a global error is encountered processing this DatadogPodAutoscaler.
	DatadogPodAutoscalerErrorCondition DatadogPodAutoscalerConditionType = "Error"

	// DatadogPodAutoscalerActiveCondition is true when the DatadogPodAutoscaler can be used for autoscaling.
	DatadogPodAutoscalerActiveCondition DatadogPodAutoscalerConditionType = "Active"

	// DatadogPodAutoscalerHorizontalAbleToRecommendCondition is true when we can get horizontal recommendation from Datadog.
	DatadogPodAutoscalerHorizontalAbleToRecommendCondition DatadogPodAutoscalerConditionType = "HorizontalAbleToRecommend"

	// DatadogPodAutoscalerHorizontalAbleToScaleCondition is true when horizontal scaling is working correctly.
	DatadogPodAutoscalerHorizontalAbleToScaleCondition DatadogPodAutoscalerConditionType = "HorizontalAbleToScale"

	// DatadogPodAutoscalerHorizontalScalingLimitedCondition is true when horizontal scaling is limited by constraints.
	DatadogPodAutoscalerHorizontalScalingLimitedCondition DatadogPodAutoscalerConditionType = "HorizontalScalingLimited"

	// DatadogPodAutoscalerVerticalAbleToRecommendCondition is true when we can ge vertical recommendation from Datadog.
	DatadogPodAutoscalerVerticalAbleToRecommendCondition DatadogPodAutoscalerConditionType = "VerticalAbleToRecommend"

	// DatadogPodAutoscalerVerticalAbleToApply is true when we can rollout the targetRef to pick up new resources.
	DatadogPodAutoscalerVerticalAbleToApply DatadogPodAutoscalerConditionType = "VerticalAbleToApply"
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
//+kubebuilder:resource:shortName=dpa
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Apply Mode",type="string",JSONPath=".spec.policy.applyMode"
//+kubebuilder:printcolumn:name="Active",type="string",JSONPath=".status.conditions[?(@.type=='Active')].status"
//+kubebuilder:printcolumn:name="In Error",type="string",JSONPath=".status.conditions[?(@.type=='Error')].status"
//+kubebuilder:printcolumn:name="Desired Replicas",type="integer",JSONPath=".status.horizontal.target.desiredReplicas"
//+kubebuilder:printcolumn:name="Generated",type="date",JSONPath=".status.horizontal.target.generatedAt"
//+kubebuilder:printcolumn:name="Able to Scale",type="string",JSONPath=".status.conditions[?(@.type=='HorizontalAbleToScale')].status"
//+kubebuilder:printcolumn:name="Last Scale",type="date",JSONPath=".status.horizontal.lastAction.time"
//+kubebuilder:printcolumn:name="Target CPU Req",type="string",JSONPath=".status.vertical.target.podCPURequest"
//+kubebuilder:printcolumn:name="Target Memory Req",type="string",JSONPath=".status.vertical.target.podMemoryRequest"
//+kubebuilder:printcolumn:name="Generated",type="date",JSONPath=".status.vertical.target.generatedAt"
//+kubebuilder:printcolumn:name="Able to Apply",type="string",JSONPath=".status.conditions[?(@.type=='VerticalAbleToApply')].status"
//+kubebuilder:printcolumn:name="Last Trigger",type="date",JSONPath=".status.vertical.lastAction.time"

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
