// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ExtendedDaemonSetSpec defines the desired state of ExtendedDaemonSet
// +k8s:openapi-gen=true
type ExtendedDaemonSetSpec struct {
	// A label query over pods that are managed by the daemon set.
	// Must match in order to be controlled.
	// If empty, defaulted to labels on Pod template.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors
	// +optional
	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// An object that describes the pod that will be created.
	// The ExtendedDaemonSet will create exactly one copy of this pod on every node
	// that matches the template's node selector (or on every node if no node
	// selector is specified).
	// More info: https://kubernetes.io/docs/concepts/workloads/controllers/replicationcontroller#pod-template
	Template corev1.PodTemplateSpec `json:"template"`

	// Daemonset deployment strategy.
	Strategy ExtendedDaemonSetSpecStrategy `json:"strategy"`
}

// ExtendedDaemonSetSpecStrategy defines the deployment strategy of ExtendedDaemonSet.
// +k8s:openapi-gen=true
type ExtendedDaemonSetSpecStrategy struct {
	RollingUpdate ExtendedDaemonSetSpecStrategyRollingUpdate `json:"rollingUpdate,omitempty"`
	// Canary deployment configuration
	Canary *ExtendedDaemonSetSpecStrategyCanary `json:"canary,omitempty"`
	// ReconcileFrequency use to configure how often the ExtendedDeamonset will be fully reconcile, default is 10sec.
	ReconcileFrequency *metav1.Duration `json:"reconcileFrequency,omitempty"`
}

