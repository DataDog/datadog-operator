package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ExtendedDaemonSetReplicaSetSpec defines the desired state of ExtendedDaemonSetReplicaSet
// +k8s:openapi-gen=true
type ExtendedDaemonSetReplicaSetSpec struct {
	// DaemonSetReplicaSet deployment strategy
	Strategy ExtendedDaemonSetReplicaSetSpecStrategy `json:"strategy"`
	// A label query over pods that are managed by the daemon set.
	// Must match in order to be controlled.
	// If empty, defaulted to labels on Pod template.
	// +optional
	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// An object that describes the pod that will be created.
	// The ExtendedDaemonSetReplicaSet will create exactly one copy of this pod on every node
	// that matches the template's node selector (or on every node if no node
	// selector is specified).
	Template corev1.PodTemplateSpec `json:"template"`
	// A sequence hash representing a specific generation of the template.
	// Populated by the system. It can be set only during the creation.
	// +optional
	TemplateGeneration string `json:"templateGeneration,omitempty"`
}

// ExtendedDaemonSetReplicaSetSpecStrategy defines the desired state of ExtendedDaemonSet
// +k8s:openapi-gen=true
type ExtendedDaemonSetReplicaSetSpecStrategy struct {
	RollingUpdate      ExtendedDaemonSetSpecStrategyRollingUpdate `json:"rollingUpdate,omitempty"`
	ReconcileFrequency metav1.Duration                            `json:"reconcileFrequency,omitempty"`
}

// ExtendedDaemonSetReplicaSetStatus defines the observed state of ExtendedDaemonSetReplicaSet
// +k8s:openapi-gen=true
type ExtendedDaemonSetReplicaSetStatus struct {
	Status    string `json:"status"`
	Desired   int32  `json:"desired"`
	Current   int32  `json:"current"`
	Ready     int32  `json:"ready"`
	Available int32  `json:"available"`

	// Conditions Represents the latest available observations of a DaemonSet's current state.
	// +listType=set
	Conditions []ExtendedDaemonSetReplicaSetCondition `json:"conditions,omitempty"`
}

// ExtendedDaemonSetReplicaSetCondition describes the state of a ExtendedDaemonSetReplicaSet at a certain point.
type ExtendedDaemonSetReplicaSetCondition struct {
	// Type of ExtendedDaemonSetReplicaSet condition.
	Type ExtendedDaemonSetReplicaSetConditionType `json:"type"`
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

// ExtendedDaemonSetReplicaSetConditionType type use to represent a ExtendedDaemonSetReplicaSet condition
type ExtendedDaemonSetReplicaSetConditionType string

const (
	// ConditionTypeActive ExtendedDaemonSetReplicaSet is active
	ConditionTypeActive ExtendedDaemonSetReplicaSetConditionType = "Active"
	// ConditionTypeCanary ExtendedDaemonSetReplicaSet is in canary mode
	ConditionTypeCanary ExtendedDaemonSetReplicaSetConditionType = "Canary"
	// ConditionTypeReconcileError the controller wasn't able to run properly the reconcile loop with this ExtendedDaemonSetReplicaSet
	ConditionTypeReconcileError ExtendedDaemonSetReplicaSetConditionType = "ReconcileError"
	// ConditionTypeUnschedule some pods was not scheduled properly for this ExtendedDaemonSetReplicaSet
	ConditionTypeUnschedule ExtendedDaemonSetReplicaSetConditionType = "Unschedule"
	// ConditionTypePodsCleanupDone Pod(s) cleanup condition
	ConditionTypePodsCleanupDone ExtendedDaemonSetReplicaSetConditionType = "PodsCleanupDone"
	// ConditionTypePodCreation Pod(s) creation condition
	ConditionTypePodCreation ExtendedDaemonSetReplicaSetConditionType = "PodCreation"
	// ConditionTypePodDeletion Pod(s) deletion condition
	ConditionTypePodDeletion ExtendedDaemonSetReplicaSetConditionType = "PodDeletion"
	// ConditionTypeLastFullSync last time the ExtendedDaemonSetReplicaSet sync when to the end of the reconcile function
	ConditionTypeLastFullSync ExtendedDaemonSetReplicaSetConditionType = "LastFullSync"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ExtendedDaemonSetReplicaSet is the Schema for the extendeddaemonsetreplicasets API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="desired",type="integer",JSONPath=".status.desired"
// +kubebuilder:printcolumn:name="current",type="integer",JSONPath=".status.current"
// +kubebuilder:printcolumn:name="ready",type="integer",JSONPath=".status.ready"
// +kubebuilder:printcolumn:name="available",type="integer",JSONPath=".status.available"
// +kubebuilder:printcolumn:name="node selector",type="string",JSONPath=".spec.selector"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:path=extendeddaemonsetreplicasets,shortName=ers
type ExtendedDaemonSetReplicaSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ExtendedDaemonSetReplicaSetSpec   `json:"spec,omitempty"`
	Status ExtendedDaemonSetReplicaSetStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ExtendedDaemonSetReplicaSetList contains a list of ExtendedDaemonSetReplicaSet
type ExtendedDaemonSetReplicaSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ExtendedDaemonSetReplicaSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ExtendedDaemonSetReplicaSet{}, &ExtendedDaemonSetReplicaSetList{})
}
