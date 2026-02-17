// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DatadogPodAutoscalerOwner defines the source of truth for this object (local or remote)
// +kubebuilder:validation:Enum:=Local;Remote
type DatadogPodAutoscalerOwner string

const (
	// DatadogPodAutoscalerLocalOwner states that this `DatadogPodAutoscaler` object is created/managed outside of the Datadog app.
	DatadogPodAutoscalerLocalOwner DatadogPodAutoscalerOwner = "Local"

	// DatadogPodAutoscalerRemoteOwner states that this `DatadogPodAutoscaler` object is created/managed in the Datadog app.
	DatadogPodAutoscalerRemoteOwner DatadogPodAutoscalerOwner = "Remote"
)

// DatadogPodAutoscalerUpdateStrategy defines the mode of the update policy.
// +kubebuilder:validation:Enum:=Auto;Disabled
type DatadogPodAutoscalerUpdateStrategy string

const (
	// DatadogPodAutoscalerAutoUpdateStrategy is the default mode.
	DatadogPodAutoscalerAutoUpdateStrategy DatadogPodAutoscalerUpdateStrategy = "Auto"

	// DatadogPodAutoscalerDisabledUpdateStrategy will disable the update of the target workload.
	DatadogPodAutoscalerDisabledUpdateStrategy DatadogPodAutoscalerUpdateStrategy = "Disabled"
)

// DatadogPodAutoscalerUpdatePolicy defines the policy to update the target workload.
// +kubebuilder:object:generate=true
type DatadogPodAutoscalerUpdatePolicy struct {
	// Strategy defines the mode of the update policy.
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

// DatadogPodAutoscalerScalingPolicy defines the policy to scale the target workload.
// +kubebuilder:object:generate=true
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

	// StabilizationWindowSeconds is the number of seconds the controller should lookback at previous recommendations
	// before deciding to apply a new one. Defaults to 0.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=3600
	StabilizationWindowSeconds int32 `json:"stabilizationWindowSeconds,omitempty"`
}

// DatadogPodAutoscalerScalingRuleType defines how a scaling rule value should be interpreted.
// +kubebuilder:validation:Enum:=Pods;Percent
type DatadogPodAutoscalerScalingRuleType string

const (
	// DatadogPodAutoscalerPodsScalingRuleType specifies a change in the absolute number of pods compared to the starting number of pods.
	DatadogPodAutoscalerPodsScalingRuleType DatadogPodAutoscalerScalingRuleType = "Pods"

	// DatadogPodAutoscalerPercentScalingRuleType specifies a relative amount of change compared to the starting number of pods.
	DatadogPodAutoscalerPercentScalingRuleType DatadogPodAutoscalerScalingRuleType = "Percent"
)

// DatadogPodAutoscalerScalingRule defines rules for horizontal scaling that should be true for a certain amount of time.
// +kubebuilder:object:generate=true
type DatadogPodAutoscalerScalingRule struct {
	// Type is used to specify the scaling policy.
	Type DatadogPodAutoscalerScalingRuleType `json:"type"`

	// Value contains the amount of change which is permitted by the policy.
	// Setting it to 0 will prevent any scaling in this direction.
	// +kubebuilder:validation:Minimum=0
	Value int32 `json:"value"`

	// PeriodSeconds specifies the window of time for which the policy should hold true.
	// PeriodSeconds must be greater than zero and less than or equal to 3600 (1 hour).
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=3600
	PeriodSeconds int32 `json:"periodSeconds"`
}

// DatadogPodAutoscalerObjectiveType defines the type of the objective.
// +kubebuilder:validation:Enum:=PodResource;ContainerResource;CustomQuery
type DatadogPodAutoscalerObjectiveType string