// ExtendedDaemonSetSpecStrategyRollingUpdate defines the rolling update deployment strategy of ExtendedDaemonSet.
// +k8s:openapi-gen=true
type ExtendedDaemonSetSpecStrategyRollingUpdate struct {
	// The maximum number of DaemonSet pods that can be unavailable during the
	// update. Value can be an absolute number (ex: 5) or a percentage of total
	// number of DaemonSet pods at the start of the update (ex: 10%). Absolute
	// number is calculated from percentage by rounding up.
	// This cannot be 0.
	// Default value is 1.
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`
	// MaxPodSchedulerFailure the maxinum number of not scheduled on its Node due to a
	// scheduler failure: resource constraints. Value can be an absolute number (ex: 5) or a percentage of total
	// number of DaemonSet pods at the start of the update (ex: 10%). Absolute.
	MaxPodSchedulerFailure *intstr.IntOrString `json:"maxPodSchedulerFailure,omitempty"`
	// The maxium number of pods created in parallel.
	// Default value is 250.
	MaxParallelPodCreation *int32 `json:"maxParallelPodCreation,omitempty"`
	// SlowStartIntervalDuration the duration between to 2
	// Default value is 1min.
	SlowStartIntervalDuration *metav1.Duration `json:"slowStartIntervalDuration,omitempty"`
	// SlowStartAdditiveIncrease
	// Value can be an absolute number (ex: 5) or a percentage of total
	// number of DaemonSet pods at the start of the update (ex: 10%).
	// Default value is 5.
	SlowStartAdditiveIncrease *intstr.IntOrString `json:"slowStartAdditiveIncrease,omitempty"`
}

// ExtendedDaemonSetSpecStrategyCanaryValidationMode type representing the ExtendedDaemonSetSpecStrategyCanary validation mode.
// +kubebuilder:validation:Enum=auto;manual
type ExtendedDaemonSetSpecStrategyCanaryValidationMode string

const (
	// ExtendedDaemonSetSpecStrategyCanaryValidationModeAuto the ExtendedDaemonSetSpecStrategyCanary automatic validation mode.
	ExtendedDaemonSetSpecStrategyCanaryValidationModeAuto ExtendedDaemonSetSpecStrategyCanaryValidationMode = "auto"
	// ExtendedDaemonSetSpecStrategyCanaryValidationModeManual the ExtendedDaemonSetSpecStrategyCanary manual validation mode.
	ExtendedDaemonSetSpecStrategyCanaryValidationModeManual ExtendedDaemonSetSpecStrategyCanaryValidationMode = "manual"
)

// ExtendedDaemonSetSpecStrategyCanary defines the canary deployment strategy of ExtendedDaemonSet.
// +k8s:openapi-gen=true
type ExtendedDaemonSetSpecStrategyCanary struct {
	Replicas     *intstr.IntOrString   `json:"replicas,omitempty"`
	Duration     *metav1.Duration      `json:"duration,omitempty"`
	NodeSelector *metav1.LabelSelector `json:"nodeSelector,omitempty"`
	// +listType=set
	NodeAntiAffinityKeys []string                                      `json:"nodeAntiAffinityKeys,omitempty"`
	AutoPause            *ExtendedDaemonSetSpecStrategyCanaryAutoPause `json:"autoPause,omitempty"`
	AutoFail             *ExtendedDaemonSetSpecStrategyCanaryAutoFail  `json:"autoFail,omitempty"`
	// NoRestartsDuration defines min duration since last restart to end the canary phase.
	NoRestartsDuration *metav1.Duration `json:"noRestartsDuration,omitempty"`
	// ValidationMode used to configure how a canary deployment is validated. Possible values are 'auto' (default) and 'manual'
	ValidationMode ExtendedDaemonSetSpecStrategyCanaryValidationMode `json:"validationMode,omitempty"`
}

// ExtendedDaemonSetSpecStrategyCanaryAutoPause defines the canary deployment AutoPause parameters of the ExtendedDaemonSet.
// +k8s:openapi-gen=true
type ExtendedDaemonSetSpecStrategyCanaryAutoPause struct {
	// Enabled enables AutoPause.
	// Default value is true.
	Enabled *bool `json:"enabled,omitempty"`
	// MaxRestarts defines the number of tolerable (per pod) Canary pod restarts after which the Canary deployment is autopaused.
	// Default value is 2.
	MaxRestarts *int32 `json:"maxRestarts,omitempty"`
	// MaxSlowStartDuration defines the maximum slow start duration for a pod (stuck in Creating state) after which the Canary deployment is autopaused.
	// There is no default value.
	MaxSlowStartDuration *metav1.Duration `json:"maxSlowStartDuration,omitempty"`
}

// ExtendedDaemonSetSpecStrategyCanaryAutoFail defines the canary deployment AutoFail parameters of the ExtendedDaemonSet.
// +k8s:openapi-gen=true
type ExtendedDaemonSetSpecStrategyCanaryAutoFail struct {
	// Enabled enables AutoFail.
	// Default value is true.
	Enabled *bool `json:"enabled,omitempty"`
	// MaxRestarts defines the number of tolerable (per pod) Canary pod restarts after which the Canary deployment is autofailed.
	// Default value is 5.
	MaxRestarts *int32 `json:"maxRestarts,omitempty"`
	// MaxRestartsDuration defines the maximum duration of tolerable Canary pod restarts after which the Canary deployment is autofailed.
	// There is no default value.
	MaxRestartsDuration *metav1.Duration `json:"maxRestartsDuration,omitempty"`
	// CanaryTimeout defines the maximum duration of a Canary, after which the Canary deployment is autofailed. This is a safeguard against lengthy Canary pauses.
	// There is no default value.
	CanaryTimeout *metav1.Duration `json:"canaryTimeout,omitempty"`
}

// ExtendedDaemonSetStatusState type representing the ExtendedDaemonSet state.
type ExtendedDaemonSetStatusState string

const (
	// ExtendedDaemonSetStatusStateRunning the ExtendedDaemonSet is currently Running.
	ExtendedDaemonSetStatusStateRunning ExtendedDaemonSetStatusState = "Running"
	// ExtendedDaemonSetStatusStateRollingUpdatePaused the ExtendedDaemonSet rolling update is paused.
	ExtendedDaemonSetStatusStateRollingUpdatePaused ExtendedDaemonSetStatusState = "RollingUpdate Paused"
	// ExtendedDaemonSetStatusStateRolloutFrozen the ExtendedDaemonSet rollout is frozen.
	ExtendedDaemonSetStatusStateRolloutFrozen ExtendedDaemonSetStatusState = "Rollout frozen"
	// ExtendedDaemonSetStatusStateCanary the ExtendedDaemonSet currently run a new version with a Canary deployment.
	ExtendedDaemonSetStatusStateCanary ExtendedDaemonSetStatusState = "Canary"
	// ExtendedDaemonSetStatusStateCanaryPaused the Canary deployment of the ExtendedDaemonSet is paused.
	ExtendedDaemonSetStatusStateCanaryPaused ExtendedDaemonSetStatusState = "Canary Paused"
	// ExtendedDaemonSetStatusStateCanaryFailed the Canary deployment of the ExtendedDaemonSet is considered as Failing.
	ExtendedDaemonSetStatusStateCanaryFailed ExtendedDaemonSetStatusState = "Canary Failed"
)

// ExtendedDaemonSetStatusReason type represents the reason for a ExtendedDaemonSet status state.
type ExtendedDaemonSetStatusReason string

const (
	// ExtendedDaemonSetStatusReasonCLB represents CrashLoopBackOff as the reason for the ExtendedDaemonSet status state.
	ExtendedDaemonSetStatusReasonCLB ExtendedDaemonSetStatusReason = "CrashLoopBackOff"
	// ExtendedDaemonSetStatusReasonOOM represents OOMKilled as the reason for the ExtendedDaemonSet status state.
	ExtendedDaemonSetStatusReasonOOM ExtendedDaemonSetStatusReason = "OOMKilled"
	// ExtendedDaemonSetStatusRestartsTimeoutExceeded represents timeout on restarts as the reason for the ExtendedDaemonSet status.
	ExtendedDaemonSetStatusRestartsTimeoutExceeded ExtendedDaemonSetStatusReason = "RestartsTimeoutExceeded"
	// ExtendedDaemonSetStatusTimeoutExceeded represents timeout on Canary as the reason for the ExtendedDaemonSet status.
	ExtendedDaemonSetStatusTimeoutExceeded ExtendedDaemonSetStatusReason = "TimeoutExceeded"
	// ExtendedDaemonSetStatusSlowStartTimeoutExceeded represents timeout on slow starts as the reason for the ExtendedDaemonSet status.
	ExtendedDaemonSetStatusSlowStartTimeoutExceeded ExtendedDaemonSetStatusReason = "SlowStartTimeoutExceeded"
	// ExtendedDaemonSetStatusReasonErrImagePull represent ErrImagePull as the reason for the ExtendedDaemonSet status state.
	ExtendedDaemonSetStatusReasonErrImagePull ExtendedDaemonSetStatusReason = "ErrImagePull"
	// ExtendedDaemonSetStatusReasonImagePullBackOff represent ImagePullBackOff as the reason for the ExtendedDaemonSet status state.
	ExtendedDaemonSetStatusReasonImagePullBackOff ExtendedDaemonSetStatusReason = "ImagePullBackOff"
	// ExtendedDaemonSetStatusReasonImageInspectError represent ImageInspectError as the reason for the ExtendedDaemonSet status state.
	ExtendedDaemonSetStatusReasonImageInspectError ExtendedDaemonSetStatusReason = "ImageInspectError"
	// ExtendedDaemonSetStatusReasonErrImageNeverPull represent ErrImageNeverPull as the reason for the ExtendedDaemonSet status state.
	ExtendedDaemonSetStatusReasonErrImageNeverPull ExtendedDaemonSetStatusReason = "ErrImageNeverPull"
	// ExtendedDaemonSetStatusReasonRegistryUnavailable represent RegistryUnavailable as the reason for the ExtendedDaemonSet status state.
	ExtendedDaemonSetStatusReasonRegistryUnavailable ExtendedDaemonSetStatusReason = "RegistryUnavailable"
	// ExtendedDaemonSetStatusReasonInvalidImageName represent InvalidImageName as the reason for the ExtendedDaemonSet status state.
	ExtendedDaemonSetStatusReasonInvalidImageName ExtendedDaemonSetStatusReason = "InvalidImageName"
	// ExtendedDaemonSetStatusReasonCreateContainerConfigError represent CreateContainerConfigError as the reason for the ExtendedDaemonSet status state.
	ExtendedDaemonSetStatusReasonCreateContainerConfigError ExtendedDaemonSetStatusReason = "CreateContainerConfigError"
	// ExtendedDaemonSetStatusReasonCreateContainerError represent CreateContainerError as the reason for the ExtendedDaemonSet status state.
	ExtendedDaemonSetStatusReasonCreateContainerError ExtendedDaemonSetStatusReason = "CreateContainerError"
	// ExtendedDaemonSetStatusReasonPreStartHookError represent PreStartHookError as the reason for the ExtendedDaemonSet status state.
	ExtendedDaemonSetStatusReasonPreStartHookError ExtendedDaemonSetStatusReason = "PreStartHookError"
	// ExtendedDaemonSetStatusReasonPostStartHookError represent PostStartHookError as the reason for the ExtendedDaemonSet status state.
	ExtendedDaemonSetStatusReasonPostStartHookError ExtendedDaemonSetStatusReason = "PostStartHookError"
	// ExtendedDaemonSetStatusReasonPreCreateHookError represent PreCreateHookError as the reason for the ExtendedDaemonSet status state.
	ExtendedDaemonSetStatusReasonPreCreateHookError ExtendedDaemonSetStatusReason = "PreCreateHookError"
	// ExtendedDaemonSetStatusReasonStartError represent StartError as the reason for the ExtendedDaemonSet status state.
	ExtendedDaemonSetStatusReasonStartError ExtendedDaemonSetStatusReason = "StartError"
	// ExtendedDaemonSetStatusReasonUnknown represents an Unknown reason for the status state.
	ExtendedDaemonSetStatusReasonUnknown ExtendedDaemonSetStatusReason = "Unknown"
)

// ExtendedDaemonSetConditionType type use to represent a ExtendedDaemonSetR condition.
type ExtendedDaemonSetConditionType string

const (
	// ConditionTypeEDSReconcileError the controller wasn't able to run properly the reconcile loop with this ExtendedDaemonSet.
	ConditionTypeEDSReconcileError ExtendedDaemonSetConditionType = "ReconcileError"
	// ConditionTypeEDSCanaryPaused ExtendedDaemonSet is in canary mode.
	ConditionTypeEDSCanaryPaused ExtendedDaemonSetConditionType = "Canary-Paused"
	// ConditionTypeEDSCanaryFailed ExtendedDaemonSetis in canary mode.
	ConditionTypeEDSCanaryFailed ExtendedDaemonSetConditionType = "Canary-Failed"
)

// ExtendedDaemonSetCondition describes the state of a ExtendedDaemonSet at a certain point.
type ExtendedDaemonSetCondition struct {
	// Type of ExtendedDaemonSetReplicaSet condition.
	Type ExtendedDaemonSetConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// Last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// Last time the condition was updated.
	// +optional
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
	// The reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty"`
	// A human readable message indicating details about the transition.
	// +optional
	Message string `json:"message,omitempty"`
}

