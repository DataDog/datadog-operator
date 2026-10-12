// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rollout

// Evaluator computes the rollout state of a workload.
type Evaluator interface {
	// Name identifies the evaluator; it is recorded in Result.Source.
	Name() string
	// Evaluate returns false when the evaluator has no data to decide.
	Evaluate(in Input) (Result, bool)
}

// Chain is an Evaluator trying its evaluators in order: the first one that
// returns ok wins. Authoritative evaluators go first, the heuristic last.
type Chain []Evaluator

var _ Evaluator = Chain(nil)

// Name implements Evaluator.
func (c Chain) Name() string { return "chain" }

// Evaluate implements Evaluator. The winner's name is recorded in
// Result.Source unless it set one. When none decides, it returns an Unknown
// result and false.
func (c Chain) Evaluate(in Input) (Result, bool) {
	for _, e := range c {
		if res, ok := e.Evaluate(in); ok {
			if res.Source == "" {
				res.Source = e.Name()
			}
			return res, true
		}
	}
	return Result{
		Phase:        PhaseUnknown,
		Updated:      in.Workload.Updated,
		Desired:      in.Workload.Desired,
		UpdatedReady: UpdatedReady(in),
		StepOrder:    in.Declared.StepOrder,
	}, false
}