const (
	// DatadogPodAutoscalerPodResourceObjectiveType allows to set pod-level resource objectives.
	DatadogPodAutoscalerPodResourceObjectiveType DatadogPodAutoscalerObjectiveType = "PodResource"

	// DatadogPodAutoscalerContainerResourceObjectiveType allows to set container-level resource objectives.
	DatadogPodAutoscalerContainerResourceObjectiveType DatadogPodAutoscalerObjectiveType = "ContainerResource"

	// DatadogPodAutoscalerCustomQueryObjectiveType allows to set controller-level objectives.
	DatadogPodAutoscalerCustomQueryObjectiveType DatadogPodAutoscalerObjectiveType = "CustomQuery"
)

// DatadogPodAutoscalerObjective defines the objectives to reach and maintain for the target workload.
// +kubebuilder:object:generate=true
type DatadogPodAutoscalerObjective struct {
	// Type sets the type of the objective.
	Type DatadogPodAutoscalerObjectiveType `json:"type"`

	// PodResource allows to set a pod-level resource objective.
	PodResource *DatadogPodAutoscalerPodResourceObjective `json:"podResource,omitempty"`

	// ContainerResource allows to set a container-level resource objective.
	ContainerResource *DatadogPodAutoscalerContainerResourceObjective `json:"containerResource,omitempty"`

	// CustomQuery allows to set a controller-level objective.
	CustomQuery *DatadogPodAutoscalerCustomQueryObjective `json:"customQuery,omitempty"`
}

// DatadogPodAutoscalerPodResourceObjective defines a pod-level resource objective (for instance, CPU Utilization at 80%)
// For pod-level objectives, resources are the sum of all containers resources.
// Utilization is computed from sum(usage) / sum(requests).
// +kubebuilder:object:generate=true
type DatadogPodAutoscalerPodResourceObjective struct {
	// Name is the name of the resource.
	// +kubebuilder:validation:Enum:=cpu;memory
	Name corev1.ResourceName `json:"name"`

	// Value is the value of the objective.
	Value DatadogPodAutoscalerObjectiveValue `json:"value"`
}

// DatadogPodAutoscalerContainerResourceObjective defines a container-level resource objective (for instance, CPU Utilization for container named "foo" at 80%)
// +kubebuilder:object:generate=true
type DatadogPodAutoscalerContainerResourceObjective struct {
	// Name is the name of the resource.
	// +kubebuilder:validation:Enum:=cpu;memory
	Name corev1.ResourceName `json:"name"`

	// Value is the value of the objective
	Value DatadogPodAutoscalerObjectiveValue `json:"value"`

	// Container is the name of the container.
	Container string `json:"container"`
}

// DatadogPodAutoscalerCustomQueryObjective defines a controller-level objective
// +kubebuilder:object:generate=true
type DatadogPodAutoscalerCustomQueryObjective struct {
	// Request is the timeseries query to use for the objective.
	Request DatadogPodAutoscalerTimeseriesFormulaRequest `json:"request"`

	// Value is the value of the objective
	Value DatadogPodAutoscalerObjectiveValue `json:"value"`

	// Window is the time duration over which the query is computed. It should contain at least one full sample.
	Window metav1.Duration `json:"window"`
}

// DatadogPodAutoscalerTimeseriesFormulaRequest is a subset of the v2 timeseries query API (metrics only).
// It mirrors the OpenAPI "DatadogPodAutoscalerTimeseriesFormulaRequestAttributes" fields relevant to autoscaling.
// Reference: https://github.com/DataDog/datadog-api-spec/blob/94d1542b31ad0df1da915bae84686b13ba1a65ae/spec/v2/query.yaml#L124
// +kubebuilder:object:generate=true
type DatadogPodAutoscalerTimeseriesFormulaRequest struct {
	// Formula to compute (optional).
	// +optional
	Formula string `json:"formula,omitempty"`

	// Queries is a list of timeseries queries to use for the objective.
	// At least one query must be specified
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	Queries []DatadogPodAutoscalerTimeseriesQuery `json:"queries"`
}

// +kubebuilder:validation:Enum:=Metrics;ApmMetrics
type DatadogPodAutoscalerMetricsDataSource string

