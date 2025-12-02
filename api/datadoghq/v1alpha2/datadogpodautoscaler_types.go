// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha2

import (
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
)

// spec:
//   targetRef:
//     apiVersion: apps/v1
//     kind: Deployment
//     name: test
//   owner: local
//   remoteVersion: 1
//   applyPolicy:
//     mode: Apply | Preview
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
//   objectives:
//     - type: PodResource
//       podResource:
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
//     - type: CustomQuery
//       customQueryObjective:
//         query:
//           formulas:
//             - "query1 / query2"
//           queries:
//             -
//                 name: query1
//                 dataSource: metrics
//                 metrics:
//                   query: "avg:system.cpu.user{*}"
//             -
//                 name: query2
//                 dataSource: apm_metrics
//                 apm_metrics:
//                   	stat: "latency_avg"
//						service: "my-service"
// 				   		query_filter: "account:prod"
//   fallback:
// 	   horizontal:
//       enabled: true
//       triggers:
//         staleRecommendationThresholdSeconds: 600
//		 objective:
//         type: PodResource
//         podResource:
//           name: cpu
//           value:
//             type: Utilization
//             utilization: 8
//   constraints:
//     minReplicas: 1
//     maxReplicas: 10
//     containers:
//       - name: "*"
//         enabled: true
//         requests:
//           minAllowed:
//           maxAllowed:

// DatadogPodAutoscalerSpec defines the desired state of DatadogPodAutoscaler
type DatadogPodAutoscalerSpec struct {
	// TargetRef is the reference to the resource to scale.
	TargetRef autoscalingv2.CrossVersionObjectReference `json:"targetRef"`

	// Owner defines the source of truth for this object (local or remote).
	// Value must be set when a DatadogPodAutoscaler object is created.
	Owner common.DatadogPodAutoscalerOwner `json:"owner"`

	// RemoteVersion is the version of the .Spec currently stored in this object.
	// This is only set if the owner is Remote.
	RemoteVersion *uint64 `json:"remoteVersion,omitempty"`

	// ApplyPolicy defines how recommendations should be applied.
	// +optional
	// +kubebuilder:default={}
	ApplyPolicy *DatadogPodAutoscalerApplyPolicy `json:"applyPolicy,omitempty"`

	// Objectives are the objectives to reach and maintain for the target resource.
	// Default to a single objective to maintain 80% POD CPU utilization.
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	Objectives []common.DatadogPodAutoscalerObjective `json:"objectives,omitempty"`

	// Fallback defines how recommendations should be applied when in fallback mode.
	// +optional
	// +kubebuilder:default={}
	Fallback *DatadogFallbackPolicy `json:"fallback,omitempty"`

	// Constraints defines constraints that should always be respected.
	Constraints *common.DatadogPodAutoscalerConstraints `json:"constraints,omitempty"`
}

// DatadogPodAutoscalerApplyMode specifies if the controller should apply recommendations.
// +kubebuilder:validation:Enum:=Apply;Preview
type DatadogPodAutoscalerApplyMode string

// DatadogPodAutoscalerFallbackDirection specifies the direction that fallback recommendations can be applied.
// +kubebuilder:validation:Enum:=ScaleUp;ScaleDown;All
type DatadogPodAutoscalerFallbackDirection string

const (
	// DatadogPodAutoscalerApplyModeApply allows the controller to apply all recommendations (regular and manual)
	DatadogPodAutoscalerApplyModeApply DatadogPodAutoscalerApplyMode = "Apply"

	// DatadogPodAutoscalerApplyModePreview doesn't allow the controller to apply any recommendations
	DatadogPodAutoscalerApplyModePreview DatadogPodAutoscalerApplyMode = "Preview"

	// DatadogPodAutoscalerFallbackDirectionScaleUp allows the controller to apply fallback recommendations to scale up the target resource.
	DatadogPodAutoscalerFallbackDirectionScaleUp DatadogPodAutoscalerFallbackDirection = "ScaleUp"

	// DatadogPodAutoscalerFallbackDirectionScaleDown allows the controller to apply fallback recommendations to scale down the target resource.
	DatadogPodAutoscalerFallbackDirectionScaleDown DatadogPodAutoscalerFallbackDirection = "ScaleDown"

	// DatadogPodAutoscalerFallbackDirectionAll allows the controller to apply fallback recommendations to scale up or down the target resource.
	DatadogPodAutoscalerFallbackDirectionAll DatadogPodAutoscalerFallbackDirection = "All"
)

