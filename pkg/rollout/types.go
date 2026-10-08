// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rollout evaluates the rollout state of agent workloads.
//
// It is pure: evaluators only read their Input (including the clock) and do
// no I/O, so the same rules can be shared by the kubectl plugin and the
// operator. Inputs are plain facts, not Kubernetes API types.
package rollout

import "time"

// Phase is the rollout phase of a workload or step. It is an open string type:
// values not listed below (e.g. new phases from a newer operator) must be
// carried and rendered as-is.
type Phase string

const (
	PhaseUnknown  Phase = "Unknown"
	PhaseComplete Phase = "Complete"
	PhaseRolling  Phase = "Rolling"
	// PhaseSettling is a converged rollout whose unavailable pods are above
	// the availability threshold, within the settling period.
	PhaseSettling Phase = "Settling"
	PhaseStalled  Phase = "Stalled"
	PhaseFailed   Phase = "Failed"

	// Phases used by operator-gated rollouts.
	PhasePending    Phase = "Pending"
	PhaseBaking     Phase = "Baking"
	PhasePaused     Phase = "Paused"
	PhaseRolledBack Phase = "RolledBack"
	PhaseTimedOut   Phase = "TimedOut"
	PhaseSucceeded  Phase = "Succeeded"
)

// Severity ranks phases from best to worst. Unrecognized
// phases rank like PhaseUnknown.
type Severity int

const (
	SeverityDone Severity = iota
	SeverityUnknown
	SeverityProgressing
	SeverityDegraded
	SeverityError
)

// Severity returns the severity of the phase.
func (p Phase) Severity() Severity {
	switch p {
	case PhaseComplete, PhaseSucceeded:
		return SeverityDone
	case PhaseRolling, PhaseSettling, PhasePending, PhaseBaking:
		return SeverityProgressing
	case PhaseStalled, PhasePaused:
		return SeverityDegraded
	case PhaseFailed, PhaseTimedOut, PhaseRolledBack:
		return SeverityError
	default:
		return SeverityUnknown
	}
}

// WorkloadKind is the kind of the workload being rolled out.
type WorkloadKind string

const (
	KindDaemonSet  WorkloadKind = "DaemonSet"
	KindDeployment WorkloadKind = "Deployment"
)

// Workload holds the rollout facts of a DaemonSet or Deployment, read from
// the workload's own status.
type Workload struct {
	Kind WorkloadKind
	Name string

	Generation         int64
	ObservedGeneration int64

	Desired     int32
	Updated     int32
	Available   int32
	Unavailable int32

	// OnDelete is true for DaemonSets using the OnDelete update strategy.
	OnDelete bool
	// ProgressDeadlineExceeded is true when the Deployment Progressing
	// condition has reason ProgressDeadlineExceeded.
	ProgressDeadlineExceeded bool

	// CurrentRevision is the hash of the newest ControllerRevision
	// (DaemonSet) or ReplicaSet (Deployment).
	CurrentRevision string
	// RolloutStart is the creation time of the newest ControllerRevision or
	// ReplicaSet, used as a proxy for the rollout start.
	RolloutStart time.Time
	// RolloutStartApprox is true when RolloutStart is known to be inexact,
	// e.g. after a rollback that reused an older revision.
	RolloutStartApprox bool
}

// PodFact is the reduced view of a workload pod.
type PodFact struct {
	Name string
	Node string
	// Revision is the pod's controller-revision-hash (DaemonSet) or
	// pod-template-hash (Deployment) label.
	Revision string
	Created  time.Time
	// ReadySince is the last transition time of the Ready=True condition;
	// zero when the pod is not ready.
	ReadySince time.Time
	// WaitingReason is the waiting reason of a failing container, e.g.
	// CrashLoopBackOff or ImagePullBackOff; empty when none.
	WaitingReason string
}

// Declared holds rollout configuration declared on the DDA or DAP. It takes
// precedence over evaluator defaults.
type Declared struct {
	// ProgressDeadline overrides the evaluator's stall threshold when set.
	ProgressDeadline time.Duration
	// StepOrder is the rollout order of the step owning the workload, if
	// declared (DAP rollout priority, DDA default profile position).
	StepOrder *int
}

// Input is everything an evaluator may use to evaluate one workload.
type Input struct {
	Workload Workload
	// PodsWatched is true when Pods holds the workload's pods. When false,
	// Pods is ignored.
	PodsWatched bool
	Pods        []PodFact
	// FirstSeen is when the tool first observed the current revision; zero
	// when unknown. Callers must set it when RolloutStartApprox is true,
	// otherwise stall is not evaluated.
	FirstSeen time.Time
	// LastProgress is when the tool last saw updated or available counts
	// change, kept in memory; zero when unknown.
	LastProgress time.Time
	// ConvergedSince is when the tool first saw every pod updated for the
	// current revision, kept in memory; zero when unknown.
	ConvergedSince time.Time
	// AvailabilityThreshold is how many unavailable pods a converged
	// workload tolerates before it is reported unavailable, resolved
	// against Desired.
	AvailabilityThreshold int32
	// SettlingPeriod is how long after convergence unavailable pods above
	// the threshold are attributed to the rollout. Defaults to
	// DefaultSettlingPeriod.
	SettlingPeriod time.Duration
	// Stale is true when the workload or its pods come from a source that
	// lost its connection: progress cannot be observed, so Stalled is not
	// evaluated.
	Stale    bool
	Declared Declared
	Now      time.Time
}

// Result is the evaluated rollout state of a workload or step.
type Result struct {
	Phase  Phase
	Reason string

	Updated int32
	Desired int32
	// UpdatedReady is how many updated pods are ready: exact when pods are
	// watched, otherwise the lower bound Updated - Unavailable.
	UpdatedReady int32

	// Since is when the rollout started; zero when unknown.
	Since time.Time
	// SinceApprox is true when Since is approximate.
	SinceApprox bool
	// LastProgress is when the rollout last made progress; zero when unknown.
	LastProgress time.Time
	// Deadline is when the rollout is considered stuck; zero when none.
	Deadline time.Time
	// SettlingUntil is when a Settling rollout ends its settling period;
	// zero for other phases.
	SettlingUntil time.Time
	// StepOrder is the rollout order of the step, if declared.
	StepOrder *int

	// Source is the name of the evaluator that produced the result.
	Source string
}