const (
	// DatadogPodAutoscalerMetricsDataSourceMetrics defines the source of the timeseries query as a standard Datadog metrics query.
	DatadogPodAutoscalerMetricsDataSourceMetrics DatadogPodAutoscalerMetricsDataSource = "Metrics"

	// DatadogPodAutoscalerMetricsDataSourceApmMetrics defines the source of the timeseries query as an APM metrics query.
	DatadogPodAutoscalerMetricsDataSourceApmMetrics DatadogPodAutoscalerMetricsDataSource = "ApmMetrics"
)

// TimeseriesQuery is a discriminated union. Only Metrics and APMMetrics are supported for autoscaling.
// +kubebuilder:object:generate=true
type DatadogPodAutoscalerTimeseriesQuery struct {
	// Optional variable name ("a", "b", etc.) to reference in formulas.
	// +optional
	Name string `json:"name"`

	// Source defines the source of the timeseries query.
	Source DatadogPodAutoscalerMetricsDataSource `json:"source"`

	// Metrics is a standard Datadog metrics query.
	// +optional
	Metrics *DatadogPodAutoscalerMetricsTimeseriesQuery `json:"metrics,omitempty"`

	// ApmMetrics is allows to query APM metrics.
	// +optional
	ApmMetrics *DatadogPodAutoscalerApmMetricsTimeseriesQuery `json:"apmMetrics,omitempty"`
}

// +kubebuilder:object:generate=true
type DatadogPodAutoscalerMetricsTimeseriesQuery struct {
	// Classic Datadog metrics query, e.g. "avg:system.cpu.user{*} by {env}".
	// +kubebuilder:validation:MinLength=1
	Query string `json:"query"`
}

// +kubebuilder:object:generate=true
type DatadogPodAutoscalerApmMetricsTimeseriesQuery struct {
	// Stat defines the statistic to compute for the APM metrics query.
	Stat DatadogPodAutoscalerApmMetricsStat `json:"stat"`

	// Service is the name of the service to query.
	// +optional
	Service string `json:"service,omitempty"`

	// ResourceName is the name of the resource to query.
	// +optional
	ResourceName string `json:"resourceName,omitempty"`

	// ResourceHash is a fingerprint of the resource name that can be used to identify the resource instead of the resource name.
	// +optional
	ResourceHash string `json:"resourceHash,omitempty"`

	// OperationName is the name of the operation to query.
	// +optional
	OperationName string `json:"operationName,omitempty"`

	// GroupBy is the list of tags to group by.
	// +optional
	GroupBy []string `json:"groupBy,omitempty"`

	// QueryFilter is the filter to apply to the query.
	// +optional
	QueryFilter string `json:"queryFilter,omitempty"`

	// SpanKind is the kind of span to query.
	// +optional
	SpanKind string `json:"spanKind,omitempty"`
}

// DatadogPodAutoscalerApmMetricsStat represents the statistic to compute for an APM metrics query.
// +kubebuilder:validation:Enum:=error_rate;errors;errors_per_second;hits;hits_per_second;apdex;latency_avg;latency_max;latency_p50;latency_p75;latency_p90;latency_p95;latency_p99;latency_p999;latency_distribution;total_time
type DatadogPodAutoscalerApmMetricsStat string