// DatadogPodAutoscalerApplyPolicy defines how recommendations should be applied.
type DatadogPodAutoscalerApplyPolicy struct {
	// Mode determines recommendations that should be applied by the controller:
	// - Apply: Apply all recommendations.
	// - Preview: Recommendations are received and visible through .Status, but the controller does not apply them.
	// It's also possible to selectively deactivate upscale, downscale or update actions thanks to the `ScaleUp`, `ScaleDown` and `Update` fields.
	// +optional
	// +kubebuilder:default=Apply
	Mode DatadogPodAutoscalerApplyMode `json:"mode"`

	// Update defines the policy for updating the target resource.
	Update *common.DatadogPodAutoscalerUpdatePolicy `json:"update,omitempty"`

	// ScaleUp defines the policy to scale up the target resource.
	ScaleUp *common.DatadogPodAutoscalerScalingPolicy `json:"scaleUp,omitempty"`

	// ScaleDown defines the policy to scale down the target resource.
	ScaleDown *common.DatadogPodAutoscalerScalingPolicy `json:"scaleDown,omitempty"`
}

// DatadogFallbackPolicy configures the behavior during fallback mode.
type DatadogFallbackPolicy struct {
	// Horizontal configures the behavior during horizontal fallback mode.
	// +optional
	// +kubebuilder:default={}
	Horizontal DatadogPodAutoscalerHorizontalFallbackPolicy `json:"horizontal,omitempty"`
}

// DatadogPodAutoscalerHorizontalFallbackPolicy defines how recommendations should be applied in fallback mode.
type DatadogPodAutoscalerHorizontalFallbackPolicy struct {
	// Enabled determines whether recommendations should be applied by the controller:
	// +optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled"`

	// Triggers defines the triggers that will generate recommendations.
	// +optional
	// +kubebuilder:default={}
	Triggers HorizontalFallbackTriggers `json:"triggers,omitempty"`

	// Direction determines the direction that recommendations should be applied.
	// +optional
	// +kubebuilder:default=ScaleUp
	Direction DatadogPodAutoscalerFallbackDirection `json:"direction,omitempty"`

	// Objectives are the objectives to reach and maintain for the target resource in fallback mode.
	// If not set, the regular objectives will be used.
	// +listType=atomic
	Objectives []common.DatadogPodAutoscalerObjective `json:"objectives,omitempty"`
}

// HorizontalFallbackTriggers defines the triggers that will cause local fallback to be enabled.
type HorizontalFallbackTriggers struct {
	// StaleRecommendationThresholdSeconds defines the time window the controller will wait after detecting an error before applying recommendations.
	// +optional
	// +kubebuilder:default=600
	// +kubebuilder:validation:Minimum=100
	// +kubebuilder:validation:Maximum=3600
	StaleRecommendationThresholdSeconds int32 `json:"staleRecommendationThresholdSeconds,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=dpa
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Apply Mode",type="string",JSONPath=".spec.applyPolicy.mode"
// +kubebuilder:printcolumn:name="Active",type="string",JSONPath=".status.conditions[?(@.type=='Active')].status"
// +kubebuilder:printcolumn:name="In Error",type="string",JSONPath=".status.conditions[?(@.type=='Error')].status"
// +kubebuilder:printcolumn:name="Desired Replicas",type="integer",JSONPath=".status.horizontal.target.desiredReplicas"
// +kubebuilder:printcolumn:name="Generated",type="date",JSONPath=".status.horizontal.target.generatedAt"
// +kubebuilder:printcolumn:name="Able to Scale",type="string",JSONPath=".status.conditions[?(@.type=='HorizontalAbleToScale')].status"
// +kubebuilder:printcolumn:name="Last Scale",type="date",JSONPath=".status.horizontal.lastAction.time"
// +kubebuilder:printcolumn:name="Target CPU Req",type="string",JSONPath=".status.vertical.target.podCPURequest"
// +kubebuilder:printcolumn:name="Target Memory Req",type="string",JSONPath=".status.vertical.target.podMemoryRequest"
// +kubebuilder:printcolumn:name="Generated",type="date",JSONPath=".status.vertical.target.generatedAt"
// +kubebuilder:printcolumn:name="Able to Apply",type="string",JSONPath=".status.conditions[?(@.type=='VerticalAbleToApply')].status"
// +kubebuilder:printcolumn:name="Last Trigger",type="date",JSONPath=".status.vertical.lastAction.time"
// +kubebuilder:storageversion
// DatadogPodAutoscaler is the Schema for the datadogpodautoscalers API
type DatadogPodAutoscaler struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatadogPodAutoscalerSpec          `json:"spec,omitempty"`
	Status common.DatadogPodAutoscalerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DatadogPodAutoscalerList contains a list of DatadogPodAutoscalers
type DatadogPodAutoscalerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatadogPodAutoscaler `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatadogPodAutoscaler{}, &DatadogPodAutoscalerList{})
}
