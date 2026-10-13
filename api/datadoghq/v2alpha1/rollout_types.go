// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v2alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// RolloutTimeoutAction is the action taken when a rollout step exceeds its timeout.
type RolloutTimeoutAction string

const (
	// RolloutTimeoutActionContinue lets later rollout steps start after a step times out.
	RolloutTimeoutActionContinue RolloutTimeoutAction = "Continue"
	// RolloutTimeoutActionHalt keeps later rollout steps from starting after a step times out.
	RolloutTimeoutActionHalt RolloutTimeoutAction = "Halt"
)

// RolloutStrategy configures the ordered rollout of DatadogAgentInternal updates.
// Steps with a lower priority roll first; steps with the same priority roll in parallel.
// +k8s:openapi-gen=true
type RolloutStrategy struct {
	// Priority orders rollout steps. Lower values roll earlier; equal values roll in parallel. Defaults to 0.
	// +optional
	// +kubebuilder:validation:Minimum=0
	Priority *int32 `json:"priority,omitempty"`

	// StepTimeout is the maximum time a step may take to complete. Zero or omitted means no timeout.
	// +optional
	StepTimeout *metav1.Duration `json:"stepTimeout,omitempty"`

	// StepSoak is how long a step must stay within the completion threshold before later priorities may start.
	// Zero or omitted means no soak.
	// +optional
	StepSoak *metav1.Duration `json:"stepSoak,omitempty"`

	// MaxUnavailable is the number or percentage of unavailable Agent pods tolerated for a step to be complete.
	// Defaults to 0, which is strict: one persistently unavailable pod blocks later priorities.
	// +optional
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`

	// OnStepTimeout is the action taken when a step times out: Continue or Halt. Defaults to Continue.
	// +optional
	// +kubebuilder:validation:Enum=Continue;Halt
	OnStepTimeout *RolloutTimeoutAction `json:"onStepTimeout,omitempty"`
}

// RolloutPhase is the phase of a rollout or rollout step.
type RolloutPhase string

const (
	RolloutPhaseIdle       RolloutPhase = "Idle"
	RolloutPhasePending    RolloutPhase = "Pending"
	RolloutPhaseInProgress RolloutPhase = "InProgress"
	RolloutPhaseBaking     RolloutPhase = "Baking"
	RolloutPhaseCompleted  RolloutPhase = "Completed"
	RolloutPhaseTimedOut   RolloutPhase = "TimedOut"
	RolloutPhaseHeld       RolloutPhase = "Held"
	RolloutPhaseSkipped    RolloutPhase = "Skipped"
	RolloutPhaseBypassed   RolloutPhase = "Bypassed"
	RolloutPhaseBlocked    RolloutPhase = "Blocked"
)

// RolloutStepKind is the kind of a rollout step.
type RolloutStepKind string

const (
	// RolloutStepKindDefault is the default DatadogAgentInternal step.
	RolloutStepKindDefault RolloutStepKind = "Default"
	// RolloutStepKindProfile is a DatadogAgentProfile DatadogAgentInternal step.
	RolloutStepKindProfile RolloutStepKind = "DatadogAgentProfile"
)

// RolloutStatus is the aggregate state of an ordered rollout.
// +k8s:openapi-gen=true
type RolloutStatus struct {
	// Phase is the aggregate rollout phase.
	// +optional
	Phase RolloutPhase `json:"phase,omitempty"`
	// Generation is the DatadogAgent generation evaluated for this status.
	// +optional
	Generation int64 `json:"generation,omitempty"`
	// RolloutID identifies the set of step target hashes being rolled out.
	// +optional
	RolloutID string `json:"rolloutID,omitempty"`
	// BypassRolloutID is the RolloutID covered by the current manual bypass.
	// +optional
	BypassRolloutID string `json:"bypassRolloutID,omitempty"`
	// LastBypass is the last consumed value of the rollout bypass annotation.
	// +optional
	LastBypass string `json:"lastBypass,omitempty"`
	// CurrentPriority is the priority of the active wave.
	// +optional
	CurrentPriority *int32 `json:"currentPriority,omitempty"`
	// StartedAt is when the current rollout started its first step.
	// +optional
	StartedAt *metav1.Time `json:"startedAt,omitempty"`
	// LastTransitionTime is the last time the phase or reason changed.
	// +optional
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`
	// Reason is a machine-readable reason for the phase.
	// +optional
	Reason string `json:"reason,omitempty"`
	// Message is a human-readable description of the rollout state.
	// +optional
	Message string `json:"message,omitempty"`
	// Warnings lists active non-gating warning reasons, such as NoStepTimeout or DefaultNotFirst.
	// +optional
	// +listType=atomic
	Warnings []string `json:"warnings,omitempty"`
	// Steps is the state of each rollout step, in rollout order.
	// +optional
	// +listType=atomic
	Steps []RolloutStepStatus `json:"steps,omitempty"`
}

// RolloutStepStatus is the state of one rollout step.
// +k8s:openapi-gen=true
type RolloutStepStatus struct {
	// Name of the step: "default" or the DatadogAgentProfile name.
	// +optional
	Name string `json:"name,omitempty"`
	// Namespace of the DatadogAgentProfile, empty for the default step.
	// +optional
	Namespace string `json:"namespace,omitempty"`
	// Kind of the step: Default or DatadogAgentProfile.
	// +optional
	Kind RolloutStepKind `json:"kind,omitempty"`
	// Phase of the step.
	// +optional
	Phase RolloutPhase `json:"phase,omitempty"`
	// Reason is a machine-readable reason for the phase.
	// +optional
	Reason string `json:"reason,omitempty"`
	// Message is a human-readable description of the step state.
	// +optional
	Message string `json:"message,omitempty"`
	// Priority of the step.
	// +optional
	Priority int32 `json:"priority"`
	// TargetHash is the rollout target hash of the desired DatadogAgentInternal.
	// +optional
	TargetHash string `json:"targetHash,omitempty"`
	// CurrentHash is the rollout target hash of the live DatadogAgentInternal.
	// +optional
	CurrentHash string `json:"currentHash,omitempty"`
	// StartedAt is when the step was written for TargetHash.
	// +optional
	StartedAt *metav1.Time `json:"startedAt,omitempty"`
	// CompletedAt is when the step completed for TargetHash.
	// +optional
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`
	// LastTransitionTime is the last time the phase or reason changed.
	// +optional
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`
	// SoakStartedAt is when the step entered the completion threshold.
	// +optional
	SoakStartedAt *metav1.Time `json:"soakStartedAt,omitempty"`
	// Deadline is when the step times out.
	// +optional
	Deadline *metav1.Time `json:"deadline,omitempty"`
	// Source is the object whose rollout configuration applies to this step.
	// +optional
	Source string `json:"source,omitempty"`
	// Progress reports the Agent DaemonSet rollout progress.
	// +optional
	Progress *RolloutProgress `json:"progress,omitempty"`
}

// RolloutProgress reports the Agent DaemonSet rollout progress of a step.
// +k8s:openapi-gen=true
type RolloutProgress struct {
	// +optional
	DesiredNumberScheduled int32 `json:"desiredNumberScheduled,omitempty"`
	// +optional
	UpdatedNumberScheduled int32 `json:"updatedNumberScheduled,omitempty"`
	// +optional
	NumberUnavailable int32 `json:"numberUnavailable,omitempty"`
	// MaxUnavailable is the resolved unavailable tolerance.
	// +optional
	MaxUnavailable int32 `json:"maxUnavailable,omitempty"`
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// +optional
	Generation int64 `json:"generation,omitempty"`
}
