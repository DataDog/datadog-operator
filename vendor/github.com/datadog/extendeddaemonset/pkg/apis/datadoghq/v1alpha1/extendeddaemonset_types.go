// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	corev1 "k8s.io/api/core/v1"
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

	// Daemonset deployment strategy
	Strategy ExtendedDaemonSetSpecStrategy `json:"strategy"`
}

// ExtendedDaemonSetSpecStrategy defines the deployment strategy of ExtendedDaemonSet
// +k8s:openapi-gen=true
type ExtendedDaemonSetSpecStrategy struct {
	RollingUpdate ExtendedDaemonSetSpecStrategyRollingUpdate `json:"rollingUpdate,omitempty"`
	// Canary deployment configuration
	Canary *ExtendedDaemonSetSpecStrategyCanary `json:"canary,omitempty"`
	// ReconcileFrequency use to configure how often the ExtendedDeamonset will be fully reconcile, default is 10sec
	ReconcileFrequency *metav1.Duration `json:"reconcileFrequency,omitempty"`
}

// ExtendedDaemonSetSpecStrategyRollingUpdate defines the rolling update deployment strategy of ExtendedDaemonSet
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
	// number of DaemonSet pods at the start of the update (ex: 10%). Absolute
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

// ExtendedDaemonSetSpecStrategyCanary defines the canary deployment strategy of ExtendedDaemonSet
// +k8s:openapi-gen=true
type ExtendedDaemonSetSpecStrategyCanary struct {
	Replicas *intstr.IntOrString `json:"replicas,omitempty"`
	Duration *metav1.Duration    `json:"duration,omitempty"`
	// +optional
	Paused bool `json:"paused,omitempty"`
}

// ExtendedDaemonSetStatusState type representing the ExtendedDaemonSet state
type ExtendedDaemonSetStatusState string

const (
	// ExtendedDaemonSetStatusStateCanary the ExtendedDaemonSet currently run a new version with a Canary deployment
	ExtendedDaemonSetStatusStateCanary ExtendedDaemonSetStatusState = "Canary"
	// ExtendedDaemonSetStatusStateRunning the ExtendedDaemonSet is currently running
	ExtendedDaemonSetStatusStateRunning ExtendedDaemonSetStatusState = "Running"
	// ExtendedDaemonSetStatusStateFailed the current state of the ExtendedDaemonSet is considered as Failing
	ExtendedDaemonSetStatusStateFailed ExtendedDaemonSetStatusState = "Failed"
)

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
}

// ExtendedDaemonSetStatusCanary defines the observed state of ExtendedDaemonSet canary deployment
// +k8s:openapi-gen=true
type ExtendedDaemonSetStatusCanary struct {
	ReplicaSet string `json:"replicaSet"`
	// +listType=set
	Nodes []string `json:"nodes,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ExtendedDaemonSet is the Schema for the extendeddaemonsets API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="desired",type="integer",JSONPath=".status.desired"
// +kubebuilder:printcolumn:name="current",type="integer",JSONPath=".status.current"
// +kubebuilder:printcolumn:name="ready",type="integer",JSONPath=".status.ready"
// +kubebuilder:printcolumn:name="up-to-date",type="integer",JSONPath=".status.upToDate"
// +kubebuilder:printcolumn:name="available",type="integer",JSONPath=".status.available"
// +kubebuilder:printcolumn:name="ignored unresponsive nodes",type="integer",JSONPath=".status.ignoredunresponsivenodes"
// +kubebuilder:printcolumn:name="status",type="string",JSONPath=".status.state"
// +kubebuilder:printcolumn:name="active rs",type="string",JSONPath=".status.activeReplicaSet"
// +kubebuilder:printcolumn:name="canary rs",type="string",JSONPath=".status.canary.replicaSet"
// +kubebuilder:printcolumn:name="canary paused",type="boolean",JSONPath=".spec.strategy.canary.paused"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:path=extendeddaemonsets,shortName=eds
type ExtendedDaemonSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ExtendedDaemonSetSpec   `json:"spec,omitempty"`
	Status ExtendedDaemonSetStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ExtendedDaemonSetList contains a list of
// +k8s:openapi-gen=true
type ExtendedDaemonSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// +listType=set
	Items []ExtendedDaemonSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ExtendedDaemonSet{}, &ExtendedDaemonSetList{})
}