const (
	APM_METRIC_STAT_ERROR_RATE           DatadogPodAutoscalerApmMetricsStat = "error_rate"
	APM_METRIC_STAT_ERRORS               DatadogPodAutoscalerApmMetricsStat = "errors"
	APM_METRIC_STAT_ERRORS_PER_SECOND    DatadogPodAutoscalerApmMetricsStat = "errors_per_second"
	APM_METRIC_STAT_HITS                 DatadogPodAutoscalerApmMetricsStat = "hits"
	APM_METRIC_STAT_HITS_PER_SECOND      DatadogPodAutoscalerApmMetricsStat = "hits_per_second"
	APM_METRIC_STAT_APDEX                DatadogPodAutoscalerApmMetricsStat = "apdex"
	APM_METRIC_STAT_LATENCY_AVG          DatadogPodAutoscalerApmMetricsStat = "latency_avg"
	APM_METRIC_STAT_LATENCY_MAX          DatadogPodAutoscalerApmMetricsStat = "latency_max"
	APM_METRIC_STAT_LATENCY_P50          DatadogPodAutoscalerApmMetricsStat = "latency_p50"
	APM_METRIC_STAT_LATENCY_P75          DatadogPodAutoscalerApmMetricsStat = "latency_p75"
	APM_METRIC_STAT_LATENCY_P90          DatadogPodAutoscalerApmMetricsStat = "latency_p90"
	APM_METRIC_STAT_LATENCY_P95          DatadogPodAutoscalerApmMetricsStat = "latency_p95"
	APM_METRIC_STAT_LATENCY_P99          DatadogPodAutoscalerApmMetricsStat = "latency_p99"
	APM_METRIC_STAT_LATENCY_P999         DatadogPodAutoscalerApmMetricsStat = "latency_p999"
	APM_METRIC_STAT_LATENCY_DISTRIBUTION DatadogPodAutoscalerApmMetricsStat = "latency_distribution"
	APM_METRIC_STAT_TOTAL_TIME           DatadogPodAutoscalerApmMetricsStat = "total_time"
)

// DatadogPodAutoscalerObjectiveValueType specifies the type of objective value.
// +kubebuilder:validation:Enum:=Utilization;AbsoluteValue
type DatadogPodAutoscalerObjectiveValueType string

const (
	// DatadogPodAutoscalerUtilizationObjectiveValueType declares an objective based on a Utilization (percentage, 0-100).
	DatadogPodAutoscalerUtilizationObjectiveValueType DatadogPodAutoscalerObjectiveValueType = "Utilization"
	// DatadogPodAutoscalerAbsoluteValueObjectiveValueType declares an objective based on an AbsoluteValue.
	DatadogPodAutoscalerAbsoluteValueObjectiveValueType DatadogPodAutoscalerObjectiveValueType = "AbsoluteValue"
)

// DatadogPodAutoscalerObjectiveValue defines the target value of the objective.
// +kubebuilder:object:generate=true
type DatadogPodAutoscalerObjectiveValue struct {
	// Type specifies how the value is expressed (possible values: Utilization, AbsoluteValue).
	Type DatadogPodAutoscalerObjectiveValueType `json:"type"`

	// Utilization defines a percentage of the target compared to requested workload
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	Utilization *int32 `json:"utilization,omitempty"`

	// AbsoluteValue defines a target as an absolute value divided by the number of running pods.
	// Use a plain number (e.g., "11" or "11.5").
	// Represented as a resource.Quantity to avoid floating point in CRDs.
	AbsoluteValue *resource.Quantity `json:"absoluteValue,omitempty"`
}

// DatadogPodAutoscalerConstraints defines constraints that should always be respected.
// +kubebuilder:object:generate=true
type DatadogPodAutoscalerConstraints struct {
	// MinReplicas is the lower limit for the number of pod replicas. Needs to be >= 1. Defaults to 1.
	// +kubebuilder:validation:Minimum=1
	MinReplicas *int32 `json:"minReplicas,omitempty"`

	// MaxReplicas is the upper limit for the number of POD replicas. Needs to be >= minReplicas.
	// +kubebuilder:validation:Minimum=1
	MaxReplicas *int32 `json:"maxReplicas,omitempty"`

	// Containers defines constraints for the containers.
	// +patchMergeKey=name
	// +patchStrategy=merge
	Containers []DatadogPodAutoscalerContainerConstraints `json:"containers,omitempty"`
}

// DatadogPodAutoscalerContainerControlledValues specifies which resource values should be controlled.
// +kubebuilder:validation:Enum:=RequestsAndLimits;RequestsOnly
type DatadogPodAutoscalerContainerControlledValues string

const (
	DatadogPodAutoscalerContainerControlledValuesRequestsAndLimits DatadogPodAutoscalerContainerControlledValues = "RequestsAndLimits"
	DatadogPodAutoscalerContainerControlledValuesRequestsOnly      DatadogPodAutoscalerContainerControlledValues = "RequestsOnly"
)

