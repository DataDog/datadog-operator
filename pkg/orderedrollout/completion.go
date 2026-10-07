// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orderedrollout

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// Step reasons.
const (
	ReasonUnchanged           = "Unchanged"
	ReasonPriorityWavePending = "PriorityWavePending"
	ReasonDDAHold             = "DDAHold"
	ReasonDAPHold             = "DAPHold"
	ReasonDAPSkip             = "DAPSkip"
	ReasonSpecApplying        = "SpecApplying"
	ReasonRolling             = "Rolling"
	ReasonSoaking             = "Soaking"
	ReasonComplete            = "Complete"
	ReasonStepTimedOut        = "StepTimedOut"
	ReasonManualBypass        = "ManualBypass"

	ReasonDaemonSetMissing          = "DaemonSetMissing"
	ReasonObservedGenerationStale   = "ObservedGenerationStale"
	ReasonUpdating                  = "Updating"
	ReasonUnavailableAboveThreshold = "UnavailableAboveThreshold"
)

// DaemonSetFacts are the Agent DaemonSet fields used for completion.
type DaemonSetFacts struct {
	Generation             int64
	ObservedGeneration     int64
	DesiredNumberScheduled int32
	UpdatedNumberScheduled int32
	NumberUnavailable      int32
}

// Completion is the result of evaluating a DaemonSet against a step configuration.
type Completion struct {
	Phase           v2alpha1.RolloutPhase
	Reason          string
	Message         string
	Complete        bool
	WithinThreshold bool
	Progress        *v2alpha1.RolloutProgress
	SoakStartedAt   *metav1.Time
}

// EvaluateDaemonSet evaluates whether a DaemonSet completed its rollout under
// cfg. soakStartedAt is the persisted soak start, reset when the DaemonSet
// leaves the threshold.
func EvaluateDaemonSet(ds *DaemonSetFacts, cfg Config, soakStartedAt *metav1.Time, now time.Time) Completion {
	rolling := func(reason, msg string, progress *v2alpha1.RolloutProgress) Completion {
		return Completion{Phase: v2alpha1.RolloutPhaseInProgress, Reason: reason, Message: msg, Progress: progress}
	}
	if ds == nil {
		return rolling(ReasonDaemonSetMissing, "Agent DaemonSet not found", nil)
	}
	progress := &v2alpha1.RolloutProgress{
		DesiredNumberScheduled: ds.DesiredNumberScheduled,
		UpdatedNumberScheduled: ds.UpdatedNumberScheduled,
		NumberUnavailable:      ds.NumberUnavailable,
		ObservedGeneration:     ds.ObservedGeneration,
		Generation:             ds.Generation,
	}
	maxUnavailable, err := ResolveMaxUnavailable(cfg.MaxUnavailable, ds.DesiredNumberScheduled)
	if err != nil {
		maxUnavailable = 0
	}
	progress.MaxUnavailable = maxUnavailable

	if ds.ObservedGeneration < ds.Generation {
		return rolling(ReasonObservedGenerationStale, "Waiting for the DaemonSet controller to observe the new generation", progress)
	}
	if ds.UpdatedNumberScheduled != ds.DesiredNumberScheduled {
		return rolling(ReasonUpdating, fmt.Sprintf("%d/%d pods updated", ds.UpdatedNumberScheduled, ds.DesiredNumberScheduled), progress)
	}
	if ds.NumberUnavailable > maxUnavailable {
		return rolling(ReasonUnavailableAboveThreshold, fmt.Sprintf("%d pods unavailable, %d tolerated", ds.NumberUnavailable, maxUnavailable), progress)
	}

	if cfg.StepSoak > 0 {
		if soakStartedAt == nil {
			start := metav1.NewTime(now)
			soakStartedAt = &start
		}
		if now.Sub(soakStartedAt.Time) < cfg.StepSoak {
			return Completion{
				Phase:           v2alpha1.RolloutPhaseBaking,
				Reason:          ReasonSoaking,
				Message:         fmt.Sprintf("Within threshold, soaking for %s", cfg.StepSoak),
				WithinThreshold: true,
				Progress:        progress,
				SoakStartedAt:   soakStartedAt,
			}
		}
	}
	return Completion{
		Phase:           v2alpha1.RolloutPhaseCompleted,
		Reason:          ReasonComplete,
		Message:         "Rollout complete",
		Complete:        true,
		WithinThreshold: true,
		Progress:        progress,
		SoakStartedAt:   soakStartedAt,
	}
}
