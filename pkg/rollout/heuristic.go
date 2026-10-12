// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rollout

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	// HeuristicName is the Result.Source of the heuristic evaluator.
	HeuristicName = "heuristic"
	// DefaultStallAfter is the default stall threshold.
	DefaultStallAfter = 10 * time.Minute
	// DefaultSettlingPeriod is the default Input.SettlingPeriod.
	DefaultSettlingPeriod = 5 * time.Minute

	ReasonProgressDeadlineExceeded = "ProgressDeadlineExceeded"
	ReasonNoProgress               = "NoProgress"
	ReasonGenerationNotObserved    = "GenerationNotObserved"
	ReasonUpdatingPods             = "UpdatingPods"
	ReasonPodsUnavailable          = "PodsUnavailable"
	// ReasonAvailabilityBelowThreshold is the reason of a Complete rollout
	// whose unavailable pods are above the availability threshold after
	// the settling period.
	ReasonAvailabilityBelowThreshold = "AvailabilityBelowThreshold"
)

// HeuristicEvaluator infers the rollout phase from workload status and,
// when pods are watched, from the pods of the current revision. It always
// decides. The first matching rule wins:
//
//	Failed    the Deployment progress deadline is exceeded
//	Rolling   the generation is not observed, or pods are not all updated
//	          (Stalled when pods are watched and make no progress)
//	Settling  unavailable pods above the threshold, converged less than the
//	          settling period ago
//	Complete  otherwise; with reason AvailabilityBelowThreshold when
//	          unavailable pods are above the threshold
type HeuristicEvaluator struct {
	// StallAfter is how long without progress before a rollout is Stalled.
	// Declared.ProgressDeadline takes precedence. Defaults to
	// DefaultStallAfter.
	StallAfter time.Duration
}

var _ Evaluator = HeuristicEvaluator{}

// Name implements Evaluator.
func (h HeuristicEvaluator) Name() string { return HeuristicName }

// Evaluate implements Evaluator.
func (h HeuristicEvaluator) Evaluate(in Input) (Result, bool) {
	w := in.Workload
	res := Result{
		Updated:      w.Updated,
		Desired:      w.Desired,
		UpdatedReady: UpdatedReady(in),
		Since:        w.RolloutStart,
		SinceApprox:  w.RolloutStartApprox,
		LastProgress: in.LastProgress,
		StepOrder:    in.Declared.StepOrder,
	}
	// After a rollback the revision start is old; prefer the first time the
	// tool saw the revision when it knows it.
	if w.RolloutStartApprox && in.FirstSeen.After(res.Since) {
		res.Since = in.FirstSeen
	}

	switch {
	case w.ProgressDeadlineExceeded:
		res.Phase, res.Reason = PhaseFailed, ReasonProgressDeadlineExceeded
		return res, true
	case w.ObservedGeneration < w.Generation:
		res.Phase, res.Reason = PhaseRolling, ReasonGenerationNotObserved
	case w.Updated < w.Desired:
		res.Phase, res.Reason = PhaseRolling, ReasonUpdatingPods
	case w.Unavailable <= in.AvailabilityThreshold:
		res.Phase = PhaseComplete
		return res, true
	default:
		if until := settlingUntil(in, res.Since); in.Now.Before(until) {
			res.Phase, res.Reason, res.SettlingUntil = PhaseSettling, ReasonPodsUnavailable, until
		} else {
			res.Phase, res.Reason = PhaseComplete, ReasonAvailabilityBelowThreshold
		}
		return res, true
	}

	// Stalled is only evaluated from pods, never for OnDelete (pods are only
	// replaced when deleted), not before the controller has seen the new
	// template, and not from stale data.
	if !in.PodsWatched || in.Stale || w.OnDelete || w.ObservedGeneration < w.Generation {
		return res, true
	}
	threshold := h.StallAfter
	if in.Declared.ProgressDeadline > 0 {
		threshold = in.Declared.ProgressDeadline
	}
	if threshold <= 0 {
		threshold = DefaultStallAfter
	}
	// Progress is bounded below by the rollout start, since pods that never
	// left a restored revision carry old timestamps. An approximate start
	// without FirstSeen is too old to bound anything: skip stall.
	if w.RolloutStartApprox && in.FirstSeen.IsZero() {
		return res, true
	}
	lastProgress := podProgress(w.CurrentRevision, in.Pods)
	if res.Since.After(lastProgress) {
		lastProgress = res.Since
	}
	if lastProgress.IsZero() {
		return res, true
	}
	res.LastProgress = lastProgress
	res.Deadline = res.LastProgress.Add(threshold)
	if in.Now.Sub(res.LastProgress) > threshold {
		res.Phase, res.Reason = PhaseStalled, noProgressReason(in.Pods)
	}
	return res, true
}

// settlingUntil returns the end of the settling period of a converged
// workload: the period after the latest of the rollout start (since) and
// Input.ConvergedSince. It is zero when neither is known, or when the start
// is approximate and the tool saw neither the revision nor the convergence.
func settlingUntil(in Input, since time.Time) time.Time {
	if in.Workload.RolloutStartApprox && in.FirstSeen.IsZero() && in.ConvergedSince.IsZero() {
		return time.Time{}
	}
	converged := since
	if in.ConvergedSince.After(converged) {
		converged = in.ConvergedSince
	}
	if converged.IsZero() {
		return time.Time{}
	}
	period := in.SettlingPeriod
	if period <= 0 {
		period = DefaultSettlingPeriod
	}
	return converged.Add(period)
}

// UpdatedReady returns how many updated pods are ready. The lower bound
// max(0, Updated-Unavailable) always holds; when pods are watched, the ready
// pods of the current revision can raise it. It is capped at Updated.
func UpdatedReady(in Input) int32 {
	w := in.Workload
	n := max(w.Updated-w.Unavailable, 0)
	if in.PodsWatched && w.CurrentRevision != "" {
		var ready int32
		for _, p := range in.Pods {
			if p.Revision == w.CurrentRevision && !p.ReadySince.IsZero() {
				ready++
			}
		}
		n = max(n, ready)
	}
	return min(n, w.Updated)
}

// podProgress returns the latest creation or ready time of the pods of the
// current revision, or zero when there is none.
func podProgress(revision string, pods []PodFact) time.Time {
	var last time.Time
	for _, p := range pods {
		if revision == "" || p.Revision != revision {
			continue
		}
		if p.Created.After(last) {
			last = p.Created
		}
		if p.ReadySince.After(last) {
			last = p.ReadySince
		}
	}
	return last
}

// noProgressReason returns NoProgress with a hint counting pod waiting
// reasons, e.g. "NoProgress: 3 pods CrashLoopBackOff".
func noProgressReason(pods []PodFact) string {
	counts := map[string]int{}
	for _, p := range pods {
		if p.WaitingReason != "" {
			counts[p.WaitingReason]++
		}
	}
	if len(counts) == 0 {
		return ReasonNoProgress
	}
	reasons := make([]string, 0, len(counts))
	for r := range counts {
		reasons = append(reasons, r)
	}
	slices.SortFunc(reasons, func(a, b string) int {
		if c := cmp.Compare(counts[b], counts[a]); c != 0 {
			return c
		}
		return cmp.Compare(a, b)
	})
	hints := make([]string, len(reasons))
	for i, r := range reasons {
		noun := "pods"
		if counts[r] == 1 {
			noun = "pod"
		}
		hints[i] = fmt.Sprintf("%d %s %s", counts[r], noun, r)
	}
	return ReasonNoProgress + ": " + strings.Join(hints, ", ")
}
