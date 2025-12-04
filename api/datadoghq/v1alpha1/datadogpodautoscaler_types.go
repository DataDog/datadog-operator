// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

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
//   recommendationOptions:
//     oomBumpUpRatio: 1.2

// DatadogPodAutoscalerSpec defines the desired state of DatadogPodAutoscaler
type DatadogPodAutoscalerSpec struct {
	// TargetRef is the reference to the resource to scale.
	TargetRef autoscalingv2.CrossVersionObjectReference `json:"targetRef"`

	// Owner defines the source of truth for this object (local or remote)
	// Value needs to be set when a DatadogPodAutoscaler object is created.
	Owner common.DatadogPodAutoscalerOwner `json:"owner"`

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
	Targets []common.DatadogPodAutoscalerObjective `json:"targets,omitempty"`

	// Constraints defines constraints that should always be respected.
	Constraints *common.DatadogPodAutoscalerConstraints `json:"constraints,omitempty"`

	// RecommendationOptions defines options for how recommendations are generated.
	// +optional
	RecommendationOptions *common.DatadogPodAutoscalerRecommendationOptions `json:"recommendationOptions,omitempty"`
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
	Update *common.DatadogPodAutoscalerUpdatePolicy `json:"update,omitempty"`

	// Upscale defines the policy to scale up the target resource.
	Upscale *common.DatadogPodAutoscalerScalingPolicy `json:"upscale,omitempty"`

	// Downscale defines the policy to scale down the target resource.
	Downscale *common.DatadogPodAutoscalerScalingPolicy `json:"downscale,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=dpa
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Apply Mode",type="string",JSONPath=".spec.policy.applyMode"
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
// DatadogPodAutoscaler is the Schema for the datadogpodautoscalers API
type DatadogPodAutoscaler struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatadogPodAutoscalerSpec          `json:"spec,omitempty"`
	Status common.DatadogPodAutoscalerStatus `json:"status,omitempty"`
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
