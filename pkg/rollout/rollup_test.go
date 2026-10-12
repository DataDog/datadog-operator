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

func TestWorstOf(t *testing.T) {
	tests := []struct {
		name    string
		results []Result
		want    Result
	}{
		{
			name: "empty is unknown",
			want: Result{Phase: PhaseUnknown},
		},
		{
			name: "all complete",
			results: []Result{
				{Phase: PhaseComplete, Updated: 3, Desired: 3, Since: ago(time.Hour), Source: HeuristicName},
				{Phase: PhaseComplete, Updated: 1, Desired: 1, Since: ago(2 * time.Hour), Source: HeuristicName},
			},
			want: Result{Phase: PhaseComplete, Updated: 4, Desired: 4, Source: HeuristicName},
		},
		{
			name: "worst phase wins and counts are summed",
			results: []Result{
				{Phase: PhaseComplete, Updated: 1, Desired: 1, UpdatedReady: 1, Since: ago(2 * time.Hour), Source: HeuristicName},
				{
					Phase: PhaseRolling, Reason: ReasonUpdatingPods, Updated: 2, Desired: 5, UpdatedReady: 1,
					Since: ago(30 * time.Minute), LastProgress: ago(5 * time.Minute), Deadline: now.Add(5 * time.Minute),
					Source: HeuristicName,
				},
				{
					Phase: PhaseStalled, Reason: ReasonNoProgress, Updated: 0, Desired: 1,
					Since: ago(20 * time.Minute), SinceApprox: true, LastProgress: ago(15 * time.Minute), Deadline: ago(5 * time.Minute),
					Source: "operator",
				},
			},
			want: Result{
				Phase: PhaseStalled, Reason: ReasonNoProgress, Updated: 3, Desired: 7, UpdatedReady: 2,
				Since: ago(30 * time.Minute), LastProgress: ago(5 * time.Minute), Deadline: ago(5 * time.Minute),
				Source: "operator",
			},
		},
		{
			name: "settling beats complete and keeps the latest settling end",
			results: []Result{
				{Phase: PhaseComplete, Reason: ReasonAvailabilityBelowThreshold, Updated: 2, Desired: 2, UpdatedReady: 1, Since: ago(time.Hour)},
				{Phase: PhaseSettling, Reason: ReasonPodsUnavailable, Updated: 3, Desired: 3, UpdatedReady: 2, Since: ago(2 * time.Minute), SettlingUntil: now.Add(3 * time.Minute)},
				{Phase: PhaseSettling, Reason: ReasonPodsUnavailable, Updated: 1, Desired: 1, Since: ago(time.Minute), SettlingUntil: now.Add(4 * time.Minute)},
			},
			want: Result{
				Phase: PhaseSettling, Reason: ReasonPodsUnavailable, Updated: 6, Desired: 6, UpdatedReady: 3,
				Since: ago(2 * time.Minute), SettlingUntil: now.Add(4 * time.Minute),
			},
		},
		{
			name: "rolling beats settling and drops the settling end",
			results: []Result{
				{Phase: PhaseSettling, Reason: ReasonPodsUnavailable, Updated: 3, Desired: 3, Since: ago(2 * time.Minute), SettlingUntil: now.Add(3 * time.Minute)},
				{Phase: PhaseRolling, Reason: ReasonUpdatingPods, Updated: 1, Desired: 2, Since: ago(time.Minute)},
			},
			want: Result{Phase: PhaseRolling, Reason: ReasonUpdatingPods, Updated: 4, Desired: 5, Since: ago(2 * time.Minute)},
		},
		{
			name: "error beats degraded",
			results: []Result{
				{Phase: PhasePaused, Reason: "Paused"},
				{Phase: PhaseFailed, Reason: ReasonProgressDeadlineExceeded},
				{Phase: PhaseStalled, Reason: ReasonNoProgress},
			},
			want: Result{Phase: PhaseFailed, Reason: ReasonProgressDeadlineExceeded},
		},
		{
			name: "ties keep the first",
			results: []Result{
				{Phase: PhasePending, Reason: "first"},
				{Phase: PhaseRolling, Reason: "second"},
			},
			want: Result{Phase: PhasePending, Reason: "first"},
		},
		{
			name: "unknown phase ranks above complete and below rolling",
			results: []Result{
				{Phase: PhaseComplete},
				{Phase: "Canarying", Reason: "Wave1"},
			},
			want: Result{Phase: "Canarying", Reason: "Wave1"},
		},
		{
			name: "rolling beats unknown phase",
			results: []Result{
				{Phase: "Canarying"},
				{Phase: PhaseRolling, Reason: ReasonUpdatingPods},
			},
			want: Result{Phase: PhaseRolling, Reason: ReasonUpdatingPods},
		},
		{
			name: "first step order set is kept",
			results: []Result{
				{Phase: PhaseComplete},
				{Phase: PhaseComplete, StepOrder: intPtr(3)},
				{Phase: PhaseComplete, StepOrder: intPtr(4)},
			},
			want: Result{Phase: PhaseComplete, StepOrder: intPtr(3)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, WorstOf(tt.results))
		})
	}
}