// DatadogPodAutoscalerContainerConstraints defines constraints that should always be respected for a container.
// If no constraints are set, it enables resource scaling for all containers without any constraints.
// +kubebuilder:object:generate=true
type DatadogPodAutoscalerContainerConstraints struct {
	// Name is the name of the container. Can be "*" to apply to all containers.
	Name string `json:"name"`

	// Enabled, if false, allows one to disable resource autoscaling for the container. Defaults to true.
	Enabled *bool `json:"enabled,omitempty"`

	// Requests defines the constraints for the requests of the container.
	// WARNING: Deprecated
	// +deprecated: use MinAllowed and MaxAllowed instead
	Requests *DatadogPodAutoscalerContainerResourceConstraints `json:"requests,omitempty"`

	// MinAllowed is the lower limit for the requests of the container.
	// +optional
	MinAllowed corev1.ResourceList `json:"minAllowed,omitempty"`

	// MaxAllowed is the upper limit for the requests of the container.
	// +optional
	MaxAllowed corev1.ResourceList `json:"maxAllowed,omitempty"`

	// Specifies the resources for which recommendations will be computed.
	// If not specified, it defaults to CPU and Memory.
	// If an empty list is provided, no resource will be controlled (equivalent to Enabled=false).
	// +patchStrategy=merge
	ControlledResources []corev1.ResourceName `json:"controlledResources,omitempty"`

	// Specifies whether recommendations are made to Requests and Limits (RequestsAndLimits) or Requests only (RequestsOnly).
	// The default is "RequestsAndLimits".
	ControlledValues *DatadogPodAutoscalerContainerControlledValues `json:"controlledValues,omitempty"`
}

// DatadogPodAutoscalerContainerResourceConstraints defines constraints for the resources recommended for a container.
// +kubebuilder:object:generate=true
type DatadogPodAutoscalerContainerResourceConstraints struct {
	// MinAllowed is the lower limit for the requests of the container.
	// +optional
	MinAllowed corev1.ResourceList `json:"minAllowed,omitempty"`

	// MaxAllowed is the upper limit for the requests of the container.
	// +optional
	MaxAllowed corev1.ResourceList `json:"maxAllowed,omitempty"`
}

// DatadogPodAutoscalerOptions defines optional behavior modifications for the autoscaler.
// +kubebuilder:object:generate=true
type DatadogPodAutoscalerOptions struct {
	// OutOfMemory configures behavior when OOM events are detected.
	// +optional
	OutOfMemory *DatadogPodAutoscalerOutOfMemoryOptions `json:"outOfMemory,omitempty"`
}

// DatadogPodAutoscalerOutOfMemoryOptions configures the behavior when out-of-memory events are detected.
// +kubebuilder:object:generate=true
type DatadogPodAutoscalerOutOfMemoryOptions struct {
	// BumpUpRatio defines the ratio to multiply memory by when OOM is detected.
	// For example, "1.2" means increase memory by 20%.
	// Represented as a resource.Quantity to avoid floating point in CRDs.
	// +optional
	BumpUpRatio *resource.Quantity `json:"bumpUpRatio,omitempty"`
}

