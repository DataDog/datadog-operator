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
//   options:
//     outOfMemory:
//       bumpUpRatio: "1.2"
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
	ApplyPolicy *DatadogPodAutoscalerApplyPolicy `json:"applyPolicy,omitempty"`

	// Objectives are the objectives used to compute recommendations.
	Objectives []common.DatadogPodAutoscalerObjective `json:"objectives,omitempty"`

	// Fallback defines the policy to use for fallback recommendations.
	Fallback *DatadogFallbackPolicy `json:"fallback,omitempty"`

	// Constraints defines the constraints to be enforced on recommendations.
	Constraints *common.DatadogPodAutoscalerConstraints `json:"constraints,omitempty"`

	// Options are optional settings for this autoscaler.
	Options *common.DatadogPodAutoscalerOptions `json:"options,omitempty"`
}

// Type aliases for backward compatibility.
// These types are defined in the common package to enable reusage.
// The +kubebuilder:object:generate=false marker prevents controller-gen from generating
// duplicate deepcopy methods for these aliases, as the methods already exist in the common package.

// +kubebuilder:object:generate=false
type DatadogPodAutoscalerApplyMode = common.DatadogPodAutoscalerApplyModeV2

// +kubebuilder:object:generate=false
type DatadogPodAutoscalerFallbackDirection = common.DatadogPodAutoscalerFallbackDirection

// +kubebuilder:object:generate=false
type DatadogPodAutoscalerApplyPolicy = common.DatadogPodAutoscalerApplyPolicy

// +kubebuilder:object:generate=false
type DatadogFallbackPolicy = common.DatadogFallbackPolicy

// +kubebuilder:object:generate=false
type DatadogPodAutoscalerHorizontalFallbackPolicy = common.DatadogPodAutoscalerHorizontalFallbackPolicy

// +kubebuilder:object:generate=false
type HorizontalFallbackTriggers = common.HorizontalFallbackTriggers

const (
	DatadogPodAutoscalerApplyModeApply   = common.DatadogPodAutoscalerApplyModeV2Apply
	DatadogPodAutoscalerApplyModePreview = common.DatadogPodAutoscalerApplyModeV2Preview

	DatadogPodAutoscalerFallbackDirectionScaleUp   = common.DatadogPodAutoscalerFallbackDirectionScaleUp
	DatadogPodAutoscalerFallbackDirectionScaleDown = common.DatadogPodAutoscalerFallbackDirectionScaleDown
	DatadogPodAutoscalerFallbackDirectionAll       = common.DatadogPodAutoscalerFallbackDirectionAll
)

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