// ExtendedDaemonSetStatus defines the observed state of ExtendedDaemonSet
// +k8s:openapi-gen=true
type ExtendedDaemonSetStatus struct {
	Desired                  int32 `json:"desired"`
	Current                  int32 `json:"current"`
	Ready                    int32 `json:"ready"`
	Available                int32 `json:"available"`
	UpToDate                 int32 `json:"upToDate"`
	IgnoredUnresponsiveNodes int32 `json:"ignoredUnresponsiveNodes"`

	State            ExtendedDaemonSetStatusState   `json:"state,omitempty"`
	ActiveReplicaSet string                         `json:"activeReplicaSet"`
	Canary           *ExtendedDaemonSetStatusCanary `json:"canary,omitempty"`

	// Reason provides an explanation for canary deployment autopause
	// +optional
	Reason ExtendedDaemonSetStatusReason `json:"reason,omitempty"`

	// Conditions Represents the latest available observations of a DaemonSet's current state.
	// +listType=map
	// +listMapKey=type
	Conditions []ExtendedDaemonSetCondition `json:"conditions,omitempty"`
}

// ExtendedDaemonSetStatusCanary defines the observed state of ExtendedDaemonSet canary deployment
// +k8s:openapi-gen=true
type ExtendedDaemonSetStatusCanary struct {
	ReplicaSet string `json:"replicaSet"`
	// +listType=set
	Nodes []string `json:"nodes,omitempty"`
}

