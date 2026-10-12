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

// fakeAuthoritative stands for an evaluator reading operator-owned rollout
// status: it decides only when it has data.
type fakeAuthoritative struct {
	res *Result
}

func (f fakeAuthoritative) Name() string { return "operator" }

func (f fakeAuthoritative) Evaluate(Input) (Result, bool) {
	if f.res == nil {
		return Result{}, false
	}
	return *f.res, true
}

func TestChain(t *testing.T) {
	in := Input{Workload: rollingDS(), Declared: Declared{StepOrder: intPtr(1)}, Now: now}

	tests := []struct {
		name   string
		chain  Chain
		want   Result
		wantOK bool
	}{
		{
			name:   "authoritative wins when it has data",
			chain:  Chain{fakeAuthoritative{res: &Result{Phase: PhaseBaking, Reason: "Bake", Updated: 5, Desired: 5}}, HeuristicEvaluator{}},
			want:   Result{Phase: PhaseBaking, Reason: "Bake", Updated: 5, Desired: 5, Source: "operator"},
			wantOK: true,
		},
		{
			name:  "heuristic is the fallback",
			chain: Chain{fakeAuthoritative{}, HeuristicEvaluator{}},
			want: Result{
				Phase: PhaseRolling, Reason: ReasonUpdatingPods, Updated: 2, Desired: 5, UpdatedReady: 1,
				Since: ago(30 * time.Minute), StepOrder: intPtr(1), Source: HeuristicName,
			},
			wantOK: true,
		},
		{
			name:   "unknown phase passed through",
			chain:  Chain{fakeAuthoritative{res: &Result{Phase: "Canarying", Reason: "Wave1"}}, HeuristicEvaluator{}},
			want:   Result{Phase: "Canarying", Reason: "Wave1", Source: "operator"},
			wantOK: true,
		},
		{
			name:  "nested chain keeps the deciding evaluator",
			chain: Chain{Chain{fakeAuthoritative{}, HeuristicEvaluator{}}},
			want: Result{
				Phase: PhaseRolling, Reason: ReasonUpdatingPods, Updated: 2, Desired: 5, UpdatedReady: 1,
				Since: ago(30 * time.Minute), StepOrder: intPtr(1), Source: HeuristicName,
			},
			wantOK: true,
		},
		{
			name:   "no evaluator decides",
			chain:  Chain{fakeAuthoritative{}},
			want:   Result{Phase: PhaseUnknown, Updated: 2, Desired: 5, UpdatedReady: 1, StepOrder: intPtr(1)},
			wantOK: false,
		},
		{
			name:   "empty chain",
			chain:  nil,
			want:   Result{Phase: PhaseUnknown, Updated: 2, Desired: 5, UpdatedReady: 1, StepOrder: intPtr(1)},
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.chain.Evaluate(in)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPhaseSeverity(t *testing.T) {
	tests := []struct {
		phase Phase
		want  Severity
	}{
		{PhaseComplete, SeverityDone},
		{PhaseSucceeded, SeverityDone},
		{PhaseUnknown, SeverityUnknown},
		{"Canarying", SeverityUnknown},
		{"", SeverityUnknown},
		{PhaseRolling, SeverityProgressing},
		{PhaseSettling, SeverityProgressing},
		{PhasePending, SeverityProgressing},
		{PhaseBaking, SeverityProgressing},
		{PhaseStalled, SeverityDegraded},
		{PhasePaused, SeverityDegraded},
		{PhaseFailed, SeverityError},
		{PhaseTimedOut, SeverityError},
		{PhaseRolledBack, SeverityError},
	}
	for _, tt := range tests {
		t.Run(string(tt.phase), func(t *testing.T) {
			assert.Equal(t, tt.want, tt.phase.Severity())
		})
	}
}
