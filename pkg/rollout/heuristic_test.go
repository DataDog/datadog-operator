// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rollout

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var now = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

func ago(d time.Duration) time.Time { return now.Add(-d) }

func intPtr(i int) *int { return &i }

// rollingDS is a DaemonSet with 2 of 5 pods updated, started 30m ago.
func rollingDS() Workload {
	return Workload{
		Kind: KindDaemonSet, Name: "agent",
		Generation: 2, ObservedGeneration: 2,
		Desired: 5, Updated: 2, Available: 4, Unavailable: 1,
		CurrentRevision: "new",
		RolloutStart:    ago(30 * time.Minute),
	}
}

// rollbackDS is rollingDS rolled back to a revision created 30 days ago.
func rollbackDS() Workload {
	w := rollingDS()
	w.RolloutStart = ago(30 * 24 * time.Hour)
	w.RolloutStartApprox = true
	return w
}

func TestHeuristicEvaluator(t *testing.T) {
	tests := []struct {
		name       string
		stallAfter time.Duration
		in         Input
		want       Result
	}{
		{
			name: "complete",
			in: Input{
				Workload: Workload{
					Generation: 3, ObservedGeneration: 3,
					Desired: 5, Updated: 5, Available: 5,
					RolloutStart: ago(time.Hour),
				},
				Now: now,
			},
			want: Result{Phase: PhaseComplete, Updated: 5, Desired: 5, UpdatedReady: 5, Since: ago(time.Hour)},
		},
		{
			name: "complete ignores pods",
			in: Input{
				Workload: Workload{
					Generation: 3, ObservedGeneration: 3,
					Desired: 1, Updated: 1, Available: 1,
					CurrentRevision: "new", RolloutStart: ago(time.Hour),
				},
				PodsWatched: true,
				Pods:        []PodFact{{Revision: "new", Created: ago(time.Hour)}},
				Now:         now,
			},
			want: Result{Phase: PhaseComplete, Updated: 1, Desired: 1, UpdatedReady: 1, Since: ago(time.Hour)},
		},
		{
			name: "rolling: generation not observed",
			in: Input{
				Workload: Workload{Generation: 4, ObservedGeneration: 3, Desired: 5, Updated: 5, Available: 5},
				Now:      now,
			},
			want: Result{Phase: PhaseRolling, Reason: ReasonGenerationNotObserved, Updated: 5, Desired: 5, UpdatedReady: 5},
		},
		{
			name: "rolling: updating pods",
			in:   Input{Workload: rollingDS(), Now: now},
			want: Result{Phase: PhaseRolling, Reason: ReasonUpdatingPods, Updated: 2, Desired: 5, UpdatedReady: 1, Since: ago(30 * time.Minute)},
		},
		{
			name: "complete: pods unavailable, start unknown",
			in: Input{
				Workload: Workload{Generation: 1, ObservedGeneration: 1, Desired: 5, Updated: 5, Available: 4, Unavailable: 1},
				Now:      now,
			},
			want: Result{Phase: PhaseComplete, Reason: ReasonAvailabilityBelowThreshold, Updated: 5, Desired: 5, UpdatedReady: 4},
		},
		{
			name: "failed: progress deadline exceeded",
			in: Input{
				Workload: Workload{
					Kind: KindDeployment, Generation: 2, ObservedGeneration: 2,
					Desired: 2, Updated: 1, Available: 1, Unavailable: 1,
					ProgressDeadlineExceeded: true, RolloutStart: ago(20 * time.Minute),
				},
				LastProgress: ago(15 * time.Minute),
				Now:          now,
			},
			want: Result{
				Phase: PhaseFailed, Reason: ReasonProgressDeadlineExceeded,
				Updated: 1, Desired: 2, Since: ago(20 * time.Minute), LastProgress: ago(15 * time.Minute),
			},
		},
		{
			name: "no pods: never stalled, last progress passed through",
			in: Input{
				Workload:     rollingDS(),
				LastProgress: ago(25 * time.Minute),
				Now:          now,
			},
			want: Result{
				Phase: PhaseRolling, Reason: ReasonUpdatingPods, Updated: 2, Desired: 5, UpdatedReady: 1,
				Since: ago(30 * time.Minute), LastProgress: ago(25 * time.Minute),
			},
		},
		{
			name: "pods not watched: pods ignored",
			in: Input{
				Workload: rollingDS(),
				Pods:     []PodFact{{Revision: "new", Created: ago(25 * time.Minute), WaitingReason: "CrashLoopBackOff"}},
				Now:      now,
			},
			want: Result{Phase: PhaseRolling, Reason: ReasonUpdatingPods, Updated: 2, Desired: 5, UpdatedReady: 1, Since: ago(30 * time.Minute)},
		},
		{
			name: "pods: stalled with waiting reason hint",
			in: Input{
				Workload:    rollingDS(),
				PodsWatched: true,
				Pods: []PodFact{
					{Name: "a", Revision: "new", Created: ago(28 * time.Minute), WaitingReason: "CrashLoopBackOff"},
					{Name: "b", Revision: "new", Created: ago(27 * time.Minute), ReadySince: ago(20 * time.Minute)},
					{Name: "c", Revision: "old", Created: ago(2 * time.Hour), WaitingReason: "CrashLoopBackOff"},
					{Name: "d", Revision: "old", Created: ago(2 * time.Hour), WaitingReason: "CrashLoopBackOff"},
					// Recent, but of the old revision: not progress.
					{Name: "e", Revision: "old", Created: ago(time.Minute), WaitingReason: "ImagePullBackOff"},
				},
				Now: now,
			},
			want: Result{
				Phase: PhaseStalled, Reason: "NoProgress: 3 pods CrashLoopBackOff, 1 pod ImagePullBackOff",
				Updated: 2, Desired: 5, UpdatedReady: 1, Since: ago(30 * time.Minute),
				LastProgress: ago(20 * time.Minute), Deadline: ago(10 * time.Minute),
			},
		},
		{
			name: "pods: stalled without waiting reason",
			in: Input{
				Workload:    rollingDS(),
				PodsWatched: true,
				Pods:        []PodFact{{Revision: "new", Created: ago(25 * time.Minute)}},
				Now:         now,
			},
			want: Result{
				Phase: PhaseStalled, Reason: ReasonNoProgress, Updated: 2, Desired: 5, UpdatedReady: 1,
				Since: ago(30 * time.Minute), LastProgress: ago(25 * time.Minute), Deadline: ago(15 * time.Minute),
			},
		},
		{
			name: "pods: recent progress keeps rolling",
			in: Input{
				Workload:    rollingDS(),
				PodsWatched: true,
				Pods: []PodFact{
					{Revision: "new", Created: ago(25 * time.Minute), ReadySince: ago(24 * time.Minute)},
					{Revision: "new", Created: ago(2 * time.Minute)},
				},
				Now: now,
			},
			want: Result{
				Phase: PhaseRolling, Reason: ReasonUpdatingPods, Updated: 2, Desired: 5, UpdatedReady: 1,
				Since: ago(30 * time.Minute), LastProgress: ago(2 * time.Minute), Deadline: now.Add(8 * time.Minute),
			},
		},
		{
			name: "pods: no current-revision pod falls back to rollout start",
			in: Input{
				Workload:    rollingDS(),
				PodsWatched: true,
				Pods:        []PodFact{{Revision: "old", Created: ago(time.Minute)}},
				Now:         now,
			},
			want: Result{
				Phase: PhaseStalled, Reason: ReasonNoProgress, Updated: 2, Desired: 5, UpdatedReady: 1,
				Since: ago(30 * time.Minute), LastProgress: ago(30 * time.Minute), Deadline: ago(20 * time.Minute),
			},
		},
		{
			name:       "pods: stall-after flag",
			stallAfter: time.Hour,
			in: Input{
				Workload:    rollingDS(),
				PodsWatched: true,
				Pods:        []PodFact{{Revision: "new", Created: ago(25 * time.Minute)}},
				Now:         now,
			},
			want: Result{
				Phase: PhaseRolling, Reason: ReasonUpdatingPods, Updated: 2, Desired: 5, UpdatedReady: 1,
				Since: ago(30 * time.Minute), LastProgress: ago(25 * time.Minute), Deadline: now.Add(35 * time.Minute),
			},
		},
		{
			name:       "pods: declared progress deadline overrides flag",
			stallAfter: time.Hour,
			in: Input{
				Workload:    rollingDS(),
				PodsWatched: true,
				Pods:        []PodFact{{Revision: "new", Created: ago(25 * time.Minute)}},
				Declared:    Declared{ProgressDeadline: 5 * time.Minute},
				Now:         now,
			},
			want: Result{
				Phase: PhaseStalled, Reason: ReasonNoProgress, Updated: 2, Desired: 5, UpdatedReady: 1,
				Since: ago(30 * time.Minute), LastProgress: ago(25 * time.Minute), Deadline: ago(20 * time.Minute),
			},
		},
		{
			name: "stale data: never stalled",
			in: Input{
				Workload:    rollingDS(),
				PodsWatched: true,
				Pods:        []PodFact{{Revision: "new", Created: ago(25 * time.Minute)}},
				Stale:       true,
				Now:         now,
			},
			want: Result{
				Phase: PhaseRolling, Reason: ReasonUpdatingPods, Updated: 2, Desired: 5, UpdatedReady: 1,
				Since: ago(30 * time.Minute),
			},
		},
		{
			name: "OnDelete: never stalled",
			in: Input{
				Workload: func() Workload {
					w := rollingDS()
					w.OnDelete = true
					return w
				}(),
				PodsWatched:  true,
				Pods:         []PodFact{{Revision: "old", Created: ago(2 * time.Hour), WaitingReason: "CrashLoopBackOff"}},
				LastProgress: ago(time.Hour),
				Now:          now,
			},
			want: Result{
				Phase: PhaseRolling, Reason: ReasonUpdatingPods, Updated: 2, Desired: 5, UpdatedReady: 1,
				Since: ago(30 * time.Minute), LastProgress: ago(time.Hour),
			},
		},
		{
			name: "rollback: since approximate",
			in: Input{
				Workload: func() Workload {
					w := rollingDS()
					w.RolloutStart = ago(30 * 24 * time.Hour)
					w.RolloutStartApprox = true
					return w
				}(),
				Now: now,
			},
			want: Result{
				Phase: PhaseRolling, Reason: ReasonUpdatingPods, Updated: 2, Desired: 5, UpdatedReady: 1,
				Since: ago(30 * 24 * time.Hour), SinceApprox: true,
			},
		},
		{
			name: "rollback: first seen replaces stale start",
			in: Input{
				Workload: func() Workload {
					w := rollingDS()
					w.RolloutStart = ago(30 * 24 * time.Hour)
					w.RolloutStartApprox = true
					return w
				}(),
				FirstSeen: ago(3 * time.Minute),
				Now:       now,
			},
			want: Result{
				Phase: PhaseRolling, Reason: ReasonUpdatingPods, Updated: 2, Desired: 5, UpdatedReady: 1,
				Since: ago(3 * time.Minute), SinceApprox: true,
			},
		},
		{
			name: "rollback: stale start is not used as progress",
			in: Input{
				Workload: func() Workload {
					w := rollingDS()
					w.RolloutStart = ago(30 * 24 * time.Hour)
					w.RolloutStartApprox = true
					return w
				}(),
				PodsWatched: true,
				Now:         now,
			},
			want: Result{
				Phase: PhaseRolling, Reason: ReasonUpdatingPods, Updated: 2, Desired: 5, UpdatedReady: 1,
				Since: ago(30 * 24 * time.Hour), SinceApprox: true,
			},
		},
		{
			name: "rollback: old current-revision pods without first seen are not stalled",
			in: Input{
				Workload:    rollbackDS(),
				PodsWatched: true,
				Pods: []PodFact{
					{Name: "a", Revision: "new", Created: ago(3 * 24 * time.Hour), ReadySince: ago(3 * 24 * time.Hour)},
					{Name: "b", Revision: "new", Created: ago(3 * 24 * time.Hour), ReadySince: ago(3 * 24 * time.Hour)},
					{Name: "c", Revision: "bad", Created: ago(5 * time.Minute), WaitingReason: "CrashLoopBackOff"},
				},
				Now: now,
			},
			want: Result{
				Phase: PhaseRolling, Reason: ReasonUpdatingPods, Updated: 2, Desired: 5, UpdatedReady: 2,
				Since: ago(30 * 24 * time.Hour), SinceApprox: true,
			},
		},
		{
			name: "rollback: old current-revision pods are bounded by first seen",
			in: Input{
				Workload:    rollbackDS(),
				PodsWatched: true,
				Pods: []PodFact{
					{Name: "a", Revision: "new", Created: ago(3 * 24 * time.Hour), ReadySince: ago(3 * 24 * time.Hour)},
					{Name: "b", Revision: "new", Created: ago(3 * 24 * time.Hour), ReadySince: ago(3 * 24 * time.Hour)},
					{Name: "c", Revision: "bad", Created: ago(5 * time.Minute), WaitingReason: "CrashLoopBackOff"},
				},
				FirstSeen: ago(time.Minute),
				Now:       now,
			},
			want: Result{
				Phase: PhaseRolling, Reason: ReasonUpdatingPods, Updated: 2, Desired: 5, UpdatedReady: 2,
				Since: ago(time.Minute), SinceApprox: true,
				LastProgress: ago(time.Minute), Deadline: ago(time.Minute).Add(DefaultStallAfter),
			},
		},
		{
			name: "rollback: first seen is the progress fallback",
			in: Input{
				Workload:    rollbackDS(),
				PodsWatched: true,
				FirstSeen:   ago(15 * time.Minute),
				Now:         now,
			},
			want: Result{
				Phase: PhaseStalled, Reason: ReasonNoProgress, Updated: 2, Desired: 5, UpdatedReady: 1,
				Since: ago(15 * time.Minute), SinceApprox: true,
				LastProgress: ago(15 * time.Minute), Deadline: ago(5 * time.Minute),
			},
		},
		{
			name: "approximate start: earlier first seen keeps the rollout start",
			in: Input{
				Workload: func() Workload {
					w := rollingDS()
					w.RolloutStartApprox = true
					return w
				}(),
				PodsWatched: true,
				FirstSeen:   ago(2 * time.Hour),
				Now:         now,
			},
			want: Result{
				Phase: PhaseStalled, Reason: ReasonNoProgress, Updated: 2, Desired: 5, UpdatedReady: 1,
				Since: ago(30 * time.Minute), SinceApprox: true,
				LastProgress: ago(30 * time.Minute), Deadline: ago(20 * time.Minute),
			},
		},
		{
			name: "pods: generation not observed skips stall",
			in: Input{
				Workload: func() Workload {
					w := rollingDS()
					w.Generation = 3
					return w
				}(),
				PodsWatched: true,
				Pods:        []PodFact{{Revision: "new", Created: ago(25 * time.Minute)}},
				Now:         now,
			},
			want: Result{
				Phase: PhaseRolling, Reason: ReasonGenerationNotObserved, Updated: 2, Desired: 5, UpdatedReady: 1,
				Since: ago(30 * time.Minute),
			},
		},
		{
			name: "step order passed through",
			in: Input{
				Workload: rollingDS(),
				Declared: Declared{StepOrder: intPtr(2)},
				Now:      now,
			},
			want: Result{
				Phase: PhaseRolling, Reason: ReasonUpdatingPods, Updated: 2, Desired: 5, UpdatedReady: 1,
				Since: ago(30 * time.Minute), StepOrder: intPtr(2),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := HeuristicEvaluator{StallAfter: tt.stallAfter}.Evaluate(tt.in)
			assert.True(t, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestHeuristicAvailability covers the rules of a workload whose generation
// is observed: rollout progress first, then unavailable pods
// against the threshold and the settling period.
func TestHeuristicAvailability(t *testing.T) {
	// converged is a DaemonSet with every pod updated and 1 of 5
	// unavailable, started start ago.
	converged := func(start time.Duration) Workload {
		return Workload{
			Kind: KindDaemonSet, Name: "agent",
			Generation: 2, ObservedGeneration: 2,
			Desired: 5, Updated: 5, Available: 4, Unavailable: 1,
			CurrentRevision: "new", RolloutStart: ago(start),
		}
	}
	with := func(w Workload, f func(*Workload)) Workload {
		f(&w)
		return w
	}
	tests := []struct {
		name        string
		in          Input
		phase       Phase
		reason      string
		settleUntil time.Time
	}{
		{
			name:  "failed: progress deadline exceeded beats everything",
			in:    Input{Workload: with(converged(time.Hour), func(w *Workload) { w.Kind, w.ProgressDeadlineExceeded = KindDeployment, true })},
			phase: PhaseFailed, reason: ReasonProgressDeadlineExceeded,
		},
		{
			name:  "rolling: generation not observed beats the threshold",
			in:    Input{Workload: with(converged(time.Hour), func(w *Workload) { w.Generation = 3 }), AvailabilityThreshold: 5},
			phase: PhaseRolling, reason: ReasonGenerationNotObserved,
		},
		{
			name:  "rolling: pods not all updated beats the threshold",
			in:    Input{Workload: with(converged(time.Hour), func(w *Workload) { w.Updated = 4 }), AvailabilityThreshold: 5},
			phase: PhaseRolling, reason: ReasonUpdatingPods,
		},
		{
			name:  "rolling: OnDelete pods not all updated",
			in:    Input{Workload: with(converged(time.Hour), func(w *Workload) { w.Updated, w.OnDelete = 4, true })},
			phase: PhaseRolling, reason: ReasonUpdatingPods,
		},
		{
			name:  "settling: settling right after the start",
			in:    Input{Workload: converged(time.Minute)},
			phase: PhaseSettling, reason: ReasonPodsUnavailable, settleUntil: ago(time.Minute).Add(DefaultSettlingPeriod),
		},
		{
			name:  "settling: last instant of the settling period",
			in:    Input{Workload: converged(DefaultSettlingPeriod - time.Second)},
			phase: PhaseSettling, reason: ReasonPodsUnavailable, settleUntil: now.Add(time.Second),
		},
		{
			name:  "degraded: settling period over exactly",
			in:    Input{Workload: converged(DefaultSettlingPeriod)},
			phase: PhaseComplete, reason: ReasonAvailabilityBelowThreshold,
		},
		{
			name:  "degraded: converged long ago",
			in:    Input{Workload: converged(time.Hour)},
			phase: PhaseComplete, reason: ReasonAvailabilityBelowThreshold,
		},
		{
			name:  "settling: convergence seen after the start",
			in:    Input{Workload: converged(time.Hour), ConvergedSince: ago(2 * time.Minute)},
			phase: PhaseSettling, reason: ReasonPodsUnavailable, settleUntil: ago(2 * time.Minute).Add(DefaultSettlingPeriod),
		},
		{
			name:  "settling: custom settling period",
			in:    Input{Workload: converged(10 * time.Minute), SettlingPeriod: 15 * time.Minute},
			phase: PhaseSettling, reason: ReasonPodsUnavailable, settleUntil: now.Add(5 * time.Minute),
		},
		{
			name:  "degraded: approximate start, nothing seen: settling skipped",
			in:    Input{Workload: with(converged(time.Minute), func(w *Workload) { w.RolloutStartApprox = true })},
			phase: PhaseComplete, reason: ReasonAvailabilityBelowThreshold,
		},
		{
			name:  "settling: approximate start, revision first seen recently",
			in:    Input{Workload: with(converged(30*24*time.Hour), func(w *Workload) { w.RolloutStartApprox = true }), FirstSeen: ago(time.Minute)},
			phase: PhaseSettling, reason: ReasonPodsUnavailable, settleUntil: ago(time.Minute).Add(DefaultSettlingPeriod),
		},
		{
			name:  "settling: approximate start, convergence seen recently",
			in:    Input{Workload: with(converged(30*24*time.Hour), func(w *Workload) { w.RolloutStartApprox = true }), ConvergedSince: ago(time.Minute)},
			phase: PhaseSettling, reason: ReasonPodsUnavailable, settleUntil: ago(time.Minute).Add(DefaultSettlingPeriod),
		},
		{
			name:  "degraded: no start known",
			in:    Input{Workload: with(converged(0), func(w *Workload) { w.RolloutStart = time.Time{} })},
			phase: PhaseComplete, reason: ReasonAvailabilityBelowThreshold,
		},
		{
			name:  "degraded: unavailable above a non-zero threshold",
			in:    Input{Workload: with(converged(time.Hour), func(w *Workload) { w.Unavailable = 3 }), AvailabilityThreshold: 2},
			phase: PhaseComplete, reason: ReasonAvailabilityBelowThreshold,
		},
		{
			name:  "degraded: OnDelete",
			in:    Input{Workload: with(converged(time.Hour), func(w *Workload) { w.OnDelete = true })},
			phase: PhaseComplete, reason: ReasonAvailabilityBelowThreshold,
		},
		{
			name: "degraded: Deployment",
			in: Input{Workload: Workload{
				Kind: KindDeployment, Generation: 1, ObservedGeneration: 1,
				Desired: 2, Updated: 2, Available: 1, Unavailable: 1, RolloutStart: ago(time.Hour),
			}},
			phase: PhaseComplete, reason: ReasonAvailabilityBelowThreshold,
		},
		{
			name:  "complete: unavailable at the threshold",
			in:    Input{Workload: converged(time.Minute), AvailabilityThreshold: 1},
			phase: PhaseComplete,
		},
		{
			name:  "complete: all available",
			in:    Input{Workload: with(converged(time.Minute), func(w *Workload) { w.Unavailable, w.Available = 0, 5 })},
			phase: PhaseComplete,
		},
		{
			name: "complete: Deployment within the threshold",
			in: Input{Workload: Workload{
				Kind: KindDeployment, Generation: 1, ObservedGeneration: 1,
				Desired: 2, Updated: 2, Available: 1, Unavailable: 1, RolloutStart: ago(time.Minute),
			}, AvailabilityThreshold: 1},
			phase: PhaseComplete,
		},
		{
			name: "settling is never stalled",
			in: Input{
				Workload:    converged(time.Minute),
				PodsWatched: true,
				Pods:        []PodFact{{Revision: "new", Created: ago(time.Hour), WaitingReason: "CrashLoopBackOff"}},
			},
			phase: PhaseSettling, reason: ReasonPodsUnavailable, settleUntil: ago(time.Minute).Add(DefaultSettlingPeriod),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.in.Now = now
			got, ok := HeuristicEvaluator{StallAfter: time.Second}.Evaluate(tt.in)
			assert.True(t, ok)
			assert.Equal(t, tt.phase, got.Phase)
			assert.Equal(t, tt.reason, got.Reason)
			assert.Equal(t, tt.settleUntil, got.SettlingUntil)
			assert.True(t, got.Deadline.IsZero(), "no stall deadline")
		})
	}
}

func TestUpdatedReady(t *testing.T) {
	tests := []struct {
		name string
		in   Input
		want int32
	}{
		{name: "no pods: updated minus unavailable", in: Input{Workload: rollingDS()}, want: 1},
		{name: "no pods: never negative", in: Input{Workload: Workload{Desired: 5, Updated: 1, Unavailable: 3}}, want: 0},
		{
			name: "pods: ready pods of the current revision",
			in: Input{
				Workload:    rollingDS(),
				PodsWatched: true,
				Pods: []PodFact{
					{Revision: "new", ReadySince: ago(time.Minute)},
					{Revision: "new"},
					{Revision: "old", ReadySince: ago(time.Hour)},
				},
			},
			want: 1,
		},
		{
			name: "pods: capped at updated",
			in: Input{
				Workload:    Workload{Desired: 5, Updated: 1, CurrentRevision: "new"},
				PodsWatched: true,
				Pods:        []PodFact{{Revision: "new", ReadySince: ago(time.Minute)}, {Revision: "new", ReadySince: ago(time.Minute)}},
			},
			want: 1,
		},
		{
			name: "pods: lagging pod data keeps the lower bound",
			in: Input{
				Workload:    Workload{Desired: 5, Updated: 5, Available: 5, CurrentRevision: "new"},
				PodsWatched: true,
				Pods:        []PodFact{{Revision: "old", ReadySince: ago(time.Hour)}},
			},
			want: 5,
		},
		{
			name: "pods: unknown current revision keeps the lower bound",
			in: Input{
				Workload:    Workload{Desired: 5, Updated: 4, Unavailable: 1},
				PodsWatched: true,
				Pods:        []PodFact{{Revision: "new", ReadySince: ago(time.Minute)}},
			},
			want: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, UpdatedReady(tt.in))
		})
	}
}