// ExtendedDaemonSet is the Schema for the extendeddaemonsets API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="desired",type="integer",JSONPath=".status.desired"
// +kubebuilder:printcolumn:name="current",type="integer",JSONPath=".status.current"
// +kubebuilder:printcolumn:name="ready",type="integer",JSONPath=".status.ready"
// +kubebuilder:printcolumn:name="up-to-date",type="integer",JSONPath=".status.upToDate"
// +kubebuilder:printcolumn:name="available",type="integer",JSONPath=".status.available"
// +kubebuilder:printcolumn:name="ignored unresponsive nodes",type="integer",JSONPath=".status.ignoredunresponsivenodes"
// +kubebuilder:printcolumn:name="status",type="string",JSONPath=".status.state"
// +kubebuilder:printcolumn:name="reason",type="string",JSONPath=".status.reason"
// +kubebuilder:printcolumn:name="active rs",type="string",JSONPath=".status.activeReplicaSet"
// +kubebuilder:printcolumn:name="canary rs",type="string",JSONPath=".status.canary.replicaSet"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:path=extendeddaemonsets,shortName=eds
// +k8s:openapi-gen=true
// +genclient
type ExtendedDaemonSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ExtendedDaemonSetSpec   `json:"spec,omitempty"`
	Status ExtendedDaemonSetStatus `json:"status,omitempty"`
}

// ExtendedDaemonSetList contains a list of ExtendedDaemonSet
// +kubebuilder:object:root=true
type ExtendedDaemonSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ExtendedDaemonSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ExtendedDaemonSet{}, &ExtendedDaemonSetList{})
}