// DatadogPodAutoscalerStatus defines the observed state of DatadogPodAutoscaler
// +kubebuilder:object:generate=true
type DatadogPodAutoscalerStatus struct {
	// Vertical is the status of the vertical scaling, if activated.
	// +optional
	Vertical *DatadogPodAutoscalerVerticalStatus `json:"vertical,omitempty"`

	// Horizontal is the status of the horizontal scaling, if activated.
	// +optional
	Horizontal *DatadogPodAutoscalerHorizontalStatus `json:"horizontal,omitempty"`

	// CurrentReplicas is the current number of pods for the targetRef observed by the controller.
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

// DatadogPodAutoscalerValueSource defines the source of the value used to scale the target workload.
type DatadogPodAutoscalerValueSource string

const (
	// DatadogPodAutoscalerAutoscalingValueSource is a recommendation that comes from active autoscaling.
	DatadogPodAutoscalerAutoscalingValueSource DatadogPodAutoscalerValueSource = "Autoscaling"

	// DatadogPodAutoscalerManualValueSource is a recommendation that comes from manually applying a recommendation.
	DatadogPodAutoscalerManualValueSource DatadogPodAutoscalerValueSource = "Manual"

	// DatadogPodAutoscalerLocalValueSource is a recommendation that comes from the local fallback.
	DatadogPodAutoscalerLocalValueSource DatadogPodAutoscalerValueSource = "Local"

	// DatadogPodAutoscalerExternalValueSource is a recommendation that comes from an external source.
	DatadogPodAutoscalerExternalValueSource DatadogPodAutoscalerValueSource = "External"
)

// DatadogPodAutoscalerHorizontalStatus defines the status of the horizontal scaling
// +kubebuilder:object:generate=true
type DatadogPodAutoscalerHorizontalStatus struct {
	// Target is the current target of the horizontal scaling
	Target *DatadogPodAutoscalerHorizontalRecommendation `json:"target,omitempty"`

	// LastActions are the last successful actions done by the controller
	LastActions []DatadogPodAutoscalerHorizontalAction `json:"lastActions,omitempty"`

	// LastRecommendations stores the most recent recommendations
	LastRecommendations []DatadogPodAutoscalerHorizontalRecommendation `json:"lastRecommendations,omitempty"`
}

// DatadogPodAutoscalerHorizontalRecommendation defines a horizontal scaling recommendation
// +kubebuilder:object:generate=true
type DatadogPodAutoscalerHorizontalRecommendation struct {
	// Source is the source of the value used to scale the target workload
	// +optional
	Source DatadogPodAutoscalerValueSource `json:"source,omitempty"`

	// GeneratedAt is the timestamp at which the recommendation was generated
	GeneratedAt metav1.Time `json:"generatedAt,omitempty"`

	// Replicas is the recommended number of replicas for the workload
	Replicas int32 `json:"desiredReplicas"`
}

// DatadogPodAutoscalerHorizontalAction represents a horizontal action done by the controller
// +kubebuilder:object:generate=true
type DatadogPodAutoscalerHorizontalAction struct {
	// Time is the timestamp of the action
	Time metav1.Time `json:"time"`

	// FromReplicas is the number of replicas before the action
	FromReplicas int32 `json:"replicas"`

	// ToReplicas is the effective number of replicas after the action
	ToReplicas int32 `json:"toReplicas"`

	// RecommendedReplicas is the original number of replicas recommended by Datadog
	RecommendedReplicas *int32 `json:"recommendedReplicas,omitempty"`

	// LimitedReason is the reason why the action was limited (that is ToReplicas != RecommendedReplicas)
	LimitedReason *string `json:"limitedReason,omitempty"`
}

// DatadogPodAutoscalerVerticalStatus defines the status of the vertical scaling
// +kubebuilder:object:generate=true
type DatadogPodAutoscalerVerticalStatus struct {
	// Target is the current target of the vertical scaling
	Target *DatadogPodAutoscalerVerticalTargetStatus `json:"target,omitempty"`

	// LastAction is the last successful action done by the controller
	LastAction *DatadogPodAutoscalerVerticalAction `json:"lastAction,omitempty"`
}

// DatadogPodAutoscalerVerticalTargetStatus defines the current target of the vertical scaling
// +kubebuilder:object:generate=true
type DatadogPodAutoscalerVerticalTargetStatus struct {
	// Source is the source of the value used to scale the target resource
	Source DatadogPodAutoscalerValueSource `json:"source"`

	// GeneratedAt is the timestamp at which the recommendation was generated
	GeneratedAt metav1.Time `json:"generatedAt,omitempty"`

	// Version is the current version of the received recommendation
	Version string `json:"version"`

	// Scaled is the current number of pods having desired resources
	Scaled *int32 `json:"scaled,omitempty"`

	// DesiredResources is the desired resources for containers
	DesiredResources []DatadogPodAutoscalerContainerResources `json:"desiredResources"`

	// PodCPURequest is the sum of CPU requests for all containers (used for display)
	PodCPURequest resource.Quantity `json:"podCPURequest"`

	// PodMemoryRequest is the sum of memory requests for all containers (used for display)
	PodMemoryRequest resource.Quantity `json:"podMemoryRequest"`
}

// DatadogPodAutoscalerVerticalActionType represents the type of action done by the controller
type DatadogPodAutoscalerVerticalActionType string

const (
	// DatadogPodAutoscalerRolloutTriggeredVerticalActionType is the action when the controller triggers a rollout of the targetRef
	DatadogPodAutoscalerRolloutTriggeredVerticalActionType DatadogPodAutoscalerVerticalActionType = "RolloutTriggered"
)

// DatadogPodAutoscalerVerticalAction represents a vertical action done by the controller
// +kubebuilder:object:generate=true
type DatadogPodAutoscalerVerticalAction struct {
	// Time is the timestamp of the action
	Time metav1.Time `json:"time"`

	// Version is the version of the recommendation used for the action
	Version string `json:"version"`

	// Type is the type of action
	Type DatadogPodAutoscalerVerticalActionType `json:"type"`
}

// +kubebuilder:object:generate=true
type DatadogPodAutoscalerContainerResources struct {
	// Name is the name of the container
	Name string `json:"name"`

	// Limits describes the maximum amount of compute resources allowed.
	// +optional
	Limits corev1.ResourceList `json:"limits,omitempty"`

	// Requests describes the requested amount of compute resources.
	// +optional
	Requests corev1.ResourceList `json:"requests,omitempty"`
}

// DatadogPodAutoscalerConditionType is the type used to represent a DatadogPodAutoscaler condition
type DatadogPodAutoscalerConditionType string

const (
	// DatadogPodAutoscalerErrorCondition is true when a global error is encountered processing this DatadogPodAutoscaler.
	DatadogPodAutoscalerErrorCondition DatadogPodAutoscalerConditionType = "Error"

	// DatadogPodAutoscalerActiveCondition is true when the DatadogPodAutoscaler can be used for autoscaling.
	DatadogPodAutoscalerActiveCondition DatadogPodAutoscalerConditionType = "Active"

	// DatadogPodAutoscalerHorizontalAbleToRecommendCondition is true when a horizontal recommendation can be received from Datadog.
	DatadogPodAutoscalerHorizontalAbleToRecommendCondition DatadogPodAutoscalerConditionType = "HorizontalAbleToRecommend"

	// DatadogPodAutoscalerHorizontalAbleToScaleCondition is true when horizontal scaling is working correctly.
	DatadogPodAutoscalerHorizontalAbleToScaleCondition DatadogPodAutoscalerConditionType = "HorizontalAbleToScale"

	// DatadogPodAutoscalerHorizontalScalingLimitedCondition is true when horizontal scaling is limited by constraints.
	DatadogPodAutoscalerHorizontalScalingLimitedCondition DatadogPodAutoscalerConditionType = "HorizontalScalingLimited"

	// DatadogPodAutoscalerVerticalAbleToRecommendCondition is true when a vertical recommendation can be received from Datadog.
	DatadogPodAutoscalerVerticalAbleToRecommendCondition DatadogPodAutoscalerConditionType = "VerticalAbleToRecommend"

	// DatadogPodAutoscalerVerticalAbleToApply is true when the targetRef can be rolled out to pick up new resources.
	DatadogPodAutoscalerVerticalAbleToApply DatadogPodAutoscalerConditionType = "VerticalAbleToApply"

	// DatadogPodAutoscalerVerticalScalingLimitedCondition is true when vertical scaling is limited by constraints.
	DatadogPodAutoscalerVerticalScalingLimitedCondition DatadogPodAutoscalerConditionType = "VerticalScalingLimited"
)

// DatadogPodAutoscalerCondition describes the state of DatadogPodAutoscaler.
// +kubebuilder:object:generate=true
type DatadogPodAutoscalerCondition struct {
	// DatadogPodAutoscalerConditionType is the type of DatadogPodAutoscaler condition.
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
